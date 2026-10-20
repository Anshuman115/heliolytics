package store

import (
	"context"
	"fmt"
	"time"

	"github.com/Anshuman115/heliolytics/internal/store/db"
	"github.com/jackc/pgx/v5/pgtype"
)

func (v *TxView) EnsureDailyMetricsRows(ctx context.Context, days []time.Time) error {
	supported, err := v.ListCanonicalDayKeys(ctx, days)
	if err != nil {
		return err
	}
	if len(supported) == 0 {
		return nil
	}
	return v.q.EnsureDailyRows(ctx, pgDates(supported))
}

func (v *TxView) ListCanonicalDayKeys(ctx context.Context, days []time.Time) ([]time.Time, error) {
	unique := uniqueDays(days)
	if len(unique) == 0 {
		return []time.Time{}, nil
	}
	rows, err := v.q.ListCanonicalDayKeys(ctx, pgDates(unique))
	if err != nil {
		return nil, err
	}
	return daysOf(rows), nil
}

func (v *TxView) NormalizeDailyDomainTx(ctx context.Context, days []time.Time) ([]time.Time, error) {
	if len(days) == 0 {
		return []time.Time{}, nil
	}
	supported, err := v.ListCanonicalDayKeys(ctx, days)
	if err != nil {
		return nil, err
	}
	deleted, err := v.q.DeleteUnsupportedDailyRows(ctx, db.DeleteUnsupportedDailyRowsParams{
		Days:      pgDates(days),
		Supported: pgDates(supported),
	})
	if err != nil {
		return nil, err
	}
	present, err := v.q.ListDailyRowDays(ctx, pgDates(supported))
	if err != nil {
		return nil, err
	}
	have := make(map[int64]struct{}, len(present))
	for _, d := range present {
		have[dateOf(d).UnixNano()] = struct{}{}
	}
	for _, d := range supported {
		if _, ok := have[d.UnixNano()]; !ok {
			return nil, fmt.Errorf("daily_metrics missing computed row for supported day %s",
				d.Format(dateLayout))
		}
	}
	return daysOf(deleted), nil
}

func (v *TxView) CanonicalDayBounds(ctx context.Context) (*time.Time, *time.Time, error) {
	queries := []func(context.Context) ([]pgtype.Date, error){
		v.q.HeartRateDayBounds,
		v.q.StepDayBounds,
		v.q.TemperatureDayBounds,
		v.q.HrvDayBounds,
		v.q.StressDayBounds,
		v.q.RespDayBounds,
		v.q.Spo2DayBounds,
		v.q.SleepDayBounds,
		v.q.ActivityDayBounds,
		v.q.PaiDayBounds,
	}
	var first, last *time.Time
	for _, query := range queries {
		days, err := query(ctx)
		if err != nil {
			return nil, nil, err
		}
		if len(days) == 0 {
			continue
		}
		candidates := []time.Time{dateOf(days[0]), dateOf(days[len(days)-1])}
		for _, d := range candidates {
			if first == nil || d.Before(*first) {
				first = new(d)
			}
			if last == nil || d.After(*last) {
				last = new(d)
			}
		}
	}
	return first, last, nil
}

func (v *TxView) ReadRollupInputs(ctx context.Context, fromDay, toDay time.Time) (RollupInputs, error) {
	fromAt := fromDay
	toAt := nextDay(toDay)

	sleepRows, err := v.q.ListSleepSessions(ctx, db.ListSleepSessionsParams{
		FromDay: pgDate(fromDay),
		ToDay:   pgDate(toDay),
	})
	if err != nil {
		return RollupInputs{}, err
	}
	sleep := make([]SleepRow, 0, len(sleepRows))
	readFrom := fromAt
	for _, r := range sleepRows {
		row, err := sleepRowOf(r.StartedAt.Time, r.EndedAt.Time, r.DayKey, r.IsNap, r.Score,
			r.TotalMins, r.DeepMins, r.RemMins, r.LightMins, r.WakeMins, r.Stages, r.StagesTruncated)
		if err != nil {
			return RollupInputs{}, err
		}
		if row.StartedAt.Before(readFrom) {
			readFrom = row.StartedAt
		}
		sleep = append(sleep, row)
	}

	out := RollupInputs{Sleep: sleep}
	if out.HR, err = listHeartRate(ctx, v.q, readFrom, toAt); err != nil {
		return RollupInputs{}, err
	}
	if out.Steps, err = listSteps(ctx, v.q, readFrom, toAt); err != nil {
		return RollupInputs{}, err
	}
	if out.Temperatures, err = listTemperature(ctx, v.q, readFrom, toAt); err != nil {
		return RollupInputs{}, err
	}
	if out.HRV, err = listHRV(ctx, v.q, readFrom, toAt); err != nil {
		return RollupInputs{}, err
	}
	if out.Stress, err = listStress(ctx, v.q, readFrom, toAt); err != nil {
		return RollupInputs{}, err
	}
	if out.Resp, err = listResp(ctx, v.q, readFrom, toAt); err != nil {
		return RollupInputs{}, err
	}
	if out.SpO2, err = listSpO2(ctx, v.q, readFrom, toAt); err != nil {
		return RollupInputs{}, err
	}
	if out.Activities, err = listActivities(ctx, v.q, fromDay, toDay); err != nil {
		return RollupInputs{}, err
	}
	if out.PAI, err = listPAI(ctx, v.q, fromDay, toDay); err != nil {
		return RollupInputs{}, err
	}
	return out, nil
}

func listActivities(ctx context.Context, q *db.Queries, fromDay, toDay time.Time) ([]ActivityRow, error) {
	rows, err := q.ListActivitiesRange(ctx, db.ListActivitiesRangeParams{
		FromDay: pgDate(fromDay),
		ToDay:   pgDate(toDay),
	})
	if err != nil {
		return nil, err
	}
	out := make([]ActivityRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, ActivityRow{
			StartedAt: r.StartedAt.Time,
			EndedAt:   r.EndedAt.Time,
			Kind:      r.Kind,
			SportType: int(r.SportType),
			SportName: r.SportName,
			Calories:  int2Val(r.Calories),
			AvgHR:     int2Val(r.AvgHr),
			MaxHR:     int2Val(r.MaxHr),
			MinHR:     int2Val(r.MinHr),
			Steps:     int4Val(r.Steps),
			HasDetail: r.HasDetail,
		})
	}
	return out, nil
}

func listPAI(ctx context.Context, q *db.Queries, fromDay, toDay time.Time) ([]PAIScore, error) {
	rows, err := q.ListPaiScoresRange(ctx, db.ListPaiScoresRangeParams{
		FromDay: pgDate(fromDay),
		ToDay:   pgDate(toDay),
	})
	if err != nil {
		return nil, err
	}
	out := make([]PAIScore, 0, len(rows))
	for _, r := range rows {
		out = append(out, PAIScore{
			DayKey:    dateOf(r.DayKey),
			SampledAt: r.SampledAt.Time,
			Score:     int(r.Score),
		})
	}
	return out, nil
}

func (v *TxView) AffectedSleepDays(ctx context.Context, from, to time.Time) ([]time.Time, error) {
	rows, err := v.q.AffectedSleepDays(ctx, db.AffectedSleepDaysParams{
		ToAt:   tsAt(to),
		FromAt: tsAt(from),
	})
	if err != nil {
		return nil, err
	}
	return daysOf(rows), nil
}

func uniqueDailyDays(table string, days []time.Time) ([]time.Time, error) {
	unique := uniqueDays(days)
	if len(unique) != len(days) {
		return nil, invalidRow(table, "duplicate day_key in one write batch")
	}
	return unique, nil
}

func (v *TxView) supportedSet(ctx context.Context, days []time.Time) (map[int64]struct{}, error) {
	supported, err := v.ListCanonicalDayKeys(ctx, days)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]struct{}, len(supported))
	for _, d := range supported {
		out[d.UnixNano()] = struct{}{}
	}
	return out, nil
}

func (v *TxView) WriteDailyAggregates(ctx context.Context, rows []DailyAggregate) error {
	if len(rows) == 0 {
		return nil
	}
	days := make([]time.Time, 0, len(rows))
	for _, r := range rows {
		days = append(days, dayOf(r.DayKey))
	}
	if _, err := uniqueDailyDays("daily_metrics", days); err != nil {
		return err
	}
	supported, err := v.supportedSet(ctx, days)
	if err != nil {
		return err
	}

	params := db.WriteDailyAggregatesParams{
		DayKey:             make([]pgtype.Date, 0, len(rows)),
		Steps:              make([]pgtype.Numeric, 0, len(rows)),
		AvgHr:              make([]pgtype.Numeric, 0, len(rows)),
		RestingHr:          make([]pgtype.Numeric, 0, len(rows)),
		HrvRmssdMs:         make([]pgtype.Numeric, 0, len(rows)),
		RespRate:           make([]pgtype.Numeric, 0, len(rows)),
		Spo2Avg:            make([]pgtype.Numeric, 0, len(rows)),
		Spo2Min:            make([]pgtype.Numeric, 0, len(rows)),
		Spo2SleepAvg:       make([]pgtype.Numeric, 0, len(rows)),
		Spo2SleepMin:       make([]pgtype.Numeric, 0, len(rows)),
		Spo2N:              make([]int32, 0, len(rows)),
		StressAvg:          make([]pgtype.Numeric, 0, len(rows)),
		SkinTempNightC:     make([]pgtype.Numeric, 0, len(rows)),
		SleepScore:         make([]pgtype.Numeric, 0, len(rows)),
		SleepMins:          make([]pgtype.Numeric, 0, len(rows)),
		SleepDeepMins:      make([]pgtype.Numeric, 0, len(rows)),
		SleepRemMins:       make([]pgtype.Numeric, 0, len(rows)),
		SleepLightMins:     make([]pgtype.Numeric, 0, len(rows)),
		SleepWakeMins:      make([]pgtype.Numeric, 0, len(rows)),
		TimeInBedMins:      make([]pgtype.Numeric, 0, len(rows)),
		SleepEfficiencyPct: make([]pgtype.Numeric, 0, len(rows)),
		NapCount:           make([]pgtype.Numeric, 0, len(rows)),
		WorkoutCount:       make([]pgtype.Numeric, 0, len(rows)),
		AutoActivityCount:  make([]pgtype.Numeric, 0, len(rows)),
		WorkoutCalories:    make([]pgtype.Numeric, 0, len(rows)),
		PaiScore:           make([]pgtype.Numeric, 0, len(rows)),
	}
	for _, r := range rows {
		day := dayOf(r.DayKey)
		if _, ok := supported[day.UnixNano()]; !ok {
			continue
		}
		steps, err := numericIntPtr(r.Steps)
		if err != nil {
			return invalidRow("daily_metrics", "steps not representable")
		}
		avgHR, err := numericPtr(r.AvgHR)
		if err != nil {
			return invalidRow("daily_metrics", "avg_hr not representable")
		}
		restingHR, err := numericPtr(r.RestingHR)
		if err != nil {
			return invalidRow("daily_metrics", "resting_hr not representable")
		}
		hrv, err := numericPtr(r.HRVRMSSDMS)
		if err != nil {
			return invalidRow("daily_metrics", "hrv_rmssd_ms not representable")
		}
		resp, err := numericPtr(r.RespRate)
		if err != nil {
			return invalidRow("daily_metrics", "resp_rate not representable")
		}
		spo2Avg, err := numericPtr(r.SpO2Avg)
		if err != nil {
			return invalidRow("daily_metrics", "spo2_avg not representable")
		}
		spo2Min, err := numericIntPtr(r.SpO2Min)
		if err != nil {
			return invalidRow("daily_metrics", "spo2_min not representable")
		}
		spo2SleepAvg, err := numericPtr(r.SpO2SleepAvg)
		if err != nil {
			return invalidRow("daily_metrics", "spo2_sleep_avg not representable")
		}
		spo2SleepMin, err := numericIntPtr(r.SpO2SleepMin)
		if err != nil {
			return invalidRow("daily_metrics", "spo2_sleep_min not representable")
		}
		stress, err := numericPtr(r.StressAvg)
		if err != nil {
			return invalidRow("daily_metrics", "stress_avg not representable")
		}
		skinTemp, err := numericPtr(r.SkinTempNightC)
		if err != nil {
			return invalidRow("daily_metrics", "skin_temp_night_c not representable")
		}
		sleepScore, err := numericIntPtr(r.SleepScore)
		if err != nil {
			return invalidRow("daily_metrics", "sleep_score not representable")
		}
		sleepMins, err := numericIntPtr(r.SleepMins)
		if err != nil {
			return invalidRow("daily_metrics", "sleep_mins not representable")
		}
		deepMins, err := numericIntPtr(r.SleepDeepMins)
		if err != nil {
			return invalidRow("daily_metrics", "sleep_deep_mins not representable")
		}
		remMins, err := numericIntPtr(r.SleepRemMins)
		if err != nil {
			return invalidRow("daily_metrics", "sleep_rem_mins not representable")
		}
		lightMins, err := numericIntPtr(r.SleepLightMins)
		if err != nil {
			return invalidRow("daily_metrics", "sleep_light_mins not representable")
		}
		wakeMins, err := numericIntPtr(r.SleepWakeMins)
		if err != nil {
			return invalidRow("daily_metrics", "sleep_wake_mins not representable")
		}
		timeInBed, err := numericIntPtr(r.TimeInBedMins)
		if err != nil {
			return invalidRow("daily_metrics", "time_in_bed_mins not representable")
		}
		efficiency, err := numericIntPtr(r.SleepEfficiencyPct)
		if err != nil {
			return invalidRow("daily_metrics", "sleep_efficiency_pct not representable")
		}
		ops := []struct {
			dst *[]pgtype.Numeric
			val pgtype.Numeric
		}{
			{&params.Steps, steps}, {&params.AvgHr, avgHR}, {&params.RestingHr, restingHR},
			{&params.HrvRmssdMs, hrv}, {&params.RespRate, resp}, {&params.Spo2Avg, spo2Avg},
			{&params.Spo2Min, spo2Min}, {&params.Spo2SleepAvg, spo2SleepAvg},
			{&params.Spo2SleepMin, spo2SleepMin}, {&params.StressAvg, stress},
			{&params.SkinTempNightC, skinTemp}, {&params.SleepScore, sleepScore},
			{&params.SleepMins, sleepMins}, {&params.SleepDeepMins, deepMins},
			{&params.SleepRemMins, remMins}, {&params.SleepLightMins, lightMins},
			{&params.SleepWakeMins, wakeMins}, {&params.TimeInBedMins, timeInBed},
			{&params.SleepEfficiencyPct, efficiency},
		}
		for _, op := range ops {
			*op.dst = append(*op.dst, op.val)
		}
		nap, err := numericFromInt(r.NapCount)
		if err != nil {
			return invalidRow("daily_metrics", "nap_count not representable")
		}
		workout, err := numericFromInt(r.WorkoutCount)
		if err != nil {
			return invalidRow("daily_metrics", "workout_count not representable")
		}
		auto, err := numericFromInt(r.AutoActivityCount)
		if err != nil {
			return invalidRow("daily_metrics", "auto_activity_count not representable")
		}
		calories, err := numericIntPtr(r.WorkoutCalories)
		if err != nil {
			return invalidRow("daily_metrics", "workout_calories not representable")
		}
		pai, err := numericIntPtr(r.PAIScore)
		if err != nil {
			return invalidRow("daily_metrics", "pai_score not representable")
		}
		params.DayKey = append(params.DayKey, pgDate(day))
		params.Spo2N = append(params.Spo2N, int32(r.SpO2N))
		params.NapCount = append(params.NapCount, nap)
		params.WorkoutCount = append(params.WorkoutCount, workout)
		params.AutoActivityCount = append(params.AutoActivityCount, auto)
		params.WorkoutCalories = append(params.WorkoutCalories, calories)
		params.PaiScore = append(params.PaiScore, pai)
	}
	if len(params.DayKey) == 0 {
		return nil
	}
	return v.q.WriteDailyAggregates(ctx, params)
}

func (v *TxView) WriteLoad(ctx context.Context, rows []LoadUpdate) error {
	if len(rows) == 0 {
		return nil
	}
	days := make([]time.Time, 0, len(rows))
	for _, r := range rows {
		days = append(days, dayOf(r.DayKey))
	}
	if _, err := uniqueDailyDays("daily_metrics", days); err != nil {
		return err
	}
	supported, err := v.supportedSet(ctx, days)
	if err != nil {
		return err
	}
	params := db.WriteDailyLoadParams{
		DayKey: make([]pgtype.Date, 0, len(rows)),
		Trimp:  make([]pgtype.Numeric, 0, len(rows)),
	}
	for _, r := range rows {
		day := dayOf(r.DayKey)
		if _, ok := supported[day.UnixNano()]; !ok {
			continue
		}
		trimp, err := numericPtr(r.TRIMP)
		if err != nil {
			return invalidRow("daily_metrics", "trimp not representable")
		}
		params.DayKey = append(params.DayKey, pgDate(day))
		params.Trimp = append(params.Trimp, trimp)
	}
	if len(params.DayKey) == 0 {
		return nil
	}
	return v.q.WriteDailyLoad(ctx, params)
}

func (v *TxView) WriteReadiness(ctx context.Context, rows []ReadinessUpdate) error {
	if len(rows) == 0 {
		return nil
	}
	days := make([]time.Time, 0, len(rows))
	for _, r := range rows {
		days = append(days, dayOf(r.DayKey))
	}
	if _, err := uniqueDailyDays("daily_metrics", days); err != nil {
		return err
	}
	supported, err := v.supportedSet(ctx, days)
	if err != nil {
		return err
	}
	params := db.WriteDailyReadinessParams{
		DayKey:       make([]pgtype.Date, 0, len(rows)),
		Readiness:    make([]pgtype.Numeric, 0, len(rows)),
		Components:   make([][]byte, 0, len(rows)),
		SkinTempDevC: make([]pgtype.Numeric, 0, len(rows)),
	}
	for _, r := range rows {
		day := dayOf(r.DayKey)
		if _, ok := supported[day.UnixNano()]; !ok {
			continue
		}
		score, err := numericIntPtr(r.Score)
		if err != nil {
			return invalidRow("daily_metrics", "readiness not representable")
		}
		dev, err := numericPtr(r.TempDeviationC)
		if err != nil {
			return invalidRow("daily_metrics", "skin_temp_dev_c not representable")
		}
		components := []byte(nil)
		if len(r.Components) > 0 {
			components = []byte(r.Components)
		}
		params.DayKey = append(params.DayKey, pgDate(day))
		params.Readiness = append(params.Readiness, score)
		params.Components = append(params.Components, components)
		params.SkinTempDevC = append(params.SkinTempDevC, dev)
	}
	if len(params.DayKey) == 0 {
		return nil
	}
	return v.q.WriteDailyReadiness(ctx, params)
}

func (v *TxView) ReadReadinessHistory(ctx context.Context, fromDay, toDay time.Time) ([]DailyAggregate, error) {
	rows, err := v.q.ListDailyMetrics(ctx, db.ListDailyMetricsParams{
		FromDay: pgDate(fromDay),
		ToDay:   pgDate(toDay),
	})
	if err != nil {
		return nil, err
	}
	days := make([]time.Time, 0, len(rows))
	for _, r := range rows {
		days = append(days, dateOf(r.DayKey))
	}
	supported, err := v.supportedSet(ctx, days)
	if err != nil {
		return nil, err
	}
	out := make([]DailyAggregate, 0, len(rows))
	for _, r := range rows {
		day := dateOf(r.DayKey)
		if _, ok := supported[day.UnixNano()]; !ok {
			continue
		}
		out = append(out, dailyAggregateFromRow(r))
	}
	return out, nil
}

func dailyAggregateFromRow(r db.DailyMetric) DailyAggregate {
	return DailyAggregate{
		DayKey:             dateOf(r.DayKey),
		Steps:              int4Val(r.Steps),
		AvgHR:              float4Val(r.AvgHr),
		RestingHR:          float4Val(r.RestingHr),
		HRVRMSSDMS:         float4Val(r.HrvRmssdMs),
		RespRate:           float4Val(r.RespRate),
		SpO2Avg:            float4Val(r.Spo2Avg),
		SpO2Min:            int2Val(r.Spo2Min),
		SpO2SleepAvg:       float4Val(r.Spo2SleepAvg),
		SpO2SleepMin:       int2Val(r.Spo2SleepMin),
		SpO2N:              int(r.Spo2N),
		StressAvg:          float4Val(r.StressAvg),
		SkinTempNightC:     float4Val(r.SkinTempNightC),
		SleepScore:         int2Val(r.SleepScore),
		SleepMins:          int2Val(r.SleepMins),
		SleepDeepMins:      int2Val(r.SleepDeepMins),
		SleepRemMins:       int2Val(r.SleepRemMins),
		SleepLightMins:     int2Val(r.SleepLightMins),
		SleepWakeMins:      int2Val(r.SleepWakeMins),
		TimeInBedMins:      int2Val(r.TimeInBedMins),
		SleepEfficiencyPct: int2Val(r.SleepEfficiencyPct),
		NapCount:           int(r.NapCount),
		WorkoutCount:       int(r.WorkoutCount),
		AutoActivityCount:  int(r.AutoActivityCount),
		WorkoutCalories:    int4Val(r.WorkoutCalories),
		PAIScore:           int2Val(r.PaiScore),
		TRIMP:              float4Val(r.Trimp),
	}
}

func (v *TxView) ReadActivityMinutes(ctx context.Context, from, to time.Time) ([]ActivityMinute, error) {
	steps, err := listSteps(ctx, v.q, from, to)
	if err != nil {
		return nil, err
	}
	hr, err := listHeartRate(ctx, v.q, from, to)
	if err != nil {
		return nil, err
	}
	hrByInstant := make(map[int64]int, len(hr))
	for _, s := range hr {
		if s.Source != "0x01" {
			continue
		}
		hrByInstant[s.At.UTC().UnixNano()] = s.BPM
	}
	out := make([]ActivityMinute, 0, len(steps))
	for _, s := range steps {
		minute := ActivityMinute{At: s.At, Steps: s.Steps}
		if bpm, ok := hrByInstant[s.At.UTC().UnixNano()]; ok {
			minute.HR = new(bpm)
		}
		out = append(out, minute)
	}
	return out, nil
}

func (v *TxView) ListWorkoutIntervals(ctx context.Context, from, to time.Time) ([]WorkoutInterval, error) {
	rows, err := v.q.ListWorkoutIntervalsRange(ctx, db.ListWorkoutIntervalsRangeParams{
		FromAt: tsAt(from),
		ToAt:   tsAt(to),
	})
	if err != nil {
		return nil, err
	}
	out := make([]WorkoutInterval, 0, len(rows))
	for _, r := range rows {
		out = append(out, WorkoutInterval{Start: r.StartedAt.Time, End: r.EndedAt.Time})
	}
	return out, nil
}

func (v *TxView) ListAutoActivitiesOverlapping(ctx context.Context, from, to time.Time) ([]ActivityRow, error) {
	rows, err := v.q.ListAutoActivities(ctx, db.ListAutoActivitiesParams{
		FromAt: tsAt(from),
		ToAt:   tsAt(to),
	})
	if err != nil {
		return nil, err
	}
	out := make([]ActivityRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, ActivityRow{
			StartedAt: r.StartedAt.Time,
			EndedAt:   r.EndedAt.Time,
			Kind:      "auto",
			SportType: int(r.SportType),
			SportName: r.SportName,
			Calories:  int2Val(r.Calories),
			AvgHR:     int2Val(r.AvgHr),
			MaxHR:     int2Val(r.MaxHr),
			MinHR:     int2Val(r.MinHr),
			Steps:     int4Val(r.Steps),
			HasDetail: r.HasDetail,
		})
	}
	return out, nil
}

func (v *TxView) ReplaceAutoActivities(ctx context.Context, from, to time.Time,
	rows []ActivityRow, sessionID string, observedAt time.Time) (Mutation, error) {
	b := newMutation()
	if err := validateSessionRef(sessionID, "activities"); err != nil {
		return Mutation{}, err
	}
	if err := validateObservedAt(observedAt, "activities"); err != nil {
		return Mutation{}, err
	}
	from = normInstant(from)
	to = normInstant(to)
	observedAt = normInstant(observedAt)
	if !to.After(from) {
		return Mutation{}, invalidRow("activities", "replacement interval must be non-empty")
	}

	normalized := make([]ActivityRow, 0, len(rows))
	for _, r := range rows {
		if err := validateActivityRow(r, "activities"); err != nil {
			return Mutation{}, err
		}
		r.Kind = "auto"
		r.StartedAt = normInstant(r.StartedAt)
		r.EndedAt = normInstant(r.EndedAt)
		normalized = append(normalized, r)
	}
	if err := validateWorkoutBatch(normalized, "activities"); err != nil {
		return Mutation{}, err
	}

	current, err := v.ListAutoActivitiesOverlapping(ctx, from, to)
	if err != nil {
		return Mutation{}, err
	}
	if err := v.q.DeleteAutoActivities(ctx, db.DeleteAutoActivitiesParams{
		FromAt: tsAt(from),
		ToAt:   tsAt(to),
	}); err != nil {
		return Mutation{}, err
	}

	if len(normalized) > 0 {
		params := db.InsertAutoActivitiesParams{
			StartedAt:  make([]pgtype.Timestamptz, 0, len(normalized)),
			EndedAt:    make([]pgtype.Timestamptz, 0, len(normalized)),
			SportType:  make([]int16, 0, len(normalized)),
			SportName:  make([]string, 0, len(normalized)),
			Calories:   make([]pgtype.Numeric, 0, len(normalized)),
			AvgHr:      make([]pgtype.Numeric, 0, len(normalized)),
			MaxHr:      make([]pgtype.Numeric, 0, len(normalized)),
			MinHr:      make([]pgtype.Numeric, 0, len(normalized)),
			Steps:      make([]pgtype.Numeric, 0, len(normalized)),
			HasDetail:  make([]bool, 0, len(normalized)),
			ObservedAt: make([]pgtype.Timestamptz, 0, len(normalized)),
			SessionID:  make([]string, 0, len(normalized)),
		}
		for _, r := range normalized {
			calories, err := numericIntPtr(r.Calories)
			if err != nil {
				return Mutation{}, invalidRow("activities", "calories not representable")
			}
			avgHR, err := numericIntPtr(r.AvgHR)
			if err != nil {
				return Mutation{}, invalidRow("activities", "avg_hr not representable")
			}
			maxHR, err := numericIntPtr(r.MaxHR)
			if err != nil {
				return Mutation{}, invalidRow("activities", "max_hr not representable")
			}
			minHR, err := numericIntPtr(r.MinHR)
			if err != nil {
				return Mutation{}, invalidRow("activities", "min_hr not representable")
			}
			steps, err := numericIntPtr(r.Steps)
			if err != nil {
				return Mutation{}, invalidRow("activities", "steps not representable")
			}
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
			params.SessionID = append(params.SessionID, sessionID)
		}
		if err := v.q.InsertAutoActivities(ctx, params); err != nil {
			return Mutation{}, err
		}
	}

	newByStart := make(map[int64]ActivityRow, len(normalized))
	for _, r := range normalized {
		newByStart[r.StartedAt.UnixNano()] = r
	}
	for _, old := range current {
		if next, ok := newByStart[old.StartedAt.UnixNano()]; ok && activityRowsEqual(old, next) {
			continue
		}
		b.addDay(dayOf(old.StartedAt))
		b.addSpan(old.StartedAt, old.EndedAt)
	}
	oldByStart := make(map[int64]ActivityRow, len(current))
	for _, old := range current {
		oldByStart[old.StartedAt.UnixNano()] = old
	}
	for _, next := range normalized {
		if old, ok := oldByStart[next.StartedAt.UnixNano()]; ok && activityRowsEqual(old, next) {
			continue
		}
		b.addDay(dayOf(next.StartedAt))
		b.addSpan(next.StartedAt, next.EndedAt)
	}
	return b.result(), nil
}

func (v *TxView) SessionTypeDataThrough(ctx context.Context, sessionID string,
	typeCodes []string) (map[string]*time.Time, error) {
	if sessionID == "" {
		return nil, invalidRow("sync_sessions", "session_id required")
	}
	out := make(map[string]*time.Time, len(typeCodes))
	session := pgtype.Text{String: sessionID, Valid: true}
	for _, code := range typeCodes {
		if !isMeasuredType(code) {
			out[code] = nil
			continue
		}
		at, err := maxCanonicalAt(ctx, v.q, code, session)
		if err != nil {
			return nil, err
		}
		out[code] = at
	}
	return out, nil
}
