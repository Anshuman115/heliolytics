package store

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/Anshuman115/heliolytics/internal/store/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// sleepObservation is one immutable typed sleep candidate.
type sleepObservation struct {
	Row        SleepRow
	SessionID  string
	ObservedAt time.Time
}

// workoutObservation is one immutable typed workout candidate.
type workoutObservation struct {
	Row        ActivityRow
	SessionID  string
	ObservedAt time.Time
}

// WriteSleep records this session's sleep candidates, then reconciles every
// affected overlap component from the complete candidate history. existing
// marks a retried envelope, so its candidate set must match the stored one.
func (s *Store) WriteSleep(ctx context.Context, tx pgx.Tx, rows []SleepRow,
	sessionID string, observedAt time.Time, existing bool) (Mutation, error) {
	const table = "sleep_observations"
	b := newMutation()
	if err := validateSessionRef(sessionID, table); err != nil {
		return Mutation{}, err
	}
	if err := validateObservedAt(observedAt, table); err != nil {
		return Mutation{}, err
	}
	observedAt = normInstant(observedAt)

	normalized := make([]SleepRow, 0, len(rows))
	for _, r := range rows {
		if err := validateSleepRow(r); err != nil {
			return Mutation{}, err
		}
		r.StartedAt = normInstant(r.StartedAt)
		r.EndedAt = normInstant(r.EndedAt)
		r.DayKey = dayOf(r.DayKey)
		normalized = append(normalized, r)
	}
	sort.Slice(normalized, func(i, j int) bool { return normalized[i].StartedAt.Before(normalized[j].StartedAt) })
	if err := validateSleepBatch(normalized, table); err != nil {
		return Mutation{}, err
	}

	q := db.New(tx)
	stored, err := q.ListSleepObservationsBySession(ctx, sessionID)
	if err != nil {
		return Mutation{}, err
	}
	if len(stored) > 0 {
		if err := verifySleepRetry(stored, normalized); err != nil {
			return Mutation{}, err
		}
		return b.result(), nil
	}
	if len(normalized) == 0 {
		return b.result(), nil
	}
	if existing {
		return Mutation{}, conflict("stored sleep candidates differ from the retried candidates")
	}
	if err := insertSleepObservations(ctx, q, normalized, sessionID, observedAt); err != nil {
		return Mutation{}, err
	}
	seeds := make([]InstantInterval, 0, len(normalized))
	for _, r := range normalized {
		seeds = append(seeds, InstantInterval{From: r.StartedAt, To: r.EndedAt})
	}
	if err := reconcileSleepComponents(ctx, q, seeds, b); err != nil {
		return Mutation{}, err
	}
	return b.result(), nil
}

// WriteWorkouts records workout candidates and reconciles affected components.
// existing marks a retried envelope, so its candidate set must match the
// stored one.
func (s *Store) WriteWorkouts(ctx context.Context, tx pgx.Tx, rows []ActivityRow,
	sessionID string, observedAt time.Time, existing bool) (Mutation, error) {
	const table = "workout_observations"
	b := newMutation()
	if err := validateSessionRef(sessionID, table); err != nil {
		return Mutation{}, err
	}
	if err := validateObservedAt(observedAt, table); err != nil {
		return Mutation{}, err
	}
	observedAt = normInstant(observedAt)

	normalized := make([]ActivityRow, 0, len(rows))
	for _, r := range rows {
		if err := validateActivityRow(r, table); err != nil {
			return Mutation{}, err
		}
		r.Kind = "workout"
		r.StartedAt = normInstant(r.StartedAt)
		r.EndedAt = normInstant(r.EndedAt)
		normalized = append(normalized, r)
	}
	sort.Slice(normalized, func(i, j int) bool { return normalized[i].StartedAt.Before(normalized[j].StartedAt) })
	if err := validateWorkoutBatch(normalized, table); err != nil {
		return Mutation{}, err
	}

	q := db.New(tx)
	stored, err := q.ListWorkoutObservationsBySession(ctx, sessionID)
	if err != nil {
		return Mutation{}, err
	}
	if len(stored) > 0 {
		if err := verifyWorkoutRetry(stored, normalized); err != nil {
			return Mutation{}, err
		}
		return b.result(), nil
	}
	if len(normalized) == 0 {
		return b.result(), nil
	}
	if existing {
		return Mutation{}, conflict("stored workout candidates differ from the retried candidates")
	}
	if err := insertWorkoutObservations(ctx, q, normalized, sessionID, observedAt); err != nil {
		return Mutation{}, err
	}
	seeds := make([]InstantInterval, 0, len(normalized))
	for _, r := range normalized {
		seeds = append(seeds, InstantInterval{From: r.StartedAt, To: r.EndedAt})
	}
	if err := reconcileWorkoutComponents(ctx, q, seeds, b); err != nil {
		return Mutation{}, err
	}
	return b.result(), nil
}

// WritePAI stores device PAI records. sampled_at is the record epoch and
// observed_at only orders retries.
func (s *Store) WritePAI(ctx context.Context, tx pgx.Tx, rows []PAIScore,
	sessionID string, observedAt time.Time) (Mutation, error) {
	const table = "pai_scores"
	b := newMutation()
	if err := validateSessionRef(sessionID, table); err != nil {
		return Mutation{}, err
	}
	if err := validateObservedAt(observedAt, table); err != nil {
		return Mutation{}, err
	}
	observedAt = normInstant(observedAt)

	normalized := make([]PAIScore, 0, len(rows))
	for _, r := range rows {
		if r.Score < 0 || r.Score > 1000 {
			return Mutation{}, invalidRow(table, "score out of range")
		}
		if err := validateInstant(r.SampledAt, table); err != nil {
			return Mutation{}, err
		}
		r.DayKey = dayOf(r.DayKey)
		r.SampledAt = normInstant(r.SampledAt)
		normalized = append(normalized, r)
	}
	normalized, err := dedupeSamples(table, normalized,
		func(r PAIScore) sampleKey { return keyOf(r.DayKey, "") },
		func(r PAIScore) any { return fmt.Sprintf("%d|%d", r.Score, r.SampledAt.UnixNano()) })
	if err != nil {
		return Mutation{}, err
	}
	if len(normalized) == 0 {
		return b.result(), nil
	}

	q := db.New(tx)
	days := make([]time.Time, 0, len(normalized))
	for _, r := range normalized {
		days = append(days, r.DayKey)
	}
	stored, err := q.ListPaiScoresByDays(ctx, pgDates(days))
	if err != nil {
		return Mutation{}, err
	}
	type storedRow struct {
		score     int
		sampledAt time.Time
		observed  time.Time
		sessionID string
	}
	prev := make(map[int64]storedRow, len(stored))
	for _, r := range stored {
		prev[dateOf(r.DayKey).UnixNano()] = storedRow{
			score:     int(r.Score),
			sampledAt: r.SampledAt.Time,
			observed:  r.ObservedAt.Time,
			sessionID: r.SessionID,
		}
	}

	params := db.UpsertPaiScoresParams{
		DayKey:     make([]pgtype.Date, 0, len(normalized)),
		Score:      make([]int16, 0, len(normalized)),
		SampledAt:  make([]pgtype.Timestamptz, 0, len(normalized)),
		ObservedAt: make([]pgtype.Timestamptz, 0, len(normalized)),
		SessionID:  make([]string, 0, len(normalized)),
	}
	for _, r := range normalized {
		old, exists := prev[r.DayKey.UnixNano()]
		if exists && !tupleGE(observedAt, sessionID, old.observed, old.sessionID) {
			continue
		}
		if !exists || old.score != r.Score || !old.sampledAt.Equal(r.SampledAt) {
			b.addDay(r.DayKey)
		}
		params.DayKey = append(params.DayKey, pgDate(r.DayKey))
		params.Score = append(params.Score, int16(r.Score))
		params.SampledAt = append(params.SampledAt, tsAt(r.SampledAt))
		params.ObservedAt = append(params.ObservedAt, tsAt(observedAt))
		params.SessionID = append(params.SessionID, sessionID)
	}
	if len(params.DayKey) == 0 {
		return b.result(), nil
	}
	if err := q.UpsertPaiScores(ctx, params); err != nil {
		return Mutation{}, err
	}
	return b.result(), nil
}

// ---- candidate validation ----

func validateSleepRow(r SleepRow) error {
	const table = "sleep_observations"
	if err := validateInstant(r.StartedAt, table); err != nil {
		return err
	}
	if err := validateInstant(r.EndedAt, table); err != nil {
		return err
	}
	if !r.EndedAt.After(r.StartedAt) {
		return invalidRow(table, "ended_at must be after started_at")
	}
	if r.TotalMins < 0 || r.TotalMins > 1440 {
		return invalidRow(table, "total_mins out of range")
	}
	if r.DeepMins < 0 || r.RemMins < 0 || r.LightMins < 0 || r.WakeMins < 0 {
		return invalidRow(table, "stage minutes must not be negative")
	}
	if r.DeepMins+r.RemMins+r.LightMins != r.TotalMins {
		return invalidRow(table, "asleep stage minutes must equal total_mins")
	}
	if r.Score != nil && (*r.Score < 0 || *r.Score > 100) {
		return invalidRow(table, "score out of range")
	}
	return nil
}

func validateSleepBatch(rows []SleepRow, table string) error {
	for i := 1; i < len(rows); i++ {
		prev, cur := rows[i-1], rows[i]
		if prev.StartedAt.Equal(cur.StartedAt) {
			if !sleepRowsEqual(prev, cur) {
				return invalidRow(table, "conflicting candidates share one start instant")
			}
			return invalidRow(table, "duplicate candidate start instant")
		}
		if cur.StartedAt.Before(prev.EndedAt) {
			return invalidRow(table, "candidates within one session overlap")
		}
	}
	return nil
}

func validateActivityRow(r ActivityRow, table string) error {
	if err := validateInstant(r.StartedAt, table); err != nil {
		return err
	}
	if err := validateInstant(r.EndedAt, table); err != nil {
		return err
	}
	if !r.EndedAt.After(r.StartedAt) {
		return invalidRow(table, "ended_at must be after started_at")
	}
	if r.SportType < 0 || r.SportType > 32767 {
		return invalidRow(table, "sport_type out of range")
	}
	if r.SportName == "" {
		return invalidRow(table, "sport_name required")
	}
	for _, v := range []*int{r.Calories, r.AvgHR, r.MaxHR, r.MinHR, r.Steps} {
		if v == nil {
			continue
		}
		if *v < 0 || *v > 100000 {
			return invalidRow(table, "activity metric out of range")
		}
	}
	return nil
}

func validateWorkoutBatch(rows []ActivityRow, table string) error {
	for i := 1; i < len(rows); i++ {
		prev, cur := rows[i-1], rows[i]
		if prev.StartedAt.Equal(cur.StartedAt) {
			if !activityRowsEqual(prev, cur) {
				return invalidRow(table, "conflicting candidates share one start instant")
			}
			return invalidRow(table, "duplicate candidate start instant")
		}
		if cur.StartedAt.Before(prev.EndedAt) {
			return invalidRow(table, "candidates within one session overlap")
		}
	}
	return nil
}

// ---- candidate insertion ----

// insertSleepObservations stores the candidate rows of a session that has no
// stored candidates yet.
func insertSleepObservations(ctx context.Context, q *db.Queries, rows []SleepRow,
	sessionID string, observedAt time.Time) error {
	params := db.InsertSleepObservationsParams{
		SessionID:       make([]string, 0, len(rows)),
		StartedAt:       make([]pgtype.Timestamptz, 0, len(rows)),
		EndedAt:         make([]pgtype.Timestamptz, 0, len(rows)),
		DayKey:          make([]pgtype.Date, 0, len(rows)),
		IsNap:           make([]bool, 0, len(rows)),
		Score:           make([]pgtype.Numeric, 0, len(rows)),
		TotalMins:       make([]int16, 0, len(rows)),
		DeepMins:        make([]int16, 0, len(rows)),
		RemMins:         make([]int16, 0, len(rows)),
		LightMins:       make([]int16, 0, len(rows)),
		WakeMins:        make([]int16, 0, len(rows)),
		Stages:          make([][]byte, 0, len(rows)),
		StagesTruncated: make([]bool, 0, len(rows)),
		ObservedAt:      make([]pgtype.Timestamptz, 0, len(rows)),
	}
	for _, r := range rows {
		score, err := numericIntPtr(r.Score)
		if err != nil {
			return invalidRow("sleep_observations", "score not representable")
		}
		stages, err := encodeSleepStages(r.Stages)
		if err != nil {
			return invalidRow("sleep_observations", "stages could not be encoded")
		}
		params.SessionID = append(params.SessionID, sessionID)
		params.StartedAt = append(params.StartedAt, tsAt(r.StartedAt))
		params.EndedAt = append(params.EndedAt, tsAt(r.EndedAt))
		params.DayKey = append(params.DayKey, pgDate(r.DayKey))
		params.IsNap = append(params.IsNap, r.IsNap)
		params.Score = append(params.Score, score)
		params.TotalMins = append(params.TotalMins, int16(r.TotalMins))
		params.DeepMins = append(params.DeepMins, int16(r.DeepMins))
		params.RemMins = append(params.RemMins, int16(r.RemMins))
		params.LightMins = append(params.LightMins, int16(r.LightMins))
		params.WakeMins = append(params.WakeMins, int16(r.WakeMins))
		params.Stages = append(params.Stages, stages)
		params.StagesTruncated = append(params.StagesTruncated, r.StagesTruncated)
		params.ObservedAt = append(params.ObservedAt, tsAt(observedAt))
	}
	return q.InsertSleepObservations(ctx, params)
}

// verifySleepRetry compares a retry's re-parsed candidates with the complete
// stored set for the session. Both sides are ordered by start time.
func verifySleepRetry(stored []db.ListSleepObservationsBySessionRow, rows []SleepRow) error {
	if len(stored) != len(rows) {
		return conflict("stored sleep candidates differ from the retried candidates")
	}
	for i, r := range stored {
		prior, err := sleepRowOf(r.StartedAt.Time, r.EndedAt.Time, r.DayKey, r.IsNap, r.Score,
			r.TotalMins, r.DeepMins, r.RemMins, r.LightMins, r.WakeMins, r.Stages, r.StagesTruncated)
		if err != nil {
			return err
		}
		if !sleepRowsEqual(prior, rows[i]) {
			return conflict("stored sleep candidate differs from the retried candidate")
		}
	}
	return nil
}

// insertWorkoutObservations stores the candidate rows of a session that has no
// stored candidates yet.
func insertWorkoutObservations(ctx context.Context, q *db.Queries, rows []ActivityRow,
	sessionID string, observedAt time.Time) error {
	params := db.InsertWorkoutObservationsParams{
		SessionID:  make([]string, 0, len(rows)),
		StartedAt:  make([]pgtype.Timestamptz, 0, len(rows)),
		EndedAt:    make([]pgtype.Timestamptz, 0, len(rows)),
		SportType:  make([]int16, 0, len(rows)),
		SportName:  make([]string, 0, len(rows)),
		Calories:   make([]pgtype.Numeric, 0, len(rows)),
		AvgHr:      make([]pgtype.Numeric, 0, len(rows)),
		MaxHr:      make([]pgtype.Numeric, 0, len(rows)),
		MinHr:      make([]pgtype.Numeric, 0, len(rows)),
		Steps:      make([]pgtype.Numeric, 0, len(rows)),
		HasDetail:  make([]bool, 0, len(rows)),
		ObservedAt: make([]pgtype.Timestamptz, 0, len(rows)),
	}
	for _, r := range rows {
		calories, err := numericIntPtr(r.Calories)
		if err != nil {
			return invalidRow("workout_observations", "calories not representable")
		}
		avgHR, err := numericIntPtr(r.AvgHR)
		if err != nil {
			return invalidRow("workout_observations", "avg_hr not representable")
		}
		maxHR, err := numericIntPtr(r.MaxHR)
		if err != nil {
			return invalidRow("workout_observations", "max_hr not representable")
		}
		minHR, err := numericIntPtr(r.MinHR)
		if err != nil {
			return invalidRow("workout_observations", "min_hr not representable")
		}
		steps, err := numericIntPtr(r.Steps)
		if err != nil {
			return invalidRow("workout_observations", "steps not representable")
		}
		params.SessionID = append(params.SessionID, sessionID)
		params.StartedAt = append(params.StartedAt, tsAt(r.StartedAt))
		params.EndedAt = append(params.EndedAt, tsAt(r.EndedAt))
		params.SportType = append(params.SportType, int16(r.SportType))
		params.SportName = append(params.SportName, r.SportName)
		params.Calories = append(params.Calories, calories)
		params.AvgHr = append(params.AvgHr, avgHR)
		params.MaxHr = append(params.MaxHr, maxHR)
		params.MinHr = append(params.MinHr, minHR)
		params.Steps = append(params.Steps, steps)
		params.HasDetail = append(params.HasDetail, r.HasDetail)
		params.ObservedAt = append(params.ObservedAt, tsAt(observedAt))
	}
	return q.InsertWorkoutObservations(ctx, params)
}

// verifyWorkoutRetry compares a retry's re-parsed candidates with the complete
// stored set for the session. Both sides are ordered by start time.
func verifyWorkoutRetry(stored []db.ListWorkoutObservationsBySessionRow, rows []ActivityRow) error {
	if len(stored) != len(rows) {
		return conflict("stored workout candidates differ from the retried candidates")
	}
	for i, r := range stored {
		prior := ActivityRow{
			StartedAt: r.StartedAt.Time,
			EndedAt:   r.EndedAt.Time,
			Kind:      "workout",
			SportType: int(r.SportType),
			SportName: r.SportName,
			Calories:  int2Val(r.Calories),
			AvgHR:     int2Val(r.AvgHr),
			MaxHR:     int2Val(r.MaxHr),
			MinHR:     int2Val(r.MinHr),
			Steps:     int4Val(r.Steps),
			HasDetail: r.HasDetail,
		}
		if !activityRowsEqual(prior, rows[i]) {
			return conflict("stored workout candidate differs from the retried candidate")
		}
	}
	return nil
}

// ---- component reconciliation ----

func reconcileSleepComponents(ctx context.Context, q *db.Queries, seeds []InstantInterval,
	b *mutationBuilder) error {
	components := mergeIntervals(seeds)
	for {
		grew := false
		next := make([]InstantInterval, 0, len(components))
		for _, c := range components {
			from, to, err := sleepComponentBounds(ctx, q, c.From, c.To)
			if err != nil {
				return err
			}
			if from.Before(c.From) || to.After(c.To) {
				grew = true
			}
			next = append(next, InstantInterval{From: from, To: to})
		}
		components = mergeIntervals(next)
		if !grew {
			break
		}
	}
	for _, c := range components {
		if err := applySleepWinners(ctx, q, c.From, c.To, b); err != nil {
			return err
		}
	}
	return nil
}

func sleepComponentBounds(ctx context.Context, q *db.Queries, from, to time.Time) (time.Time, time.Time, error) {
	rows, err := q.ListSleepObservationsOverlapping(ctx, db.ListSleepObservationsOverlappingParams{
		FromAt: tsAt(from),
		ToAt:   tsAt(to),
	})
	if err != nil {
		return from, to, err
	}
	for _, r := range rows {
		if r.StartedAt.Time.Before(from) {
			from = r.StartedAt.Time
		}
		if r.EndedAt.Time.After(to) {
			to = r.EndedAt.Time
		}
	}
	return from, to, nil
}

func applySleepWinners(ctx context.Context, q *db.Queries, from, to time.Time,
	b *mutationBuilder) error {
	candidateRows, err := q.ListSleepObservationsOverlapping(ctx, db.ListSleepObservationsOverlappingParams{
		FromAt: tsAt(from),
		ToAt:   tsAt(to),
	})
	if err != nil {
		return err
	}
	candidates := make([]sleepObservation, 0, len(candidateRows))
	for _, r := range candidateRows {
		row, err := sleepRowOf(r.StartedAt.Time, r.EndedAt.Time, r.DayKey, r.IsNap, r.Score,
			r.TotalMins, r.DeepMins, r.RemMins, r.LightMins, r.WakeMins, r.Stages, r.StagesTruncated)
		if err != nil {
			return err
		}
		candidates = append(candidates, sleepObservation{Row: row, SessionID: r.SessionID, ObservedAt: r.ObservedAt.Time})
	}
	winners := selectSleepWinners(candidates)

	currentRows, err := q.ListSleepSessionsOverlapping(ctx, db.ListSleepSessionsOverlappingParams{
		FromAt: tsAt(from),
		ToAt:   tsAt(to),
	})
	if err != nil {
		return err
	}
	type canonicalSleep struct {
		row        SleepRow
		observedAt time.Time
		sessionID  string
	}
	current := make(map[int64]canonicalSleep, len(currentRows))
	for _, r := range currentRows {
		row, err := sleepRowOf(r.StartedAt.Time, r.EndedAt.Time, r.DayKey, r.IsNap, r.Score,
			r.TotalMins, r.DeepMins, r.RemMins, r.LightMins, r.WakeMins, r.Stages, r.StagesTruncated)
		if err != nil {
			return err
		}
		current[r.StartedAt.Time.UnixNano()] = canonicalSleep{
			row:        row,
			observedAt: r.ObservedAt.Time,
			sessionID:  r.SessionID,
		}
	}

	winnerByStart := make(map[int64]sleepObservation, len(winners))
	for _, w := range winners {
		winnerByStart[w.Row.StartedAt.UnixNano()] = w
	}

	deleteKeys := make([]pgtype.Timestamptz, 0, len(current))
	for start, existing := range current {
		if _, keep := winnerByStart[start]; keep {
			continue
		}
		row := existing.row
		deleteKeys = append(deleteKeys, tsAt(row.StartedAt))
		b.addDay(row.DayKey)
		b.addSpan(row.StartedAt, row.EndedAt)
	}
	if len(deleteKeys) > 0 {
		if err := q.DeleteSleepSessionsIn(ctx, deleteKeys); err != nil {
			return err
		}
	}

	for _, w := range winners {
		prior, exists := current[w.Row.StartedAt.UnixNano()]
		same := exists && sleepRowsEqual(prior.row, w.Row)
		if same && !tupleAfter(w.ObservedAt, w.SessionID, prior.observedAt, prior.sessionID) {
			continue
		}
		if !same {
			b.addDay(w.Row.DayKey)
			b.addSpan(w.Row.StartedAt, w.Row.EndedAt)
			if exists {
				b.addDay(prior.row.DayKey)
				b.addSpan(prior.row.StartedAt, prior.row.EndedAt)
			}
		}
		stages, err := encodeSleepStages(w.Row.Stages)
		if err != nil {
			return err
		}
		if err := q.UpsertSleepSession(ctx, db.UpsertSleepSessionParams{
			StartedAt:       tsAt(w.Row.StartedAt),
			EndedAt:         tsAt(w.Row.EndedAt),
			DayKey:          pgDate(w.Row.DayKey),
			IsNap:           w.Row.IsNap,
			Score:           int2Ptr(w.Row.Score),
			TotalMins:       int16(w.Row.TotalMins),
			DeepMins:        int16(w.Row.DeepMins),
			RemMins:         int16(w.Row.RemMins),
			LightMins:       int16(w.Row.LightMins),
			WakeMins:        int16(w.Row.WakeMins),
			Stages:          stages,
			StagesTruncated: w.Row.StagesTruncated,
			ObservedAt:      tsAt(w.ObservedAt),
			SessionID:       w.SessionID,
		}); err != nil {
			return err
		}
	}
	return nil
}

// selectSleepWinners keeps the newest candidate of each disjoint interval.
func selectSleepWinners(candidates []sleepObservation) []sleepObservation {
	ordered := make([]sleepObservation, len(candidates))
	copy(ordered, candidates)
	sort.Slice(ordered, func(i, j int) bool {
		a, b := ordered[i], ordered[j]
		if !a.ObservedAt.Equal(b.ObservedAt) {
			return a.ObservedAt.After(b.ObservedAt)
		}
		if a.SessionID != b.SessionID {
			return a.SessionID > b.SessionID
		}
		if !a.Row.StartedAt.Equal(b.Row.StartedAt) {
			return a.Row.StartedAt.Before(b.Row.StartedAt)
		}
		return a.Row.EndedAt.Before(b.Row.EndedAt)
	})
	winners := make([]sleepObservation, 0, len(ordered))
	for _, c := range ordered {
		blocked := false
		for _, w := range winners {
			if c.Row.StartedAt.Before(w.Row.EndedAt) && c.Row.EndedAt.After(w.Row.StartedAt) {
				blocked = true
				break
			}
		}
		if blocked {
			continue
		}
		winners = append(winners, c)
	}
	return winners
}

func reconcileWorkoutComponents(ctx context.Context, q *db.Queries, seeds []InstantInterval,
	b *mutationBuilder) error {
	components := mergeIntervals(seeds)
	for {
		grew := false
		next := make([]InstantInterval, 0, len(components))
		for _, c := range components {
			from, to, err := workoutComponentBounds(ctx, q, c.From, c.To)
			if err != nil {
				return err
			}
			if from.Before(c.From) || to.After(c.To) {
				grew = true
			}
			next = append(next, InstantInterval{From: from, To: to})
		}
		components = mergeIntervals(next)
		if !grew {
			break
		}
	}
	for _, c := range components {
		if err := applyWorkoutWinners(ctx, q, c.From, c.To, b); err != nil {
			return err
		}
	}
	return nil
}

func workoutComponentBounds(ctx context.Context, q *db.Queries, from, to time.Time) (time.Time, time.Time, error) {
	rows, err := q.ListWorkoutObservationsOverlapping(ctx, db.ListWorkoutObservationsOverlappingParams{
		FromAt: tsAt(from),
		ToAt:   tsAt(to),
	})
	if err != nil {
		return from, to, err
	}
	for _, r := range rows {
		if r.StartedAt.Time.Before(from) {
			from = r.StartedAt.Time
		}
		if r.EndedAt.Time.After(to) {
			to = r.EndedAt.Time
		}
	}
	return from, to, nil
}

func applyWorkoutWinners(ctx context.Context, q *db.Queries, from, to time.Time,
	b *mutationBuilder) error {
	candidateRows, err := q.ListWorkoutObservationsOverlapping(ctx, db.ListWorkoutObservationsOverlappingParams{
		FromAt: tsAt(from),
		ToAt:   tsAt(to),
	})
	if err != nil {
		return err
	}
	candidates := make([]workoutObservation, 0, len(candidateRows))
	for _, r := range candidateRows {
		row := ActivityRow{
			StartedAt: r.StartedAt.Time,
			EndedAt:   r.EndedAt.Time,
			Kind:      "workout",
			SportType: int(r.SportType),
			SportName: r.SportName,
			Calories:  int2Val(r.Calories),
			AvgHR:     int2Val(r.AvgHr),
			MaxHR:     int2Val(r.MaxHr),
			MinHR:     int2Val(r.MinHr),
			Steps:     int4Val(r.Steps),
			HasDetail: r.HasDetail,
		}
		candidates = append(candidates, workoutObservation{Row: row, SessionID: r.SessionID, ObservedAt: r.ObservedAt.Time})
	}
	winners := selectWorkoutWinners(candidates)

	currentRows, err := q.ListWorkoutSessionsOverlapping(ctx, db.ListWorkoutSessionsOverlappingParams{
		FromAt: tsAt(from),
		ToAt:   tsAt(to),
	})
	if err != nil {
		return err
	}
	type canonicalWorkout struct {
		row        ActivityRow
		observedAt time.Time
		sessionID  string
	}
	current := make(map[int64]canonicalWorkout, len(currentRows))
	for _, r := range currentRows {
		current[r.StartedAt.Time.UnixNano()] = canonicalWorkout{
			row: ActivityRow{
				StartedAt: r.StartedAt.Time,
				EndedAt:   r.EndedAt.Time,
				Kind:      "workout",
				SportType: int(r.SportType),
				SportName: r.SportName,
				Calories:  int2Val(r.Calories),
				AvgHR:     int2Val(r.AvgHr),
				MaxHR:     int2Val(r.MaxHr),
				MinHR:     int2Val(r.MinHr),
				Steps:     int4Val(r.Steps),
				HasDetail: r.HasDetail,
			},
			observedAt: r.ObservedAt.Time,
			sessionID:  r.SessionID,
		}
	}

	winnerByStart := make(map[int64]workoutObservation, len(winners))
	for _, w := range winners {
		winnerByStart[w.Row.StartedAt.UnixNano()] = w
	}

	deleteKeys := make([]pgtype.Timestamptz, 0, len(current))
	for start, existing := range current {
		if _, keep := winnerByStart[start]; keep {
			continue
		}
		row := existing.row
		deleteKeys = append(deleteKeys, tsAt(row.StartedAt))
		b.addDay(dayOf(row.StartedAt))
		b.addSpan(row.StartedAt, row.EndedAt)
	}
	if len(deleteKeys) > 0 {
		if err := q.DeleteWorkoutSessionsIn(ctx, deleteKeys); err != nil {
			return err
		}
	}

	for _, w := range winners {
		prior, exists := current[w.Row.StartedAt.UnixNano()]
		same := exists && activityRowsEqual(prior.row, w.Row)
		if same && !tupleAfter(w.ObservedAt, w.SessionID, prior.observedAt, prior.sessionID) {
			continue
		}
		if !same {
			b.addDay(dayOf(w.Row.StartedAt))
			b.addSpan(w.Row.StartedAt, w.Row.EndedAt)
			if exists {
				b.addDay(dayOf(prior.row.StartedAt))
				b.addSpan(prior.row.StartedAt, prior.row.EndedAt)
			}
		}
		if err := q.UpsertWorkoutSession(ctx, db.UpsertWorkoutSessionParams{
			StartedAt:  tsAt(w.Row.StartedAt),
			EndedAt:    tsAt(w.Row.EndedAt),
			SportType:  int16(w.Row.SportType),
			SportName:  w.Row.SportName,
			Calories:   int2Ptr(w.Row.Calories),
			AvgHr:      int2Ptr(w.Row.AvgHR),
			MaxHr:      int2Ptr(w.Row.MaxHR),
			MinHr:      int2Ptr(w.Row.MinHR),
			Steps:      int4Ptr(w.Row.Steps),
			HasDetail:  w.Row.HasDetail,
			ObservedAt: tsAt(w.ObservedAt),
			SessionID:  w.SessionID,
		}); err != nil {
			return err
		}
	}
	return nil
}

func selectWorkoutWinners(candidates []workoutObservation) []workoutObservation {
	ordered := make([]workoutObservation, len(candidates))
	copy(ordered, candidates)
	sort.Slice(ordered, func(i, j int) bool {
		a, b := ordered[i], ordered[j]
		if !a.ObservedAt.Equal(b.ObservedAt) {
			return a.ObservedAt.After(b.ObservedAt)
		}
		if a.SessionID != b.SessionID {
			return a.SessionID > b.SessionID
		}
		if !a.Row.StartedAt.Equal(b.Row.StartedAt) {
			return a.Row.StartedAt.Before(b.Row.StartedAt)
		}
		return a.Row.EndedAt.Before(b.Row.EndedAt)
	})
	winners := make([]workoutObservation, 0, len(ordered))
	for _, c := range ordered {
		blocked := false
		for _, w := range winners {
			if c.Row.StartedAt.Before(w.Row.EndedAt) && c.Row.EndedAt.After(w.Row.StartedAt) {
				blocked = true
				break
			}
		}
		if blocked {
			continue
		}
		winners = append(winners, c)
	}
	return winners
}

// ---- row conversion and equality ----

func sleepRowOf(startedAt, endedAt time.Time, dayKey pgtype.Date, isNap bool, score pgtype.Int2,
	totalMins, deepMins, remMins, lightMins, wakeMins int16, stages []byte,
	truncated bool) (SleepRow, error) {
	decoded, err := decodeSleepStages(stages)
	if err != nil {
		return SleepRow{}, err
	}
	return SleepRow{
		StartedAt:       startedAt,
		EndedAt:         endedAt,
		DayKey:          dateOf(dayKey),
		IsNap:           isNap,
		Score:           int2Val(score),
		TotalMins:       int(totalMins),
		DeepMins:        int(deepMins),
		RemMins:         int(remMins),
		LightMins:       int(lightMins),
		WakeMins:        int(wakeMins),
		Stages:          decoded,
		StagesTruncated: truncated,
	}, nil
}

func sleepRowsEqual(a, b SleepRow) bool {
	if !a.StartedAt.Equal(b.StartedAt) || !a.EndedAt.Equal(b.EndedAt) {
		return false
	}
	if !a.DayKey.Equal(b.DayKey) || a.IsNap != b.IsNap || a.StagesTruncated != b.StagesTruncated {
		return false
	}
	if !equalIntPtr(a.Score, b.Score) {
		return false
	}
	if a.TotalMins != b.TotalMins || a.DeepMins != b.DeepMins || a.RemMins != b.RemMins ||
		a.LightMins != b.LightMins || a.WakeMins != b.WakeMins {
		return false
	}
	return stagesEqual(a.Stages, b.Stages)
}

func stagesEqual(a, b []SleepStagePoint) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !a[i].Start.Equal(b[i].Start) || !a[i].End.Equal(b[i].End) || a[i].Type != b[i].Type {
			return false
		}
	}
	return true
}

func activityRowsEqual(a, b ActivityRow) bool {
	if a.Kind != b.Kind || !a.StartedAt.Equal(b.StartedAt) || !a.EndedAt.Equal(b.EndedAt) {
		return false
	}
	if a.SportType != b.SportType || a.SportName != b.SportName || a.HasDetail != b.HasDetail {
		return false
	}
	return equalIntPtr(a.Calories, b.Calories) && equalIntPtr(a.AvgHR, b.AvgHR) &&
		equalIntPtr(a.MaxHR, b.MaxHR) && equalIntPtr(a.MinHR, b.MinHR) &&
		equalIntPtr(a.Steps, b.Steps)
}

func equalIntPtr(a, b *int) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
