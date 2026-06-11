package parse

func decodeWorkoutMsg(b []byte, start int) (*WorkoutRecord, int) {
	end := start
	o := start
	var startSec *int64
	var sport, duration int
	var calories, avgHr, maxHr *int
	for o < len(b) {
		field, wire, n := readTag(b, o)
		o = n
		switch wire {
		case 0:
			v, n2 := readVarint(b, o)
			o = n2
			if field == 1 && plausibleEpoch(v) && startSec == nil {
				s := int64(v)
				startSec = &s
			}
			if field == 3 && sport == 0 {
				sport = v
			}
			if field == 7 && duration == 0 {
				duration = v
			}
		case 2:
			chunk, n2 := readBytes(b, o)
			o = n2
			switch field {
			case 2:
				if ts := decodeWorkoutStart(chunk); startSec == nil && ts != nil {
					startSec = ts
				}
			case 3:
				if sp := decodeSportField(chunk); sport == 0 && sp != 0 {
					sport = sp
				}
			case 7:
				if d := decodeDurationField(chunk); duration == 0 && d > 0 {
					duration = d
				}
			case 16:
				if c := decodeScalarField(chunk, 1); c != nil {
					calories = c
				}
			case 19:
				avgHr, maxHr = decodeHrStats(chunk)
			}
		default:
			return nil, end - start
		}
		end = o
	}
	if startSec == nil || duration == 0 {
		return nil, maxInt(1, end-start)
	}
	return &WorkoutRecord{
		DayKey: IstDayKey(*startSec), StartedAt: EpochUTC(*startSec),
		SportType: sport, DurationSec: duration,
		Calories: calories, AvgHr: avgHr, MaxHr: maxHr,
	}, end - start
}

func decodeWorkoutStart(b []byte) *int64 {
	o := 0
	for o < len(b) {
		field, wire, n := readTag(b, o)
		o = n
		if wire != 0 {
			break
		}
		v, n2 := readVarint(b, o)
		o = n2
		if field == 1 && plausibleEpoch(v) {
			s := int64(v)
			return &s
		}
	}
	return nil
}

func decodeSportField(b []byte) int {
	o := 0
	for o < len(b) {
		field, wire, n := readTag(b, o)
		o = n
		if wire != 0 {
			break
		}
		v, _ := readVarint(b, o)
		if field == 1 {
			return v
		}
	}
	return 0
}

func decodeDurationField(b []byte) int {
	o := 0
	best := 0
	for o < len(b) {
		field, wire, n := readTag(b, o)
		o = n
		if wire != 0 {
			break
		}
		v, n2 := readVarint(b, o)
		o = n2
		if field == 1 || field == 12 {
			if v > best {
				best = v
			}
		}
	}
	return best
}

func decodeScalarField(b []byte, want int) *int {
	o := 0
	for o < len(b) {
		field, wire, n := readTag(b, o)
		o = n
		if wire != 0 {
			break
		}
		v, _ := readVarint(b, o)
		if field == want {
			return &v
		}
	}
	return nil
}

func decodeHrStats(b []byte) (avg, max *int) {
	o := 0
	for o < len(b) {
		field, wire, n := readTag(b, o)
		o = n
		if wire != 0 {
			break
		}
		v, n2 := readVarint(b, o)
		o = n2
		if field == 1 {
			avg = &v
		}
		if field == 2 {
			max = &v
		}
	}
	return avg, max
}

func plausibleEpoch(v int) bool { return v > 1500000000 && v < 2000000000 }

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
