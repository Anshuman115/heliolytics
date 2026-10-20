package parse

import (
	"encoding/binary"
	"errors"
	"math"
	"testing"
	"time"
)

func paiRecord(t testing.TB, epoch int64, score float32) []byte {
	t.Helper()
	record := make([]byte, paiStride)
	record[0] = paiMarker
	binary.LittleEndian.PutUint32(record[paiEpochAt:], uint32(epoch))
	record[paiTimezoneAt] = istTimezoneQuarter
	binary.LittleEndian.PutUint32(record[paiScoreAt:], math.Float32bits(score))
	return record
}

func paiWindow(center time.Time) Window {
	return Window{StartedAt: center.Add(-time.Hour), EndedAt: center.Add(time.Hour)}
}

func TestPAIRealFixtureRoundsFloatTotals(t *testing.T) {
	rows, result, err := parsePAI(fixtureBytes(t, typePAI), loadFixtureWindow(t))
	if err != nil {
		t.Fatalf("parse 0x0D: %v", err)
	}
	if len(rows) != 2 || result.Rows != 2 || result.Dropped != 0 {
		t.Fatalf("rows=%v result=%+v, want two PAI days", rows, result)
	}
	if rows[0].DayKey != "2026-06-21" || rows[0].Score != 81 {
		t.Errorf("first PAI = %+v, want 81 on 2026-06-21", rows[0])
	}
	if rows[1].DayKey != "2026-06-22" || rows[1].Score != 82 {
		t.Errorf("second PAI = %+v, want 82 on 2026-06-22", rows[1])
	}
}

func TestPAILatestEpochWinsWithinADay(t *testing.T) {
	day := time.Date(2026, 7, 25, 18, 30, 0, 0, time.UTC) // 2026-07-26 00:00 IST
	raw := append(paiRecord(t, day.Add(10*time.Hour).Unix(), 50), paiRecord(t, day.Add(12*time.Hour).Unix(), 60)...)
	rows, _, err := parsePAI(raw, paiWindow(day.Add(11*time.Hour)))
	if err != nil {
		t.Fatalf("parse 0x0D: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %+v, want one row for the day", rows)
	}
	if rows[0].Score != 60 || !rows[0].SampledAt.Equal(day.Add(12*time.Hour)) {
		t.Fatalf("row = %+v, want the later epoch and score", rows[0])
	}
}

func TestPAIRejectsNonFiniteAndOutOfRangeScores(t *testing.T) {
	base := time.Date(2026, 7, 25, 18, 30, 0, 0, time.UTC)
	scores := []float32{float32(math.NaN()), float32(math.Inf(1)), 1001, -1, 0}
	wantRows := 1
	var raw []byte
	for index, score := range scores {
		raw = append(raw, paiRecord(t, base.Add(time.Duration(index)*time.Hour).Unix(), score)...)
	}
	rows, result, err := parsePAI(raw, Window{StartedAt: base.Add(-time.Hour), EndedAt: base.Add(5 * time.Hour)})
	if err != nil {
		t.Fatalf("parse 0x0D: %v", err)
	}
	if len(rows) != wantRows || result.Dropped != len(scores)-wantRows {
		t.Fatalf("rows=%v result=%+v, want only the genuine zero accepted", rows, result)
	}
	if rows[0].Score != 0 {
		t.Fatalf("score = %d, want the genuine zero", rows[0].Score)
	}
}

func TestPAIRejectsUnexpectedMarkerAndTimezone(t *testing.T) {
	base := time.Date(2026, 7, 25, 18, 30, 0, 0, time.UTC)
	badMarker := paiRecord(t, base.Unix(), 50)
	badMarker[0] = 0x06
	badTimezone := paiRecord(t, base.Unix(), 50)
	badTimezone[paiTimezoneAt] = 0

	tests := []struct {
		name string
		raw  []byte
		code string
	}{
		{name: "marker", raw: badMarker, code: ErrCodeInvalidFraming},
		{name: "timezone", raw: badTimezone, code: ErrCodeUnsupportedTimezone},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := parsePAI(test.raw, paiWindow(base))
			var parseErr *ParseError
			if !errors.As(err, &parseErr) || parseErr.Code != test.code {
				t.Fatalf("error = %v, want %s", err, test.code)
			}
		})
	}
}

func TestPAIRetainsCompletePrefixOnFragment(t *testing.T) {
	base := time.Date(2026, 7, 25, 18, 30, 0, 0, time.UTC)
	raw := append(paiRecord(t, base.Unix(), 42), 0x01, 0x02, 0x03, 0x04, 0x05)
	rows, result, err := parsePAI(raw, paiWindow(base))
	if err != nil {
		t.Fatalf("parse 0x0D: %v", err)
	}
	if len(rows) != 1 || rows[0].Score != 42 {
		t.Fatalf("rows = %+v, want the complete record", rows)
	}
	if result.TrailingBytes != 5 || result.Status != "partial" {
		t.Fatalf("result = %+v, want a five byte fragment", result)
	}
}
