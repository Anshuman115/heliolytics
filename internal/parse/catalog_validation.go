package parse

import (
	"fmt"
	"time"
)

type catalogSegmentRule struct {
	code           string
	stride         func([]byte) int
	requiresAnchor bool
}

var catalogSegmentRules = []catalogSegmentRule{
	{code: "0x01", stride: func(raw []byte) int { return activityStrideFor(len(raw)) }, requiresAnchor: true},
	{code: "0x13", stride: func([]byte) int { return 1 }, requiresAnchor: true},
	{code: "0x2E", stride: func([]byte) int { return 8 }, requiresAnchor: true},
	{code: "0x06", stride: func([]byte) int { return 1 }},
}

func validateCatalogAnchors(cat Catalog, blobs map[string][]byte, fetchEnd time.Time) error {
	for _, rule := range catalogSegmentRules {
		raw := blobs[rule.code]
		if len(raw) == 0 {
			continue
		}
		entry := FindEntry(cat, rule.code)
		if entry == nil {
			if rule.code == "0x01" {
				return validateFetchEndFallback(raw, rule.stride(raw), fetchEnd)
			}
			if rule.requiresAnchor {
				return catalogError(rule.code, "missing catalog entry with a round anchor")
			}
			continue
		}
		if len(entry.RoundSegments) > 0 {
			if err := validateRoundSegments(rule.code, raw, rule.stride(raw), entry.RoundSegments); err != nil {
				return err
			}
			continue
		}
		if !rule.requiresAnchor {
			continue
		}
		if entry.RoundStart == "" && rule.code == "0x01" {
			return validateFetchEndFallback(raw, rule.stride(raw), fetchEnd)
		}
		if err := validateRoundTimestamp(rule.code, "roundStart", entry.RoundStart); err != nil {
			return err
		}
	}
	return nil
}

func validateRoundSegments(code string, raw []byte, stride int, segments []RoundSegment) error {
	previous := -1
	for index, segment := range segments {
		if segment.ByteOffset < 0 || segment.ByteOffset >= len(raw) {
			return catalogError(code, "roundSegments[%d].byteOffset=%d is outside payload length %d", index, segment.ByteOffset, len(raw))
		}
		if stride <= 0 || segment.ByteOffset%stride != 0 {
			return catalogError(code, "roundSegments[%d].byteOffset=%d is not aligned to stride %d", index, segment.ByteOffset, stride)
		}
		if index > 0 && segment.ByteOffset <= previous {
			return catalogError(code, "roundSegments[%d].byteOffset=%d is not strictly after %d", index, segment.ByteOffset, previous)
		}
		if err := validateRoundTimestamp(code, fmt.Sprintf("roundSegments[%d].roundStart", index), segment.RoundStart); err != nil {
			return err
		}
		previous = segment.ByteOffset
	}
	return nil
}

func validateRoundTimestamp(code, field, raw string) error {
	sec := ParseRoundStartIst(raw)
	if !IsPlausibleUnixSec(sec) {
		return catalogError(code, "%s=%q is not a plausible timestamp", field, raw)
	}
	return nil
}

func validateFetchEndFallback(raw []byte, stride int, fetchEnd time.Time) error {
	if stride <= 0 {
		return catalogError("0x01", "missing a valid round anchor and fetch-end fallback")
	}
	count := len(raw) / stride
	endSec := fetchEnd.UTC().Unix()
	startSec := endSec - int64(count-1)*60
	if count == 0 || !IsPlausibleUnixSec(startSec) || !IsPlausibleUnixSec(endSec) {
		return catalogError("0x01", "missing a valid round anchor and fetch-end fallback")
	}
	return nil
}

func catalogError(code, format string, args ...any) error {
	detail := fmt.Sprintf(format, args...)
	return fmt.Errorf("%w: %s: %s", ErrInvalidCatalog, code, detail)
}
