package rollup

import (
	"context"
	"log"
)

// RecomputeDailySleep fills sleep_score/mins/deep/rem/light from the day's
// best (highest score) non-nap sleep_sessions row, and nap_count from a
// simple count of that day's nap rows. A day can have multiple sleep
// sessions (e.g. woke and fell back asleep); the highest-scored one is what
// "today's sleep" means to the app.
//
// nap_count is independent of whether a main (non-nap) session exists for
// the day: it always reflects the true nap count (0 if none), even on a
// nap-only day. The main-sleep fields (sleep_score/sleep_mins/deep/rem/light)
// only get overwritten when a non-nap session exists that day; otherwise
// their prior values are left untouched (COALESCE onto the existing column).
// The CTE + LEFT JOIN back to daily_metrics ensures the UPDATE always
// matches the day's row on day_key alone — it never depends on the "best"
// subquery returning a row, which is what let nap_count silently no-op on
// nap-only days under the old UPDATE...FROM join.
const recomputeDailySleepSQL = `
	WITH best AS (
		SELECT score, total_mins, deep_mins, rem_mins, light_mins
		FROM sleep_sessions
		WHERE day_key = $1::date AND is_nap = false
		ORDER BY score DESC LIMIT 1
	), naps AS (
		SELECT COUNT(*) AS n FROM sleep_sessions
		WHERE day_key = $1::date AND is_nap = true
	)
	UPDATE daily_metrics d
	SET sleep_score = COALESCE(best.score, d.sleep_score),
	    sleep_mins = COALESCE(best.total_mins, d.sleep_mins),
	    sleep_deep_mins = COALESCE(best.deep_mins, d.sleep_deep_mins),
	    sleep_rem_mins = COALESCE(best.rem_mins, d.sleep_rem_mins),
	    sleep_light_mins = COALESCE(best.light_mins, d.sleep_light_mins),
	    nap_count = naps.n,
	    updated_at = NOW()
	FROM naps
	LEFT JOIN best ON true
	WHERE d.day_key = $1::date`

func RecomputeDailySleep(ctx context.Context, st Target, days []string) error {
	log.Printf("rollup sleep days=%v", days)
	return st.ExecDayKeys(ctx, recomputeDailySleepSQL, days)
}
