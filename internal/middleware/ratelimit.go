package middleware

import (
	"container/list"
	"errors"
	"math"
	"net"
	"net/http"
	"net/netip"
	"sync"
	"time"
)

// ErrProxyHeader reports a trusted socket peer that did not supply exactly one
// valid CF-Connecting-IP address literal.
var ErrProxyHeader = errors.New("middleware: invalid proxy header")

// MaxIPKeys bounds the pre-auth limiter table. The oldest key is evicted when
// the table is full, so memory stays fixed under address rotation.
const MaxIPKeys = 10000

// invalidPeerKey is the shared bucket for a peer address that cannot be
// parsed. It keeps such peers limited under one bounded key.
const invalidPeerKey = "invalid"

// IPPolicy decides which address a request is limited by. Forwarded headers
// are consulted only for a configured trusted socket peer.
type IPPolicy struct {
	TrustProxy bool
	Trusted    []netip.Prefix
}

// ClientKey returns the limiter identity for a request. X-Forwarded-For and
// X-Real-IP are never read.
func (p IPPolicy) ClientKey(r *http.Request) (string, error) {
	peer, err := socketPeer(r.RemoteAddr)
	if err != nil {
		return invalidPeerKey, nil
	}
	if !p.TrustProxy || !p.trusted(peer) {
		return peer.String(), nil
	}
	values := r.Header.Values("CF-Connecting-IP")
	if len(values) != 1 {
		return "", ErrProxyHeader
	}
	address, err := netip.ParseAddr(values[0])
	if err != nil || address.Zone() != "" {
		return "", ErrProxyHeader
	}
	return address.Unmap().String(), nil
}

func (p IPPolicy) trusted(peer netip.Addr) bool {
	for _, prefix := range p.Trusted {
		if prefix.Contains(peer) {
			return true
		}
	}
	return false
}

// socketPeer parses the immediate peer and canonicalizes IPv4-mapped IPv6.
func socketPeer(remoteAddr string) (netip.Addr, error) {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return netip.Addr{}, err
	}
	address, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}, err
	}
	if address.Zone() != "" {
		return netip.Addr{}, errors.New("middleware: scoped peer address")
	}
	return address.Unmap(), nil
}

// Limiter is a token bucket with a bounded LRU key table. One instance serves
// one key class and is created once per mux.
type Limiter struct {
	mu        sync.Mutex
	burst     float64
	perSecond float64
	maxKeys   int
	clock     func() time.Time
	entries   map[string]*limiterEntry
	order     *list.List
}

type limiterEntry struct {
	tokens float64
	last   time.Time
	elem   *list.Element
}

// NewLimiter builds a limiter that allows perMinute requests per key with a
// burst equal to that budget.
func NewLimiter(perMinute, maxKeys int) *Limiter {
	return NewLimiterWithClock(perMinute, maxKeys, time.Now)
}

// NewLimiterWithClock is NewLimiter with an injected clock.
func NewLimiterWithClock(perMinute, maxKeys int, clock func() time.Time) *Limiter {
	if perMinute <= 0 {
		perMinute = 1
	}
	if maxKeys <= 0 {
		maxKeys = 1
	}
	if clock == nil {
		clock = time.Now
	}
	return &Limiter{
		burst:     float64(perMinute),
		perSecond: float64(perMinute) / 60,
		maxKeys:   maxKeys,
		clock:     clock,
		entries:   make(map[string]*limiterEntry),
		order:     list.New(),
	}
}

// Allow takes one token for key. When no token is available it reports the
// whole seconds until the next one, never less than one.
func (l *Limiter) Allow(key string) (bool, int) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.clock()
	entry, seen := l.entries[key]
	if !seen {
		entry = &limiterEntry{tokens: l.burst, last: now}
		entry.elem = l.order.PushFront(key)
		l.entries[key] = entry
		if len(l.entries) > l.maxKeys {
			l.evictOldest()
		}
	} else {
		if elapsed := now.Sub(entry.last).Seconds(); elapsed > 0 {
			entry.tokens = math.Min(l.burst, entry.tokens+elapsed*l.perSecond)
			entry.last = now
		}
		l.order.MoveToFront(entry.elem)
	}
	if entry.tokens >= 1 {
		entry.tokens--
		return true, 0
	}
	return false, int(math.Ceil((1 - entry.tokens) / l.perSecond))
}

func (l *Limiter) evictOldest() {
	oldest := l.order.Back()
	if oldest == nil {
		return
	}
	if key, ok := oldest.Value.(string); ok {
		delete(l.entries, key)
	}
	l.order.Remove(oldest)
}

// Semaphore bounds concurrent retained bodies without queueing.
type Semaphore struct {
	mu       sync.Mutex
	free     int
	capacity int
}

// NewSemaphore builds a semaphore with slots permits.
func NewSemaphore(slots int) *Semaphore {
	if slots <= 0 {
		slots = 1
	}
	return &Semaphore{free: slots, capacity: slots}
}

// Acquire takes a permit without waiting.
func (s *Semaphore) Acquire() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.free == 0 {
		return false
	}
	s.free--
	return true
}

// Release returns a permit, ignoring an extra release.
func (s *Semaphore) Release() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.free < s.capacity {
		s.free++
	}
}
