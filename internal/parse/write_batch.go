package parse

import (
	"context"

	"github.com/heliolytics/api/internal/rollup"
	"github.com/heliolytics/api/internal/store"
	"github.com/jackc/pgx/v5"
)

// WriteBatch persists one sync and recomputes its daily metrics in one
// transaction. Any write or rollup failure rolls the entire ingest back.
func WriteBatch(ctx context.Context, st *store.Store, sid string, meta store.SessionMeta, blobs map[string][]byte, batch AggregatedBatch) error {
	return writeBatch(ctx, st, sid, meta, blobs, batch, runRollup)
}

type rollupRunner func(context.Context, rollup.Target, []string) error

func writeBatch(ctx context.Context, st *store.Store, sid string, meta store.SessionMeta, blobs map[string][]byte, batch AggregatedBatch, recompute rollupRunner) error {
	return st.WithTx(ctx, func(tx pgx.Tx) error {
		if err := st.UpsertSessionTx(ctx, tx, meta); err != nil {
			return err
		}
		for typeCode, raw := range blobs {
			if err := st.UpsertRawTx(ctx, tx, sid, typeCode, raw); err != nil {
				return err
			}
		}
		if err := st.UpsertSleepSessionsTx(ctx, tx, sid, toSleepRows(batch.Sleep)); err != nil {
			return err
		}
		if err := st.UpsertWorkoutsTx(ctx, tx, sid, toWorkoutRows(batch.Workouts)); err != nil {
			return err
		}
		if err := st.UpsertActivitySessionsTx(ctx, tx, sid, toActivitySessionRows(batch.ActivitySessions)); err != nil {
			return err
		}
		if err := st.UpsertTemperatureTx(ctx, tx, sid, toTempRows(batch.TempSeries)); err != nil {
			return err
		}
		if err := st.UpsertHeartRateSamplesTx(ctx, tx, sid, toHrRows(batch.HrSeries)); err != nil {
			return err
		}
		if err := st.UpsertStepSamplesTx(ctx, tx, sid, toStepRows(batch.StepSeries)); err != nil {
			return err
		}
		if err := st.UpsertHrvSamplesTx(ctx, tx, sid, toSampleValues(batch.HrvSeries)); err != nil {
			return err
		}
		if err := st.UpsertSpo2SamplesTx(ctx, tx, sid, toSpo2Values(batch.Spo2Series)); err != nil {
			return err
		}
		if err := st.UpsertStressSamplesTx(ctx, tx, sid, toSampleValues(batch.StressSeries)); err != nil {
			return err
		}
		if err := st.UpsertRespSamplesTx(ctx, tx, sid, toSampleValues(batch.RespRateSeries)); err != nil {
			return err
		}
		if err := st.UpsertRhrSamplesTx(ctx, tx, sid, toSampleValues(batch.RhrSeries)); err != nil {
			return err
		}
		if err := st.UpsertPaiScoresTx(ctx, tx, sid, batch.PaiScores); err != nil {
			return err
		}
		if err := st.UpsertReadinessScoresTx(ctx, tx, sid, batch.ReadinessScores); err != nil {
			return err
		}
		if err := st.EnsureDailyMetricsRowsTx(ctx, tx, batch.TouchedDays); err != nil {
			return err
		}
		return recompute(ctx, store.NewTxView(tx), batch.TouchedDays)
	})
}

func runRollup(ctx context.Context, st rollup.Target, days []string) error {
	if len(days) == 0 {
		return nil
	}
	if err := rollup.RecomputeDailySteps(ctx, st, days); err != nil {
		return err
	}
	if err := rollup.RecomputeDailyTemperature(ctx, st, days); err != nil {
		return err
	}
	if err := rollup.RecomputeDailyVitals(ctx, st, days); err != nil {
		return err
	}
	if err := rollup.RecomputeDailyRhr(ctx, st, days); err != nil {
		return err
	}
	if err := rollup.RecomputeDailySleep(ctx, st, days); err != nil {
		return err
	}
	if err := rollup.RecomputeDailyStress(ctx, st, days); err != nil {
		return err
	}
	if err := rollup.RecomputeDailyWorkoutCounts(ctx, st, days); err != nil {
		return err
	}
	if err := rollup.RecomputeDailyAvgHr(ctx, st, days); err != nil {
		return err
	}
	if err := rollup.RecomputeDailyPai(ctx, st, days); err != nil {
		return err
	}
	if err := rollup.RecomputeDailyReadiness(ctx, st, days); err != nil {
		return err
	}
	// computed_readiness reads other daily_metrics fields (hrv/rhr/sleep/resp),
	// so it must run last, after everything above has written its values.
	return st.RecomputeReadiness(ctx, days)
}
