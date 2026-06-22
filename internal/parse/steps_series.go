package parse

import "time"

// StepSample is one per-minute step count from BLE 0x01, timestamped absolutely
// so it can be stored idempotently (overlapping re-syncs overwrite, not add).
type StepSample struct {
	SampledAt time.Time
	DayKey    string
	Steps     int
}

// ParseStepSeries emits a per-minute step sample for every 0x01 minute that has
// a real step reading (non-zero, not the 0xFF "no reading" sentinel). It mirrors
// SumStepsByDay's minute walk, so the SUM of these samples equals that day total.
func ParseStepSeries(raw []byte, catalogJSON []byte, fetchEnd time.Time) []StepSample {
	stride := activityStrideFor(len(raw))
	if len(raw) < stride {
		return nil
	}
	entry := FindEntry(ParseCatalog(catalogJSON), "0x01")
	segs := buildByteSegments(raw, entry, fetchEnd, stride)
	if len(segs) == 0 {
		return nil
	}
	var out []StepSample
	for si, seg := range segs {
		end := len(raw)
		if si+1 < len(segs) {
			end = segs[si+1].byteStart
		}
		ts := seg.rs
		for off := seg.byteStart; off+stride <= end; off += stride {
			st := int(raw[off+2])
			if st != 0 && st != 0xFF {
				out = append(out, StepSample{
					SampledAt: time.Unix(ts, 0).UTC(),
					DayKey:    IstDayKey(ts),
					Steps:     st,
				})
			}
			ts += 60
		}
	}
	return out
}
