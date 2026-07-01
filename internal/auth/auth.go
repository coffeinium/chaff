package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"time"

	"github.com/coffeinium/chaff/internal/store"
)

const tokenPrefix = "chaff_"

func GenerateToken() (plaintext, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	plaintext = tokenPrefix + base64.RawURLEncoding.EncodeToString(b)
	return plaintext, HashToken(plaintext), nil
}

func HashToken(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

type Options struct {
	Secure      bool
	IdleTTL     time.Duration
	AbsoluteTTL time.Duration
}

type Manager struct {
	st       *store.Store
	sessions *sessionStore
	limiter  *loginLimiter
	opt      Options
	now      func() time.Time
}

func NewManager(st *store.Store, opt Options) *Manager {
	if opt.IdleTTL <= 0 {
		opt.IdleTTL = 30 * time.Minute
	}
	if opt.AbsoluteTTL <= 0 {
		opt.AbsoluteTTL = 12 * time.Hour
	}
	now := time.Now
	return &Manager{
		st:       st,
		sessions: newSessionStore(opt.IdleTTL, opt.AbsoluteTTL, now),
		limiter:  newLoginLimiter(now),
		opt:      opt,
		now:      now,
	}
}

func (m *Manager) verifyToken(plaintext string) (store.WebToken, bool) {
	if len(plaintext) < len(tokenPrefix)+8 {
		return store.WebToken{}, false
	}
	tok, ok, err := m.st.TokenByHash(HashToken(plaintext))
	if err != nil || !ok {
		return store.WebToken{}, false
	}
	now := m.now().Unix()
	if tok.ExpiresAt != 0 && now > tok.ExpiresAt {
		return store.WebToken{}, false
	}
	_ = m.st.TouchToken(tok.ID, now)
	return tok, true
}

func (m *Manager) Sessions() int { return m.sessions.count() }

func (m *Manager) GC() { m.sessions.gc(); m.limiter.gc() }
