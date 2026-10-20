package parse

import (
	"encoding/binary"
	"sort"
	"time"
)

const continuousHRHeaderSize = 8

// parseContinuousHR decodes 0x46 counted frames: a u32 UTC epoch, a u32 sample
// count and that many one-second BPM bytes. A BPM byte run that happens to look
// like an epoch is still a sample, never a new header.
func parseContinuousHR(raw []byte, window Window) ([]HeartRateRow, TypeResult, error) {
	byTime := make(map[int64]HeartRateRow)
	dropped := 0
	pos := 0
	for pos < len(raw) {
		if len(raw)-pos < continuousHRHeaderSize {
			return nil, TypeResult{}, newError(ErrCodeInvalidFraming, typeContinuousHR, pos,
				"incomplete %d-byte frame header", continuousHRHeaderSize)
		}
		epoch := int64(binary.LittleEndian.Uint32(raw[pos:]))
		count := int(binary.LittleEndian.Uint32(raw[pos+4:]))
		available := len(raw) - pos - continuousHRHeaderSize
		if count > available {
			return nil, TypeResult{}, newError(ErrCodeCountOverflow, typeContinuousHR, pos,
				"declared %d samples exceed %d payload bytes", count, available)
		}
		start := EpochUTC(epoch)
		for i := range count {
			at := start.Add(time.Duration(i) * time.Second)
			bpm := raw[pos+continuousHRHeaderSize+i]
			if !window.contains(at) || bpm < hrMinBPM || bpm > hrMaxBPM {
				dropped++
				continue
			}
			byTime[at.Unix()] = HeartRateRow{SampledAt: at, Source: typeContinuousHR, BPM: int(bpm)}
		}
		pos += continuousHRHeaderSize + count
	}
	out := make([]HeartRateRow, 0, len(byTime))
	for _, row := range byTime {
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SampledAt.Before(out[j].SampledAt) })
	result := TypeResult{Status: "accepted", Rows: len(out), Dropped: dropped}
	for i := range out {
		result.ProcessedThrough = maxTime(result.ProcessedThrough, out[i].SampledAt)
	}
	return out, result, nil
}
