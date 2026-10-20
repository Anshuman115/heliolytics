//go:build heliolytics_smoke

package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestUISplitNightManifestStageTotals(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.json")
	uiWriteManifest(t, path, time.Date(2026, 10, 20, 0, 0, 0, 0, istZone), 0, nil)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Scenarios map[string]struct {
			Expected json.RawMessage `json:"expected"`
		} `json:"scenarios"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	var splitNight map[string]int
	if err := json.Unmarshal(manifest.Scenarios["split-night"].Expected, &splitNight); err != nil {
		t.Fatal(err)
	}
	want := map[string]int{"sleepMins": 460, "sleepDeepMins": 120, "sleepRemMins": 60, "sleepLightMins": 280, "sleepWakeMins": 20}
	for key, expected := range want {
		if got := splitNight[key]; got != expected {
			t.Errorf("split-night manifest %s = %d, want %d", key, got, expected)
		}
	}
}
