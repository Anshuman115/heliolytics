package api

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/heliolytics/api/internal/store"
)

func TestCoverageResponseSerializesWorkoutTypesAsNull(t *testing.T) {
	cov := store.DataCoverage{
		HasData: false,
		Types: map[string]*time.Time{
			"0x05": nil,
			"0x06": nil,
			"0x3B": nil,
		},
	}
	out := buildCoverageResponse(cov)
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	types, ok := decoded["types"].(map[string]any)
	if !ok {
		t.Fatalf("response missing types map: %s", raw)
	}
	for _, key := range []string{"0x05", "0x06", "0x3B"} {
		v, ok := types[key]
		if !ok {
			t.Fatalf("types missing key %s", key)
		}
		if v != nil {
			t.Fatalf("types[%s] want null got %v", key, v)
		}
	}
}
