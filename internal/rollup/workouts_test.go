package rollup

import (
	"testing"

	"github.com/Anshuman115/heliolytics/internal/store"
)

func TestActivityCountsSeparateAutoAndKeepNullCalories(t *testing.T) {
	rows := []store.ActivityRow{
		{StartedAt: at(8, 0), Kind: "workout", Calories: new(300)},
		{StartedAt: at(18, 0), Kind: "workout", Calories: new(366)},
		{StartedAt: at(12, 0), Kind: "auto"},
	}
	workouts, autos, calories := activityCounts(rows)
	if workouts != 2 || autos != 1 {
		t.Fatalf("counts = %d/%d, want 2/1", workouts, autos)
	}
	if calories == nil || *calories != 666 {
		t.Fatalf("calories = %v, want 666", calories)
	}

	noCalories := []store.ActivityRow{{StartedAt: at(8, 0), Kind: "workout", Calories: nil}}
	if _, _, got := activityCounts(noCalories); got != nil {
		t.Fatalf("unreported calories = %v, want NULL", got)
	}

	zero := []store.ActivityRow{{StartedAt: at(8, 0), Kind: "workout", Calories: new(0)}}
	if _, _, got := activityCounts(zero); got == nil || *got != 0 {
		t.Fatalf("reported zero = %v, want 0", got)
	}

	if workouts, autos, calories := activityCounts(nil); workouts != 0 || autos != 0 || calories != nil {
		t.Fatal("a day without activities has no counts and NULL calories")
	}
}
