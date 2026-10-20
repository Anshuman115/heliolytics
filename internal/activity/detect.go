package activity

import (
	"sort"
	"time"
)

// Minute is one observed UTC minute bucket of canonical step and HR data.
// A zero Steps value is an observed zero; an absent bucket is missing data.
type Minute struct {
	At    time.Time
	Steps int
	HR    *int
}

// Interval is a half-open UTC range.
type Interval struct {
	Start time.Time
	End   time.Time
}

// Session is one detected sustained-movement interval.
type Session struct {
	Start time.Time
	End   time.Time
	Steps int
	AvgHR *int
	MaxHR *int
	MinHR *int
	Label string
}

// Params holds detector thresholds. See doc.go for the unit of each field.
type Params struct {
	CadenceMin  int
	GapMax      int
	DurationMin int
	RunCadence  int
}

// Labels reported for auto-detected sessions. Both are heuristic.
const (
	LabelWalk = "Walk"
	LabelRun  = "Run"
)

// Frozen by activity-eval select on development range 2026-01-01..2026-05-01 (activity-selection.json).
const (
	defaultCadenceMin  = 60
	defaultGapMax      = 3
	defaultDurationMin = 10
	defaultRunCadence  = 130
)

// DefaultParams returns the production thresholds.
func DefaultParams() Params {
	return Params{
		CadenceMin:  defaultCadenceMin,
		GapMax:      defaultGapMax,
		DurationMin: defaultDurationMin,
		RunCadence:  defaultRunCadence,
	}
}

// Detect runs the detector with the production thresholds.
func Detect(minutes []Minute, workouts []Interval) []Session {
	return DetectWithParams(minutes, workouts, DefaultParams())
}

// DetectWithParams runs the detector with explicit thresholds. Neither input
// slice is mutated.
func DetectWithParams(minutes []Minute, workouts []Interval, p Params) []Session {
	if len(minutes) == 0 {
		return nil
	}
	barriers := coalesce(workouts)
	var (
		out  []Session
		run  runState
		prev time.Time
		bi   int
	)
	for i, m := range minutes {
		if i > 0 && m.At.Sub(prev) != time.Minute {
			out = run.close(p, out)
		}
		prev = m.At

		end := m.At.Add(time.Minute)
		for bi < len(barriers) && !barriers[bi].End.After(m.At) {
			bi++
		}
		if bi < len(barriers) && barriers[bi].Start.Before(end) {
			out = run.close(p, out)
			continue
		}
		if m.Steps >= p.CadenceMin {
			run.move(m)
			continue
		}
		out = run.gap(p, m, out)
	}
	return run.close(p, out)
}

// pendingGap is one observed below-threshold bucket held until the run resumes.
type pendingGap struct {
	steps int
	hr    *int
}

// runState accumulates one open candidate without storing its buckets.
type runState struct {
	active     bool
	start      time.Time
	lastMoving time.Time
	steps      int
	hrSum      int
	hrCount    int
	hrMin      int
	hrMax      int
	gaps       []pendingGap
}

func (r *runState) reset() {
	r.active = false
	r.start = time.Time{}
	r.lastMoving = time.Time{}
	r.steps = 0
	r.hrSum = 0
	r.hrCount = 0
	r.hrMin = 0
	r.hrMax = 0
	r.gaps = r.gaps[:0]
}

func (r *runState) accumulate(steps int, hr *int) {
	r.steps += steps
	if hr == nil {
		return
	}
	if r.hrCount == 0 {
		r.hrMin = *hr
		r.hrMax = *hr
	} else {
		if *hr < r.hrMin {
			r.hrMin = *hr
		}
		if *hr > r.hrMax {
			r.hrMax = *hr
		}
	}
	r.hrSum += *hr
	r.hrCount++
}

func (r *runState) move(m Minute) {
	if !r.active {
		r.reset()
		r.active = true
		r.start = m.At
	}
	for _, gap := range r.gaps {
		r.accumulate(gap.steps, gap.hr)
	}
	r.gaps = r.gaps[:0]
	r.accumulate(m.Steps, m.HR)
	r.lastMoving = m.At.Add(time.Minute)
}

func (r *runState) gap(p Params, m Minute, out []Session) []Session {
	if !r.active {
		return out
	}
	r.gaps = append(r.gaps, pendingGap{steps: m.Steps, hr: m.HR})
	if len(r.gaps) > p.GapMax {
		return r.close(p, out)
	}
	return out
}

func (r *runState) close(p Params, out []Session) []Session {
	if !r.active {
		return out
	}
	elapsed := int(r.lastMoving.Sub(r.start) / time.Minute)
	if elapsed > 0 && elapsed >= p.DurationMin {
		session := Session{Start: r.start, End: r.lastMoving, Steps: r.steps, Label: LabelWalk}
		if r.steps >= p.RunCadence*elapsed {
			session.Label = LabelRun
		}
		if r.hrCount > 0 {
			avg := (2*r.hrSum + r.hrCount) / (2 * r.hrCount)
			lo, hi := r.hrMin, r.hrMax
			session.AvgHR = &avg
			session.MinHR = &lo
			session.MaxHR = &hi
		}
		out = append(out, session)
	}
	r.reset()
	return out
}

// coalesce sorts and merges overlapping, nested or touching intervals.
func coalesce(intervals []Interval) []Interval {
	if len(intervals) == 0 {
		return nil
	}
	sorted := make([]Interval, len(intervals))
	copy(sorted, intervals)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Start.Equal(sorted[j].Start) {
			return sorted[i].End.Before(sorted[j].End)
		}
		return sorted[i].Start.Before(sorted[j].Start)
	})
	out := make([]Interval, 0, len(sorted))
	for _, interval := range sorted {
		if !interval.End.After(interval.Start) {
			continue
		}
		if last := len(out); last > 0 && !interval.Start.After(out[last-1].End) {
			if interval.End.After(out[last-1].End) {
				out[last-1].End = interval.End
			}
			continue
		}
		out = append(out, interval)
	}
	return out
}
