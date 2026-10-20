package parse

import "time"

const istOffsetSec int64 = 5*3600 + 30*60

func IstDayKey(epochSec int64) string {
	days := floorDiv(epochSec+istOffsetSec, 86400)
	return time.Unix(days*86400, 0).UTC().Format("2006-01-02")
}

func floorDiv(value, divisor int64) int64 {
	if value >= 0 {
		return value / divisor
	}
	return -((-value + divisor - 1) / divisor)
}

func ParseRoundStartIst(raw string) int64 {
	sec, _, ok := parseRoundStart(raw)
	if !ok {
		return -1
	}
	return sec
}

func parseRoundStart(raw string) (sec int64, fraction bool, ok bool) {
	if len(raw) < 19 {
		return 0, false, false
	}
	if len(raw) > 19 {
		if raw[19] != '.' || len(raw) == 20 || len(raw) > 29 {
			return 0, false, false
		}
		for i := 20; i < len(raw); i++ {
			if raw[i] < '0' || raw[i] > '9' {
				return 0, false, false
			}
			if raw[i] != '0' {
				fraction = true
			}
		}
	}
	parsed, err := time.Parse("2006-01-02T15:04:05", raw[:19])
	if err != nil {
		return 0, false, false
	}
	utc := time.Date(parsed.Year(), parsed.Month(), parsed.Day(),
		parsed.Hour(), parsed.Minute(), parsed.Second(), 0, time.UTC)
	return utc.Unix() - istOffsetSec, fraction, true
}
