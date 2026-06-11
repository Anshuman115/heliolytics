package auth

import (
	"sync"
	"time"
)

type NonceStore struct {
	mu   sync.Mutex
	seen map[string]time.Time
	ttl  time.Duration
}

func NewNonceStore(ttl time.Duration) *NonceStore {
	return &NonceStore{seen: map[string]time.Time{}, ttl: ttl}
}

func (s *NonceStore) UseOnce(nonce string, tokenTime time.Time) bool {
	if nonce == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for k, exp := range s.seen {
		if now.After(exp) {
			delete(s.seen, k)
		}
	}
	if exp, ok := s.seen[nonce]; ok && now.Before(exp) {
		return false
	}
	s.seen[nonce] = tokenTime.Add(s.ttl)
	return true
}

var defaultNonceStore = NewNonceStore(TokenWindow + time.Minute)
