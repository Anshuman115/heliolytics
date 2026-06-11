package parse

var psmHeader = []byte{0x01, 0x0c, 'p', 's', 'm', 'h'}

// ParseWorkoutDetails extracts sessions from 0x06 detail blobs.
// Uses embedded protobuf summaries inside each psmh block — not raw epoch scans.
func ParseWorkoutDetails(raw []byte) []WorkoutRecord {
	return ParseWorkoutsFromDetailBlob(raw)
}

// ParseWorkoutsFromDetailBlob splits on psmh headers and parses protobuf summaries.
func ParseWorkoutsFromDetailBlob(raw []byte) []WorkoutRecord {
	if len(raw) < len(psmHeader) {
		return nil
	}
	var out []WorkoutRecord
	seen := map[int64]bool{}
	for i := 0; i+len(psmHeader) <= len(raw); i++ {
		if !matchHeader(raw, i) {
			continue
		}
		end := nextHeader(raw, i+len(psmHeader))
		for _, w := range ParseWorkouts(raw[i:end]) {
			key := w.StartedAt.Unix()
			if seen[key] {
				continue
			}
			seen[key] = true
			if w.SportName == "" {
				w.SportName = SportName(w.SportType)
			}
			out = append(out, w)
		}
	}
	return out
}

func matchHeader(b []byte, i int) bool {
	for j, v := range psmHeader {
		if b[i+j] != v {
			return false
		}
	}
	return true
}

func nextHeader(b []byte, from int) int {
	for i := from; i+len(psmHeader) <= len(b); i++ {
		if matchHeader(b, i) {
			return i
		}
	}
	return len(b)
}

func MergeWorkouts(parts ...[]WorkoutRecord) []WorkoutRecord {
	byStart := map[int64]WorkoutRecord{}
	for _, list := range parts {
		for _, w := range list {
			key := w.StartedAt.Unix()
			prev, ok := byStart[key]
			if !ok {
				if w.SportName == "" {
					w.SportName = SportName(w.SportType)
				}
				byStart[key] = w
				continue
			}
			byStart[key] = mergeWorkout(prev, w)
		}
	}
	out := make([]WorkoutRecord, 0, len(byStart))
	for _, w := range byStart {
		out = append(out, w)
	}
	return out
}

func mergeWorkout(a, b WorkoutRecord) WorkoutRecord {
	if workoutRichness(b) > workoutRichness(a) {
		a, b = b, a
	}
	if b.SportType != 0 && a.SportType != b.SportType {
		a.SportType = b.SportType
		a.SportName = SportName(b.SportType)
	}
	if b.DurationSec > a.DurationSec {
		a.DurationSec = b.DurationSec
	}
	if a.Calories == nil && b.Calories != nil {
		a.Calories = b.Calories
	}
	if a.AvgHr == nil && b.AvgHr != nil {
		a.AvgHr = b.AvgHr
	}
	if a.MaxHr == nil && b.MaxHr != nil {
		a.MaxHr = b.MaxHr
	}
	if a.SportName == "" {
		a.SportName = SportName(a.SportType)
	}
	return a
}

func workoutRichness(w WorkoutRecord) int {
	n := 0
	if w.Calories != nil {
		n += 2
	}
	if w.AvgHr != nil {
		n += 2
	}
	if w.MaxHr != nil {
		n += 1
	}
	if w.SportType != 0 {
		n += 1
	}
	if w.DurationSec > 0 {
		n += 1
	}
	return n
}
