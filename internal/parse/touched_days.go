package parse

// TouchedDayKeys returns the set of IST day_keys present anywhere in a parsed
// batch, including the per-minute series (StepSeries, HrSeries, TempSeries)
// that feed the step_samples/heart_rate_samples/temperature_samples rollups.
// Ingest uses this to know which days to hand to internal/rollup after
// the raw rows are committed — it is NOT used to compute any daily_metrics
// value directly; rollup re-reads the DB for that.
func TouchedDayKeys(b ParsedBatch) []string {
	set := map[string]bool{}
	for day := range b.StepsByDay {
		set[day] = true
	}
	for _, s := range b.Sleep {
		set[s.DayKey] = true
	}
	for _, p := range b.Pai {
		set[p.DayKey] = true
	}
	for _, r := range b.Readiness {
		set[r.DayKey] = true
	}
	for _, t := range b.Temperature {
		set[t.DayKey] = true
	}
	for _, s := range b.StressSeries {
		set[s.DayKey] = true
	}
	for _, s := range b.HrvSeries {
		set[s.DayKey] = true
	}
	for _, s := range b.Spo2Spot {
		set[s.DayKey] = true
	}
	for _, s := range b.Spo2Sleep {
		set[s.DayKey] = true
	}
	for _, s := range b.RhrSeries {
		set[s.DayKey] = true
	}
	for _, s := range b.RespRateSeries {
		set[s.DayKey] = true
	}
	for _, w := range b.Workouts {
		set[w.DayKey] = true
	}
	for _, a := range b.ActivitySessions {
		set[a.DayKey] = true
	}
	for _, s := range b.StepSeries {
		set[s.DayKey] = true
	}
	for _, s := range b.HrSeries {
		set[s.DayKey] = true
	}
	for _, s := range b.TempSeries {
		set[s.DayKey] = true
	}
	out := make([]string, 0, len(set))
	for day := range set {
		out = append(out, day)
	}
	return out
}
