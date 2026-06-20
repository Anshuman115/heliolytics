package store

import (
	"fmt"
	"time"
)

func validateSessionMeta(m SessionMeta) error {
	if m.ID == "" {
		return fmt.Errorf("sync_sessions.session_id required")
	}
	if m.StartedAt.IsZero() {
		return fmt.Errorf("sync_sessions.started_at required")
	}
	return nil
}

func validateRawBlob(sessionID, typeCode string, raw []byte) error {
	if sessionID == "" {
		return fmt.Errorf("raw_type_blobs.session_id required")
	}
	if typeCode == "" {
		return fmt.Errorf("raw_type_blobs.type_code required")
	}
	if len(raw) == 0 {
		return fmt.Errorf("raw_type_blobs.payload required")
	}
	return nil
}

func validateTempPoint(p TempPoint) error {
	if err := dayKeyRequired(p.DayKey, "temperature_samples"); err != nil {
		return err
	}
	if p.SampledAt.IsZero() {
		return fmt.Errorf("temperature_samples.sampled_at required")
	}
	if _, err := numericFromFloat(p.Celsius); err != nil {
		return fmt.Errorf("temperature_samples.celsius: %w", err)
	}
	return nil
}

func validateHealthSample(p HealthSample) error {
	if err := textRequired(p.Metric, "health_samples.metric"); err != nil {
		return err
	}
	if err := dayKeyRequired(p.DayKey, "health_samples"); err != nil {
		return err
	}
	if p.SampledAt.IsZero() {
		return fmt.Errorf("health_samples.sampled_at required")
	}
	if _, err := numericFromFloat(p.Value); err != nil {
		return fmt.Errorf("health_samples.value: %w", err)
	}
	return nil
}

func validateHeartRateSample(p HeartRateSample) error {
	if err := dayKeyRequired(p.DayKey, "heart_rate_samples"); err != nil {
		return err
	}
	if p.SampledAt.IsZero() {
		return fmt.Errorf("heart_rate_samples.sampled_at required")
	}
	if p.Bpm < 30 || p.Bpm > 220 {
		return fmt.Errorf("heart_rate_samples.bpm out of range")
	}
	return nil
}

func validateSleepRow(r SleepRow) error {
	return validateTimedRow("sleep_sessions", r.DayKey, r.StartedAt)
}

func validateWorkoutRow(r WorkoutRow) error {
	return validateTimedRow("workouts", r.DayKey, r.StartedAt)
}

func validateActivityRow(r ActivitySessionRow) error {
	return validateTimedRow("activity_sessions", r.DayKey, r.StartedAt)
}

func validateDayMetric(d DayMetric) error {
	if err := dayKeyRequired(d.DayKey, "daily_metrics"); err != nil {
		return err
	}
	if d.TempAvgC != nil {
		if _, err := numericFromFloat(*d.TempAvgC); err != nil {
			return fmt.Errorf("daily_metrics.temp_avg_c: %w", err)
		}
	}
	return nil
}

func validateTimedRow(table, dayKey string, startedAt time.Time) error {
	if err := dayKeyRequired(dayKey, table); err != nil {
		return err
	}
	if startedAt.IsZero() {
		return fmt.Errorf("%s.started_at required", table)
	}
	return nil
}
