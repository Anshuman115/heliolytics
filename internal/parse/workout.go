package parse

import (
	"bytes"
	"sort"
)

var workoutVersionMarker = []byte{0x0a, 0x03, 0x32, 0x2e}

// parseWorkoutSummaries splits 0x05 payloads on the version marker. Only a
// confirmed summary creates a workout; detail blocks never do.
func parseWorkoutSummaries(raw []byte, window Window) ([]WorkoutRow, TypeResult, error) {
	starts := markerOffsets(raw, workoutVersionMarker)
	if len(starts) == 0 {
		return nil, TypeResult{}, newError(ErrCodeUnsupportedVersion, typeWorkout, 0,
			"payload carries no supported workout version marker")
	}
	byStart := make(map[int64]WorkoutRow, len(starts))
	droppedRecords, droppedStats := 0, 0
	firstStatsOffset := -1
	for i, start := range starts {
		end := len(raw)
		if i+1 < len(starts) {
			end = starts[i+1]
		}
		summary, statsDropped, err := decodeWorkoutSummary(raw[start:end], start)
		if err != nil {
			return nil, TypeResult{}, err
		}
		if statsDropped > 0 {
			droppedStats += statsDropped
			if firstStatsOffset < 0 {
				firstStatsOffset = start
			}
		}
		if summary == nil {
			droppedRecords++
			continue
		}
		row, ok := summary.row(window)
		if !ok {
			droppedRecords++
			continue
		}
		byStart[row.StartedAt.Unix()] = row
	}
	out := make([]WorkoutRow, 0, len(byStart))
	for _, row := range byStart {
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.Before(out[j].StartedAt) })
	result := TypeResult{
		Status:  "accepted",
		Rows:    len(out),
		Dropped: droppedRecords + droppedStats,
	}
	if droppedStats > 0 {
		result.Warnings = append(result.Warnings, Warning{
			Code: "workout_stat_out_of_range", ByteOffset: firstStatsOffset, Count: droppedStats,
		})
	}
	for i := range out {
		result.ProcessedThrough = maxTime(result.ProcessedThrough, out[i].EndedAt)
	}
	return out, result, nil
}

func markerOffsets(raw, marker []byte) []int {
	var out []int
	for offset := 0; offset+len(marker) <= len(raw); {
		found := bytes.Index(raw[offset:], marker)
		if found < 0 {
			break
		}
		out = append(out, offset+found)
		offset += found + 1
	}
	return out
}

// row converts a decoded summary into a row when its interval fits the window.
func (s *workoutSummary) row(window Window) (WorkoutRow, bool) {
	if !anchorPlausible(s.startedAt) || s.durationSec <= 0 || s.durationSec > maxAnchorSec {
		return WorkoutRow{}, false
	}
	end := s.startedAt + s.durationSec
	if end > maxAnchorSec {
		return WorkoutRow{}, false
	}
	start := EpochUTC(s.startedAt)
	stopped := EpochUTC(end)
	if !window.containsInterval(start, stopped) {
		return WorkoutRow{}, false
	}
	return WorkoutRow{
		StartedAt: start, EndedAt: stopped,
		SportType: s.sportType, SportName: SportName(s.sportType),
		Calories: s.calories, AvgHR: s.avgHR, MaxHR: s.maxHR, MinHR: s.minHR,
	}, true
}
