package store

import (
	"encoding/json"
	"fmt"
	"time"
)

type SleepStagePoint struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
	Type  int       `json:"type"`
}

func encodeSleepStages(st []SleepStagePoint) ([]byte, error) {
	if st == nil {
		st = []SleepStagePoint{}
	}
	return json.Marshal(st)
}

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
