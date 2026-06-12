package parse

import (
	"context"
	"time"

	"github.com/heliolytics/api/internal/store"
)

func RunIngest(
	ctx context.Context,
	st *store.Store,
	syncSessionID string,
	catalogJSON []byte,
	blobs map[string][]byte,
	fetchEnd time.Time,
) error {
	parsed := ParseBlobs(catalogJSON, blobs, fetchEnd)
	agg := Aggregate(parsed)
	return WriteBatch(ctx, st, syncSessionID, agg)
}

func acc(m map[string]*DayAcc, day string) *DayAcc {
	if d, ok := m[day]; ok {
		return d
	}
	d := &DayAcc{}
	m[day] = d
	return d
}

func mergeSleep(days map[string]*DayAcc, recs []SleepRecord) {
	best := map[string]SleepRecord{}
	for _, s := range recs {
		if s.IsNap {
			continue
		}
		if prev, ok := best[s.DayKey]; !ok || s.Score > prev.Score {
			best[s.DayKey] = s
		}
	}
	for day, s := range best {
		a := acc(days, day)
		a.SleepScore = &s.Score
		a.SleepMins = s.TotalMin
		a.SleepDeep = s.DeepMin
		a.SleepRem = s.RemMin
		a.SleepLight = s.LightMin
	}
}
