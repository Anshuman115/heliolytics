package rollup

import (
	"testing"

	"github.com/Anshuman115/heliolytics/internal/store"
)

func TestPAIKeepsZeroAndNull(t *testing.T) {
	if paiScore(nil) != nil {
		t.Fatal("a day without a PAI record must stay NULL")
	}
	zero := paiScore([]store.PAIScore{{DayKey: dayStart(testDay), Score: 0}})
	if zero == nil || *zero != 0 {
		t.Fatalf("legitimate zero = %v, want 0", zero)
	}
	score := paiScore([]store.PAIScore{{DayKey: dayStart(testDay), Score: 42}})
	if score == nil || *score != 42 {
		t.Fatalf("score = %v, want 42", score)
	}
}
