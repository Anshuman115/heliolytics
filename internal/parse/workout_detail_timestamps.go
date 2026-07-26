package parse

const (
	detailTimestampBytes = 6
	detailMinEpochMillis = int64(946684800000)
	detailMaxEpochMillis = int64(4102444800000)
	detailFallbackGapMs  = int64(2 * 60 * 1000)
)

var detailSessionStartMarker = []byte{0x00, 0x00, 0x04, 0x04}

type detailSession struct {
	startedMs int64
	lastMs    int64
}

func parseTimestampedDetailSessions(raw []byte) []WorkoutRecord {
	var sessions []detailSession
	for offset := 0; offset+len(psmHeader) <= len(raw); offset++ {
		if !matchHeader(raw, offset) {
			continue
		}
		millis, ok := detailBlockMillis(raw, offset)
		if !ok {
			continue
		}
		startsSession := matchDetailStartMarker(raw, offset)
		if len(sessions) == 0 || startsSession || millis-sessions[len(sessions)-1].lastMs > detailFallbackGapMs {
			sessions = append(sessions, detailSession{startedMs: millis, lastMs: millis})
			continue
		}
		sessions[len(sessions)-1].lastMs = millis
	}

	out := make([]WorkoutRecord, 0, len(sessions))
	for _, session := range sessions {
		durationSec := int((session.lastMs - session.startedMs) / 1000)
		if durationSec < 1 {
			durationSec = 1
		}
		startedSec := session.startedMs / 1000
		out = append(out, WorkoutRecord{
			DayKey:      IstDayKey(startedSec),
			StartedAt:   EpochUTC(startedSec),
			DurationSec: durationSec,
		})
	}
	return out
}

func detailBlockMillis(raw []byte, offset int) (int64, bool) {
	start := offset + len(psmHeader)
	if start+detailTimestampBytes > len(raw) {
		return 0, false
	}
	var millis int64
	for index := 0; index < detailTimestampBytes; index++ {
		millis |= int64(raw[start+index]) << (8 * index)
	}
	return millis, millis >= detailMinEpochMillis && millis <= detailMaxEpochMillis
}

func matchDetailStartMarker(raw []byte, offset int) bool {
	start := offset + len(psmHeader) + detailTimestampBytes
	if start+len(detailSessionStartMarker) > len(raw) {
		return false
	}
	return matchBytes(raw, start, detailSessionStartMarker)
}
