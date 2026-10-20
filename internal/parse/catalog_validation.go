package parse

import "time"

// headeredRoundVersion is the only round version 0x25 and 0x26 accept.
const headeredRoundVersion = 0x02

// roundAnchor is one validated catalog round position inside a payload.
type roundAnchor struct {
	off int
	at  time.Time
}

// minuteRecord is one fixed-stride record anchored to an absolute minute.
type minuteRecord struct {
	at  time.Time
	off int
}

// headeredRecord is one record inside a version-headered round.
type headeredRecord struct {
	off       int
	nextRound *time.Time
}

// minuteRecords expands the anchored segments of a minute type into per-record
// offsets and times, reporting any trailing fragment that is not a record.
func minuteRecords(code string, raw []byte, entry *CatalogEntry, stride int) ([]minuteRecord, int, error) {
	anchors, err := minuteAnchors(code, raw, entry, stride)
	if err != nil {
		return nil, 0, err
	}
	var out []minuteRecord
	trailing := 0
	for i, anchor := range anchors {
		end := len(raw)
		if i+1 < len(anchors) {
			end = anchors[i+1].off
		}
		size := end - anchor.off
		complete := size / stride
		remainder := size % stride
		if remainder != 0 {
			if i+1 < len(anchors) {
				return nil, 0, newError(ErrCodeInvalidFraming, code, end-remainder,
					"interior segment ends with %d trailing bytes", remainder)
			}
			if len(out) == 0 {
				return nil, 0, newError(ErrCodeInvalidFraming, code, anchor.off,
					"payload holds no complete %d-byte record", stride)
			}
			trailing = remainder
		}
		for j := range complete {
			out = append(out, minuteRecord{
				at:  anchor.at.Add(time.Duration(j) * time.Minute),
				off: anchor.off + j*stride,
			})
		}
	}
	return out, trailing, nil
}

func minuteAnchors(code string, raw []byte, entry *CatalogEntry, stride int) ([]roundAnchor, error) {
	if entry == nil {
		return nil, newError(ErrCodeMissingAnchor, code, 0, "no catalog entry provides a round anchor")
	}
	if len(entry.RoundSegments) == 0 {
		if entry.RoundStart == "" {
			return nil, newError(ErrCodeMissingAnchor, code, 0, "catalog entry has no round anchor")
		}
		anchor, err := validateMinuteAnchor(code, 0, entry.RoundStart, len(raw), stride)
		if err != nil {
			return nil, err
		}
		return []roundAnchor{anchor}, nil
	}
	out := make([]roundAnchor, 0, len(entry.RoundSegments))
	previous := -1
	for i, segment := range entry.RoundSegments {
		anchor, err := validateMinuteAnchor(code, segment.ByteOffset, segment.RoundStart, len(raw), stride)
		if err != nil {
			return nil, err
		}
		if i == 0 && segment.ByteOffset != 0 {
			return nil, newError(ErrCodeInvalidCatalog, code, segment.ByteOffset,
				"first round offset must be 0")
		}
		if i > 0 && segment.ByteOffset <= previous {
			return nil, newError(ErrCodeInvalidCatalog, code, segment.ByteOffset,
				"round offset is not strictly after %d", previous)
		}
		out = append(out, anchor)
		previous = segment.ByteOffset
	}
	return out, nil
}

func validateMinuteAnchor(code string, offset int, roundStart string, size, stride int) (roundAnchor, error) {
	if offset < 0 {
		return roundAnchor{}, newError(ErrCodeInvalidCatalog, code, offset,
			"round offset is negative")
	}
	// A round position the payload cannot satisfy is a framing conflict, not a
	// catalog syntax error.
	if offset >= size {
		return roundAnchor{}, newError(ErrCodeInvalidFraming, code, offset,
			"round offset is outside the %d-byte payload", size)
	}
	if offset%stride != 0 {
		return roundAnchor{}, newError(ErrCodeInvalidFraming, code, offset,
			"round offset is not aligned to stride %d", stride)
	}
	sec, fraction, ok := parseRoundStart(roundStart)
	if !ok {
		return roundAnchor{}, newError(ErrCodeInvalidCatalog, code, offset,
			"round start is not an offsetless IST wall clock")
	}
	if !anchorPlausible(sec) || sec%60 != 0 || fraction {
		return roundAnchor{}, newError(ErrCodeInvalidCatalog, code, offset,
			"minute anchor must be a whole minute in the accepted range")
	}
	return roundAnchor{off: offset, at: EpochUTC(sec)}, nil
}

// payloadSegments returns the round offsets of a version-headered type. A blob
// without catalog segments is one round at offset 0.
func payloadSegments(code string, raw []byte, entry *CatalogEntry) ([]int, error) {
	size := len(raw)
	if entry == nil || len(entry.RoundSegments) == 0 {
		return []int{0}, nil
	}
	out := make([]int, 0, len(entry.RoundSegments))
	previous := -1
	for i, segment := range entry.RoundSegments {
		if segment.ByteOffset < 0 {
			return nil, newError(ErrCodeInvalidCatalog, code, segment.ByteOffset,
				"round offset is negative")
		}
		// A round position the payload cannot satisfy is a framing conflict, not
		// a catalog syntax error.
		if segment.ByteOffset >= size {
			return nil, newError(ErrCodeInvalidFraming, code, segment.ByteOffset,
				"round offset is outside the %d-byte payload", size)
		}
		if i == 0 && segment.ByteOffset != 0 {
			return nil, newError(ErrCodeInvalidCatalog, code, segment.ByteOffset,
				"first round offset must be 0")
		}
		if i > 0 && segment.ByteOffset <= previous {
			return nil, newError(ErrCodeInvalidCatalog, code, segment.ByteOffset,
				"round offset is not strictly after %d", previous)
		}
		out = append(out, segment.ByteOffset)
		previous = segment.ByteOffset
	}
	return out, nil
}

// headeredRecords splits version-headered rounds into record offsets and
// reports a trailing fragment in the final round.
func headeredRecords(code string, raw []byte, entry *CatalogEntry, stride int) ([]headeredRecord, int, error) {
	starts, err := payloadSegments(code, raw, entry)
	if err != nil {
		return nil, 0, err
	}
	var out []headeredRecord
	trailing := 0
	for i, start := range starts {
		end := len(raw)
		if i+1 < len(starts) {
			end = starts[i+1]
		}
		if raw[start] != headeredRoundVersion {
			return nil, 0, newError(ErrCodeUnsupportedVersion, code, start,
				"round version is not 2")
		}
		size := end - start - 1
		complete := size / stride
		remainder := size % stride
		if remainder != 0 {
			if i+1 < len(starts) {
				return nil, 0, newError(ErrCodeInvalidFraming, code, end-remainder,
					"interior round ends with %d trailing bytes", remainder)
			}
			if len(out) == 0 {
				return nil, 0, newError(ErrCodeInvalidFraming, code, start,
					"round holds no complete %d-byte record", stride)
			}
			trailing = remainder
		}
		var limit *time.Time
		if entry != nil && i+1 < len(starts) && i+1 < len(entry.RoundSegments) {
			if sec, _, ok := parseRoundStart(entry.RoundSegments[i+1].RoundStart); ok && anchorPlausible(sec) {
				bound := EpochUTC(sec)
				limit = &bound
			}
		}
		for j := range complete {
			out = append(out, headeredRecord{off: start + 1 + j*stride, nextRound: limit})
		}
	}
	return out, trailing, nil
}

// fixedRecords returns the offsets of whole fixed-stride records and the size
// of a trailing fragment.
func fixedRecords(code string, raw []byte, stride int) ([]int, int, error) {
	complete := len(raw) / stride
	remainder := len(raw) % stride
	if remainder != 0 && complete == 0 {
		return nil, 0, newError(ErrCodeInvalidFraming, code, 0,
			"payload holds no complete %d-byte record", stride)
	}
	out := make([]int, complete)
	for i := range out {
		out[i] = i * stride
	}
	return out, remainder, nil
}

func timezoneError(code string, offset int, value byte) error {
	return newError(ErrCodeUnsupportedTimezone, code, offset,
		"timezone byte %d is not the IST value 22", value)
}
