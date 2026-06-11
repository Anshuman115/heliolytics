package parse

import "time"

const minPlausibleSec = 1577836800 // 2020-01-01

func IsPlausibleUnixSec(sec int64) bool {
	max := time.Now().UTC().Unix() + 86400
	return sec >= minPlausibleSec && sec <= max
}

func EpochUTC(sec int64) time.Time {
	return time.Unix(sec, 0).UTC()
}
