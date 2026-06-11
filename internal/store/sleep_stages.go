package store

import (
	"encoding/json"
	"time"
)

type SleepStagePoint struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
	Type  int       `json:"type"`
}

func encodeSleepStages(st []SleepStagePoint) ([]byte, error) {
	if len(st) == 0 {
		return nil, nil
	}
	return json.Marshal(st)
}

func decodeSleepStages(raw []byte) []SleepStagePoint {
	if len(raw) == 0 {
		return nil
	}
	var out []SleepStagePoint
	if json.Unmarshal(raw, &out) != nil {
		return nil
	}
	return out
}
