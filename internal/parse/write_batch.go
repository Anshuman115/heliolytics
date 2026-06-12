package parse

import (
	"context"

	"github.com/heliolytics/api/internal/store"
)

// WriteBatch persists an aggregated ingest batch for one sync session.
func WriteBatch(ctx context.Context, st *store.Store, syncSessionID string, batch AggregatedBatch) error {
	if err := st.ReplaceSleepSessions(ctx, syncSessionID, toSleepRows(syncSessionID, batch.Sleep)); err != nil {
		return err
	}
	if err := st.ReplaceWorkouts(ctx, syncSessionID, toWorkoutRows(syncSessionID, batch.Workouts)); err != nil {
		return err
	}
	if err := st.ReplaceActivitySessions(ctx, syncSessionID, toActivitySessionRows(syncSessionID, batch.ActivitySessions)); err != nil {
		return err
	}
	if err := st.ReplaceTemperature(ctx, syncSessionID, toTempRows(batch.TempSeries)); err != nil {
		return err
	}
	if err := st.ReplaceHealthSamples(ctx, syncSessionID, toHealthRows(batch.HealthSeries)); err != nil {
		return err
	}
	return st.UpsertDayMetrics(ctx, batch.Days)
}
