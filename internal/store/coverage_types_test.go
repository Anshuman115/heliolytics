package store

import (
	"testing"
	"time"
)

func TestBuildTypeCoverageKeepsSpo2SourcesSeparate(t *testing.T) {
	spot := time.Date(2026, 7, 24, 10, 0, 0, 0, time.UTC)
	sleep := time.Date(2026, 7, 26, 3, 0, 0, 0, time.UTC)
	types := buildTypeCoverage(
		nil, nil, nil, nil, nil,
		nil, nil, &spot, &sleep, nil, nil, nil, nil, nil,
	)
	if types["0x25"] != &spot {
		t.Fatalf("0x25=%v want spot coverage", types["0x25"])
	}
	if types["0x26"] != &sleep {
		t.Fatalf("0x26=%v want sleep coverage", types["0x26"])
	}
}

func TestBuildTypeCoverageKeepsWorkoutSourcesSeparate(t *testing.T) {
	summary := time.Date(2026, 7, 24, 10, 0, 0, 0, time.UTC)
	detail := time.Date(2026, 7, 25, 11, 0, 0, 0, time.UTC)
	types := buildTypeCoverage(
		&summary, &detail, nil, nil, nil,
		nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)
	if types["0x05"] != &summary {
		t.Fatalf("0x05=%v want summary coverage", types["0x05"])
	}
	if types["0x06"] != &detail {
		t.Fatalf("0x06=%v want detail coverage", types["0x06"])
	}
}
