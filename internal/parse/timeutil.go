package parse

import "time"

const (
	measuredWindowSlack = 30 * 24 * time.Hour
	measuredWindowTail  = 5 * time.Minute
	istTimezoneQuarter  = 22
	minAnchorSec        = int64(1577836800) // 2020-01-01T00:00:00Z
	maxAnchorSec        = int64(4102444800) // 2100-01-01T00:00:00Z
)

func EpochUTC(sec int64) time.Time {
	return time.Unix(sec, 0).UTC()
}

func anchorPlausible(sec int64) bool {
	return sec >= minAnchorSec && sec <= maxAnchorSec
}
