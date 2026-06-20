package parse

import (
	"context"

	"github.com/heliolytics/api/internal/store"
)

// WriteBatch persists an aggregated ingest batch for one sync session.
func WriteBatch(ctx context.Context, st *store.Store, syncSessionID string, batch AggregatedBatch) error {
	if err := st.UpsertSleepSessions(ctx, syncSessionID, toSleepRows(batch.Sleep)); err != nil {
		return err
	}
	if err := st.UpsertWorkouts(ctx, syncSessionID, toWorkoutRows(batch.Workouts)); err != nil {
		return err
	}
	if err := st.UpsertActivitySessions(ctx, syncSessionID, toActivitySessionRows(batch.ActivitySessions)); err != nil {
		return err
	}
	if err := st.UpsertTemperature(ctx, syncSessionID, toTempRows(batch.TempSeries)); err != nil {
		return err
	}
	if err := st.UpsertHeartRateSamples(ctx, syncSessionID, toHrRows(batch.HrSeries)); err != nil {
		return err
	}
	if err := st.UpsertHealthSamples(ctx, syncSessionID, toHealthRows(batch.HealthSeries)); err != nil {
		return err
	}
	return st.UpsertDayMetrics(ctx, syncSessionID, batch.Days)
}
