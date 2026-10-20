package parse

// attachWorkoutDetails marks summaries that carry detail evidence. A block
// attaches to the single summary whose half-open window contains it; an
// ambiguous or unmatched block is warned and never creates a session.
func attachWorkoutDetails(workouts []WorkoutRow, blocks []detailBlock) TypeResult {
	unmatched := 0
	firstUnmatched := -1
	for _, block := range blocks {
		matched := -1
		matches := 0
		for i := range workouts {
			if block.at.Before(workouts[i].StartedAt) || !block.at.Before(workouts[i].EndedAt) {
				continue
			}
			matched = i
			matches++
		}
		if matches != 1 {
			unmatched++
			if firstUnmatched < 0 {
				firstUnmatched = block.off
			}
			continue
		}
		workouts[matched].HasDetail = true
	}
	result := TypeResult{Status: "accepted", Rows: 0, Dropped: unmatched}
	if unmatched > 0 {
		result.Warnings = append(result.Warnings, Warning{
			Code: "unmatched_workout_detail", ByteOffset: firstUnmatched, Count: unmatched,
		})
	}
	for i := range workouts {
		if workouts[i].HasDetail {
			result.ProcessedThrough = maxTime(result.ProcessedThrough, workouts[i].EndedAt)
		}
	}
	return result
}
