// Package readiness computes the Heliolytics v1 daily readiness score from
// nightly vitals and prior training load. It is pure math: no queries, no
// clock, no store imports. The caller supplies normalized IST dates.
package readiness

import (
	"encoding/json"
	"math"
	"sort"
	"time"
)

// Version is the algorithm version persisted with every explanation.
const Version = 1

// Component names in persistence order.
const (
	NameHRV        = "hrv"
	NameHRVBalance = "hrv_balance"
	NameSleepingHR = "sleeping_hr"
	NameSleep      = "sleep"
	NameSkinTemp   = "skin_temp"
	NameRespRate   = "resp_rate"
	NameLoad       = "load"
)

// Status values persisted with every explanation.
const (
	StatusScored      = "scored"
	StatusCalibrating = "calibrating"
	StatusNoBaseline  = "no_baseline"
	StatusNoData      = "no_data"
)

// Night is one IST day's inputs. A nil field means the input is missing; it is
// never treated as zero.
type Night struct {
	HRV        *float64
	SleepingHR *float64
	RespRate   *float64
	SkinTempC  *float64
	SleepScore *float64
	TRIMP      *float64
}

// Component is one uniform explanation entry. Fields that do not apply to a
// component stay nil so the JSON shape never varies.
type Component struct {
	Name       string   `json:"name"`
	Value      *float64 `json:"value"`
	X          *float64 `json:"x"`
	Center     *float64 `json:"center"`
	SD         *float64 `json:"sd"`
	SDPersonal *float64 `json:"sdPersonal"`
	N          *int     `json:"n"`
	Z          *float64 `json:"z"`
	Sub        *float64 `json:"sub"`
	Weight     *float64 `json:"weight"`
	Points     *float64 `json:"points"`
	Nights     *int     `json:"nights"`
	Chronic    *float64 `json:"chronic"`
	Ratio      *float64 `json:"ratio"`
	Dev        *float64 `json:"dev"`
}

// Result is the complete persisted explanation plus the derived columns.
type Result struct {
	Version        int         `json:"version"`
	Status         string      `json:"status"`
	Score          *int        `json:"score"`
	Raw            *float64    `json:"raw"`
	BaselineNights int         `json:"baselineNights"`
	Components     []Component `json:"components"`
	Flags          []string    `json:"flags"`
	TempDeviationC *float64    `json:"-"`
}

// JSON renders the persisted component document.
func (r Result) JSON() ([]byte, error) { return json.Marshal(r) }

// Params is the immutable version 1 constant set. Changing any value requires
// a version bump and a fresh-target replay, so the struct is never edited at
// runtime.
type Params struct {
	Version          int
	BaselineDays     int
	RecencyDays      int
	MinBaseline      int
	MinRecent        int
	ShrinkNights     float64
	MinScoredNights  int
	BalanceDays      int
	BalanceMinNights int
	ChronicDays      int
	ChronicMinDays   int
	AcuteDays        int
	LoadFloor        float64
	LoadSpikeRatio   float64
	LowCoverage      float64
	TempFlagZ        float64

	WeightHRV        float64
	WeightHRVBalance float64
	WeightSleepingHR float64
	WeightSleep      float64
	WeightSkinTemp   float64
	WeightRespRate   float64
	WeightLoad       float64

	PriorHRV      float64
	PriorSleepHR  float64
	PriorRespRate float64
	PriorSkinTemp float64

	FloorHRV      float64
	FloorSleepHR  float64
	FloorRespRate float64
	FloorSkinTemp float64
}

// DefaultParams returns the shipped version 1 constants. The weights, priors,
// floors, gain and thresholds are Heliolytics product choices.
func DefaultParams() Params {
	return Params{
		Version:          Version,
		BaselineDays:     60,
		RecencyDays:      28,
		MinBaseline:      3,
		MinRecent:        3,
		ShrinkNights:     7,
		MinScoredNights:  14,
		BalanceDays:      6,
		BalanceMinNights: 3,
		ChronicDays:      28,
		ChronicMinDays:   14,
		AcuteDays:        7,
		LoadFloor:        10,
		LoadSpikeRatio:   1.5,
		LowCoverage:      0.70,
		TempFlagZ:        2,

		WeightHRV:        0.30,
		WeightHRVBalance: 0.15,
		WeightSleepingHR: 0.20,
		WeightSleep:      0.15,
		WeightSkinTemp:   0.10,
		WeightRespRate:   0.05,
		WeightLoad:       0.05,

		PriorHRV:      0.10,
		PriorSleepHR:  2.5,
		PriorRespRate: 0.5,
		PriorSkinTemp: 0.35,

		FloorHRV:      0.05,
		FloorSleepHR:  1.0,
		FloorRespRate: 0.25,
		FloorSkinTemp: 0.15,
	}
}

// Compute scores one day from its own night plus prior history. History keys
// are compared by calendar date, so any location representation is accepted.
func Compute(day time.Time, nights map[time.Time]Night, p Params) Result {
	c := &calc{
		p:      p,
		day:    dateOnly(day),
		nights: make(map[int]Night, len(nights)),
	}
	for k, v := range nights {
		c.nights[dateKey(k)] = v
	}
	target, _ := c.at(0)

	res := Result{
		Version:    p.Version,
		Components: make([]Component, 0, 7),
		Flags:      []string{},
	}
	var missing, stale []string
	scored := make([]bool, 7)

	hrvComp, hrvMissing, hrvStale := c.hrvComponent(target)
	res.Components = append(res.Components, hrvComp)
	missing = appendMissing(missing, NameHRV, hrvMissing)
	stale = appendBaseline(stale, NameHRV, hrvStale)
	scored[0] = hrvComp.Sub != nil
	res.BaselineNights = c.hrvN

	balComp, balMissing, balStale := c.balanceComponent(target)
	res.Components = append(res.Components, balComp)
	missing = appendMissing(missing, NameHRVBalance, balMissing)
	stale = appendBaseline(stale, NameHRVBalance, balStale)
	scored[1] = balComp.Sub != nil

	comp, miss, st := c.baselineComponent(NameSleepingHR, target.SleepingHR,
		func(n Night) *float64 { return finitePtr(n.SleepingHR) },
		p.PriorSleepHR, p.FloorSleepHR, -1)
	res.Components = append(res.Components, comp)
	missing = appendMissing(missing, NameSleepingHR, miss)
	stale = appendBaseline(stale, NameSleepingHR, st)
	scored[2] = comp.Sub != nil

	comp, miss = c.sleepComponent(target)
	res.Components = append(res.Components, comp)
	missing = appendMissing(missing, NameSleep, miss)
	scored[3] = comp.Sub != nil

	tempComp, tempMissing, tempStale, tempZ := c.temperatureComponent(target)
	res.Components = append(res.Components, tempComp)
	missing = appendMissing(missing, NameSkinTemp, tempMissing)
	stale = appendBaseline(stale, NameSkinTemp, tempStale)
	scored[4] = tempComp.Sub != nil
	res.TempDeviationC = tempComp.Dev

	comp, miss, st = c.baselineComponent(NameRespRate, target.RespRate,
		func(n Night) *float64 { return finitePtr(n.RespRate) },
		p.PriorRespRate, p.FloorRespRate, -1)
	res.Components = append(res.Components, comp)
	missing = appendMissing(missing, NameRespRate, miss)
	stale = appendBaseline(stale, NameRespRate, st)
	scored[5] = comp.Sub != nil

	loadComp, loadMissing, loadStale, spike := c.loadComponent(target)
	res.Components = append(res.Components, loadComp)
	missing = appendMissing(missing, NameLoad, loadMissing)
	stale = appendBaseline(stale, NameLoad, loadStale)
	scored[6] = loadComp.Sub != nil

	weights := [7]float64{
		p.WeightHRV, p.WeightHRVBalance, p.WeightSleepingHR, p.WeightSleep,
		p.WeightSkinTemp, p.WeightRespRate, p.WeightLoad,
	}
	var raw, baseWeight float64
	for i := range res.Components {
		if !scored[i] {
			continue
		}
		baseWeight += weights[i]
	}
	if baseWeight > 0 {
		for i := range res.Components {
			if !scored[i] {
				continue
			}
			weight := weights[i] / baseWeight
			points := weight * *res.Components[i].Sub
			res.Components[i].Weight = new(weight)
			res.Components[i].Points = new(points)
			raw += points
		}
	}

	switch {
	case hrvMissing:
		res.Status = StatusNoData
	case hrvStale:
		res.Status = StatusNoBaseline
	case c.hrvN < p.MinScoredNights:
		res.Status = StatusCalibrating
	default:
		res.Status = StatusScored
	}
	if res.Status == StatusScored {
		res.Score = new(int(math.Round(raw)))
		res.Raw = new(raw)
	}

	res.Flags = append(res.Flags, missing...)
	res.Flags = append(res.Flags, stale...)
	if tempZ != nil {
		switch {
		case *tempZ >= p.TempFlagZ:
			res.Flags = append(res.Flags, "temp_high")
		case *tempZ <= -p.TempFlagZ:
			res.Flags = append(res.Flags, "temp_low")
		}
	}
	if spike {
		res.Flags = append(res.Flags, "load_spike")
	}
	if baseWeight < p.LowCoverage {
		res.Flags = append(res.Flags, "low_coverage")
	}
	return res
}

type calc struct {
	p      Params
	day    time.Time
	nights map[int]Night

	hrvCenter float64
	hrvSD     float64
	hrvN      int
	hrvOK     bool
}

func (c *calc) at(offset int) (Night, bool) {
	n, ok := c.nights[dateKey(c.day.AddDate(0, 0, offset))]
	return n, ok
}

// values collects transformed values in the closed offset range [from, to],
// where both offsets are negative or zero and from is the more recent edge.
func (c *calc) values(from, to int, get func(Night) *float64,
	transform func(float64) float64) []float64 {
	out := make([]float64, 0, 8)
	for off := from; off >= to; off-- {
		n, ok := c.at(off)
		if !ok {
			continue
		}
		v := get(n)
		if v == nil {
			continue
		}
		out = append(out, transform(*v))
	}
	return out
}

func (c *calc) baseline(get func(Night) *float64, transform func(float64) float64,
	prior, floor float64) (center, sd, personal float64, n int, ok bool) {
	all := c.values(-1, -c.p.BaselineDays, get, transform)
	recent := c.values(-1, -c.p.RecencyDays, get, transform)
	n = len(all)
	if len(all) < c.p.MinBaseline || len(recent) < c.p.MinRecent {
		return 0, 0, 0, n, false
	}
	center = median(all)
	deviations := make([]float64, len(all))
	for i, v := range all {
		deviations[i] = math.Abs(v - center)
	}
	personal = 1.4826 * median(deviations)
	variance := (float64(len(all))*personal*personal + c.p.ShrinkNights*prior*prior) /
		(float64(len(all)) + c.p.ShrinkNights)
	sd = math.Sqrt(variance)
	if sd < floor {
		sd = floor
	}
	return center, sd, personal, n, true
}

func (c *calc) hrvComponent(target Night) (Component, bool, bool) {
	comp := Component{Name: NameHRV}
	v := finitePtr(target.HRV)
	if v == nil || *v <= 0 {
		return comp, true, false
	}
	x := math.Log(*v)
	comp.Value = new(*v)
	comp.X = new(x)
	center, sd, personal, n, ok := c.baseline(hrvValue, math.Log, c.p.PriorHRV, c.p.FloorHRV)
	c.hrvN = n
	if !ok {
		comp.N = new(n)
		return comp, false, true
	}
	c.hrvCenter, c.hrvSD, c.hrvOK = center, sd, true
	z := (x - center) / sd
	comp.Center = new(center)
	comp.SD = new(sd)
	comp.SDPersonal = new(personal)
	comp.N = new(n)
	comp.Z = new(z)
	comp.Sub = new(clamp(50+25*z, 0, 100))
	return comp, false, false
}

func (c *calc) balanceComponent(target Night) (Component, bool, bool) {
	comp := Component{Name: NameHRVBalance}
	if finitePtr(target.HRV) == nil || *target.HRV <= 0 {
		return comp, true, false
	}
	if !c.hrvOK {
		return comp, false, true
	}
	logs := c.values(0, -c.p.BalanceDays, hrvValue, math.Log)
	if len(logs) < c.p.BalanceMinNights {
		return comp, true, false
	}
	value := mean(logs)
	z := (value - c.hrvCenter) / (0.5 * c.hrvSD)
	nights := len(logs)
	comp.Value = new(value)
	comp.X = new(value)
	comp.Center = new(c.hrvCenter)
	comp.SD = new(c.hrvSD)
	comp.Nights = &nights
	comp.Z = new(z)
	comp.Sub = new(clamp(50+25*z, 0, 100))
	return comp, false, false
}

func (c *calc) baselineComponent(name string, value *float64,
	get func(Night) *float64, prior, floor float64, direction float64) (Component, bool, bool) {
	comp := Component{Name: name}
	v := finitePtr(value)
	if v == nil {
		return comp, true, false
	}
	comp.Value = new(*v)
	comp.X = new(*v)
	center, sd, personal, n, ok := c.baseline(get, identity, prior, floor)
	if !ok {
		comp.N = new(n)
		return comp, false, true
	}
	z := (*v - center) / sd
	comp.Center = new(center)
	comp.SD = new(sd)
	comp.SDPersonal = new(personal)
	comp.N = new(n)
	comp.Z = new(z)
	comp.Sub = new(clamp(50+25*direction*z, 0, 100))
	return comp, false, false
}

func (c *calc) sleepComponent(target Night) (Component, bool) {
	comp := Component{Name: NameSleep}
	v := finitePtr(target.SleepScore)
	if v == nil {
		return comp, true
	}
	comp.Value = new(*v)
	comp.X = new(*v)
	comp.Sub = new(clamp(*v, 0, 100))
	return comp, false
}

func (c *calc) temperatureComponent(target Night) (Component, bool, bool, *float64) {
	comp := Component{Name: NameSkinTemp}
	v := finitePtr(target.SkinTempC)
	if v == nil {
		return comp, true, false, nil
	}
	comp.Value = new(*v)
	comp.X = new(*v)
	center, sd, personal, n, ok := c.baseline(
		func(n Night) *float64 { return finitePtr(n.SkinTempC) }, identity,
		c.p.PriorSkinTemp, c.p.FloorSkinTemp)
	if !ok {
		comp.N = new(n)
		return comp, false, true, nil
	}
	dev := *v - center
	z := dev / sd
	penalty := math.Max(0, math.Abs(z)-1)
	comp.Center = new(center)
	comp.SD = new(sd)
	comp.SDPersonal = new(personal)
	comp.N = new(n)
	comp.Z = new(z)
	comp.Dev = new(dev)
	comp.Sub = new(clamp(50-25*penalty, 0, 50))
	return comp, false, false, comp.Z
}

func (c *calc) loadComponent(target Night) (Component, bool, bool, bool) {
	comp := Component{Name: NameLoad}
	prior, ok := c.at(-1)
	if !ok {
		return comp, true, false, false
	}
	value := finitePtr(prior.TRIMP)
	if value == nil {
		return comp, true, false, false
	}
	comp.Value = new(*value)
	comp.X = new(*value)
	chronic := c.values(-1, -c.p.ChronicDays, trimpValue, identity)
	if len(chronic) < c.p.ChronicMinDays {
		n := len(chronic)
		comp.N = &n
		return comp, false, true, false
	}
	center := mean(chronic)
	ratio := *value / math.Max(center, c.p.LoadFloor)
	n := len(chronic)
	comp.Chronic = new(center)
	comp.Ratio = new(ratio)
	comp.N = &n
	comp.Sub = new(clamp(50-25*(ratio-1), 0, 75))

	acute := c.values(-1, -c.p.AcuteDays, trimpValue, identity)
	spike := false
	if len(acute) == c.p.AcuteDays {
		spike = mean(acute)/math.Max(center, c.p.LoadFloor) >= c.p.LoadSpikeRatio
	}
	return comp, false, false, spike
}

func hrvValue(n Night) *float64 {
	v := finitePtr(n.HRV)
	if v == nil || *v <= 0 {
		return nil
	}
	return v
}

func trimpValue(n Night) *float64 { return finitePtr(n.TRIMP) }

func appendMissing(flags []string, name string, present bool) []string {
	if !present {
		return flags
	}
	return append(flags, "missing:"+name)
}

func appendBaseline(flags []string, name string, present bool) []string {
	if !present {
		return flags
	}
	return append(flags, "baseline_stale:"+name)
}

func dateOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func dateKey(t time.Time) int { return t.Year()*10000 + int(t.Month())*100 + t.Day() }

func finitePtr(v *float64) *float64 {
	if v == nil || math.IsNaN(*v) || math.IsInf(*v, 0) {
		return nil
	}
	value := *v
	return &value
}

func identity(v float64) float64 { return v }

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func mean(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	var sum float64
	for _, v := range xs {
		sum += v
	}
	return sum / float64(len(xs))
}

func median(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	sorted := append([]float64(nil), xs...)
	sort.Float64s(sorted)
	n := len(sorted)
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2
}
