package readiness_test

import (
	"math"
	"testing"
	"time"

	"github.com/Anshuman115/heliolytics/internal/readiness"
)

var day = time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)

func ptr(v float64) *float64 { return &v }

// shrink is the version 1 spread formula, restated here so expectations never
// come from the production helper.
func shrink(personal float64, n int, prior, floor float64) float64 {
	variance := (float64(n)*personal*personal + 7*prior*prior) / (float64(n) + 7)
	return math.Max(floor, math.Sqrt(variance))
}

func night(hrv, sleepHR, resp, temp *float64) readiness.Night {
	return readiness.Night{HRV: hrv, SleepingHR: sleepHR, RespRate: resp, SkinTempC: temp, SleepScore: ptr(50), TRIMP: ptr(20)}
}

// filled builds a history where every one of the count prior nights carries
// identical values.
func filled(count int, hrv, sleepHR, resp, temp float64) map[time.Time]readiness.Night {
	out := map[time.Time]readiness.Night{}
	trimp := 20.0
	for i := 1; i <= count; i++ {
		out[day.AddDate(0, 0, -i)] = readiness.Night{
			HRV: ptr(hrv), SleepingHR: ptr(sleepHR), RespRate: ptr(resp),
			SkinTempC: ptr(temp), SleepScore: ptr(50), TRIMP: &trimp,
		}
	}
	return out
}

func component(t *testing.T, res readiness.Result, name string) readiness.Component {
	t.Helper()
	for _, c := range res.Components {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("component %q missing", name)
	return readiness.Component{}
}

func hasFlag(res readiness.Result, want string) bool {
	for _, f := range res.Flags {
		if f == want {
			return true
		}
	}
	return false
}

func TestNeutralDayScoresFifty(t *testing.T) {
	history := filled(14, 50, 60, 14, 33)
	target := night(ptr(50), ptr(60), ptr(14), ptr(33))
	history[day] = target

	res := readiness.Compute(day, history, readiness.DefaultParams())
	if res.Status != readiness.StatusScored {
		t.Fatalf("status = %q, want scored", res.Status)
	}
	if res.Score == nil || *res.Score != 50 {
		t.Fatalf("score = %v, want 50", res.Score)
	}
	if res.Raw == nil || math.Abs(*res.Raw-50) > 1e-9 {
		t.Fatalf("raw = %v, want 50", res.Raw)
	}
	if len(res.Components) != 7 {
		t.Fatalf("components = %d, want 7", len(res.Components))
	}
	var weights, points float64
	for _, c := range res.Components {
		if c.Sub == nil || math.Abs(*c.Sub-50) > 1e-9 {
			t.Fatalf("component %s sub = %v, want 50", c.Name, c.Sub)
		}
		weights += *c.Weight
		points += *c.Points
	}
	if math.Abs(weights-1) > 1e-9 {
		t.Fatalf("normalized weights sum = %v, want 1", weights)
	}
	if math.Abs(points-*res.Raw) > 1e-9 {
		t.Fatalf("points sum = %v, want raw %v", points, *res.Raw)
	}
	if len(res.Flags) != 0 {
		t.Fatalf("flags = %v, want none", res.Flags)
	}
}

func TestShrinkageSpreadAtIdenticalValues(t *testing.T) {
	history := filled(14, 50, 60, 14, 33)
	history[day] = night(ptr(50), ptr(60), ptr(14), ptr(33))
	res := readiness.Compute(day, history, readiness.DefaultParams())

	hrv := component(t, res, readiness.NameHRV)
	want := shrink(0, 14, 0.10, 0.05)
	if hrv.SD == nil || math.Abs(*hrv.SD-want) > 1e-9 {
		t.Fatalf("hrv sd = %v, want %v", hrv.SD, want)
	}
	if hrv.SDPersonal == nil || *hrv.SDPersonal != 0 {
		t.Fatalf("personal sd = %v, want 0", hrv.SDPersonal)
	}
	if hrv.Center == nil || math.Abs(*hrv.Center-math.Log(50)) > 1e-9 {
		t.Fatalf("center = %v, want ln(50)", hrv.Center)
	}
}

func TestMissingHRVIsNoData(t *testing.T) {
	history := filled(14, 50, 60, 14, 33)
	history[day] = night(nil, ptr(60), ptr(14), ptr(33))

	res := readiness.Compute(day, history, readiness.DefaultParams())
	if res.Status != readiness.StatusNoData {
		t.Fatalf("status = %q, want no_data", res.Status)
	}
	if res.Score != nil || res.Raw != nil {
		t.Fatalf("score/raw = %v/%v, want nil", res.Score, res.Raw)
	}
	if len(res.Components) != 7 {
		t.Fatalf("components = %d, want 7", len(res.Components))
	}
	if !hasFlag(res, "missing:hrv") {
		t.Fatalf("flags = %v, want missing:hrv", res.Flags)
	}
	if res.TempDeviationC == nil {
		t.Fatal("temperature deviation must survive a missing HRV night")
	}
	temp := component(t, res, readiness.NameSkinTemp)
	if temp.Sub == nil && temp.Center == nil {
		t.Fatal("available optional component must keep diagnostics")
	}
}

func TestBaselineNeedsThreePriorNights(t *testing.T) {
	two := map[time.Time]readiness.Night{
		day.AddDate(0, 0, -1): {HRV: ptr(50)},
		day.AddDate(0, 0, -2): {HRV: ptr(50)},
		day:                   {HRV: ptr(50)},
	}
	res := readiness.Compute(day, two, readiness.DefaultParams())
	if res.Status != readiness.StatusNoBaseline {
		t.Fatalf("two nights status = %q, want no_baseline", res.Status)
	}
	if !hasFlag(res, "baseline_stale:hrv") {
		t.Fatalf("flags = %v, want baseline_stale:hrv", res.Flags)
	}

	three := filled(3, 50, 60, 14, 33)
	three[day] = night(ptr(50), ptr(60), ptr(14), ptr(33))
	res = readiness.Compute(day, three, readiness.DefaultParams())
	if res.Status != readiness.StatusCalibrating {
		t.Fatalf("three nights status = %q, want calibrating", res.Status)
	}
	if res.BaselineNights != 3 {
		t.Fatalf("baseline nights = %d, want 3", res.BaselineNights)
	}
}

func TestScoredAtFourteenPriorNights(t *testing.T) {
	thirteen := filled(13, 50, 60, 14, 33)
	thirteen[day] = night(ptr(50), ptr(60), ptr(14), ptr(33))
	res13 := readiness.Compute(day, thirteen, readiness.DefaultParams())
	if res13.Status != readiness.StatusCalibrating {
		t.Fatalf("13 nights status = %q, want calibrating", res13.Status)
	}

	fourteen := filled(14, 50, 60, 14, 33)
	fourteen[day] = night(ptr(50), ptr(60), ptr(14), ptr(33))
	res14 := readiness.Compute(day, fourteen, readiness.DefaultParams())
	if res14.Status != readiness.StatusScored {
		t.Fatalf("14 nights status = %q, want scored", res14.Status)
	}

	sd13 := component(t, res13, readiness.NameHRV)
	sd14 := component(t, res14, readiness.NameHRV)
	if sd13.SD == nil || sd14.SD == nil {
		t.Fatal("spread must be reported on both calibration states")
	}
	if !(*sd13.SD > *sd14.SD) {
		t.Fatalf("spread must shrink continuously: %v then %v", *sd13.SD, *sd14.SD)
	}
	if math.Abs(*sd13.SD-*sd14.SD) > 0.01 {
		t.Fatalf("spread jump at the scoring boundary: %v then %v", *sd13.SD, *sd14.SD)
	}
}

func TestBaselineExcludesSixtyFirstDay(t *testing.T) {
	history := map[time.Time]readiness.Night{
		day.AddDate(0, 0, -61): {HRV: ptr(50)},
		day.AddDate(0, 0, -60): {HRV: ptr(51)},
		day.AddDate(0, 0, -59): {HRV: ptr(52)},
		day:                    {HRV: ptr(50)},
	}
	res := readiness.Compute(day, history, readiness.DefaultParams())
	if res.Status != readiness.StatusNoBaseline {
		t.Fatalf("status = %q, want no_baseline", res.Status)
	}
	hrv := component(t, res, readiness.NameHRV)
	if hrv.N == nil || *hrv.N != 2 {
		t.Fatalf("baseline n = %v, want 2", hrv.N)
	}
}

func TestBaselineRequiresRecentNights(t *testing.T) {
	history := map[time.Time]readiness.Night{}
	for i := 29; i <= 60; i++ {
		history[day.AddDate(0, 0, -i)] = readiness.Night{HRV: ptr(50)}
	}
	history[day.AddDate(0, 0, -2)] = readiness.Night{HRV: ptr(50)}
	history[day.AddDate(0, 0, -1)] = readiness.Night{HRV: ptr(50)}
	history[day] = readiness.Night{HRV: ptr(50)}

	res := readiness.Compute(day, history, readiness.DefaultParams())
	if res.Status != readiness.StatusNoBaseline {
		t.Fatalf("two recent nights status = %q, want no_baseline", res.Status)
	}

	history[day.AddDate(0, 0, -3)] = readiness.Night{HRV: ptr(50)}
	res = readiness.Compute(day, history, readiness.DefaultParams())
	// Recency gates validity only. Once three recent nights make the baseline
	// valid, C4 scores from the full baseline night count, here 35.
	if res.Status != readiness.StatusScored {
		t.Fatalf("three recent nights status = %q, want scored", res.Status)
	}
	if res.Score == nil || res.Raw == nil {
		t.Fatal("scored status must carry a score and raw value")
	}
	if res.BaselineNights != 35 {
		t.Fatalf("baseline nights = %d, want 35", res.BaselineNights)
	}
}

func TestEvenMedianAndMAD(t *testing.T) {
	values := []float64{10, 12, 14, 16}
	history := map[time.Time]readiness.Night{}
	for i, v := range values {
		history[day.AddDate(0, 0, -(i+1))] = readiness.Night{SleepingHR: ptr(v)}
	}
	history[day] = readiness.Night{SleepingHR: ptr(13)}

	res := readiness.Compute(day, history, readiness.DefaultParams())
	comp := component(t, res, readiness.NameSleepingHR)
	if comp.Center == nil || math.Abs(*comp.Center-13) > 1e-9 {
		t.Fatalf("even median = %v, want 13", comp.Center)
	}
	if comp.SDPersonal == nil || math.Abs(*comp.SDPersonal-1.4826*2) > 1e-9 {
		t.Fatalf("personal spread = %v, want 1.4826*2", comp.SDPersonal)
	}
	want := shrink(1.4826*2, 4, 2.5, 1.0)
	if comp.SD == nil || math.Abs(*comp.SD-want) > 1e-9 {
		t.Fatalf("spread = %v, want %v", comp.SD, want)
	}
}

func TestHigherHRVNeverPenalized(t *testing.T) {
	baseline := filled(14, 40, 60, 14, 33)
	var last readiness.Result
	for i, value := range []float64{45, 55, 70, 90} {
		history := map[time.Time]readiness.Night{}
		for k, v := range baseline {
			history[k] = v
		}
		history[day] = night(ptr(value), ptr(60), ptr(14), ptr(33))
		res := readiness.Compute(day, history, readiness.DefaultParams())
		if res.Status != readiness.StatusScored || res.Score == nil {
			t.Fatalf("hrv %v: status %q", value, res.Status)
		}
		hrv := component(t, res, readiness.NameHRV)
		if i > 0 {
			prior := component(t, last, readiness.NameHRV)
			if *hrv.Sub < *prior.Sub {
				t.Fatalf("hrv sub fell from %v to %v at %v", *prior.Sub, *hrv.Sub, value)
			}
			if *res.Score < *last.Score {
				t.Fatalf("score fell from %d to %d at hrv %v", *last.Score, *res.Score, value)
			}
		}
		last = res
	}
}

func TestBalanceWindowSpansSevenCalendarDays(t *testing.T) {
	history := filled(14, 50, 60, 14, 33)
	history[day.AddDate(0, 0, -7)] = readiness.Night{HRV: ptr(200)}
	history[day] = night(ptr(50), ptr(60), ptr(14), ptr(33))

	res := readiness.Compute(day, history, readiness.DefaultParams())
	balance := component(t, res, readiness.NameHRVBalance)
	if balance.Nights == nil || *balance.Nights != 7 {
		t.Fatalf("balance nights = %v, want 7", balance.Nights)
	}
	if *balance.Value > math.Log(51) {
		t.Fatalf("balance value %v includes a night outside D-6..D", *balance.Value)
	}
}

func TestTemperatureDeadBand(t *testing.T) {
	params := readiness.DefaultParams()
	history := filled(14, 50, 60, 14, 33)
	sd := shrink(0, 14, params.PriorSkinTemp, params.FloorSkinTemp)

	cases := []struct {
		name  string
		value float64
	}{
		{name: "center", value: 33},
		{name: "plus one sd", value: 33 + sd},
		{name: "minus one sd", value: 33 - sd},
		{name: "plus two sd", value: 33 + 2*sd},
		{name: "minus two sd", value: 33 - 2*sd},
	}
	for _, tc := range cases {
		hist := map[time.Time]readiness.Night{}
		for k, v := range history {
			hist[k] = v
		}
		hist[day] = night(ptr(50), ptr(60), ptr(14), ptr(tc.value))
		res := readiness.Compute(day, hist, params)
		comp := component(t, res, readiness.NameSkinTemp)

		z := (tc.value - 33) / sd
		penalty := math.Max(0, math.Abs(z)-1)
		want := math.Max(0, math.Min(50, 50-25*penalty))
		if comp.Sub == nil || math.Abs(*comp.Sub-want) > 1e-6 {
			t.Fatalf("%s sub = %v, want %v", tc.name, comp.Sub, want)
		}
		if comp.Dev == nil || math.Abs(*comp.Dev-(tc.value-33)) > 1e-9 {
			t.Fatalf("%s deviation = %v, want %v", tc.name, comp.Dev, tc.value-33)
		}
		wantHigh := z >= 2
		wantLow := z <= -2
		if hasFlag(res, "temp_high") != wantHigh {
			t.Fatalf("%s flags = %v, want temp_high=%v", tc.name, res.Flags, wantHigh)
		}
		if hasFlag(res, "temp_low") != wantLow {
			t.Fatalf("%s flags = %v, want temp_low=%v", tc.name, res.Flags, wantLow)
		}
	}
}

func TestLoadComponentEligibility(t *testing.T) {
	thirteen := filled(14, 50, 60, 14, 33)
	// Only thirteen days carry a load value, so the chronic mean is ineligible.
	for i := 1; i <= 13; i++ {
		n := thirteen[day.AddDate(0, 0, -i)]
		n.TRIMP = ptr(20)
		thirteen[day.AddDate(0, 0, -i)] = n
	}
	n := thirteen[day.AddDate(0, 0, -14)]
	n.TRIMP = nil
	thirteen[day.AddDate(0, 0, -14)] = n
	thirteen[day] = night(ptr(50), ptr(60), ptr(14), ptr(33))

	res := readiness.Compute(day, thirteen, readiness.DefaultParams())
	load := component(t, res, readiness.NameLoad)
	if load.Sub != nil {
		t.Fatalf("load sub = %v, want unavailable", *load.Sub)
	}
	if !hasFlag(res, "baseline_stale:load") {
		t.Fatalf("flags = %v, want baseline_stale:load", res.Flags)
	}

	fourteen := filled(14, 50, 60, 14, 33)
	fourteen[day] = night(ptr(50), ptr(60), ptr(14), ptr(33))
	res = readiness.Compute(day, fourteen, readiness.DefaultParams())
	load = component(t, res, readiness.NameLoad)
	if load.Sub == nil || math.Abs(*load.Sub-50) > 1e-9 {
		t.Fatalf("load sub = %v, want 50", load.Sub)
	}
	if load.Chronic == nil || *load.Chronic != 20 {
		t.Fatalf("chronic = %v, want 20", load.Chronic)
	}
}

func TestZeroLoadDayIsCapped(t *testing.T) {
	history := filled(14, 50, 60, 14, 33)
	yesterday := history[day.AddDate(0, 0, -1)]
	yesterday.TRIMP = ptr(0)
	history[day.AddDate(0, 0, -1)] = yesterday
	history[day] = night(ptr(50), ptr(60), ptr(14), ptr(33))

	res := readiness.Compute(day, history, readiness.DefaultParams())
	load := component(t, res, readiness.NameLoad)
	if load.Sub == nil || math.Abs(*load.Sub-75) > 1e-9 {
		t.Fatalf("zero-load sub = %v, want the 75 cap", load.Sub)
	}
	if load.Ratio == nil || *load.Ratio != 0 {
		t.Fatalf("ratio = %v, want 0", load.Ratio)
	}
}

func TestLoadSpikeNeedsSevenAcuteDays(t *testing.T) {
	history := filled(14, 50, 60, 14, 33)
	for i := 1; i <= 7; i++ {
		n := history[day.AddDate(0, 0, -i)]
		n.TRIMP = ptr(80)
		history[day.AddDate(0, 0, -i)] = n
	}
	history[day] = night(ptr(50), ptr(60), ptr(14), ptr(33))

	res := readiness.Compute(day, history, readiness.DefaultParams())
	if !hasFlag(res, "load_spike") {
		t.Fatalf("flags = %v, want load_spike", res.Flags)
	}

	gapped := map[time.Time]readiness.Night{}
	for k, v := range history {
		gapped[k] = v
	}
	missing := gapped[day.AddDate(0, 0, -4)]
	missing.TRIMP = nil
	gapped[day.AddDate(0, 0, -4)] = missing
	res = readiness.Compute(day, gapped, readiness.DefaultParams())
	if hasFlag(res, "load_spike") {
		t.Fatalf("flags = %v, want no spike with a missing acute day", res.Flags)
	}
}

func TestLowCoverageUsesBaseWeights(t *testing.T) {
	history := filled(14, 50, 60, 14, 33)
	yesterday := history[day.AddDate(0, 0, -1)]
	yesterday.TRIMP = nil
	history[day.AddDate(0, 0, -1)] = yesterday
	history[day] = readiness.Night{HRV: ptr(50), SleepingHR: ptr(60)}

	res := readiness.Compute(day, history, readiness.DefaultParams())
	if !hasFlag(res, "low_coverage") {
		t.Fatalf("flags = %v, want low_coverage at 0.65 base weight", res.Flags)
	}

	withResp := map[time.Time]readiness.Night{}
	for k, v := range history {
		withResp[k] = v
	}
	withResp[day] = readiness.Night{HRV: ptr(50), SleepingHR: ptr(60), RespRate: ptr(14)}
	res = readiness.Compute(day, withResp, readiness.DefaultParams())
	if hasFlag(res, "low_coverage") {
		t.Fatalf("flags = %v, want no low_coverage at 0.70 base weight", res.Flags)
	}
}

func TestDeterministicComponentOrder(t *testing.T) {
	history := filled(14, 50, 60, 14, 33)
	history[day] = night(ptr(50), ptr(60), ptr(14), ptr(33))
	first, err := readiness.Compute(day, history, readiness.DefaultParams()).JSON()
	if err != nil {
		t.Fatal(err)
	}

	reordered := map[time.Time]readiness.Night{}
	keys := make([]time.Time, 0, len(history))
	for k := range history {
		keys = append(keys, k)
	}
	for i := len(keys) - 1; i >= 0; i-- {
		reordered[keys[i]] = history[keys[i]]
	}
	second, err := readiness.Compute(day, reordered, readiness.DefaultParams()).JSON()
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatalf("JSON differs by map order:\n%s\n%s", first, second)
	}
}

func TestMissingOptionalComponentsAreOmittedFromScoring(t *testing.T) {
	history := filled(14, 50, 60, 14, 33)
	history[day] = readiness.Night{HRV: ptr(50), SleepingHR: ptr(60), TRIMP: ptr(20)}

	res := readiness.Compute(day, history, readiness.DefaultParams())
	if res.Status != readiness.StatusScored {
		t.Fatalf("status = %q, want scored", res.Status)
	}
	for _, name := range []string{readiness.NameSleep, readiness.NameSkinTemp, readiness.NameRespRate} {
		comp := component(t, res, name)
		if comp.Sub != nil || comp.Weight != nil || comp.Points != nil {
			t.Fatalf("%s must be absent from scoring", name)
		}
		if comp.Value != nil {
			t.Fatalf("%s value must stay null", name)
		}
	}
	if !hasFlag(res, "missing:sleep") || !hasFlag(res, "missing:resp_rate") {
		t.Fatalf("flags = %v", res.Flags)
	}
}

func TestFlagOrderIsStable(t *testing.T) {
	history := filled(14, 50, 60, 14, 33)
	for i := 1; i <= 14; i++ {
		n := history[day.AddDate(0, 0, -i)]
		n.SleepingHR = nil
		history[day.AddDate(0, 0, -i)] = n
	}
	history[day] = readiness.Night{HRV: ptr(50), SleepingHR: ptr(60), TRIMP: ptr(20)}

	res := readiness.Compute(day, history, readiness.DefaultParams())
	want := []string{
		"missing:sleep", "missing:skin_temp", "missing:resp_rate",
		"baseline_stale:sleeping_hr", "low_coverage",
	}
	if len(res.Flags) != len(want) {
		t.Fatalf("flags = %v, want %v", res.Flags, want)
	}
	for i, flag := range want {
		if res.Flags[i] != flag {
			t.Fatalf("flags = %v, want %v", res.Flags, want)
		}
	}
}
