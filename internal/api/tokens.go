package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/coffeinium/chaff/internal/auth"
	"github.com/coffeinium/chaff/internal/config"
	"github.com/coffeinium/chaff/internal/ipc"
	"github.com/coffeinium/chaff/internal/kernel"
)

func TokenHandlers(k *kernel.Kernel) map[string]ipc.Handler {
	st := k.Store
	h := map[string]ipc.Handler{}

	h["web.token.create"] = func(req ipc.Request) ipc.Response {
		var expiresAt int64
		if ttl := req.Arg("ttl"); ttl != "" {
			d, err := time.ParseDuration(ttl)
			if err != nil || d <= 0 {
				return ipc.Err("плохой --ttl (примеры: 24h, 168h)")
			}
			expiresAt = time.Now().Add(d).Unix()
		}
		plaintext, hash, err := auth.GenerateToken()
		if err != nil {
			return ipc.Err(err.Error())
		}
		name := req.Arg("name")
		id, err := st.AddToken(name, hash, time.Now().Unix(), expiresAt)
		if err != nil {
			return ipc.Err(err.Error())
		}
		return ipc.OK(map[string]any{
			"id":         id,
			"name":       name,
			"token":      plaintext,
			"expires_at": expiresAt,
		})
	}

	h["web.token.ls"] = func(_ ipc.Request) ipc.Response {
		toks, err := st.ListTokens()
		if err != nil {
			return ipc.Err(err.Error())
		}
		return ipc.OK(toks)
	}

	h["web.token.rm"] = func(req ipc.Request) ipc.Response {
		ref := req.Arg("ref")
		if ref == "" {
			return ipc.Err("использование: chaff web token rm ИМЯ|ID")
		}
		var n int64
		var err error
		if id, e := strconv.ParseInt(ref, 10, 64); e == nil {
			n, err = st.RemoveTokenByID(id)
		} else {
			n, err = st.RemoveTokenByName(ref)
		}
		if err != nil {
			return ipc.Err(err.Error())
		}
		if n == 0 {
			return ipc.Err(fmt.Sprintf("токен %q не найден", ref))
		}
		return ipc.OK(fmt.Sprintf("удалено токенов: %d", n))
	}

	h["web.cert"] = func(_ ipc.Request) ipc.Response {
		return certInfo(k.Config)
	}

	return h
}

func certInfo(cfg *config.Config) ipc.Response {
	path := cfg.CertFile()
	raw, err := os.ReadFile(path)
	if err != nil {
		return ipc.Err("сертификат не найден (включи webui с TLS)")
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return ipc.Err("не удалось разобрать сертификат")
	}
	sum := sha256.Sum256(block.Bytes)
	return ipc.OK(map[string]any{
		"path":        path,
		"fingerprint": colonHex(sum[:]),
	})
}

func colonHex(b []byte) string {
	s := hex.EncodeToString(b)
	var parts []string
	for i := 0; i < len(s); i += 2 {
		parts = append(parts, s[i:i+2])
	}
	return strings.ToUpper(strings.Join(parts, ":"))
}
