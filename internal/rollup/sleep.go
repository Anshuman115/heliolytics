package rollup

import (
	"math"
	"sort"
	"time"

	"github.com/Anshuman115/heliolytics/internal/store"
)

const nightFloorMinutes = 180

type span struct {
	Start time.Time
	End   time.Time
}

// unionSpans sorts and merges touching or overlapping half-open intervals.
// Inter-session gaps are never filled, so samples in them cannot contribute.
func unionSpans(in []span) []span {
	if len(in) == 0 {
		return nil
	}
	sorted := make([]span, len(in))
	copy(sorted, in)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Start.Before(sorted[j].Start) })
	out := make([]span, 0, len(sorted))
	for _, s := range sorted {
		if !s.End.After(s.Start) {
			continue
		}
		last := len(out) - 1
		if last >= 0 && !s.Start.After(out[last].End) {
			if s.End.After(out[last].End) {
				out[last].End = s.End
			}
			continue
		}
		out = append(out, s)
	}
	return out
}

func totalMinutes(spans []span) int {
	var total float64
	for _, s := range spans {
		total += s.End.Sub(s.Start).Minutes()
	}
	return int(math.Round(total))
}

type sleepDay struct {
	union      []span
	minutes    int
	quality    bool
	naps       int
	score      *int
	asleep     *int
	deep       *int
	rem        *int
	light      *int
	wake       *int
	inBed      *int
	efficiency *int
}

func summarizeSleep(rows []store.SleepRow, day time.Time) sleepDay {
	var out sleepDay
	spans := make([]span, 0, len(rows))
	var best *store.SleepRow
	asleep, deep, rem, light, wake := 0, 0, 0, 0, 0
	for i := range rows {
		row := rows[i]
		if !sameDay(row.DayKey, day) {
			continue
		}
		if row.IsNap {
			out.naps++
			continue
		}
		spans = append(spans, span{Start: row.StartedAt, End: row.EndedAt})
		asleep += row.TotalMins
		deep += row.DeepMins
		rem += row.RemMins
		light += row.LightMins
		wake += row.WakeMins
		if best == nil || row.TotalMins > best.TotalMins ||
			(row.TotalMins == best.TotalMins && row.StartedAt.Before(best.StartedAt)) {
			best = &rows[i]
		}
	}
	out.union = unionSpans(spans)
	out.minutes = totalMinutes(out.union)
	out.quality = best != nil && out.minutes >= nightFloorMinutes
	if best == nil {
		return out
	}
	out.score = best.Score
	out.asleep = new(asleep)
	out.deep = new(deep)
	out.rem = new(rem)
	out.light = new(light)
	out.wake = new(wake)
	inBed := asleep + wake
	out.inBed = new(inBed)
	if inBed > 0 {
		out.efficiency = new(int(math.Round(100 * float64(asleep) / float64(inBed))))
	}
	return out
}

func sameDay(a, b time.Time) bool {
	return a.Year() == b.Year() && a.Month() == b.Month() && a.Day() == b.Day()
}
