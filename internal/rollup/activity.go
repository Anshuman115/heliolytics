package rollup

import (
	"context"
	"time"

	"github.com/Anshuman115/heliolytics/internal/activity"
	"github.com/Anshuman115/heliolytics/internal/store"
)

const activityContextSpan = 12 * time.Hour

// replaceAuto closes the affected old-auto intervals and detector context before
// replacing any rows. Query chunk edges are never detector boundaries.
func replaceAuto(ctx context.Context, view *store.TxView, seeds []store.InstantInterval,
	trigger AutoTrigger) (store.Mutation, error) {
	from, to, ok := intervalBounds(seeds)
	if !ok {
		return store.Mutation{}, nil
	}
	from = from.Truncate(time.Minute)
	if !to.Equal(to.Truncate(time.Minute)) {
		to = to.Truncate(time.Minute).Add(time.Minute)
	}
	params := activity.DefaultParams()
	leftSafe, rightSafe := false, false
	for {
		closedFrom, closedTo, err := expandAutoBounds(ctx, view, from, to)
		if err != nil {
			return store.Mutation{}, err
		}
		leftSafe = leftSafe && closedFrom.Equal(from)
		rightSafe = rightSafe && closedTo.Equal(to)
		from, to = closedFrom, closedTo
		if leftSafe && rightSafe {
			break
		}
		if !leftSafe {
			from, err = findActivityBoundary(ctx, view, from, true, params)
			if err != nil {
				return store.Mutation{}, err
			}
			leftSafe = true
		}
		if !rightSafe {
			to, err = findActivityBoundary(ctx, view, to, false, params)
			if err != nil {
				return store.Mutation{}, err
			}
			rightSafe = true
		}
	}

	var minutes []activity.Minute
	var barriers []activity.Interval
	for start := from; start.Before(to); {
		if err := ctx.Err(); err != nil {
			return store.Mutation{}, err
		}
		end := minInstant(start.Add(activityContextSpan), to)
		chunk, workouts, err := readActivityChunk(ctx, view, start, end)
		if err != nil {
			return store.Mutation{}, err
		}
		for _, m := range chunk {
			minutes = append(minutes, activity.Minute{At: m.At, Steps: m.Steps, HR: m.HR})
		}
		for _, w := range workouts {
			barriers = append(barriers, activity.Interval{
				Start: maxInstant(w.Start, start), End: minInstant(w.End, end),
			})
		}
		start = end
	}
	sessions := activity.DetectWithParams(minutes, barriers, params)
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

// expandAutoBounds includes every intersecting old auto interval, including
// intervals reached only after context grows. Endpoint adjacency is not overlap.
func expandAutoBounds(ctx context.Context, view *store.TxView, from, to time.Time) (time.Time, time.Time, error) {
	for {
		if err := ctx.Err(); err != nil {
			return from, to, err
		}
		autos, err := view.ListAutoActivitiesOverlapping(ctx, from, to)
		if err != nil {
			return from, to, err
		}
		oldFrom, oldTo := from, to
		for _, a := range autos {
			from = minInstant(from, a.StartedAt.Truncate(time.Minute))
			end := a.EndedAt.Truncate(time.Minute)
			if end.Before(a.EndedAt) {
				end = end.Add(time.Minute)
			}
			to = maxInstant(to, end)
		}
		if from.Equal(oldFrom) && to.Equal(oldTo) {
			return from, to, nil
		}
	}
}

func readActivityChunk(ctx context.Context, view *store.TxView, from, to time.Time) ([]store.ActivityMinute, []store.WorkoutInterval, error) {
	minutes, err := view.ReadActivityMinutes(ctx, from, to)
	if err != nil {
		return nil, nil, err
	}
	workouts, err := view.ListWorkoutIntervals(ctx, from, to)
	return minutes, workouts, err
}

func findActivityBoundary(ctx context.Context, view *store.TxView, edge time.Time,
	backwards bool, params activity.Params) (time.Time, error) {
	idle := 0
	for {
		if err := ctx.Err(); err != nil {
			return edge, err
		}
		from, to := edge, edge.Add(activityContextSpan)
		if backwards {
			from, to = edge.Add(-activityContextSpan), edge
		}
		minutes, workouts, err := readActivityChunk(ctx, view, from, to)
		if err != nil {
			return edge, err
		}
		if boundary, ok := activityChunkBoundary(minutes, workouts, from, to, backwards, params, &idle); ok {
			return boundary, nil
		}
		edge = to
		if backwards {
			edge = from
		}
	}
}

// activityChunkBoundary walks ordered observed minutes towards a reset. Idle
// state carries across chunk edges so a bridgeable gap never becomes a barrier.
func activityChunkBoundary(minutes []store.ActivityMinute, workouts []store.WorkoutInterval,
	from, to time.Time, backwards bool, params activity.Params, idle *int) (time.Time, bool) {
	at, index, step := from, 0, 1
	if backwards {
		at, index, step = to.Add(-time.Minute), len(minutes)-1, -1
	}
	for !at.Before(from) && at.Before(to) {
		if index < 0 || index >= len(minutes) || !minutes[index].At.Equal(at) || inWorkout(workouts, at) {
			if backwards {
				return at.Add(time.Minute), true
			}
			return at, true
		}
		if minutes[index].Steps < params.CadenceMin {
			*idle = *idle + 1
			if *idle > params.GapMax {
				if backwards {
					return at, true
				}
				return at.Add(time.Minute), true
			}
		} else {
			*idle = 0
		}
		at = at.Add(time.Duration(step) * time.Minute)
		index += step
	}
	return time.Time{}, false
}

func inWorkout(workouts []store.WorkoutInterval, at time.Time) bool {
	for _, w := range workouts {
		if w.Start.Before(at.Add(time.Minute)) && w.End.After(at) {
			return true
		}
	}
	return false
}

func minInstant(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func maxInstant(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}
