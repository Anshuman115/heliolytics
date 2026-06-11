package middleware

import (
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type ipWindow struct {
	count int
	start time.Time
}

type RateLimiter struct {
	mu         sync.Mutex
	byIP       map[string]*ipWindow
	limit      int
	window     time.Duration
	trustProxy bool
}

func NewRateLimiter(perMin int, trustProxy bool) *RateLimiter {
	return &RateLimiter{
		byIP:       map[string]*ipWindow{},
		limit:      perMin,
		window:     time.Minute,
		trustProxy: trustProxy,
	}
}

func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	for k, w := range rl.byIP {
		if now.Sub(w.start) >= rl.window {
			delete(rl.byIP, k)
		}
	}
	w, ok := rl.byIP[ip]
	if !ok || now.Sub(w.start) >= rl.window {
		rl.byIP[ip] = &ipWindow{count: 1, start: now}
		return true
	}
	if w.count >= rl.limit {
		return false
	}
	w.count++
	return true
}

func RateLimit(rl *RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := clientIP(r, rl.trustProxy)
			if !rl.Allow(ip) {
				log.Printf("rate limit reject path=%s method=%s remote=%s ip=%s", r.URL.Path, r.Method, r.RemoteAddr, ip)
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func clientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			return strings.TrimSpace(strings.Split(xff, ",")[0])
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
