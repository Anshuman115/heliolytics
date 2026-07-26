package parse

import (
	"sort"
	"time"
)

func ParseActivityHrSeries(raw, catalogJSON []byte, fetchEnd time.Time) []HrSamplePoint {
	const stride = 8
	if len(raw) < stride {
		return nil
	}
	entry := FindEntry(ParseCatalog(catalogJSON), "0x01")
	segments := buildByteSegments(raw, entry, fetchEnd, stride)
	var out []HrSamplePoint
	for i, segment := range segments {
		end := len(raw)
		if i+1 < len(segments) {
			end = segments[i+1].byteStart
		}
		sec := segment.rs
		for off := segment.byteStart; off+stride <= end; off += stride {
			bpm := int(raw[off+3])
			if bpm >= 30 && bpm <= 220 {
				out = append(out, HrSamplePoint{
					Ts: EpochUTC(sec), DayKey: IstDayKey(sec), Bpm: bpm, SourceType: "0x01",
				})
			}
			sec += 60
		}
	}
	return out
}

func MergeHeartRateSeries(parts ...[]HrSamplePoint) []HrSamplePoint {
	byTimestamp := make(map[int64]HrSamplePoint)
	for _, part := range parts {
		for _, point := range part {
			byTimestamp[point.Ts.Unix()] = point
		}
	}
	out := make([]HrSamplePoint, 0, len(byTimestamp))
	for _, point := range byTimestamp {
		out = append(out, point)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Ts.Before(out[j].Ts) })
	return out
}
