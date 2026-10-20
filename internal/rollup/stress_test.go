package rollup

import (
	"testing"

	"github.com/Anshuman115/heliolytics/internal/store"
)

func TestMeanStressIncludesRealZero(t *testing.T) {
	rows := []store.StressSample{
		{At: at(9, 0), Stress: 10},
		{At: at(10, 0), Stress: 20},
		{At: at(11, 0), Stress: 60},
	}
	got := meanStress(rows)
	if got == nil || *got != 30 {
		t.Fatalf("mean = %v, want the arithmetic 30, not the last sample", got)
	}
	if meanStress(nil) != nil {
		t.Fatal("no rows must stay NULL")
	}
	zero := meanStress([]store.StressSample{{At: at(9, 0), Stress: 0}})
	if zero == nil || *zero != 0 {
		t.Fatalf("a single zero = %v, want 0", zero)
	}
}
