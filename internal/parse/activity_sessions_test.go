package parse

import (
	"testing"
	"time"
)

func TestDetectActivitySessionsFromMinuteStream(t *testing.T) {
	const records = 25
	raw := make([]byte, records*8)
	for i := 0; i < records; i++ {
		off := i * 8
		raw[off] = 0x50
		raw[off+1] = 10
		raw[off+2] = 0
		raw[off+3] = 70
	}
	for _, i := range []int{5, 6, 7, 8, 9, 11, 12, 13, 14} {
		off := i * 8
		raw[off+1] = 80
		raw[off+2] = 2
		raw[off+3] = byte(100 + i)
	}
	catalog := []byte(`{"chunked":[{"code":"0x01","roundStart":"2026-07-26T06:00:00"}]}`)

	sessions := DetectActivitySessions(raw, catalog, time.Time{})
	if len(sessions) != 1 {
		t.Fatalf("sessions=%d want 1", len(sessions))
	}
	if sessions[0].DurationSec != 10*60 {
		t.Fatalf("duration=%d want 600", sessions[0].DurationSec)
	}
	if sessions[0].SportName != "Activity" || sessions[0].AvgHr == nil || sessions[0].MaxHr == nil {
		t.Fatalf("incomplete session: %+v", sessions[0])
	}
}

func TestDetectActivitySessionsRejectsShortBurst(t *testing.T) {
	raw := make([]byte, 12*8)
	for i := 0; i < 7; i++ {
		off := i * 8
		raw[off+1] = 90
		raw[off+3] = 120
	}
	catalog := []byte(`{"chunked":[{"code":"0x01","roundStart":"2026-07-26T06:00:00"}]}`)
	if sessions := DetectActivitySessions(raw, catalog, time.Time{}); len(sessions) != 0 {
		t.Fatalf("short burst produced %d sessions", len(sessions))
	}
}
