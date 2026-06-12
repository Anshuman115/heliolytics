package api

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/heliolytics/api/internal/store"
)

func TestCoverageResponseSerializesWorkoutTypesAsNull(t *testing.T) {
	types := map[string]*time.Time{}
	for _, key := range store.SyncedBLETypeCodes {
		types[key] = nil
	}
	cov := store.DataCoverage{HasData: false, Types: types}
	out := buildCoverageResponse(cov)
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	decodedTypes, ok := decoded["types"].(map[string]any)
	if !ok {
		t.Fatalf("response missing types map: %s", raw)
	}
	for _, key := range store.SyncedBLETypeCodes {
		v, ok := decodedTypes[key]
		if !ok {
			t.Fatalf("types missing key %s", key)
		}
		if v != nil {
			t.Fatalf("types[%s] want null got %v", key, v)
		}
	}
}
