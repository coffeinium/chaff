package auth

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"
)

const CookieName = "chaff_session"

const (
	maxLoginAttempts = 10
	loginWindow      = time.Minute
	maxBodyBytes     = 4 << 10
)

type loginLimiter struct {
	mu  sync.Mutex
	m   map[string]*attempt
	now func() time.Time
}

type attempt struct {
	count int
	start time.Time
}

func newLoginLimiter(now func() time.Time) *loginLimiter {
	return &loginLimiter{m: make(map[string]*attempt), now: now}
}

func (l *loginLimiter) allow(ip string) bool {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	a, ok := l.m[ip]
	if !ok || now.Sub(a.start) > loginWindow {
		l.m[ip] = &attempt{count: 1, start: now}
		return true
	}
	a.count++
	return a.count <= maxLoginAttempts
}

func (l *loginLimiter) gc() {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	for ip, a := range l.m {
		if now.Sub(a.start) > loginWindow {
			delete(l.m, ip)
		}
	}
}

func (m *Manager) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(CookieName)
		if err != nil {
			respondErr(w, http.StatusUnauthorized, "требуется вход")
			return
		}
		tokenID, ok := m.sessions.validate(c.Value)
		if !ok {
			respondErr(w, http.StatusUnauthorized, "требуется вход")
			return
		}
		if !m.tokenLive(tokenID) {
			m.sessions.destroy(c.Value)
			respondErr(w, http.StatusUnauthorized, "сессия отозвана")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (m *Manager) tokenLive(id int64) bool {
	tok, ok, err := m.st.TokenByID(id)
	if err != nil || !ok {
		return false
	}
	return tok.ExpiresAt == 0 || m.now().Unix() <= tok.ExpiresAt
}

func (m *Manager) LoginHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !SameOrigin(r) {
			respondErr(w, http.StatusForbidden, "запрещённый origin")
			return
		}
		ip := clientIP(r)
		if !m.limiter.allow(ip) {
			respondErr(w, http.StatusTooManyRequests, "слишком много попыток входа")
			return
		}
		var body struct {
			Token string `json:"token"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, maxBodyBytes)).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "плохой запрос")
			return
		}
		tok, ok := m.verifyToken(body.Token)
		if !ok {
			respondErr(w, http.StatusUnauthorized, "неверный токен")
			return
		}
		sid, err := m.sessions.create(tok.ID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "не удалось создать сессию")
			return
		}
		m.setCookie(w, sid)
		respondOK(w, map[string]any{"name": tok.Name})
	}
}

func (m *Manager) LogoutHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie(CookieName); err == nil {
			m.sessions.destroy(c.Value)
		}
		m.clearCookie(w)
		respondOK(w, "выход выполнен")
	}
}

func (m *Manager) MeHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		respondOK(w, map[string]any{"sessions": m.sessions.count()})
	}
}

func (m *Manager) setCookie(w http.ResponseWriter, sid string) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    sid,
		Path:     "/",
		HttpOnly: true,
		Secure:   m.opt.Secure,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(m.opt.AbsoluteTTL.Seconds()),
	})
}

func (m *Manager) clearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   m.opt.Secure,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
}

func SameOrigin(r *http.Request) bool {
	o := r.Header.Get("Origin")
	if o == "" {
		return true
	}
	u, err := url.Parse(o)
	if err != nil {
		return false
	}
	return u.Host == r.Host
}

func clientIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

func respondOK(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": data})
}

func respondErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]any{"ok": false, "error": msg})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
