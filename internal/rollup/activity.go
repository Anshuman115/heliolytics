package rollup

import (
	"context"
	"sort"
	"time"

	"github.com/Anshuman115/heliolytics/internal/activity"
	"github.com/Anshuman115/heliolytics/internal/store"
)

// Auto detection context limits. The window is grown until real barriers bound
// it, never a fixed halo.
const (
	activityContextSpan  = 12 * time.Hour
	activityContextTries = 3
	autoWindowExpansions = 16
)

// replaceAuto recomputes every auto activity touched by the seed intervals and
// replaces only interval-overlapping kind=auto rows.
func replaceAuto(ctx context.Context, view *store.TxView, seeds []store.InstantInterval,
	trigger AutoTrigger) (store.Mutation, error) {
	from, to, ok := intervalBounds(seeds)
	if !ok {
		return store.Mutation{}, nil
	}
	var (
		minutes  []store.ActivityMinute
		workouts []store.WorkoutInterval
	)
	for i := range activityContextTries {
		var err error
		from, to, err = expandAutoBounds(ctx, view, from, to)
		if err != nil {
			return store.Mutation{}, err
		}
		contextSpan := activityContextSpan << i
		minutes, err = view.ReadActivityMinutes(ctx, from.Add(-contextSpan), to.Add(contextSpan))
		if err != nil {
			return store.Mutation{}, err
		}
		workouts, err = view.ListWorkoutIntervals(ctx, from.Add(-contextSpan), to.Add(contextSpan))
		if err != nil {
			return store.Mutation{}, err
		}
		trimFrom, trimTo, found := trimBlocks(minutes, workouts, from, to)
		if !found {
			return view.ReplaceAutoActivities(ctx, from, to, nil, trigger.SessionID, trigger.ObservedAt)
		}
		stable := trimFrom.After(from.Add(-contextSpan)) && trimTo.Before(to.Add(contextSpan))
		from, to = trimFrom, trimTo
		if stable {
			break
		}
	}

	sort.Slice(minutes, func(i, j int) bool { return minutes[i].At.Before(minutes[j].At) })
	window := between(minutes, func(m store.ActivityMinute) time.Time { return m.At }, from, to)
	barriers := make([]activity.Interval, 0, len(workouts))
	for _, w := range workouts {
		if !w.End.After(from) || !w.Start.Before(to) {
			continue
		}
		barriers = append(barriers, activity.Interval{Start: w.Start, End: w.End})
	}
	sessions := activity.DetectWithParams(detectorMinutes(window), barriers, activity.DefaultParams())
	rows := make([]store.ActivityRow, 0, len(sessions))
	for _, s := range sessions {
		rows = append(rows, store.ActivityRow{
			StartedAt: s.Start,
			EndedAt:   s.End,
			SportType: 0,
			SportName: s.Label,
			AvgHR:     s.AvgHR,
			MaxHR:     s.MaxHR,
			MinHR:     s.MinHR,
			Steps:     new(s.Steps),
		})
	}
	return view.ReplaceAutoActivities(ctx, from, to, rows, trigger.SessionID, trigger.ObservedAt)
}

// detectorMinutes converts canonical step minutes for the detector. An absent
// row stays absent, so a missing minute remains a hard barrier.
func detectorMinutes(rows []store.ActivityMinute) []activity.Minute {
	out := make([]activity.Minute, 0, len(rows))
	for _, r := range rows {
		out = append(out, activity.Minute{At: r.At, Steps: r.Steps, HR: r.HR})
	}
	return out
}

// expandAutoBounds widens the window until it contains every old auto row that
// touches it, so no stale session survives on either edge.
func expandAutoBounds(ctx context.Context, view *store.TxView, from, to time.Time) (time.Time, time.Time, error) {
	for range autoWindowExpansions {
		autos, err := view.ListAutoActivitiesOverlapping(ctx, from, to)
		if err != nil {
			return from, to, err
		}
		grown := false
		for _, a := range autos {
			if a.StartedAt.Before(from) {
				from = a.StartedAt
				grown = true
			}
			if a.EndedAt.After(to) {
				to = a.EndedAt
				grown = true
			}
		}
		if !grown {
			break
		}
	}
	return from, to, nil
}

// contiguousBlocks groups observed step minutes into maximal blocks. A missing
// minute or a workout minute is a hard boundary.
func contiguousBlocks(minutes []store.ActivityMinute, workouts []store.WorkoutInterval) []span {
	out := make([]span, 0, 4)
	var current span
	open := false
	for _, m := range minutes {
		at := m.At.Truncate(time.Minute)
		if inWorkout(workouts, at) {
			if open {
				out = append(out, current)
				open = false
			}
			continue
		}
		if open && at.Equal(current.End) {
			current.End = at.Add(time.Minute)
			continue
		}
		if open {
			out = append(out, current)
		}
		current = span{Start: at, End: at.Add(time.Minute)}
		open = true
	}
	if open {
		out = append(out, current)
	}
	return out
}

// trimBlocks narrows the replacement window to the observed blocks that
// intersect the seed range.
func trimBlocks(minutes []store.ActivityMinute, workouts []store.WorkoutInterval,
	from, to time.Time) (time.Time, time.Time, bool) {
	var first, last time.Time
	found := false
	for _, b := range contiguousBlocks(minutes, workouts) {
		if !b.End.After(from) || !b.Start.Before(to) {
			continue
		}
		if !found || b.Start.Before(first) {
			first = b.Start
		}
		if !found || b.End.After(last) {
			last = b.End
		}
		found = true
	}
	return first, last, found
}

func inWorkout(workouts []store.WorkoutInterval, at time.Time) bool {
	for _, w := range workouts {
		if !at.Before(w.Start) && at.Before(w.End) {
			return true
		}
	}
	return false
}
