package parse

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/heliolytics/api/internal/store"
)

func TestParseCatalogRejectsMalformedJSON(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{name: "truncated object", raw: `{"chunked":[`},
		{name: "wrong root type", raw: `[]`},
		{name: "null root", raw: `null`},
		{name: "whitespace only", raw: " \n\t"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseCatalog([]byte(tt.raw))
			if !errors.Is(err, ErrInvalidCatalog) {
				t.Fatalf("error=%v, want ErrInvalidCatalog", err)
			}
		})
	}
}

func TestParseCatalogAllowsEmptyCompatibilityInput(t *testing.T) {
	cat, err := ParseCatalog(nil)
	if err != nil || len(cat.Chunked) != 0 {
		t.Fatalf("catalog=%+v error=%v", cat, err)
	}
}

func TestRunIngestPropagatesCatalogErrorBeforeStore(t *testing.T) {
	meta := store.SessionMeta{CatalogJSON: []byte(`{"chunked":[`)}
	err := RunIngest(context.Background(), nil, meta, nil, time.Now().UTC())
	if !errors.Is(err, ErrInvalidCatalog) {
		t.Fatalf("error=%v, want ErrInvalidCatalog", err)
	}
	if !strings.Contains(err.Error(), "parse blobs") {
		t.Fatalf("error=%q, want parse pipeline context", err)
	}
}
