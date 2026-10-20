-- name: ListDailyMetrics :many
SELECT day_key, steps, avg_hr, resting_hr, hrv_rmssd_ms, resp_rate, spo2_avg,
       spo2_min, spo2_sleep_avg, spo2_sleep_min, spo2_n, stress_avg,
       skin_temp_night_c, skin_temp_dev_c, sleep_score, sleep_mins,
       sleep_deep_mins, sleep_rem_mins, sleep_light_mins, sleep_wake_mins,
       time_in_bed_mins, sleep_efficiency_pct, nap_count, workout_count,
       auto_activity_count, workout_calories, trimp, pai_score, readiness,
       readiness_components, updated_at
FROM daily_metrics
WHERE day_key >= sqlc.arg(from_day) AND day_key <= sqlc.arg(to_day)
ORDER BY day_key ASC;
