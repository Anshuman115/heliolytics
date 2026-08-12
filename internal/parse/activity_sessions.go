package parse

import "time"

const (
	activityRecordStride     = 8
	activityMinBpm           = 90
	activityMinIntensity     = 50
	activityMaxGap           = 5 * time.Minute
	activityMinDuration      = 10 * time.Minute
	activityMinQualifiedMins = 8
)

type activityMinute struct {
	ts        time.Time
	intensity int
	steps     int
	bpm       int
}

func DetectActivitySessions(raw, catalogJSON []byte, fetchEnd time.Time) []WorkoutRecord {
	minutes := parseActivityMinutes(raw, catalogJSON, fetchEnd)
	qualified := make([]activityMinute, 0)
	for _, minute := range minutes {
		if minute.bpm >= activityMinBpm && minute.intensity >= activityMinIntensity &&
			minute.steps != 0xff {
			qualified = append(qualified, minute)
		}
	}
	if len(qualified) == 0 {
		return nil
	}

	var out []WorkoutRecord
	start := 0
	for i := 1; i <= len(qualified); i++ {
		if i < len(qualified) && qualified[i].ts.Sub(qualified[i-1].ts) <= activityMaxGap {
			continue
		}
		if session := buildDetectedSession(qualified[start:i], minutes); session != nil {
			out = append(out, *session)
		}
		start = i
	}
	return out
}

func parseActivityMinutes(raw, catalogJSON []byte, fetchEnd time.Time) []activityMinute {
	if len(raw) < activityRecordStride {
		return nil
	}
	cat, err := ParseCatalog(catalogJSON)
	if err != nil {
		return nil
	}
	entry := FindEntry(cat, "0x01")
	segments := buildByteSegments(raw, entry, fetchEnd, activityRecordStride)
	var out []activityMinute
	for i, segment := range segments {
		end := len(raw)
		if i+1 < len(segments) {
			end = segments[i+1].byteStart
		}
		sec := segment.rs
		for off := segment.byteStart; off+activityRecordStride <= end; off += activityRecordStride {
			out = append(out, activityMinute{
				ts: EpochUTC(sec), intensity: int(raw[off+1]),
				steps: int(raw[off+2]), bpm: int(raw[off+3]),
			})
			sec += 60
		}
	}
	return out
}

func buildDetectedSession(active, all []activityMinute) *WorkoutRecord {
	if len(active) < activityMinQualifiedMins {
		return nil
	}
	start, end := active[0].ts, active[len(active)-1].ts.Add(time.Minute)
	if end.Sub(start) < activityMinDuration {
		return nil
	}
	var total, count, maxBpm int
	for _, minute := range all {
		if minute.ts.Before(start) || !minute.ts.Before(end) || minute.bpm < 30 || minute.bpm > 220 {
			continue
		}
		total += minute.bpm
		count++
		if minute.bpm > maxBpm {
			maxBpm = minute.bpm
		}
	}
	if count == 0 {
		return nil
	}
	avgBpm := total / count
	return &WorkoutRecord{
		DayKey: IstDayKey(start.Unix()), StartedAt: start,
		SportName: "Activity", DurationSec: int(end.Sub(start).Seconds()),
		AvgHr: &avgBpm, MaxHr: &maxBpm,
	}
}
