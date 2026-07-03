package webui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/coffeinium/chaff/internal/api"
	"github.com/coffeinium/chaff/internal/auth"
	"github.com/coffeinium/chaff/internal/bus"
	"github.com/coffeinium/chaff/internal/ipc"
	"github.com/coffeinium/chaff/internal/kernel"
	"github.com/coffeinium/chaff/internal/model"
	"github.com/coffeinium/chaff/internal/modules/feedsync"
)

const (
	maxAPIBody    = 1 << 20
	maxUploadBody = 64 << 20
)

func buildHandler(k *kernel.Kernel, am *auth.Manager, upd *updater) http.Handler {
	handlers := api.Handlers(k)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/version", versionHandler(upd))
	mux.HandleFunc("POST /api/login", am.LoginHandler())
	mux.Handle("POST /api/logout", am.Require(am.LogoutHandler()))
	mux.Handle("GET /api/me", am.Require(am.MeHandler()))
	mux.Handle("POST /api/cmd/", am.Require(cmdHandler(handlers)))
	mux.Handle("POST /api/upload", am.Require(uploadHandler(k)))
	mux.Handle("GET /", http.FileServerFS(assetsFS()))

	return securityHeaders(mux)
}

func uploadHandler(k *kernel.Kernel) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !auth.SameOrigin(r) {
			writeResp(w, http.StatusForbidden, ipc.Err("запрещённый origin"))
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxUploadBody)
		if err := r.ParseMultipartForm(8 << 20); err != nil {
			writeResp(w, http.StatusOK, ipc.Err("файл не принят: "+err.Error()))
			return
		}
		file, hdr, err := r.FormFile("file")
		if err != nil {
			writeResp(w, http.StatusOK, ipc.Err("нет файла в запросе"))
			return
		}
		defer file.Close()

		name := strings.TrimSpace(r.FormValue("name"))
		if name == "" {
			name = strings.TrimSuffix(filepath.Base(hdr.Filename), filepath.Ext(hdr.Filename))
		}
		adapter := strings.TrimSpace(r.FormValue("adapter"))
		if adapter == "" {
			adapter = "text"
		}

		dir := filepath.Join(filepath.Dir(k.Config.DBPath), "feeds")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			writeResp(w, http.StatusOK, ipc.Err(err.Error()))
			return
		}
		dst := filepath.Join(dir, sanitizeName(name)+filepath.Ext(hdr.Filename))
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
		if err != nil {
			writeResp(w, http.StatusOK, ipc.Err(err.Error()))
			return
		}
		if _, err := io.Copy(out, file); err != nil {
			out.Close()
			writeResp(w, http.StatusOK, ipc.Err(err.Error()))
			return
		}
		out.Close()

		if specs, err := k.Store.ListSources(); err == nil {
			for _, s := range specs {
				if s.Name == name {
					if err := k.Store.RemoveSource(s.ID); err != nil {
						writeResp(w, http.StatusOK, ipc.Err(err.Error()))
						return
					}
					break
				}
			}
		}
		spec := model.SourceSpec{Name: name, Adapter: adapter, URI: dst, ColumnMap: parseColumnMap(r.FormValue("map"))}
		if _, err := k.Store.AddSource(spec); err != nil {
			writeResp(w, http.StatusOK, ipc.Err(err.Error()))
			return
		}
		n, err := feedsync.Run(context.Background(), k)
		if err != nil {
			writeResp(w, http.StatusOK, ipc.Err("файл сохранён, но синк не удался: "+err.Error()))
			return
		}
		k.Bus.Publish(bus.Event{Topic: bus.TopicReload, Data: "upload"})
		writeResp(w, http.StatusOK, ipc.OK(fmt.Sprintf("источник %q загружен, синхронизировано индикаторов: %d", name, n)))
	}
}

func sanitizeName(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), r == '.', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "feed"
	}
	return b.String()
}

func parseColumnMap(s string) map[string]int {
	out := map[string]int{}
	for _, pair := range strings.Split(s, ",") {
		kv := strings.SplitN(strings.TrimSpace(pair), ":", 2)
		if len(kv) != 2 {
			continue
		}
		var n int
		if _, err := fmt.Sscanf(strings.TrimSpace(kv[1]), "%d", &n); err == nil {
			out[strings.TrimSpace(kv[0])] = n
		}
	}
	return out
}

func cmdHandler(handlers map[string]ipc.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !auth.SameOrigin(r) {
			writeResp(w, http.StatusForbidden, ipc.Err("запрещённый origin"))
			return
		}
		cmd := strings.TrimPrefix(r.URL.Path, "/api/cmd/")
		h, ok := handlers[cmd]
		if !ok {
			writeResp(w, http.StatusNotFound, ipc.Err("неизвестная команда: "+cmd))
			return
		}
		args := map[string]string{}
		if r.Body != nil {
			_ = json.NewDecoder(io.LimitReader(r.Body, maxAPIBody)).Decode(&args)
		}
		writeResp(w, http.StatusOK, h(ipc.Request{Cmd: cmd, Args: args}))
	}
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			h.Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

func writeResp(w http.ResponseWriter, code int, resp ipc.Response) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(resp)
}
