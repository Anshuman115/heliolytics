package store

import (
	"context"
	"sync"
	"testing"
	"time"
)

func mustConsume(t *testing.T, st *Store, kid, nonce string, expiresAt time.Time) bool {
	t.Helper()
	ok, err := st.ConsumeNonce(context.Background(), kid, nonce, expiresAt)
	if err != nil {
		t.Fatalf("consume nonce: %v", err)
	}
	return ok
}

func TestConsumeNonceRejectsReplay(t *testing.T) {
	st := newTestStore(t)
	now := time.Now().UTC()
	const nonce = "00112233445566778899aabbccddeeff"

	if !mustConsume(t, st, "phone", nonce, now.Add(time.Minute)) {
		t.Fatal("first consume must succeed")
	}
	if mustConsume(t, st, "phone", nonce, now.Add(time.Minute)) {
		t.Fatal("replay of a live nonce must be rejected")
	}
	if !mustConsume(t, st, "web", nonce, now.Add(time.Minute)) {
		t.Fatal("a different kid must be independent")
	}
}

func TestConsumeNonceIsAtomicUnderConcurrency(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	const nonce = "ffeeddccbbaa99887766554433221100"

	var wg sync.WaitGroup
	results := make(chan bool, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := st.ConsumeNonce(ctx, "phone", nonce, now.Add(time.Minute))
			if err != nil {
				results <- false
				return
			}
			results <- ok
		}()
	}
	wg.Wait()
	close(results)
	wins := 0
	for ok := range results {
		if ok {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("concurrent consumers accepted %d times, want exactly 1", wins)
	}
}

func TestConsumeNonceReusesExpiredPair(t *testing.T) {
	st := newTestStore(t)
	now := time.Now().UTC()
	const nonce = "102132435465768798a9bacbdcedfe0f"

	if !mustConsume(t, st, "phone", nonce, now.Add(-time.Minute)) {
		t.Fatal("expired pair insert must succeed")
	}
	if !mustConsume(t, st, "phone", nonce, now.Add(-time.Minute)) {
		t.Fatal("an already expired pair must be reusable")
	}
	if !mustConsume(t, st, "phone", nonce, now.Add(time.Minute)) {
		t.Fatal("a live expiry must be claimable once the stored one is expired")
	}
	if mustConsume(t, st, "phone", nonce, now.Add(time.Minute)) {
		t.Fatal("the pair is live and must reject a replay")
	}
}

func TestPurgeNoncesOnlyDeletesExpiredRows(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	if !mustConsume(t, st, "phone", "00112233445566778899aabbccddeeff", now.Add(-time.Hour)) {
		t.Fatal("expired insert")
	}
	if !mustConsume(t, st, "phone", "ffeeddccbbaa99887766554433221100", now.Add(time.Hour)) {
		t.Fatal("live insert")
	}
	deleted, err := st.PurgeNonces(ctx, now)
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("purged %d rows, want 1", deleted)
	}
	if n := countRows(t, st, "auth_nonces"); n != 1 {
		t.Fatalf("remaining nonces = %d, want the live row only", n)
	}
}

func TestConsumeNonceRejectsMalformedNonce(t *testing.T) {
	st := newTestStore(t)
	if _, err := st.ConsumeNonce(context.Background(), "phone", "not-a-nonce", time.Now().Add(time.Minute)); err == nil {
		t.Fatal("malformed nonce must fail at the database constraint")
	}
	if n := countRows(t, st, "auth_nonces"); n != 0 {
		t.Fatalf("nonces after a rejected insert = %d, want 0", n)
	}
}
