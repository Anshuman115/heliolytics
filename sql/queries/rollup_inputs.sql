-- name: AffectedSleepDays :many
SELECT day_key FROM sleep_sessions
WHERE started_at < sqlc.arg(to_at) AND ended_at > sqlc.arg(from_at)
ORDER BY day_key ASC;

-- name: ListSeriesCompact :many
WITH unioned AS (
  SELECT 'hrv'::text AS metric, h.day_key, h.sampled_at, h.rmssd_ms::float8 AS value
  FROM hrv_samples h
  WHERE h.sampled_at >= sqlc.arg(from_at) AND h.sampled_at < sqlc.arg(to_at)
  UNION ALL
  SELECT 'stress'::text, s.day_key, s.sampled_at, s.stress::float8
  FROM stress_samples s
  WHERE s.sampled_at >= sqlc.arg(from_at) AND s.sampled_at < sqlc.arg(to_at)
  UNION ALL
  SELECT 'resp'::text, r.day_key, r.sampled_at, r.br_min::float8
  FROM resp_samples r
  WHERE r.sampled_at >= sqlc.arg(from_at) AND r.sampled_at < sqlc.arg(to_at)
  UNION ALL
  SELECT 'spo2'::text, sp.day_key, sp.sampled_at, sp.pct::float8
  FROM spo2_samples sp
  WHERE sp.sampled_at >= sqlc.arg(from_at) AND sp.sampled_at < sqlc.arg(to_at)
), computed AS (
  SELECT metric, day_key, sampled_at, value,
         MIN(sampled_at) OVER (PARTITION BY metric, day_key) AS day_start
  FROM unioned
)
SELECT metric, day_key, day_start::timestamptz AS start_time,
       array_agg(floor(EXTRACT(EPOCH FROM (sampled_at - day_start)))::int ORDER BY sampled_at)::int[] AS offsets,
       array_agg(value ORDER BY sampled_at)::float8[] AS values
FROM computed
GROUP BY metric, day_key, day_start
ORDER BY day_start ASC, metric COLLATE "C" ASC;
