package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/Anshuman115/heliolytics/internal/activity"
)

const (
	timezoneNote   = "IST (UTC+05:30, no DST)"
	stopConvention = "stage stop is exclusive; stop earlier than start is next-day rollover"

	precisionTarget = 0.80
	recallTarget    = 0.80

	referenceMinDuration = 10 * time.Minute

	exitOK      = 0
	exitInvalid = 1
	exitTarget  = 2
)

var (
	gridCadences   = []int{60, 70, 80, 90, 100, 110}
	gridGaps       = []int{1, 2, 3}
	gridDurations  = []int{10, 15}
	gridRunCadence = 130
)

const selectionRule = "greatest F1, then greatest min(precision, recall), then precision, then ascending (cadenceMin, gapMax, durationMin)"

const usage = `usage:
  activity-eval select --steps FILE --stages FILE --workouts FILE [--hr FILE]
      [--development-from 2026-01-01] [--development-to 2026-05-01] --out FILE
  activity-eval evaluate --selection FILE --steps FILE --stages FILE --workouts FILE
      [--hr FILE] --holdout-from 2026-05-01 --holdout-to 2026-06-23 --out FILE

select searches the fixed development grid once and writes one frozen point.
evaluate scores that frozen point on the untouched holdout without searching.
evaluate exits 0 when holdout precision and recall reach 0.80, 1 for invalid
input or a modified artifact, and 2 for a valid measurement below target.
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return exitInvalid
	}
	switch args[0] {
	case "select":
		return runSelect(args[1:], stdout, stderr)
	case "evaluate":
		return runEvaluate(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		fmt.Fprint(stdout, usage)
		return exitOK
	default:
		fmt.Fprintf(stderr, "activity-eval: unknown command %q\n%s", args[0], usage)
		return exitInvalid
	}
}

func runSelect(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("select", flag.ContinueOnError)
	flags.SetOutput(stderr)
	steps := flags.String("steps", "", "ACTIVITY_MINUTE CSV path")
	stages := flags.String("stages", "", "ACTIVITY_STAGE CSV path")
	workouts := flags.String("workouts", "", "SPORT CSV path")
	heartRate := flags.String("hr", "", "HEARTRATE_AUTO CSV path")
	developmentFrom := flags.String("development-from", "2026-01-01", "development start date (IST)")
	developmentTo := flags.String("development-to", "2026-05-01", "development end date (IST), exclusive")
	out := flags.String("out", "", "selection artifact path")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitInvalid
	}
	if *out == "" {
		fmt.Fprintln(stderr, "activity-eval: select requires --out")
		return exitInvalid
	}
	sources := Sources{Steps: *steps, Stages: *stages, Workouts: *workouts, HeartRate: *heartRate}
	export, err := LoadExport(sources)
	if err != nil {
		fmt.Fprintf(stderr, "activity-eval: %v\n", err)
		return exitInvalid
	}
	development, err := parsePartition(*developmentFrom, *developmentTo)
	if err != nil {
		fmt.Fprintf(stderr, "activity-eval: %v\n", err)
		return exitInvalid
	}
	artifact, developmentScore := selectParameters(export, sources, development)
	if err := writeJSON(*out, artifact); err != nil {
		fmt.Fprintf(stderr, "activity-eval: %v\n", err)
		return exitInvalid
	}
	fmt.Fprintf(stdout, "select: frozen %s; development %s; wrote %s\n",
		describeParams(artifact.Frozen), describeScore(developmentScore), *out)
	return exitOK
}

func runEvaluate(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("evaluate", flag.ContinueOnError)
	flags.SetOutput(stderr)
	selectionPath := flags.String("selection", "", "selection artifact path")
	steps := flags.String("steps", "", "ACTIVITY_MINUTE CSV path")
	stages := flags.String("stages", "", "ACTIVITY_STAGE CSV path")
	workouts := flags.String("workouts", "", "SPORT CSV path")
	heartRate := flags.String("hr", "", "HEARTRATE_AUTO CSV path")
	holdoutFrom := flags.String("holdout-from", "", "holdout start date (IST)")
	holdoutTo := flags.String("holdout-to", "", "holdout end date (IST), exclusive")
	out := flags.String("out", "", "evaluation report path")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitInvalid
	}
	if *out == "" {
		fmt.Fprintln(stderr, "activity-eval: evaluate requires --out")
		return exitInvalid
	}
	if *selectionPath == "" {
		fmt.Fprintln(stderr, "activity-eval: evaluate requires --selection")
		return exitInvalid
	}
	selection, selectionHash, err := loadSelection(*selectionPath)
	if err != nil {
		fmt.Fprintf(stderr, "activity-eval: %v\n", err)
		return exitInvalid
	}
	sources := Sources{Steps: *steps, Stages: *stages, Workouts: *workouts, HeartRate: *heartRate}
	export, err := LoadExport(sources)
	if err != nil {
		fmt.Fprintf(stderr, "activity-eval: %v\n", err)
		return exitInvalid
	}
	if err := compareSources(selection.Sources, export.Hashes); err != nil {
		fmt.Fprintf(stderr, "activity-eval: %v\n", err)
		return exitInvalid
	}
	development, err := parsePartition(selection.Development.From, selection.Development.To)
	if err != nil {
		fmt.Fprintf(stderr, "activity-eval: selection development bounds: %v\n", err)
		return exitInvalid
	}
	holdout, err := parsePartition(*holdoutFrom, *holdoutTo)
	if err != nil {
		fmt.Fprintf(stderr, "activity-eval: %v\n", err)
		return exitInvalid
	}
	if development.to.After(holdout.from) {
		fmt.Fprintf(stderr, "activity-eval: development and holdout bounds overlap: %s > %s\n",
			formatDay(development.to), formatDay(holdout.from))
		return exitInvalid
	}
	report, gate, holdoutScore := evaluateSelection(export, sources, selection, *selectionPath, selectionHash, development, holdout)
	if err := writeJSON(*out, report); err != nil {
		fmt.Fprintf(stderr, "activity-eval: %v\n", err)
		return exitInvalid
	}
	fmt.Fprintf(stdout, "evaluate: holdout %s; %s; wrote %s\n", describeScore(holdoutScore), gate.Reason, *out)
	if !gate.Passed {
		return exitTarget
	}
	return exitOK
}

func selectParameters(export Export, sources Sources, development partition) (selectionArtifact, Score) {
	minutes := export.Minutes()
	workouts := export.WorkoutIntervals()
	// Development stages only: selection must not see holdout stage counts.
	developmentStages, _ := stagesInPartition(export.Stages, development, referenceMinDuration)
	developmentReferences, _ := References(developmentStages, workouts, referenceMinDuration)

	size := len(gridCadences) * len(gridGaps) * len(gridDurations)
	candidates := make([]candidateRecord, 0, size)
	scored := make([]scoredCandidate, 0, size)
	for _, cadence := range gridCadences {
		for _, gap := range gridGaps {
			for _, duration := range gridDurations {
				params := activity.Params{
					CadenceMin:  cadence,
					GapMax:      gap,
					DurationMin: duration,
					RunCadence:  gridRunCadence,
				}
				kept, _ := filterPartition(sessionIntervals(activity.DetectWithParams(minutes, workouts, params)), development)
				score := ScoreIntervals(kept, developmentReferences)
				record := candidateRecordOf(params, score)
				candidates = append(candidates, record)
				scored = append(scored, scoredCandidate{record: record, params: params})
			}
		}
	}
	best := scored[0]
	for _, candidate := range scored[1:] {
		if betterCandidate(candidate, best) {
			best = candidate
		}
	}

	// Score the frozen point again so match and unmatched details come from the
	// same partition filtering used here.
	frozenPredictions := sessionIntervals(activity.DetectWithParams(minutes, workouts, best.params))
	developmentRecord, developmentScore := scorePartition(development, export, frozenPredictions, workouts)

	frozen := recordOfParams(best.params)
	artifact := selectionArtifact{
		Kind:           "activity-selection",
		Timezone:       timezoneNote,
		StopConvention: stopConvention,
		SelectionRule:  selectionRule,
		Grid: gridRecord{
			CadenceMin:  append([]int(nil), gridCadences...),
			GapMax:      append([]int(nil), gridGaps...),
			DurationMin: append([]int(nil), gridDurations...),
			RunCadence:  gridRunCadence,
		},
		Sources:           sourceRecords(sources, export.Hashes),
		Development:       dateRangeRecord{From: formatDay(development.from), To: formatDay(development.to)},
		Candidates:        candidates,
		Frozen:            frozen,
		FrozenDigest:      frozenDigest(frozen),
		DevelopmentScore:  developmentRecord,
		DevelopmentTarget: targetOf(developmentScore),
		Limitations:       limitations(),
	}
	return artifact, developmentScore
}

// targetOf records whether one score reaches the project precision/recall target.
func targetOf(score Score) targetRecord {
	target := targetRecord{
		PrecisionTarget: precisionTarget,
		RecallTarget:    recallTarget,
		Precision:       score.Precision,
		Recall:          score.Recall,
	}
	if score.Precision != nil && score.Recall != nil {
		target.Met = *score.Precision >= precisionTarget && *score.Recall >= recallTarget
	}
	return target
}

func evaluateSelection(export Export, sources Sources, selection selectionArtifact, selectionPath, selectionHash string,
	development, holdout partition) (evaluationArtifact, gateRecord, Score) {
	minutes := export.Minutes()
	workouts := export.WorkoutIntervals()
	predictions := sessionIntervals(activity.DetectWithParams(minutes, workouts, selection.Frozen.toParams()))

	developmentRecord, _ := scorePartition(development, export, predictions, workouts)
	holdoutRecord, holdoutScore := scorePartition(holdout, export, predictions, workouts)
	gate := gateOf(holdoutScore)

	report := evaluationArtifact{
		Kind:           "activity-evaluation",
		Timezone:       timezoneNote,
		StopConvention: stopConvention,
		Selection:      sourceRecord{Name: "selection", Path: selectionPath, SHA256: selectionHash},
		Sources:        sourceRecords(sources, export.Hashes),
		Parameters:     selection.Frozen,
		Split: splitRecord{
			Development: dateRangeRecord{From: formatDay(development.from), To: formatDay(development.to)},
			Holdout:     dateRangeRecord{From: formatDay(holdout.from), To: formatDay(holdout.to)},
		},
		Partitions: []namedPartition{
			{Name: "development", Score: developmentRecord},
			{Name: "holdout", Score: holdoutRecord},
		},
		Gate:        gate,
		Limitations: limitations(),
	}
	return report, gate, holdoutScore
}

// scorePartition scores one partition from its own stages and predictions.
// Intervals that cross a split boundary are excluded and counted.
func scorePartition(p partition, export Export, predictions, workouts []activity.Interval) (partitionRecord, Score) {
	stages, crossedReferences := stagesInPartition(export.Stages, p, referenceMinDuration)
	references, referenceStats := References(stages, workouts, referenceMinDuration)
	untouched, _ := References(stages, nil, referenceMinDuration)
	keptPredictions, crossedPredictions := filterPartition(predictions, p)
	score := ScoreIntervals(keptPredictions, references)
	untouchedScore := ScoreIntervals(keptPredictions, untouched)
	return partitionRecord{
		From:                 formatDay(p.from),
		To:                   formatDay(p.to),
		PredictionCount:      score.Predictions,
		ReferenceCount:       score.References,
		CrossedPredictions:   crossedPredictions,
		CrossedReferences:    crossedReferences,
		TP:                   score.TP,
		FP:                   score.FP,
		FN:                   score.FN,
		Precision:            score.Precision,
		Recall:               score.Recall,
		F1:                   score.F1,
		Matches:              matchRecordsOf(score.Matches, keptPredictions, references),
		UnmatchedPredictions: unmatchedRecords(score.UnmatchedPredictions, keptPredictions),
		UnmatchedReferences:  unmatchedRecords(score.UnmatchedReferences, references),
		Coverage:             coverageRecordOf(export, p),
		References:           referenceRecordOf(referenceStats),
		UntouchedStages: untouchedRecord{
			PredictionCount:    untouchedScore.Predictions,
			ReferenceCount:     untouchedScore.References,
			CrossedPredictions: crossedPredictions,
			CrossedReferences:  crossedReferences,
			TP:                 untouchedScore.TP,
			FP:                 untouchedScore.FP,
			FN:                 untouchedScore.FN,
			Precision:          untouchedScore.Precision,
			Recall:             untouchedScore.Recall,
			F1:                 untouchedScore.F1,
		},
	}, score
}

// stagesInPartition keeps stages fully inside p and counts stages that cross it.
func stagesInPartition(stages []StageRow, p partition, minDuration time.Duration) ([]StageRow, int) {
	kept := make([]StageRow, 0, len(stages))
	crossed := 0
	for _, stage := range stages {
		if stage.End.Sub(stage.Start) < minDuration {
			continue
		}
		interval := activity.Interval{Start: stage.Start, End: stage.End}
		if p.contains(interval) {
			kept = append(kept, stage)
			continue
		}
		if stage.Start.Before(p.to) && stage.End.After(p.from) {
			crossed++
		}
	}
	return kept, crossed
}

func betterCandidate(a, b scoredCandidate) bool {
	aF1, aFloor, aPrecision := rankOf(a)
	bF1, bFloor, bPrecision := rankOf(b)
	switch {
	case aF1 != bF1:
		return aF1 > bF1
	case aFloor != bFloor:
		return aFloor > bFloor
	case aPrecision != bPrecision:
		return aPrecision > bPrecision
	case a.record.CadenceMin != b.record.CadenceMin:
		return a.record.CadenceMin < b.record.CadenceMin
	case a.record.GapMax != b.record.GapMax:
		return a.record.GapMax < b.record.GapMax
	case a.record.DurationMin != b.record.DurationMin:
		return a.record.DurationMin < b.record.DurationMin
	}
	return false
}

// rankOf returns F1, min(precision, recall) and precision, with -1 when undefined.
func rankOf(candidate scoredCandidate) (float64, float64, float64) {
	f1, floor, precision := -1.0, -1.0, -1.0
	if candidate.record.F1 != nil {
		f1 = *candidate.record.F1
	}
	if candidate.record.Precision != nil && candidate.record.Recall != nil {
		floor = *candidate.record.Precision
		if *candidate.record.Recall < floor {
			floor = *candidate.record.Recall
		}
	}
	if candidate.record.Precision != nil {
		precision = *candidate.record.Precision
	}
	return f1, floor, precision
}

func gateOf(score Score) gateRecord {
	gate := gateRecord{
		PrecisionTarget: precisionTarget,
		RecallTarget:    recallTarget,
		Precision:       score.Precision,
		Recall:          score.Recall,
	}
	switch {
	case score.Predictions == 0 && score.References == 0:
		gate.Reason = "insufficient evidence: no holdout predictions and no holdout references"
	case score.References == 0:
		gate.Reason = "insufficient evidence: no holdout references"
	case score.Predictions == 0:
		gate.Reason = "insufficient evidence: no holdout predictions"
	case score.Precision == nil || score.Recall == nil:
		gate.Reason = "insufficient evidence: undefined holdout precision or recall"
	default:
		precision, recall := *score.Precision, *score.Recall
		switch {
		case precision >= precisionTarget && recall >= recallTarget:
			gate.Passed = true
			gate.Reason = "holdout precision and recall meet the target"
		case precision < precisionTarget && recall < recallTarget:
			gate.Reason = fmt.Sprintf("holdout precision %.4f and recall %.4f are below target", precision, recall)
		case precision < precisionTarget:
			gate.Reason = fmt.Sprintf("holdout precision %.4f is below target", precision)
		default:
			gate.Reason = fmt.Sprintf("holdout recall %.4f is below target", recall)
		}
	}
	return gate
}

func loadSelection(path string) (selectionArtifact, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return selectionArtifact{}, "", fmt.Errorf("%s: %w", path, err)
	}
	sum := sha256.Sum256(data)
	var artifact selectionArtifact
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&artifact); err != nil {
		return selectionArtifact{}, "", fmt.Errorf("%s: %w", path, err)
	}
	if artifact.Kind != "activity-selection" {
		return selectionArtifact{}, "", fmt.Errorf("%s: not a selection artifact", path)
	}
	if err := artifact.Frozen.validate(); err != nil {
		return selectionArtifact{}, "", fmt.Errorf("%s: frozen parameters: %w", path, err)
	}
	if artifact.FrozenDigest != frozenDigest(artifact.Frozen) {
		return selectionArtifact{}, "", fmt.Errorf("%s: frozen parameters were modified", path)
	}
	return artifact, hex.EncodeToString(sum[:]), nil
}

func compareSources(recorded []sourceRecord, loaded map[string]string) error {
	if len(recorded) != len(loaded) {
		return fmt.Errorf("source set changed: artifact lists %d files, evaluation loaded %d", len(recorded), len(loaded))
	}
	for _, source := range recorded {
		hash, ok := loaded[source.Name]
		if !ok {
			return fmt.Errorf("source %q from the artifact is missing", source.Name)
		}
		if hash != source.SHA256 {
			return fmt.Errorf("source %q does not match the selection SHA-256", source.Name)
		}
	}
	return nil
}

func frozenDigest(params paramsRecord) string {
	payload := fmt.Sprintf("cadenceMin=%d|gapMax=%d|durationMin=%d|runCadence=%d",
		params.CadenceMin, params.GapMax, params.DurationMin, params.RunCadence)
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// partition is one fixed half-open scoring window.
type partition struct {
	from time.Time
	to   time.Time
}

func (p partition) contains(interval activity.Interval) bool {
	return !interval.Start.Before(p.from) && !interval.End.After(p.to)
}

// filterPartition keeps fully contained intervals and counts crossed ones.
func filterPartition(intervals []activity.Interval, p partition) ([]activity.Interval, int) {
	kept := make([]activity.Interval, 0, len(intervals))
	crossed := 0
	for _, interval := range intervals {
		if p.contains(interval) {
			kept = append(kept, interval)
			continue
		}
		if interval.Start.Before(p.to) && interval.End.After(p.from) {
			crossed++
		}
	}
	return kept, crossed
}

func parsePartition(from, to string) (partition, error) {
	if from == "" || to == "" {
		return partition{}, errors.New("both split bounds are required")
	}
	start, err := ParseDay(from)
	if err != nil {
		return partition{}, err
	}
	end, err := ParseDay(to)
	if err != nil {
		return partition{}, err
	}
	if !end.After(start) {
		return partition{}, fmt.Errorf("split bounds are empty or reversed: %s to %s", from, to)
	}
	return partition{from: start, to: end}, nil
}

func formatDay(value time.Time) string {
	return value.In(ist).Format("2006-01-02")
}

func sessionIntervals(sessions []activity.Session) []activity.Interval {
	out := make([]activity.Interval, 0, len(sessions))
	for _, session := range sessions {
		out = append(out, activity.Interval{Start: session.Start, End: session.End})
	}
	return out
}

func sourceRecords(sources Sources, hashes map[string]string) []sourceRecord {
	ordered := []struct {
		name string
		path string
	}{
		{sourceSteps, sources.Steps},
		{sourceStages, sources.Stages},
		{sourceWorkouts, sources.Workouts},
		{sourceHR, sources.HeartRate},
	}
	out := make([]sourceRecord, 0, len(ordered))
	for _, item := range ordered {
		hash, ok := hashes[item.name]
		if !ok {
			continue
		}
		out = append(out, sourceRecord{Name: item.name, Path: item.path, SHA256: hash})
	}
	return out
}

func recordOfParams(params activity.Params) paramsRecord {
	return paramsRecord{
		CadenceMin:  params.CadenceMin,
		GapMax:      params.GapMax,
		DurationMin: params.DurationMin,
		RunCadence:  params.RunCadence,
	}
}

func candidateRecordOf(params activity.Params, score Score) candidateRecord {
	return candidateRecord{
		CadenceMin:  params.CadenceMin,
		GapMax:      params.GapMax,
		DurationMin: params.DurationMin,
		RunCadence:  params.RunCadence,
		TP:          score.TP,
		FP:          score.FP,
		FN:          score.FN,
		Precision:   score.Precision,
		Recall:      score.Recall,
		F1:          score.F1,
	}
}

func referenceRecordOf(stats RefStats) referenceRecord {
	return referenceRecord{
		RawStages:             stats.RawCount,
		ExcludedByWorkout:     stats.ExcludedCount,
		ShortenedByWorkout:    stats.ShortenedCount,
		ShortResiduals:        stats.ShortResidualCount,
		EligibleResiduals:     stats.EligibleCount,
		WorkoutMinutesRemoved: stats.WorkoutExcludedMinutes,
	}
}

func coverageRecordOf(export Export, p partition) coverageRecord {
	coverage := export.CoverageOf(p.from, p.to)
	record := coverageRecord{
		ExpectedMinutes:        coverage.ExpectedMinutes,
		ObservedMinutes:        coverage.ObservedMinutes,
		MissingMinutes:         coverage.ExpectedMinutes - coverage.ObservedMinutes,
		HeartRateJoinedMinutes: joinedHeartRateMinutes(export, p),
		HeartRateIgnored:       export.HeartRateIgnored,
	}
	if coverage.ExpectedMinutes > 0 {
		record.ObservedPercent = float64(coverage.ObservedMinutes) / float64(coverage.ExpectedMinutes)
	}
	if coverage.FirstObservation != nil {
		record.FirstObservation = coverage.FirstObservation.UTC().Format(time.RFC3339)
	}
	if coverage.LastObservation != nil {
		record.LastObservation = coverage.LastObservation.UTC().Format(time.RFC3339)
	}
	return record
}

func joinedHeartRateMinutes(export Export, p partition) int {
	observed := make(map[int64]bool, len(export.HeartRate))
	for _, row := range export.HeartRate {
		observed[row.At.Unix()] = true
	}
	count := 0
	for _, row := range export.Steps {
		if row.At.Before(p.from) || !row.At.Before(p.to) {
			continue
		}
		if observed[row.At.Unix()] {
			count++
		}
	}
	return count
}

func matchRecordsOf(matches []Match, predictions, references []activity.Interval) []matchRecord {
	out := make([]matchRecord, 0, len(matches))
	for _, match := range matches {
		out = append(out, matchRecord{
			Prediction:     intervalRecordOf(predictions[match.Prediction]),
			Reference:      intervalRecordOf(references[match.Reference]),
			IoU:            match.IoU,
			OverlapSeconds: match.OverlapSeconds,
			UnionSeconds:   match.UnionSeconds,
		})
	}
	return out
}

func unmatchedRecords(indices []int, intervals []activity.Interval) []intervalRecord {
	out := make([]intervalRecord, 0, len(indices))
	for _, index := range indices {
		out = append(out, intervalRecordOf(intervals[index]))
	}
	return out
}

func intervalRecordOf(interval activity.Interval) intervalRecord {
	return intervalRecord{
		Start: interval.Start.UTC().Format(time.RFC3339),
		End:   interval.End.UTC().Format(time.RFC3339),
	}
}

func limitations() []string {
	return []string{
		"Zepp ACTIVITY_STAGE intervals are another product's detections, not clinical or human-annotated ground truth.",
		"ACTIVITY_MINUTE rows are sparse; an omitted CSV minute is a missing barrier, so unrecorded steps bias recall downward.",
		"Input steps are Zepp-derived, not captured strap bytes; short raw and Zepp agreement does not prove six-month equivalence.",
		"Walk and Run are heuristic cadence labels; these CSVs cannot validate the 130 steps/minute Run threshold.",
		"Matching is temporal IoU >= 0.5 with a one-to-one maximum-cardinality assignment; it does not validate sport labels, calories or distance.",
		"Selection uses development data only; the holdout is scored once with the frozen parameters and never searched.",
	}
}

// paramsRecord is the frozen detector parameter set as written to artifacts.
type paramsRecord struct {
	CadenceMin  int `json:"cadenceMin"`
	GapMax      int `json:"gapMax"`
	DurationMin int `json:"durationMin"`
	RunCadence  int `json:"runCadence"`
}

func (r paramsRecord) toParams() activity.Params {
	return activity.Params{
		CadenceMin:  r.CadenceMin,
		GapMax:      r.GapMax,
		DurationMin: r.DurationMin,
		RunCadence:  r.RunCadence,
	}
}

func (r paramsRecord) validate() error {
	switch {
	case r.CadenceMin <= 0:
		return errors.New("cadenceMin must be positive")
	case r.GapMax < 0:
		return errors.New("gapMax must not be negative")
	case r.DurationMin <= 0:
		return errors.New("durationMin must be positive")
	case r.RunCadence <= 0:
		return errors.New("runCadence must be positive")
	}
	return nil
}

type sourceRecord struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type dateRangeRecord struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type intervalRecord struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

type matchRecord struct {
	Prediction     intervalRecord `json:"prediction"`
	Reference      intervalRecord `json:"reference"`
	IoU            float64        `json:"iou"`
	OverlapSeconds int64          `json:"overlapSeconds"`
	UnionSeconds   int64          `json:"unionSeconds"`
}

type coverageRecord struct {
	ExpectedMinutes        int     `json:"expectedMinutes"`
	ObservedMinutes        int     `json:"observedMinutes"`
	MissingMinutes         int     `json:"missingMinutes"`
	ObservedPercent        float64 `json:"observedPercent"`
	FirstObservation       string  `json:"firstObservation,omitempty"`
	LastObservation        string  `json:"lastObservation,omitempty"`
	HeartRateJoinedMinutes int     `json:"heartRateJoinedMinutes"`
	HeartRateIgnored       int     `json:"heartRateIgnoredOutOfRange"`
}

// untouchedRecord scores predictions against stages with no workout subtraction.
type untouchedRecord struct {
	PredictionCount    int      `json:"predictionCount"`
	ReferenceCount     int      `json:"referenceCount"`
	CrossedPredictions int      `json:"crossedPredictions"`
	CrossedReferences  int      `json:"crossedReferences"`
	TP                 int      `json:"tp"`
	FP                 int      `json:"fp"`
	FN                 int      `json:"fn"`
	Precision          *float64 `json:"precision"`
	Recall             *float64 `json:"recall"`
	F1                 *float64 `json:"f1"`
}

type partitionRecord struct {
	From                 string           `json:"from"`
	To                   string           `json:"to"`
	PredictionCount      int              `json:"predictionCount"`
	ReferenceCount       int              `json:"referenceCount"`
	CrossedPredictions   int              `json:"crossedPredictions"`
	CrossedReferences    int              `json:"crossedReferences"`
	TP                   int              `json:"tp"`
	FP                   int              `json:"fp"`
	FN                   int              `json:"fn"`
	Precision            *float64         `json:"precision"`
	Recall               *float64         `json:"recall"`
	F1                   *float64         `json:"f1"`
	Matches              []matchRecord    `json:"matches"`
	UnmatchedPredictions []intervalRecord `json:"unmatchedPredictions"`
	UnmatchedReferences  []intervalRecord `json:"unmatchedReferences"`
	Coverage             coverageRecord   `json:"coverage"`
	References           referenceRecord  `json:"references"`
	UntouchedStages      untouchedRecord  `json:"untouchedStages"`
}

type referenceRecord struct {
	RawStages             int `json:"rawStages"`
	ExcludedByWorkout     int `json:"excludedByWorkout"`
	ShortenedByWorkout    int `json:"shortenedByWorkout"`
	ShortResiduals        int `json:"shortResiduals"`
	EligibleResiduals     int `json:"eligibleResiduals"`
	WorkoutMinutesRemoved int `json:"workoutMinutesRemoved"`
}

type candidateRecord struct {
	CadenceMin  int      `json:"cadenceMin"`
	GapMax      int      `json:"gapMax"`
	DurationMin int      `json:"durationMin"`
	RunCadence  int      `json:"runCadence"`
	TP          int      `json:"tp"`
	FP          int      `json:"fp"`
	FN          int      `json:"fn"`
	Precision   *float64 `json:"precision"`
	Recall      *float64 `json:"recall"`
	F1          *float64 `json:"f1"`
}

type gridRecord struct {
	CadenceMin  []int `json:"cadenceMinValues"`
	GapMax      []int `json:"gapMaxValues"`
	DurationMin []int `json:"durationMinValues"`
	RunCadence  int   `json:"runCadence"`
}

type selectionArtifact struct {
	Kind              string            `json:"kind"`
	Timezone          string            `json:"timezone"`
	StopConvention    string            `json:"stopConvention"`
	SelectionRule     string            `json:"selectionRule"`
	Grid              gridRecord        `json:"grid"`
	Sources           []sourceRecord    `json:"sources"`
	Development       dateRangeRecord   `json:"development"`
	Candidates        []candidateRecord `json:"candidates"`
	Frozen            paramsRecord      `json:"frozen"`
	FrozenDigest      string            `json:"frozenDigest"`
	DevelopmentScore  partitionRecord   `json:"developmentScore"`
	DevelopmentTarget targetRecord      `json:"developmentTarget"`
	Limitations       []string          `json:"limitations"`
}

type splitRecord struct {
	Development dateRangeRecord `json:"development"`
	Holdout     dateRangeRecord `json:"holdout"`
}

type namedPartition struct {
	Name  string          `json:"name"`
	Score partitionRecord `json:"score"`
}

type gateRecord struct {
	PrecisionTarget float64  `json:"precisionTarget"`
	RecallTarget    float64  `json:"recallTarget"`
	Precision       *float64 `json:"precision"`
	Recall          *float64 `json:"recall"`
	Passed          bool     `json:"passed"`
	Reason          string   `json:"reason"`
}

// targetRecord records target status without a gate verdict.
type targetRecord struct {
	PrecisionTarget float64  `json:"precisionTarget"`
	RecallTarget    float64  `json:"recallTarget"`
	Precision       *float64 `json:"precision"`
	Recall          *float64 `json:"recall"`
	Met             bool     `json:"met"`
}

type evaluationArtifact struct {
	Kind           string           `json:"kind"`
	Timezone       string           `json:"timezone"`
	StopConvention string           `json:"stopConvention"`
	Selection      sourceRecord     `json:"selection"`
	Sources        []sourceRecord   `json:"sources"`
	Parameters     paramsRecord     `json:"parameters"`
	Split          splitRecord      `json:"split"`
	Partitions     []namedPartition `json:"partitions"`
	Gate           gateRecord       `json:"gate"`
	Limitations    []string         `json:"limitations"`
}

type scoredCandidate struct {
	record candidateRecord
	params activity.Params
}

func describeParams(params paramsRecord) string {
	return fmt.Sprintf("cadenceMin=%d gapMax=%d durationMin=%d runCadence=%d",
		params.CadenceMin, params.GapMax, params.DurationMin, params.RunCadence)
}

func describeScore(score Score) string {
	return fmt.Sprintf("precision=%s recall=%s f1=%s tp=%d fp=%d fn=%d",
		describeMetric(score.Precision), describeMetric(score.Recall), describeMetric(score.F1),
		score.TP, score.FP, score.FN)
}

func describeMetric(value *float64) string {
	if value == nil {
		return "undefined"
	}
	return fmt.Sprintf("%.4f", *value)
}
