package parse

import (
	"encoding/binary"
	"errors"
	"testing"
	"time"
)

func hrFrame(epoch int64, samples ...byte) []byte {
	frame := make([]byte, continuousHRHeaderSize+len(samples))
	binary.LittleEndian.PutUint32(frame[0:], uint32(epoch))
	binary.LittleEndian.PutUint32(frame[4:], uint32(len(samples)))
	copy(frame[continuousHRHeaderSize:], samples)
	return frame
}

func hrWindow(center time.Time) Window {
	return Window{StartedAt: center.Add(-time.Hour), EndedAt: center.Add(time.Hour)}
}

func TestParseContinuousHRRealFixture(t *testing.T) {
	start := time.Date(2026, 6, 22, 2, 20, 16, 0, time.UTC)
	rows, result, err := parseContinuousHR(fixtureBytes(t, typeContinuousHR), loadFixtureWindow(t))
	if err != nil {
		t.Fatalf("parse 0x46: %v", err)
	}
	if len(rows) != 180 || result.Rows != 180 || result.Dropped != 0 {
		t.Fatalf("rows=%d result=%+v, want 180 accepted samples", len(rows), result)
	}
	if !rows[0].SampledAt.Equal(start) {
		t.Errorf("first sample = %s, want %s", formatFixtureTime(rows[0].SampledAt), formatFixtureTime(start))
	}
	if !rows[179].SampledAt.Equal(start.Add(179 * time.Second)) {
		t.Errorf("last sample = %s, want start+179s", formatFixtureTime(rows[179].SampledAt))
	}
	for i, row := range rows {
		if row.Source != typeContinuousHR {
			t.Fatalf("sample %d source = %q", i, row.Source)
		}
		if !row.SampledAt.Equal(start.Add(time.Duration(i) * time.Second)) {
			t.Fatalf("sample %d at %s, want one-second cadence", i, formatFixtureTime(row.SampledAt))
		}
	}
}

func TestParseContinuousHRTwoCountedFrames(t *testing.T) {
	start := time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC)
	raw := append(hrFrame(start.Unix(), 91, 92, 93), hrFrame(start.Add(10*time.Second).Unix(), 94, 95)...)
	rows, result, err := parseContinuousHR(raw, hrWindow(start))
	if err != nil {
		t.Fatalf("parse frames: %v", err)
	}
	if len(rows) != 5 || result.Dropped != 0 {
		t.Fatalf("rows=%d result=%+v, want 5 samples", len(rows), result)
	}
	want := []int64{0, 1, 2, 10, 11}
	for i, offset := range want {
		if !rows[i].SampledAt.Equal(start.Add(time.Duration(offset) * time.Second)) {
			t.Errorf("sample %d at %s, want +%ds", i, formatFixtureTime(rows[i].SampledAt), offset)
		}
	}
}

func TestParseContinuousHRZeroCountProgresses(t *testing.T) {
	start := time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC)
	raw := append(hrFrame(start.Unix()), hrFrame(start.Add(time.Minute).Unix(), 90)...)
	rows, _, err := parseContinuousHR(raw, hrWindow(start))
	if err != nil {
		t.Fatalf("parse frames: %v", err)
	}
	if len(rows) != 1 || !rows[0].SampledAt.Equal(start.Add(time.Minute)) {
		t.Fatalf("rows=%+v, want one sample after the empty frame", rows)
	}
}

func TestParseContinuousHRInvalidSampleKeepsSecondOffsets(t *testing.T) {
	start := time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC)
	raw := hrFrame(start.Unix(), 80, 80, 80, 80, 80, 0, 80, 80)
	rows, result, err := parseContinuousHR(raw, hrWindow(start))
	if err != nil {
		t.Fatalf("parse frame: %v", err)
	}
	if len(rows) != 7 || result.Dropped != 1 {
		t.Fatalf("rows=%d dropped=%d, want 7 rows and one dropped sample", len(rows), result.Dropped)
	}
	if !rows[5].SampledAt.Equal(start.Add(6 * time.Second)) {
		t.Errorf("row after the invalid sample at %s, want start+6s", formatFixtureTime(rows[5].SampledAt))
	}
}

func TestParseContinuousHRRejectsBrokenFraming(t *testing.T) {
	start := time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC)
	shortHeader := make([]byte, 179)
	binary.LittleEndian.PutUint32(shortHeader[0:], uint32(start.Unix()))
	binary.LittleEndian.PutUint32(shortHeader[4:], 180)
	maximumCount := make([]byte, continuousHRHeaderSize)
	binary.LittleEndian.PutUint32(maximumCount[0:], uint32(start.Unix()))
	binary.LittleEndian.PutUint32(maximumCount[4:], 0xFFFFFFFF)
	tests := []struct {
		name   string
		raw    []byte
		code   string
		offset int
	}{
		{name: "count exceeds payload", raw: shortHeader, code: ErrCodeCountOverflow, offset: 0},
		{name: "maximum count", raw: maximumCount, code: ErrCodeCountOverflow, offset: 0},
		{name: "one byte header", raw: []byte{0x01}, code: ErrCodeInvalidFraming, offset: 0},
		{name: "seven byte header", raw: make([]byte, 7), code: ErrCodeInvalidFraming, offset: 0},
		{name: "trailing header", raw: append(hrFrame(start.Unix(), 90), 0x01, 0x02, 0x03), code: ErrCodeInvalidFraming, offset: 9},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rows, result, err := parseContinuousHR(test.raw, hrWindow(start))
			var parseErr *ParseError
			if !errors.As(err, &parseErr) || parseErr.Code != test.code {
				t.Fatalf("error = %v, want %s", err, test.code)
			}
			if parseErr.ByteOffset != test.offset {
				t.Errorf("offset = %d, want %d", parseErr.ByteOffset, test.offset)
			}
			if rows != nil || result.Rows != 0 {
				t.Fatalf("broken framing returned rows: %+v", rows)
			}
		})
	}
}
