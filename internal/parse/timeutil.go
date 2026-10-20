package parse

import "time"

const (
	// measuredWindowSlack covers samples the device still buffers from older days.
	measuredWindowSlack = 30 * 24 * time.Hour
	// measuredWindowTail allows a sample that lands just after the session end.
	measuredWindowTail = 5 * time.Minute
	// istTimezoneQuarter is the device timezone byte in fifteen minute units.
	istTimezoneQuarter = 22
	// Anchor bounds are fixed so decoding never depends on the process clock.
	minAnchorSec = int64(1577836800) // 2020-01-01T00:00:00Z
	maxAnchorSec = int64(4102444800) // 2100-01-01T00:00:00Z
)

// EpochUTC converts a UTC epoch second to a time.
func EpochUTC(sec int64) time.Time {
	return time.Unix(sec, 0).UTC()
}

func anchorPlausible(sec int64) bool {
	return sec >= minAnchorSec && sec <= maxAnchorSec
}
