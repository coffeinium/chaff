package auth

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"
)

type session struct {
	tokenID int64
	created time.Time
	seen    time.Time
}

type sessionStore struct {
	mu      sync.Mutex
	m       map[string]*session
	idleTTL time.Duration
	absTTL  time.Duration
	now     func() time.Time
}

func newSessionStore(idle, abs time.Duration, now func() time.Time) *sessionStore {
	return &sessionStore{m: make(map[string]*session), idleTTL: idle, absTTL: abs, now: now}
}

func newSessionID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func (s *sessionStore) create(tokenID int64) (string, error) {
	id, err := newSessionID()
	if err != nil {
		return "", err
	}
	t := s.now()
	s.mu.Lock()
	s.m[id] = &session{tokenID: tokenID, created: t, seen: t}
	s.mu.Unlock()
	return id, nil
}

func (s *sessionStore) validate(id string) (int64, bool) {
	if id == "" {
		return 0, false
	}
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.m[id]
	if !ok {
		return 0, false
	}
	if now.Sub(sess.created) > s.absTTL || now.Sub(sess.seen) > s.idleTTL {
		delete(s.m, id)
		return 0, false
	}
	sess.seen = now
	return sess.tokenID, true
}

func (s *sessionStore) destroy(id string) {
	s.mu.Lock()
	delete(s.m, id)
	s.mu.Unlock()
}

func (s *sessionStore) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.m)
}

func (s *sessionStore) gc() {
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, sess := range s.m {
		if now.Sub(sess.created) > s.absTTL || now.Sub(sess.seen) > s.idleTTL {
			delete(s.m, id)
		}
	}
}
