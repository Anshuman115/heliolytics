package parse

import "sort"

const (
	hrMinBPM     = 25
	hrMaxBPM     = 250
	hrAbsentZero = 0
	hrAbsentMax  = 0xFF
)

// heartRateRows emits one row per 0x01 minute that carries a valid BPM. Zero
// and 0xff are missing readings; a real value outside 25..250 is dropped.
func heartRateRows(records []activityRecord, window Window) ([]HeartRateRow, int) {
	byMinute := make(map[int64]HeartRateRow, len(records))
	dropped := 0
	for _, record := range records {
		if !window.contains(record.at) {
			dropped++
			continue
		}
		if record.bpm == hrAbsentZero || record.bpm == hrAbsentMax ||
			record.bpm < hrMinBPM || record.bpm > hrMaxBPM {
			dropped++
			continue
		}
		byMinute[record.at.Unix()] = HeartRateRow{
			SampledAt: record.at, Source: typeActivity, BPM: int(record.bpm),
		}
	}
	out := make([]HeartRateRow, 0, len(byMinute))
	for _, row := range byMinute {
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SampledAt.Before(out[j].SampledAt) })
	return out, dropped
}
