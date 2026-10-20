package rollup

import (
	"testing"

	"github.com/Anshuman115/heliolytics/internal/store"
)

func minuteRows(from, count int) []store.ActivityMinute {
	out := make([]store.ActivityMinute, 0, count)
	for i := range count {
		out = append(out, store.ActivityMinute{At: at(1, from+i), Steps: 20})
	}
	return out
}

func TestContiguousBlocksSplitOnMissingMinute(t *testing.T) {
	rows := append(minuteRows(0, 2), minuteRows(3, 2)...)
	blocks := contiguousBlocks(rows, nil)
	if len(blocks) != 2 {
		t.Fatalf("blocks = %d, want 2", len(blocks))
	}
	if !blocks[0].Start.Equal(at(1, 0)) || !blocks[0].End.Equal(at(1, 2)) {
		t.Fatalf("first block = %v-%v, want 01:00-01:02", blocks[0].Start, blocks[0].End)
	}
	if !blocks[1].Start.Equal(at(1, 3)) || !blocks[1].End.Equal(at(1, 5)) {
		t.Fatalf("second block = %v-%v, want 01:03-01:05", blocks[1].Start, blocks[1].End)
	}
}

func TestContiguousBlocksSplitOnWorkout(t *testing.T) {
	rows := minuteRows(0, 10)
	workouts := []store.WorkoutInterval{{Start: at(1, 4), End: at(1, 6)}}
	blocks := contiguousBlocks(rows, workouts)
	if len(blocks) != 2 {
		t.Fatalf("blocks = %d, want 2", len(blocks))
	}
	if !blocks[0].End.Equal(at(1, 4)) {
		t.Fatalf("first block ends %v, want 01:04", blocks[0].End)
	}
	if !blocks[1].Start.Equal(at(1, 6)) {
		t.Fatalf("second block starts %v, want 01:06", blocks[1].Start)
	}
}

func TestTrimBlocksExpandsToWholeObservedBlock(t *testing.T) {
	rows := minuteRows(0, 6)
	from, to, ok := trimBlocks(rows, nil, at(1, 2), at(1, 3))
	if !ok {
		t.Fatal("the seed range contains an observed minute")
	}
	if !from.Equal(at(1, 0)) || !to.Equal(at(1, 6)) {
		t.Fatalf("trimmed window = %v-%v, want 01:00-01:06", from, to)
	}
}

func TestTrimBlocksSpansEveryIntersectingBlock(t *testing.T) {
	rows := append(minuteRows(0, 2), minuteRows(10, 2)...)
	from, to, ok := trimBlocks(rows, nil, at(1, 1), at(1, 11))
	if !ok {
		t.Fatal("both blocks intersect the seed range")
	}
	if !from.Equal(at(1, 0)) || !to.Equal(at(1, 12)) {
		t.Fatalf("trimmed window = %v-%v, want 01:00-01:12", from, to)
	}
}

func TestTrimBlocksReportsNoObservedMinutes(t *testing.T) {
	rows := minuteRows(0, 2)
	if _, _, ok := trimBlocks(rows, nil, at(1, 30), at(1, 40)); ok {
		t.Fatal("a seed range without observed minutes must report false")
	}
	workouts := []store.WorkoutInterval{{Start: at(1, 0), End: at(1, 5)}}
	if _, _, ok := trimBlocks(rows, workouts, at(1, 0), at(1, 5)); ok {
		t.Fatal("workout minutes are barriers, not observations")
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
