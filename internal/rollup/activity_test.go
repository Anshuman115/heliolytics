package rollup

import (
	"testing"
	"time"

	"github.com/Anshuman115/heliolytics/internal/activity"
	"github.com/Anshuman115/heliolytics/internal/store"
)

func minuteRows(from, count int) []store.ActivityMinute {
	out := make([]store.ActivityMinute, 0, count)
	for i := range count {
		out = append(out, store.ActivityMinute{At: at(1, from+i), Steps: 100})
	}
	return out
}

func TestActivityBoundariesMatchDetectorResets(t *testing.T) {
	for _, tc := range []struct {
		name        string
		rows        []store.ActivityMinute
		workouts    []store.WorkoutInterval
		left, right int
		reset       bool
	}{
		{"missing", append(minuteRows(0, 2), minuteRows(3, 3)...), nil, 3, 2, true},
		{"workout", minuteRows(0, 6), []store.WorkoutInterval{{Start: at(1, 2).Add(30 * time.Second), End: at(1, 4)}}, 4, 2, true},
		{"movement", minuteRows(0, 6), nil, 0, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, backwards := range []bool{false, true} {
				idle := 0
				boundary, ok := activityChunkBoundary(tc.rows, tc.workouts, at(1, 0), at(1, 6), backwards, activity.DefaultParams(), &idle)
				want := at(1, tc.right)
				if backwards {
					want = at(1, tc.left)
				}
				if ok != tc.reset || ok && !boundary.Equal(want) {
					t.Fatalf("backwards %v: boundary %v/%v, want %v/%v", backwards, boundary, ok, want, tc.reset)
				}
			}
		})
	}
}

func TestActivityBoundariesKeepShortIdleGapsBridgeable(t *testing.T) {
	for _, gap := range []int{3, 4} {
		rows := minuteRows(0, 8)
		for i := 2; i < 2+gap; i++ {
			rows[i].Steps = 0
		}
		for _, backwards := range []bool{false, true} {
			idle := 0
			boundary, ok := activityChunkBoundary(rows, nil, at(1, 0), at(1, 8), backwards, activity.DefaultParams(), &idle)
			if gap == 3 {
				if ok {
					t.Fatalf("bridgeable gap became a boundary at %v", boundary)
				}
				continue
			}
			want := at(1, 6)
			if backwards {
				want = at(1, 2)
			}
			if !ok || !boundary.Equal(want) {
				t.Fatalf("idle reset boundary = %v/%v, want %v", boundary, ok, want)
			}
		}
	}
}

func TestActivityIdleResetCarriesAcrossChunkEdges(t *testing.T) {
	rows := minuteRows(0, 4)
	for i := range rows {
		rows[i].Steps = 0
	}
	for _, backwards := range []bool{false, true} {
		idle := 0
		first, second := rows[:2], rows[2:]
		firstFrom, firstTo, secondFrom, secondTo := at(1, 0), at(1, 2), at(1, 2), at(1, 4)
		want := at(1, 4)
		if backwards {
			first, second = second, first
			firstFrom, firstTo, secondFrom, secondTo = secondFrom, secondTo, firstFrom, firstTo
			want = at(1, 0)
		}
		if boundary, ok := activityChunkBoundary(first, nil, firstFrom, firstTo, backwards, activity.DefaultParams(), &idle); ok {
			t.Fatalf("two idle minutes became a reset at %v", boundary)
		}
		boundary, ok := activityChunkBoundary(second, nil, secondFrom, secondTo, backwards, activity.DefaultParams(), &idle)
		if !ok || !boundary.Equal(want) {
			t.Fatalf("chunk-spanning idle reset = %v/%v, want %v", boundary, ok, want)
		}
	}
}

func TestIntervalBoundsAndMerge(t *testing.T) {
	a := store.InstantInterval{From: at(1, 0), To: at(1, 30)}
	b := store.InstantInterval{From: at(1, 20), To: at(1, 40)}
	c := store.InstantInterval{From: at(3, 0), To: at(3, 10)}
	merged := mergeIntIntervals([]store.InstantInterval{c, a, b})
	if len(merged) != 2 {
		t.Fatalf("merged = %d spans, want 2", len(merged))
	}
	if !merged[0].To.Equal(at(1, 40)) {
		t.Fatalf("overlapping spans must merge, got %v", merged[0].To)
	}
	from, to, ok := intervalBounds(merged)
	if !ok || !from.Equal(at(1, 0)) || !to.Equal(at(3, 10)) {
		t.Fatalf("bounds = %v-%v/%v, want 01:00-03:10", from, to, ok)
	}
	if _, _, ok := intervalBounds(nil); ok {
		t.Fatal("empty intervals must report false")
	}
	if _, _, ok := intervalBounds([]store.InstantInterval{{From: at(1, 0), To: at(1, 0)}}); ok {
		t.Fatal("an empty interval must report false")
	}
}
