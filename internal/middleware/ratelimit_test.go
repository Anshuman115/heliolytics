package middleware

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"
)

func TestLimiterBurstRefillAndCap(t *testing.T) {
	now := time.Unix(1760000000, 0)
	clock := func() time.Time { return now }
	limiter := NewLimiterWithClock(60, 10, clock)

	for i := range 60 {
		if allowed, _ := limiter.Allow("a"); !allowed {
			t.Fatalf("burst request %d denied", i)
		}
	}
	allowed, retry := limiter.Allow("a")
	if allowed {
		t.Fatal("request 61 allowed")
	}
	if retry != 1 {
		t.Fatalf("retry = %d, want 1", retry)
	}

	now = now.Add(30 * time.Second)
	for i := range 30 {
		if allowed, _ := limiter.Allow("a"); !allowed {
			t.Fatalf("refilled request %d denied", i)
		}
	}
	if allowed, _ := limiter.Allow("a"); allowed {
		t.Fatal("request beyond the refill allowed")
	}

	now = now.Add(time.Hour)
	for i := range 60 {
		if allowed, _ := limiter.Allow("a"); !allowed {
			t.Fatalf("capped request %d denied", i)
		}
	}
	if allowed, _ := limiter.Allow("a"); allowed {
		t.Fatal("bucket refilled past its burst")
	}
}

func TestLimiterKeepsKeysIndependent(t *testing.T) {
	limiter := NewLimiterWithClock(1, 10, func() time.Time { return time.Unix(0, 0) })
	if allowed, _ := limiter.Allow("a"); !allowed {
		t.Fatal("first key denied")
	}
	if allowed, _ := limiter.Allow("b"); !allowed {
		t.Fatal("second key denied by the first key's budget")
	}
}

func TestLimiterEvictsOldestKey(t *testing.T) {
	limiter := NewLimiterWithClock(1, 2, func() time.Time { return time.Unix(0, 0) })
	for _, key := range []string{"a", "b"} {
		if allowed, _ := limiter.Allow(key); !allowed {
			t.Fatalf("%s denied", key)
		}
	}
	if allowed, _ := limiter.Allow("c"); !allowed {
		t.Fatal("third key denied")
	}

	limiter.mu.Lock()
	size := len(limiter.entries)
	_, kept := limiter.entries["a"]
	limiter.mu.Unlock()

	if size != 2 {
		t.Fatalf("tracked keys = %d, want 2", size)
	}
	if kept {
		t.Fatal("oldest key was not evicted")
	}
	if allowed, _ := limiter.Allow("a"); !allowed {
		t.Fatal("an evicted key must start with a full bucket")
	}
}

func TestIPPolicyClientKey(t *testing.T) {
	trusted := IPPolicy{TrustProxy: true, Trusted: []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}}
	direct := IPPolicy{}

	cases := []struct {
		name    string
		policy  IPPolicy
		remote  string
		headers map[string]string
		want    string
		wantErr bool
	}{
		{"socket only", direct, "198.51.100.4:1234", nil, "198.51.100.4", false},
		{"forwarded ignored", direct, "198.51.100.4:1234", map[string]string{
			"CF-Connecting-IP": "203.0.113.9", "X-Forwarded-For": "203.0.113.9",
		}, "198.51.100.4", false},
		{"trusted peer uses CF", trusted, "127.0.0.1:1234", map[string]string{
			"CF-Connecting-IP": "203.0.113.9",
		}, "203.0.113.9", false},
		{"trusted peer without CF", trusted, "127.0.0.1:1234", nil, "", true},
		{"trusted peer with a list", trusted, "127.0.0.1:1234", map[string]string{
			"CF-Connecting-IP": "203.0.113.9, 10.0.0.1",
		}, "", true},
		{"trusted peer with host port", trusted, "127.0.0.1:1234", map[string]string{
			"CF-Connecting-IP": "203.0.113.9:443",
		}, "", true},
		{"trusted peer with CIDR text", trusted, "127.0.0.1:1234", map[string]string{
			"CF-Connecting-IP": "203.0.113.0/24",
		}, "", true},
		{"untrusted peer ignores CF", trusted, "198.51.100.4:1234", map[string]string{
			"CF-Connecting-IP": "203.0.113.9",
		}, "198.51.100.4", false},
		{"mapped v4", direct, "[::ffff:192.0.2.7]:1234", nil, "192.0.2.7", false},
		{"ipv6", direct, "[2001:db8::1]:1234", nil, "2001:db8::1", false},
		{"malformed peer", direct, "not-an-address", nil, invalidPeerKey, false},
		{"zoned peer", direct, "[fe80::1%eth0]:1234", nil, invalidPeerKey, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/x", nil)
			req.RemoteAddr = tc.remote
			for name, value := range tc.headers {
				req.Header.Set(name, value)
			}
			key, err := tc.policy.ClientKey(req)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("accepted %q", key)
				}
				return
			}
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if key != tc.want {
				t.Fatalf("key = %q, want %q", key, tc.want)
			}
		})
	}
}

func TestIPPolicyRejectsDuplicateProxyHeader(t *testing.T) {
	policy := IPPolicy{TrustProxy: true, Trusted: []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}}
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Add("CF-Connecting-IP", "203.0.113.9")
	req.Header.Add("CF-Connecting-IP", "203.0.113.10")
	if _, err := policy.ClientKey(req); err == nil {
		t.Fatal("duplicate proxy header accepted")
	}
}

func TestSemaphoreBoundsSlots(t *testing.T) {
	semaphore := NewSemaphore(2)
	if !semaphore.Acquire() || !semaphore.Acquire() {
		t.Fatal("two permits rejected")
	}
	if semaphore.Acquire() {
		t.Fatal("third permit granted")
	}
	semaphore.Release()
	if !semaphore.Acquire() {
		t.Fatal("released permit not granted")
	}
	semaphore.Release()
	semaphore.Release()
	if !semaphore.Acquire() || !semaphore.Acquire() {
		t.Fatal("extra release reduced the capacity")
	}
	if semaphore.Acquire() {
		t.Fatal("capacity exceeded after an extra release")
	}
}
