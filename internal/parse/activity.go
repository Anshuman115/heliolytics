package parse

import "time"

const activityStride = 8

// activityRecord is one decoded 0x01 minute. Steps and BPM keep their raw wire
// values so the emitters can tell a known zero from a missing reading.
type activityRecord struct {
	at    time.Time
	off   int
	steps byte
	bpm   byte
}

func activityRecords(raw []byte, catalog Catalog) ([]activityRecord, int, error) {
	minutes, trailing, err := minuteRecords(typeActivity, raw, FindEntry(catalog, typeActivity), activityStride)
	if err != nil {
		return nil, 0, err
	}
	out := make([]activityRecord, len(minutes))
	for i, minute := range minutes {
		out[i] = activityRecord{
			at:    minute.at,
			off:   minute.off,
			steps: raw[minute.off+2],
			bpm:   raw[minute.off+3],
		}
	}
	return out, trailing, nil
}

// parseActivity emits one step row and one HR row per accepted 0x01 minute.
func parseActivity(raw []byte, catalog Catalog, window Window) ([]HeartRateRow, []StepRow, TypeResult, error) {
	records, trailing, err := activityRecords(raw, catalog)
	if err != nil {
		return nil, nil, TypeResult{}, err
	}
	heartRate, heartRateDropped := heartRateRows(records, window)
	steps, stepsDropped := stepRows(records, window)
	result := TypeResult{
		Status:        statusFor(trailing),
		Rows:          len(heartRate) + len(steps),
		Dropped:       heartRateDropped + stepsDropped,
		TrailingBytes: trailing,
	}
	if trailing > 0 {
		result.Warnings = append(result.Warnings, Warning{
			Code: "trailing_fragment", ByteOffset: len(raw) - trailing, Count: trailing,
		})
	}
	for i := range heartRate {
		result.ProcessedThrough = maxTime(result.ProcessedThrough, heartRate[i].SampledAt)
	}
	for i := range steps {
		result.ProcessedThrough = maxTime(result.ProcessedThrough, steps[i].SampledAt)
	}
	return heartRate, steps, result, nil
}
