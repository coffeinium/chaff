package webui

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/coffeinium/chaff/internal/api"
	"github.com/coffeinium/chaff/internal/auth"
	"github.com/coffeinium/chaff/internal/ipc"
	"github.com/coffeinium/chaff/internal/kernel"
)

const maxAPIBody = 1 << 20

func buildHandler(k *kernel.Kernel, am *auth.Manager, upd *updater) http.Handler {
	handlers := api.Handlers(k)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/version", versionHandler(upd))
	mux.HandleFunc("POST /api/login", am.LoginHandler())
	mux.Handle("POST /api/logout", am.Require(am.LogoutHandler()))
	mux.Handle("GET /api/me", am.Require(am.MeHandler()))
	mux.Handle("POST /api/cmd/", am.Require(cmdHandler(handlers)))
	mux.Handle("GET /", http.FileServerFS(assetsFS()))

	return securityHeaders(mux)
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
