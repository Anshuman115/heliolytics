package main

import (
	"testing"
	"time"

	"github.com/Anshuman115/heliolytics/internal/activity"
)

var matchBase = time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)

func span(startMin, endMin int) activity.Interval {
	return activity.Interval{
		Start: matchBase.Add(time.Duration(startMin) * time.Minute),
		End:   matchBase.Add(time.Duration(endMin) * time.Minute),
	}
}

func TestScoreExactMatch(t *testing.T) {
	score := ScoreIntervals([]activity.Interval{span(0, 20)}, []activity.Interval{span(0, 20)})
	if score.TP != 1 || score.FP != 0 || score.FN != 0 {
		t.Fatalf("tp=%d fp=%d fn=%d want 1/0/0", score.TP, score.FP, score.FN)
	}
	if len(score.Matches) != 1 || score.Matches[0].Prediction != 0 || score.Matches[0].Reference != 0 {
		t.Fatalf("matches=%+v want the single 0/0 pair", score.Matches)
	}
	if score.Matches[0].IoU != 1 {
		t.Fatalf("iou=%v want 1", score.Matches[0].IoU)
	}
	if score.Precision == nil || *score.Precision != 1 || score.Recall == nil || *score.Recall != 1 {
		t.Fatalf("precision=%v recall=%v want 1 and 1", describeMetric(score.Precision), describeMetric(score.Recall))
	}
	if score.F1 == nil || *score.F1 != 1 {
		t.Fatalf("f1=%v want 1", describeMetric(score.F1))
	}
}

func TestScoreOnePredictionTwoReferences(t *testing.T) {
	predictions := []activity.Interval{span(0, 30)}
	references := []activity.Interval{span(0, 20), span(10, 30)}
	score := ScoreIntervals(predictions, references)
	if score.TP != 1 || score.FP != 0 || score.FN != 1 {
		t.Fatalf("tp=%d fp=%d fn=%d want 1/0/1", score.TP, score.FP, score.FN)
	}
	if len(score.Matches) != 1 || score.Matches[0].Prediction != 0 || score.Matches[0].Reference != 0 {
		t.Fatalf("matches=%+v want the first reference and no second pair", score.Matches)
	}
	if len(score.UnmatchedReferences) != 1 || score.UnmatchedReferences[0] != 1 {
		t.Fatalf("unmatched references=%v want [1]", score.UnmatchedReferences)
	}
	if len(score.UnmatchedPredictions) != 0 {
		t.Fatalf("unmatched predictions=%v want none", score.UnmatchedPredictions)
	}
}

func TestScoreTwoPredictionsOneReference(t *testing.T) {
	predictions := []activity.Interval{span(0, 20), span(0, 20)}
	references := []activity.Interval{span(0, 20)}
	score := ScoreIntervals(predictions, references)
	if score.TP != 1 || score.FP != 1 || score.FN != 0 {
		t.Fatalf("tp=%d fp=%d fn=%d want 1/1/0", score.TP, score.FP, score.FN)
	}
	if len(score.Matches) != 1 || score.Matches[0].Prediction != 0 {
		t.Fatalf("matches=%+v want the first prediction only", score.Matches)
	}
	if len(score.UnmatchedPredictions) != 1 || score.UnmatchedPredictions[0] != 1 {
		t.Fatalf("unmatched predictions=%v want [1]", score.UnmatchedPredictions)
	}
}

func TestMaximumCardinalityBeatsGreedyIoU(t *testing.T) {
	predictions := []activity.Interval{span(0, 20), span(0, 36)}
	references := []activity.Interval{span(0, 18), span(10, 20)}
	score := ScoreIntervals(predictions, references)
	if score.TP != 2 || score.FP != 0 || score.FN != 0 {
		t.Fatalf("tp=%d fp=%d fn=%d want 2/0/0", score.TP, score.FP, score.FN)
	}
	if len(score.Matches) != 2 {
		t.Fatalf("matches=%+v want two pairs", score.Matches)
	}
	if score.Matches[0].Prediction != 0 || score.Matches[0].Reference != 1 {
		t.Fatalf("first pair=%d/%d want 0/1", score.Matches[0].Prediction, score.Matches[0].Reference)
	}
	if score.Matches[1].Prediction != 1 || score.Matches[1].Reference != 0 {
		t.Fatalf("second pair=%d/%d want 1/0", score.Matches[1].Prediction, score.Matches[1].Reference)
	}
}

func TestIoUThresholdIsInclusive(t *testing.T) {
	score := ScoreIntervals([]activity.Interval{span(0, 20)}, []activity.Interval{span(10, 20)})
	if score.TP != 1 {
		t.Fatalf("tp=%d want 1 at iou 0.5", score.TP)
	}
	below := ScoreIntervals([]activity.Interval{span(0, 20)}, []activity.Interval{span(10, 21)})
	if below.TP != 0 {
		t.Fatalf("tp=%d want 0 below iou 0.5", below.TP)
	}
}

func TestUndefinedMetrics(t *testing.T) {
	empty := ScoreIntervals(nil, nil)
	if empty.Precision != nil || empty.Recall != nil || empty.F1 != nil {
		t.Fatalf("empty partition metrics=%v/%v/%v want undefined",
			describeMetric(empty.Precision), describeMetric(empty.Recall), describeMetric(empty.F1))
	}
	onlyPredictions := ScoreIntervals([]activity.Interval{span(0, 20)}, nil)
	if onlyPredictions.Precision == nil || *onlyPredictions.Precision != 0 {
		t.Fatalf("precision=%s want 0", describeMetric(onlyPredictions.Precision))
	}
	if onlyPredictions.Recall != nil || onlyPredictions.F1 != nil {
		t.Fatalf("recall=%s f1=%s want undefined", describeMetric(onlyPredictions.Recall), describeMetric(onlyPredictions.F1))
	}
	onlyReferences := ScoreIntervals(nil, []activity.Interval{span(0, 20)})
	if onlyReferences.Precision != nil {
		t.Fatalf("precision=%s want undefined", describeMetric(onlyReferences.Precision))
	}
	if onlyReferences.Recall == nil || *onlyReferences.Recall != 0 {
		t.Fatalf("recall=%s want 0", describeMetric(onlyReferences.Recall))
	}
}

func TestScoreIsDeterministic(t *testing.T) {
	predictions := []activity.Interval{span(0, 20), span(0, 36), span(60, 80)}
	references := []activity.Interval{span(0, 18), span(10, 20), span(55, 80)}
	first := ScoreIntervals(predictions, references)
	for range 5 {
		again := ScoreIntervals(predictions, references)
		if len(again.Matches) != len(first.Matches) {
			t.Fatalf("match count changed from %d to %d", len(first.Matches), len(again.Matches))
		}
		for i := range first.Matches {
			if again.Matches[i] != first.Matches[i] {
				t.Fatalf("match %d changed from %+v to %+v", i, first.Matches[i], again.Matches[i])
			}
		}
	}
}
