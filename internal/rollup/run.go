package rollup

import (
	"context"
	"sort"
	"time"

	"github.com/Anshuman115/heliolytics/internal/readiness"
	"github.com/Anshuman115/heliolytics/internal/store"
	"github.com/jackc/pgx/v5"
)

const readinessHaloDays = 60

type AutoTrigger struct {
	SessionID  string
	ObservedAt time.Time
}

type RunInput struct {
	TouchedDays       []time.Time
	MutationIntervals []store.InstantInterval
	Auto              *AutoTrigger
}

func Run(ctx context.Context, st *store.Store, tx pgx.Tx, in RunInput) error {
	now := time.Now().In(store.IST)
	return runAt(ctx, st, tx, in, store.Day(now.Year(), now.Month(), now.Day()))
}

func runAt(ctx context.Context, _ *store.Store, tx pgx.Tx, in RunInput, today time.Time) error {
	view := store.NewTxView(tx)

	affected := newDaySet()
	for _, d := range in.TouchedDays {
		affected.add(d)
	}
	intervals := mergeIntIntervals(in.MutationIntervals)
	if from, to, ok := intervalBounds(intervals); ok {
		// The proxy compares temperature and HR in the same UTC minute, so the
		// refresh range needs one minute of halo on both edges.
		proxyFrom := from.Add(-time.Minute)
		proxyTo := to.Add(time.Minute)
		proxy, err := view.RecomputeTemperatureWearProxy(ctx, proxyFrom, proxyTo)
		if err != nil {
			return err
		}
		for _, d := range proxy.Days {
			affected.add(d)
		}
		sleepDays, err := view.AffectedSleepDays(ctx, proxyFrom, proxyTo)
		if err != nil {
			return err
		}
		for _, d := range sleepDays {
			affected.add(d)
		}
	}
	if in.Auto != nil && len(intervals) > 0 {
		mut, err := replaceAuto(ctx, view, intervals, *in.Auto)
		if err != nil {
			return err
		}
		for _, d := range mut.Days {
			affected.add(d)
		}
	}
	if affected.empty() {
		return nil
	}

	_, last, err := view.CanonicalDayBounds(ctx)
	if err != nil {
		return err
	}
	start := affected.min()
	end := today
	if latest := affected.max(); latest.After(end) {
		end = latest
	}
	if last != nil && last.After(end) {
		end = *last
	}
	if start.After(end) {
		start = end
	}

	days := calendarRange(start, end)
	supported, err := view.ListCanonicalDayKeys(ctx, days)
	if err != nil {
		return err
	}
	sort.Slice(supported, func(i, j int) bool { return supported[i].Before(supported[j]) })
	if len(supported) == 0 {
		_, err := view.NormalizeDailyDomainTx(ctx, days)
		return err
	}
	if err := view.EnsureDailyMetricsRows(ctx, supported); err != nil {
		return err
	}

	profile, err := view.GetProfile(ctx)
	if err != nil {
		return err
	}
	inputs, err := view.ReadRollupInputs(ctx, start, end)
	if err != nil {
		return err
	}
	sortInputs(&inputs)

	history, err := view.ReadReadinessHistory(ctx, start.AddDate(0, 0, -readinessHaloDays),
		start.AddDate(0, 0, -1))
	if err != nil {
		return err
	}
	nights := make(map[int]readiness.Night, len(history)+len(supported))
	resting := make(map[int]float64, len(history)+len(supported))
	for _, row := range history {
		nights[dayKeyOf(row.DayKey)] = nightOf(row)
		if row.RestingHR != nil {
			resting[dayKeyOf(row.DayKey)] = *row.RestingHR
		}
	}

	aggregates := make([]store.DailyAggregate, 0, len(supported))
	for _, day := range supported {
		agg := aggregateDay(day, inputs)
		aggregates = append(aggregates, agg)
		nights[dayKeyOf(day)] = nightOf(agg)
		if agg.RestingHR != nil {
			resting[dayKeyOf(day)] = *agg.RestingHR
		}
	}

	loads := make([]store.LoadUpdate, 0, len(supported))
	for _, day := range supported {
		from := dayStart(day)
		hr := between(inputs.HR, hrAt, from, from.AddDate(0, 0, 1))
		var current *float64
		if value, ok := resting[dayKeyOf(day)]; ok {
			current = new(value)
		}
		hrRest := restingHRBaseline(resting, day, current)
		value := trimp(day, hr, hrRest, profile.BirthDate, sexOf(profile))
		loads = append(loads, store.LoadUpdate{DayKey: day, TRIMP: value})
		night := nights[dayKeyOf(day)]
		night.TRIMP = value
		nights[dayKeyOf(day)] = night
	}

	params := readiness.DefaultParams()
	ready := make([]store.ReadinessUpdate, 0, len(supported))
	for _, day := range supported {
		window := make(map[time.Time]readiness.Night, readinessHaloDays+1)
		for i := range readinessHaloDays + 1 {
			d := day.AddDate(0, 0, -i)
			if night, ok := nights[dayKeyOf(d)]; ok {
				window[d] = night
			}
		}
		res := readiness.Compute(day, window, params)
		components, err := res.JSON()
		if err != nil {
			return err
		}
		ready = append(ready, store.ReadinessUpdate{
			DayKey:         day,
			Score:          res.Score,
			Components:     components,
			TempDeviationC: res.TempDeviationC,
		})
	}

	if err := view.WriteDailyAggregates(ctx, aggregates); err != nil {
		return err
	}
	if err := view.WriteLoad(ctx, loads); err != nil {
		return err
	}
	if err := view.WriteReadiness(ctx, ready); err != nil {
		return err
	}
	_, err = view.NormalizeDailyDomainTx(ctx, days)
	return err
}

func aggregateDay(day time.Time, in store.RollupInputs) store.DailyAggregate {
	from := dayStart(day)
	to := from.AddDate(0, 0, 1)

	hr := between(in.HR, hrAt, from, to)
	steps := between(in.Steps, stepAt, from, to)
	stress := between(in.Stress, stressAt, from, to)
	spo2 := between(in.SpO2, spo2At, from, to)
	night := summarizeSleep(in.Sleep, day)
	workouts, autos, calories := activityCounts(byActivityDay(in.Activities, day))

	agg := store.DailyAggregate{
		DayKey:             day,
		Steps:              sumSteps(steps),
		AvgHR:              meanDayHeartRate(hr),
		StressAvg:          meanStress(stress),
		NapCount:           night.naps,
		PAIScore:           paiScore(byPAIDay(in.PAI, day)),
		WorkoutCount:       workouts,
		AutoActivityCount:  autos,
		WorkoutCalories:    calories,
		SleepScore:         night.score,
		SleepMins:          night.asleep,
		SleepDeepMins:      night.deep,
		SleepRemMins:       night.rem,
		SleepLightMins:     night.light,
		SleepWakeMins:      night.wake,
		TimeInBedMins:      night.inBed,
		SleepEfficiencyPct: night.efficiency,
	}
	agg.SpO2Avg, agg.SpO2Min, agg.SpO2N = spo2Summary(spo2)

	if night.quality {
		agg.RestingHR = restingHeartRate(inSpans(in.HR, hrAt, night.union), night.union)
		agg.HRVRMSSDMS = meanNightHRV(inSpans(in.HRV, hrvAt, night.union), night.union)
		agg.RespRate = meanNightResp(inSpans(in.Resp, respAt, night.union))
		agg.SpO2SleepAvg, agg.SpO2SleepMin, _ = spo2Summary(inSpans(in.SpO2, spo2At, night.union))
		agg.SkinTempNightC = meanNightTemp(inSpans(in.Temperatures, tempAt, night.union), night.union)
	}
	return agg
}

func nightOf(a store.DailyAggregate) readiness.Night {
	night := readiness.Night{
		HRV:        a.HRVRMSSDMS,
		SleepingHR: a.RestingHR,
		RespRate:   a.RespRate,
		SkinTempC:  a.SkinTempNightC,
		TRIMP:      a.TRIMP,
	}
	if a.SleepScore != nil {
		night.SleepScore = new(float64(*a.SleepScore))
	}
	return night
}

func sexOf(p store.Profile) string {
	if p.Sex == nil {
		return ""
	}
	return *p.Sex
}

func between[T any](rows []T, at func(T) time.Time, from, to time.Time) []T {
	lo := sort.Search(len(rows), func(i int) bool { return !at(rows[i]).Before(from) })
	hi := sort.Search(len(rows), func(i int) bool { return !at(rows[i]).Before(to) })
	if lo >= hi {
		return nil
	}
	return rows[lo:hi]
}

func inSpans[T any](rows []T, at func(T) time.Time, spans []span) []T {
	out := make([]T, 0, len(rows))
	for _, s := range spans {
		out = append(out, between(rows, at, s.Start, s.End)...)
	}
	return out
}

func sortInputs(in *store.RollupInputs) {
	sort.Slice(in.HR, func(i, j int) bool { return in.HR[i].At.Before(in.HR[j].At) })
	sort.Slice(in.Steps, func(i, j int) bool { return in.Steps[i].At.Before(in.Steps[j].At) })
	sort.Slice(in.Temperatures, func(i, j int) bool { return in.Temperatures[i].At.Before(in.Temperatures[j].At) })
	sort.Slice(in.HRV, func(i, j int) bool { return in.HRV[i].At.Before(in.HRV[j].At) })
	sort.Slice(in.Stress, func(i, j int) bool { return in.Stress[i].At.Before(in.Stress[j].At) })
	sort.Slice(in.Resp, func(i, j int) bool { return in.Resp[i].At.Before(in.Resp[j].At) })
	sort.Slice(in.SpO2, func(i, j int) bool { return in.SpO2[i].At.Before(in.SpO2[j].At) })
	sort.Slice(in.Activities, func(i, j int) bool {
		return in.Activities[i].StartedAt.Before(in.Activities[j].StartedAt)
	})
}

func byActivityDay(rows []store.ActivityRow, day time.Time) []store.ActivityRow {
	out := make([]store.ActivityRow, 0, 2)
	for _, r := range rows {
		if sameDay(dayOf(r.StartedAt), day) {
			out = append(out, r)
		}
	}
	return out
}

func byPAIDay(rows []store.PAIScore, day time.Time) []store.PAIScore {
	out := make([]store.PAIScore, 0, 1)
	for _, r := range rows {
		if sameDay(r.DayKey, day) {
			out = append(out, r)
		}
	}
	return out
}

func hrAt(r store.HeartRateSample) time.Time      { return r.At }
func stepAt(r store.StepSample) time.Time         { return r.At }
func tempAt(r store.TemperatureReadRow) time.Time { return r.At }
func hrvAt(r store.HRVSample) time.Time           { return r.At }
func stressAt(r store.StressSample) time.Time     { return r.At }
func respAt(r store.RespSample) time.Time         { return r.At }
func spo2At(r store.SpO2Sample) time.Time         { return r.At }

func dayStart(day time.Time) time.Time { return store.Day(day.Year(), day.Month(), day.Day()) }

func dayOf(t time.Time) time.Time {
	local := t.In(store.IST)
	return store.Day(local.Year(), local.Month(), local.Day())
}

func dayKeyOf(t time.Time) int { return t.Year()*10000 + int(t.Month())*100 + t.Day() }

func calendarRange(start, end time.Time) []time.Time {
	out := make([]time.Time, 0, 32)
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		out = append(out, d)
	}
	return out
}

func mergeIntIntervals(in []store.InstantInterval) []store.InstantInterval {
	sorted := make([]store.InstantInterval, len(in))
	copy(sorted, in)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].From.Before(sorted[j].From) })
	out := make([]store.InstantInterval, 0, len(sorted))
	for _, s := range sorted {
		if !s.To.After(s.From) {
			continue
		}
		if n := len(out); n > 0 && !s.From.After(out[n-1].To) {
			if s.To.After(out[n-1].To) {
				out[n-1].To = s.To
			}
			continue
		}
		out = append(out, s)
	}
	return out
}

func intervalBounds(in []store.InstantInterval) (time.Time, time.Time, bool) {
	if len(in) == 0 {
		return time.Time{}, time.Time{}, false
	}
	from, to := in[0].From, in[0].To
	for _, s := range in[1:] {
		if s.From.Before(from) {
			from = s.From
		}
		if s.To.After(to) {
			to = s.To
		}
	}
	return from, to, to.After(from)
}

type daySet struct {
	byKey map[int]time.Time
}

func newDaySet() *daySet { return &daySet{byKey: map[int]time.Time{}} }

func (s *daySet) add(t time.Time) {
	day := dayOf(t)
	s.byKey[dayKeyOf(day)] = day
}

func (s *daySet) empty() bool { return len(s.byKey) == 0 }

func (s *daySet) min() time.Time {
	var out time.Time
	for _, d := range s.byKey {
		if out.IsZero() || d.Before(out) {
			out = d
		}
	}
	return out
}

func (s *daySet) max() time.Time {
	var out time.Time
	for _, d := range s.byKey {
		if d.After(out) {
			out = d
		}
	}
	return out
}
