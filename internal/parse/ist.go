package parse

import (
	"strings"
	"time"
)

func stringsTrimRoundStart(raw string) string {
	raw = strings.TrimSuffix(raw, "Z")
	if i := strings.Index(raw, "."); i >= 0 {
		raw = raw[:i]
	}
	return raw
}

const istOffsetSec = 5*3600 + 30*60

func IstDayKey(epochSec int64) string {
	dayNum := (epochSec + istOffsetSec) / 86400
	t := time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, int(dayNum))
	return t.Format("2006-01-02")
}

// ParseRoundStartIst converts a catalog roundStart string to a UTC Unix timestamp.
// Returns -1 if the string is empty or malformed so callers can use IsPlausibleUnixSec
// to skip the segment rather than assigning data to 1970-01-01.
func ParseRoundStartIst(raw string) int64 {
	raw = stringsTrimRoundStart(raw)
	if len(raw) < 19 {
		return -1
	}
	raw = raw[:19]
	t, err := time.Parse("2006-01-02T15:04:05", raw)
	if err != nil {
		return -1
	}
	utc := time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), 0, time.UTC)
	return utc.Add(-5*time.Hour - 30*time.Minute).Unix()
}
