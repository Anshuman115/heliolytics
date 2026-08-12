package rollup

import (
	"context"
	"log"
)

// RecomputeDailyPai and RecomputeDailyReadiness copy the day's value from
// raw_pai_scores / raw_readiness into daily_metrics. Unlike the averaged
// metrics, PAI and device readiness are single device-reported numbers per
// day with no "many samples to combine" — but they still get written to
// daily_metrics only via a DB read of their own table, same rule as
// everything else, so a partial sync can never leave daily_metrics holding a
// stale in-memory value.
const recomputeDailyPaiSQL = `
	UPDATE daily_metrics
	SET pai_score = (SELECT score FROM raw_pai_scores WHERE day_key = $1::date),
	    updated_at = NOW()
	WHERE day_key = $1::date`

func RecomputeDailyPai(ctx context.Context, st Target, days []string) error {
	log.Printf("rollup pai days=%v", days)
	return st.ExecDayKeys(ctx, recomputeDailyPaiSQL, days)
}

const recomputeDailyReadinessSQL = `
	UPDATE daily_metrics
	SET readiness = (SELECT score FROM raw_readiness WHERE day_key = $1::date),
	    updated_at = NOW()
	WHERE day_key = $1::date`

func RecomputeDailyReadiness(ctx context.Context, st Target, days []string) error {
	log.Printf("rollup readiness days=%v", days)
	return st.ExecDayKeys(ctx, recomputeDailyReadinessSQL, days)
}
