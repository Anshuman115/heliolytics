package parse

import "time"

func activityStrideFor(n int) int {
	if n%8 == 0 {
		return 8
	}
	return 4
}

func SumStepsByDay(raw []byte, catalogJSON []byte, fetchEnd time.Time) map[string]int {
	stride := activityStrideFor(len(raw))
	if len(raw) < stride {
		return nil
	}
	cat, err := ParseCatalog(catalogJSON)
	if err != nil {
		return nil
	}
	entry := FindEntry(cat, "0x01")
	segs := buildByteSegments(raw, entry, fetchEnd, stride)
	if len(segs) == 0 {
		return nil
	}
	daily := map[string]int{}
	for si, seg := range segs {
		end := len(raw)
		if si+1 < len(segs) {
			end = segs[si+1].byteStart
		}
		ts := seg.rs
		for off := seg.byteStart; off+stride <= end; off += stride {
			st := int(raw[off+2])
			if st != 0 && st != 0xFF {
				daily[IstDayKey(ts)] += st
			}
			ts += 60
		}
	}
	return daily
}

type byteSeg struct {
	byteStart int
	rs        int64
}

func buildByteSegments(raw []byte, e *CatalogEntry, fetchEnd time.Time, stride int) []byteSeg {
	if stride <= 0 || len(raw) < stride {
		return nil
	}
	if e != nil && len(e.RoundSegments) > 0 {
		out := make([]byteSeg, 0, len(e.RoundSegments))
		previous := -1
		for _, s := range e.RoundSegments {
			rs := ParseRoundStartIst(s.RoundStart)
			if s.ByteOffset < 0 || s.ByteOffset >= len(raw) || s.ByteOffset%stride != 0 ||
				s.ByteOffset <= previous || !IsPlausibleUnixSec(rs) {
				return nil
			}
			out = append(out, byteSeg{s.ByteOffset, rs})
			previous = s.ByteOffset
		}
		return out
	}
	if e != nil && e.RoundStart != "" {
		rs := ParseRoundStartIst(e.RoundStart)
		if !IsPlausibleUnixSec(rs) {
			return nil
		}
		return []byteSeg{{0, rs}}
	}
	n := len(raw) / stride
	endSec := fetchEnd.UTC().Unix()
	base := endSec - int64(n-1)*60
	if n == 0 || !IsPlausibleUnixSec(base) || !IsPlausibleUnixSec(endSec) {
		return nil
	}
	return []byteSeg{{0, base}}
}

func countStepsRaw(raw []byte, start, end, stride int) int {
	total := 0
	for off := start; off+stride <= end; off += stride {
		st := int(raw[off+2])
		if st != 0 && st != 0xFF {
			total += st
		}
	}
	return total
}
