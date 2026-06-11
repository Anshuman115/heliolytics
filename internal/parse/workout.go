package parse

import "time"

var workoutMarker = []byte{0x0a, 0x03, 0x32, 0x2e}

type WorkoutRecord struct {
	DayKey      string
	StartedAt   time.Time
	SportType   int
	SportName   string
	DurationSec int
	Calories    *int
	AvgHr       *int
	MaxHr       *int
	MinHr       *int
}

// ParseWorkouts splits on protobuf version marker 0a 03 32 2e ("2.").
func ParseWorkouts(raw []byte) []WorkoutRecord {
	raw = trimWorkoutHeader(raw)
	var starts []int
	for i := 0; i+len(workoutMarker) <= len(raw); i++ {
		if matchBytes(raw, i, workoutMarker) {
			starts = append(starts, i)
		}
	}
	if len(starts) == 0 {
		return parseWorkoutLegacyStream(raw)
	}
	var out []WorkoutRecord
	for s := 0; s < len(starts); s++ {
		end := len(raw)
		if s+1 < len(starts) {
			end = starts[s+1] - 2
		}
		if starts[s] >= end {
			continue
		}
		if w := decodeWorkoutBlob(raw[starts[s]:end]); w != nil {
			w.SportName = SportName(w.SportType)
			out = append(out, *w)
		}
	}
	return out
}

func parseWorkoutLegacyStream(raw []byte) []WorkoutRecord {
	var out []WorkoutRecord
	for o := 0; o < len(raw); {
		w, n := decodeWorkoutMsg(raw, o)
		if n <= 0 {
			break
		}
		if w != nil {
			w.SportName = SportName(w.SportType)
			out = append(out, *w)
		}
		o += n
	}
	return out
}

func decodeWorkoutBlob(blob []byte) *WorkoutRecord {
	top := readProtoMsg(blob)
	meta := protoSub(top, 2)
	dur := protoSub(top, 7)
	cal := protoSub(top, 16)
	hr := protoSub(top, 19)
	startSec := protoInt(meta, 1)
	if startSec == nil || *startSec < 1000000000 {
		return nil
	}
	duration := protoInt(dur, 1)
	if duration == nil || *duration == 0 {
		return nil
	}
	sportVal := pickSportType(blob, meta)
	var calories, avg, max, min *int
	if c := protoInt(cal, 1); c != nil {
		calories = c
	}
	if a := protoInt(hr, 1); a != nil {
		avg = a
	}
	if m := protoInt(hr, 2); m != nil {
		max = m
	}
	if mn := protoInt(hr, 3); mn != nil {
		min = mn
	}
	sec := int64(*startSec)
	return &WorkoutRecord{
		DayKey: IstDayKey(sec), StartedAt: EpochUTC(sec),
		SportType: sportVal, DurationSec: *duration,
		Calories: calories, AvgHr: avg, MaxHr: max, MinHr: min,
	}
}

func matchBytes(b []byte, i int, p []byte) bool {
	for j, v := range p {
		if b[i+j] != v {
			return false
		}
	}
	return true
}

func trimWorkoutHeader(b []byte) []byte {
	for len(b) > 0 && b[0] == 0 {
		b = b[1:]
	}
	if len(b) > 1 && b[0] == 0x80 {
		b = b[1:]
	}
	return b
}
