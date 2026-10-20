package store

import (
	"encoding/json"
	"fmt"
	"time"
)

// SleepStagePoint is one end-exclusive stage interval.
type SleepStagePoint struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
	Type  int       `json:"type"`
}

// encodeSleepStages writes the end-exclusive stage array; empty is [].
func encodeSleepStages(st []SleepStagePoint) ([]byte, error) {
	if st == nil {
		st = []SleepStagePoint{}
	}
	return json.Marshal(st)
}

// decodeSleepStages rejects malformed stored stages instead of dropping them.
func decodeSleepStages(raw []byte) ([]SleepStagePoint, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("sleep session stages missing")
	}
	var out []SleepStagePoint
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("sleep session stages invalid: %w", err)
	}
	return out, nil
}
