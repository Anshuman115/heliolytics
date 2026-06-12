package store

import (
	"context"
	"os"
	"testing"
)

func testStore(t *testing.T) (*Store, func()) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = os.Getenv("DATABASE_URL")
	}
	if url == "" {
		t.Skip("TEST_DATABASE_URL or DATABASE_URL not set")
	}
	ctx := context.Background()
	st, err := New(ctx, url)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := st.Ping(ctx); err != nil {
		st.Close()
		t.Fatalf("Ping: %v", err)
	}
	return st, func() { st.Close() }
}

func TestGetCoverageIncludesWorkoutTypeKeys(t *testing.T) {
	st, cleanup := testStore(t)
	defer cleanup()

	cov, err := st.GetCoverage(context.Background())
	if err != nil {
		t.Fatalf("GetCoverage: %v", err)
	}
	for _, key := range []string{"0x05", "0x06", "0x3B"} {
		if _, ok := cov.Types[key]; !ok {
			t.Fatalf("Types missing key %s", key)
		}
	}
}
