package parse

import (
	"bytes"
	"time"
)

var workoutDetailHeader = []byte{1, 12, 'p', 's', 'm', 'h'}

const workoutDetailTimestampSize = 6

var workoutDetailHeaderSize = len(workoutDetailHeader) + workoutDetailTimestampSize

type detailBlock struct {
	off int
	at  time.Time
}

func parseWorkoutDetailBlocks(raw []byte) ([]detailBlock, error) {
	if !bytes.HasPrefix(raw, workoutDetailHeader) {
		return nil, newError(ErrCodeInvalidFraming, typeWorkoutDetail, 0,
			"payload does not start with a detail block header")
	}
	var out []detailBlock
	pos := 0
	for {
		if !bytes.HasPrefix(raw[pos:], workoutDetailHeader) {
			return nil, newError(ErrCodeInvalidFraming, typeWorkoutDetail, pos,
				"unexpected detail block boundary")
		}
		if len(raw)-pos < workoutDetailHeaderSize {
			return nil, newError(ErrCodeInvalidFraming, typeWorkoutDetail, pos,
				"detail block header is truncated")
		}
		millis := int64(0)
		for index := range workoutDetailTimestampSize {
			millis |= int64(raw[pos+len(workoutDetailHeader)+index]) << (8 * index)
		}
		out = append(out, detailBlock{off: pos, at: time.UnixMilli(millis).UTC()})
		next := bytes.Index(raw[pos+len(workoutDetailHeader):], workoutDetailHeader)
		if next < 0 {
			break
		}
		pos += len(workoutDetailHeader) + next
	}
	return out, nil
}
