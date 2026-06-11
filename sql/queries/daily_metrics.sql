-- name: UpsertDayMetric :exec
INSERT INTO daily_metrics (
  day_key, steps, pai_score, readiness, spo2_avg, hrv_rmssd,
  resting_hr, max_hr, resp_rate_avg, stress_avg, sleep_score, sleep_mins,
  sleep_deep_mins, sleep_rem_mins, sleep_light_mins, temp_avg_c,
  nap_count, workout_count, activity_session_count
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19
)
ON CONFLICT (day_key) DO UPDATE SET
  steps = EXCLUDED.steps,
  pai_score = COALESCE(EXCLUDED.pai_score, daily_metrics.pai_score),
  readiness = COALESCE(EXCLUDED.readiness, daily_metrics.readiness),
  spo2_avg = COALESCE(EXCLUDED.spo2_avg, daily_metrics.spo2_avg),
  hrv_rmssd = COALESCE(EXCLUDED.hrv_rmssd, daily_metrics.hrv_rmssd),
  resting_hr = COALESCE(EXCLUDED.resting_hr, daily_metrics.resting_hr),
  max_hr = COALESCE(EXCLUDED.max_hr, daily_metrics.max_hr),
  resp_rate_avg = COALESCE(EXCLUDED.resp_rate_avg, daily_metrics.resp_rate_avg),
  stress_avg = COALESCE(EXCLUDED.stress_avg, daily_metrics.stress_avg),
  sleep_score = COALESCE(EXCLUDED.sleep_score, daily_metrics.sleep_score),
  sleep_mins = COALESCE(EXCLUDED.sleep_mins, daily_metrics.sleep_mins),
  sleep_deep_mins = COALESCE(EXCLUDED.sleep_deep_mins, daily_metrics.sleep_deep_mins),
  sleep_rem_mins = COALESCE(EXCLUDED.sleep_rem_mins, daily_metrics.sleep_rem_mins),
  sleep_light_mins = COALESCE(EXCLUDED.sleep_light_mins, daily_metrics.sleep_light_mins),
  temp_avg_c = COALESCE(EXCLUDED.temp_avg_c, daily_metrics.temp_avg_c),
  nap_count = GREATEST(daily_metrics.nap_count, EXCLUDED.nap_count),
  workout_count = GREATEST(daily_metrics.workout_count, EXCLUDED.workout_count),
  activity_session_count = GREATEST(daily_metrics.activity_session_count, EXCLUDED.activity_session_count),
  updated_at = NOW();

-- name: ListDays :many
SELECT day_key, steps, pai_score, readiness, spo2_avg, hrv_rmssd,
       resting_hr, max_hr, resp_rate_avg, stress_avg, sleep_score, sleep_mins,
       sleep_deep_mins, sleep_rem_mins, sleep_light_mins, temp_avg_c,
       nap_count, workout_count, activity_session_count, updated_at
FROM daily_metrics
WHERE day_key >= $1 AND day_key <= $2
ORDER BY day_key DESC;
