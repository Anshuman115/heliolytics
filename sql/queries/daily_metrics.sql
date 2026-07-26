-- name: ListDays :many
-- readiness prefers the device's 0x39 score, falling back to our computed one.
SELECT day_key, steps, pai_score,
       COALESCE(readiness, computed_readiness) AS readiness, spo2_avg, hrv_rmssd,
       resting_hr, resp_rate_avg, stress_avg, sleep_score, sleep_mins,
       sleep_deep_mins, sleep_rem_mins, sleep_light_mins, temp_avg_c,
       nap_count, workout_count, activity_session_count, calories_total, avg_hr,
       source_session_id, updated_at
FROM daily_metrics
WHERE day_key >= $1 AND day_key <= $2
ORDER BY day_key DESC;
