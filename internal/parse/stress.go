package parse

type StressSample struct {
	DayKey string
	Value  int
}

func ParseStress(raw []byte, entry *CatalogEntry) []StressSample {
	if entry == nil || len(raw) == 0 {
		return nil
	}
	var out []StressSample
	if len(entry.RoundSegments) > 0 {
		for si, seg := range entry.RoundSegments {
			start := seg.ByteOffset
			end := len(raw)
			if si+1 < len(entry.RoundSegments) {
				end = entry.RoundSegments[si+1].ByteOffset
			}
			if start >= len(raw) {
				continue
			}
			if end > len(raw) {
				end = len(raw)
			}
			rs := ParseRoundStartIst(seg.RoundStart)
			out = append(out, parseStressChunk(raw[start:end], rs)...)
		}
		return out
	}
	rs := ParseRoundStartIst(entry.RoundStart)
	return parseStressChunk(raw, rs)
}

func parseStressChunk(raw []byte, rs int64) []StressSample {
	var out []StressSample
	ts := rs
	for _, b := range raw {
		if b != 0xFF {
			out = append(out, StressSample{DayKey: IstDayKey(ts), Value: int(b)})
		}
		ts += 60
	}
	return out
}
