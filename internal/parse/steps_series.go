package parse

import "sort"

const stepsMissing = 0xFF

// stepRows emits one row per 0x01 minute with a real step reading. The wire
// value 0xff means the strap recorded nothing; a known zero is a real row.
func stepRows(records []activityRecord, window Window) ([]StepRow, int) {
	byMinute := make(map[int64]StepRow, len(records))
	dropped := 0
	for _, record := range records {
		if !window.contains(record.at) || record.steps == stepsMissing {
			dropped++
			continue
		}
		byMinute[record.at.Unix()] = StepRow{SampledAt: record.at, Steps: int(record.steps)}
	}
	out := make([]StepRow, 0, len(byMinute))
	for _, row := range byMinute {
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SampledAt.Before(out[j].SampledAt) })
	return out, dropped
}
