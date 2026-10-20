package rollup

import (
	"testing"

	"github.com/Anshuman115/heliolytics/internal/store"
)

func TestSumStepsHonestZeroAndNull(t *testing.T) {
	if got := sumSteps(nil); got != nil {
		t.Fatalf("no rows = %v, want nil", got)
	}
	zero := sumSteps([]store.StepSample{{At: at(0, 0), Steps: 0}})
	if zero == nil || *zero != 0 {
		t.Fatalf("one zero minute = %v, want 0", zero)
	}
	sum := sumSteps([]store.StepSample{{At: at(0, 0), Steps: 10}, {At: at(0, 1), Steps: 20}})
	if sum == nil || *sum != 30 {
		t.Fatalf("10+20 = %v, want 30", sum)
	}
}
