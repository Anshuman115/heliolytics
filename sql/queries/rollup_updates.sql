-- name: EnsureDailyRows :exec
INSERT INTO daily_metrics (day_key)
SELECT v.day FROM unnest(sqlc.arg(days)::date[]) AS v(day)
ON CONFLICT (day_key) DO NOTHING;

-- name: ListCanonicalDayKeys :many
-- Support probes use sampled_at bounds so Timescale excludes irrelevant chunks.
WITH bounds AS (
  SELECT d.day,
         (d.day::timestamp AT TIME ZONE 'Asia/Kolkata') AS from_at,
         ((d.day + 1)::timestamp AT TIME ZONE 'Asia/Kolkata') AS to_at
  FROM unnest(sqlc.arg(days)::date[]) AS d(day)
)
SELECT b.day::date AS day
FROM bounds b
WHERE EXISTS (SELECT 1 FROM heart_rate_samples s
              WHERE s.sampled_at >= b.from_at AND s.sampled_at < b.to_at)
   OR EXISTS (SELECT 1 FROM step_samples s
              WHERE s.sampled_at >= b.from_at AND s.sampled_at < b.to_at)
   OR EXISTS (SELECT 1 FROM temperature_samples s
              WHERE s.sampled_at >= b.from_at AND s.sampled_at < b.to_at)
   OR EXISTS (SELECT 1 FROM hrv_samples s
              WHERE s.sampled_at >= b.from_at AND s.sampled_at < b.to_at)
   OR EXISTS (SELECT 1 FROM stress_samples s
              WHERE s.sampled_at >= b.from_at AND s.sampled_at < b.to_at)
   OR EXISTS (SELECT 1 FROM resp_samples s
              WHERE s.sampled_at >= b.from_at AND s.sampled_at < b.to_at)
   OR EXISTS (SELECT 1 FROM spo2_samples s
              WHERE s.sampled_at >= b.from_at AND s.sampled_at < b.to_at)
   OR EXISTS (SELECT 1 FROM sleep_sessions ss WHERE ss.day_key = b.day)
   OR EXISTS (SELECT 1 FROM activities a WHERE a.day_key = b.day)
   OR EXISTS (SELECT 1 FROM pai_scores p WHERE p.day_key = b.day)
ORDER BY b.day ASC;

-- name: ListDailyRowDays :many
SELECT day_key FROM daily_metrics
WHERE day_key = ANY(sqlc.arg(days)::date[])
ORDER BY day_key ASC;

-- name: DeleteUnsupportedDailyRows :many
DELETE FROM daily_metrics
WHERE day_key = ANY(sqlc.arg(days)::date[])
  AND NOT (day_key = ANY(sqlc.arg(supported)::date[]))
RETURNING day_key;

-- name: WriteDailyAggregates :exec
WITH v AS (
  SELECT * FROM ROWS FROM (
    unnest(sqlc.arg(day_key)::date[]),
    unnest(sqlc.arg(steps)::numeric[]),
    unnest(sqlc.arg(avg_hr)::numeric[]),
    unnest(sqlc.arg(resting_hr)::numeric[]),
    unnest(sqlc.arg(hrv_rmssd_ms)::numeric[]),
    unnest(sqlc.arg(resp_rate)::numeric[]),
    unnest(sqlc.arg(spo2_avg)::numeric[]),
    unnest(sqlc.arg(spo2_min)::numeric[]),
    unnest(sqlc.arg(spo2_sleep_avg)::numeric[]),
    unnest(sqlc.arg(spo2_sleep_min)::numeric[]),
    unnest(sqlc.arg(spo2_n)::int[]),
    unnest(sqlc.arg(stress_avg)::numeric[]),
    unnest(sqlc.arg(skin_temp_night_c)::numeric[]),
    unnest(sqlc.arg(sleep_score)::numeric[]),
    unnest(sqlc.arg(sleep_mins)::numeric[]),
    unnest(sqlc.arg(sleep_deep_mins)::numeric[]),
    unnest(sqlc.arg(sleep_rem_mins)::numeric[]),
    unnest(sqlc.arg(sleep_light_mins)::numeric[]),
    unnest(sqlc.arg(sleep_wake_mins)::numeric[]),
    unnest(sqlc.arg(time_in_bed_mins)::numeric[]),
    unnest(sqlc.arg(sleep_efficiency_pct)::numeric[]),
    unnest(sqlc.arg(nap_count)::numeric[]),
    unnest(sqlc.arg(workout_count)::numeric[]),
    unnest(sqlc.arg(auto_activity_count)::numeric[]),
    unnest(sqlc.arg(workout_calories)::numeric[]),
    unnest(sqlc.arg(pai_score)::numeric[])
  ) AS t(day_key, steps, avg_hr, resting_hr, hrv_rmssd_ms, resp_rate, spo2_avg,
         spo2_min, spo2_sleep_avg, spo2_sleep_min, spo2_n, stress_avg,
         skin_temp_night_c, sleep_score, sleep_mins, sleep_deep_mins,
         sleep_rem_mins, sleep_light_mins, sleep_wake_mins, time_in_bed_mins,
         sleep_efficiency_pct, nap_count, workout_count, auto_activity_count,
         workout_calories, pai_score)
), typed AS (
  SELECT day_key,
         steps::int AS steps,
         avg_hr::real AS avg_hr,
         resting_hr::real AS resting_hr,
         hrv_rmssd_ms::real AS hrv_rmssd_ms,
         resp_rate::real AS resp_rate,
         spo2_avg::real AS spo2_avg,
         spo2_min::smallint AS spo2_min,
         spo2_sleep_avg::real AS spo2_sleep_avg,
         spo2_sleep_min::smallint AS spo2_sleep_min,
         spo2_n,
         stress_avg::real AS stress_avg,
         skin_temp_night_c::real AS skin_temp_night_c,
         sleep_score::smallint AS sleep_score,
         sleep_mins::smallint AS sleep_mins,
         sleep_deep_mins::smallint AS sleep_deep_mins,
         sleep_rem_mins::smallint AS sleep_rem_mins,
         sleep_light_mins::smallint AS sleep_light_mins,
         sleep_wake_mins::smallint AS sleep_wake_mins,
         time_in_bed_mins::smallint AS time_in_bed_mins,
         sleep_efficiency_pct::smallint AS sleep_efficiency_pct,
         nap_count::smallint AS nap_count,
         workout_count::smallint AS workout_count,
         auto_activity_count::smallint AS auto_activity_count,
         workout_calories::int AS workout_calories,
         pai_score::smallint AS pai_score
  FROM v
)
UPDATE daily_metrics d SET
  steps = typed.steps,
  avg_hr = typed.avg_hr,
  resting_hr = typed.resting_hr,
  hrv_rmssd_ms = typed.hrv_rmssd_ms,
  resp_rate = typed.resp_rate,
  spo2_avg = typed.spo2_avg,
  spo2_min = typed.spo2_min,
  spo2_sleep_avg = typed.spo2_sleep_avg,
  spo2_sleep_min = typed.spo2_sleep_min,
  spo2_n = typed.spo2_n,
  stress_avg = typed.stress_avg,
  skin_temp_night_c = typed.skin_temp_night_c,
  sleep_score = typed.sleep_score,
  sleep_mins = typed.sleep_mins,
  sleep_deep_mins = typed.sleep_deep_mins,
  sleep_rem_mins = typed.sleep_rem_mins,
  sleep_light_mins = typed.sleep_light_mins,
  sleep_wake_mins = typed.sleep_wake_mins,
  time_in_bed_mins = typed.time_in_bed_mins,
  sleep_efficiency_pct = typed.sleep_efficiency_pct,
  nap_count = typed.nap_count,
  workout_count = typed.workout_count,
  auto_activity_count = typed.auto_activity_count,
  workout_calories = typed.workout_calories,
  pai_score = typed.pai_score,
  updated_at = clock_timestamp()
FROM typed
WHERE d.day_key = typed.day_key
  AND (d.steps, d.avg_hr, d.resting_hr, d.hrv_rmssd_ms, d.resp_rate, d.spo2_avg,
       d.spo2_min, d.spo2_sleep_avg, d.spo2_sleep_min, d.spo2_n, d.stress_avg,
       d.skin_temp_night_c, d.sleep_score, d.sleep_mins, d.sleep_deep_mins,
       d.sleep_rem_mins, d.sleep_light_mins, d.sleep_wake_mins, d.time_in_bed_mins,
       d.sleep_efficiency_pct, d.nap_count, d.workout_count, d.auto_activity_count,
       d.workout_calories, d.pai_score)
      IS DISTINCT FROM
      (typed.steps, typed.avg_hr, typed.resting_hr, typed.hrv_rmssd_ms,
       typed.resp_rate, typed.spo2_avg, typed.spo2_min, typed.spo2_sleep_avg,
       typed.spo2_sleep_min, typed.spo2_n, typed.stress_avg,
       typed.skin_temp_night_c, typed.sleep_score, typed.sleep_mins,
       typed.sleep_deep_mins, typed.sleep_rem_mins, typed.sleep_light_mins,
       typed.sleep_wake_mins, typed.time_in_bed_mins, typed.sleep_efficiency_pct,
       typed.nap_count, typed.workout_count, typed.auto_activity_count,
       typed.workout_calories, typed.pai_score);

-- name: WriteDailyLoad :exec
WITH v AS (
  SELECT * FROM ROWS FROM (
    unnest(sqlc.arg(day_key)::date[]),
    unnest(sqlc.arg(trimp)::numeric[])
  ) AS t(day_key, trimp)
)
UPDATE daily_metrics d SET trimp = v.trimp::real, updated_at = clock_timestamp()
FROM v
WHERE d.day_key = v.day_key
  AND d.trimp IS DISTINCT FROM v.trimp::real;

-- name: WriteDailyReadiness :exec
WITH v AS (
  SELECT * FROM ROWS FROM (
    unnest(sqlc.arg(day_key)::date[]),
    unnest(sqlc.arg(readiness)::numeric[]),
    unnest(sqlc.arg(components)::jsonb[]),
    unnest(sqlc.arg(skin_temp_dev_c)::numeric[])
  ) AS t(day_key, readiness, components, skin_temp_dev_c)
)
UPDATE daily_metrics d SET
  readiness = v.readiness::smallint,
  readiness_components = v.components,
  skin_temp_dev_c = v.skin_temp_dev_c::real,
  updated_at = clock_timestamp()
FROM v
WHERE d.day_key = v.day_key
  AND (d.readiness, d.readiness_components, d.skin_temp_dev_c)
      IS DISTINCT FROM (v.readiness::smallint, v.components, v.skin_temp_dev_c::real);
