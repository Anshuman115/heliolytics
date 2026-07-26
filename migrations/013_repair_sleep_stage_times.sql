WITH stage_bounds AS (
  SELECT s.id, MIN((stage->>'start')::timestamptz) AS first_stage
  FROM sleep_sessions s
  CROSS JOIN LATERAL jsonb_array_elements(s.stages_json) stage
  WHERE NOT s.is_nap
  GROUP BY s.id
)
UPDATE sleep_sessions s
SET started_at = b.first_stage, updated_at = NOW()
FROM stage_bounds b
WHERE s.id = b.id AND s.started_at IS DISTINCT FROM b.first_stage;

WITH nap_totals AS (
  SELECT
    s.id,
    COALESCE(SUM(minutes) FILTER (WHERE stage_type = 4), 0)::int AS light_mins,
    COALESCE(SUM(minutes) FILTER (WHERE stage_type = 5), 0)::int AS deep_mins,
    COALESCE(SUM(minutes) FILTER (WHERE stage_type = 8), 0)::int AS rem_mins,
    COALESCE(SUM(minutes) FILTER (WHERE stage_type = 7), 0)::int AS wake_mins
  FROM sleep_sessions s
  CROSS JOIN LATERAL (
    SELECT
      (stage->>'type')::int AS stage_type,
      EXTRACT(EPOCH FROM (
        (stage->>'end')::timestamptz - (stage->>'start')::timestamptz
      )) / 60 + 1 AS minutes
    FROM jsonb_array_elements(s.stages_json) stage
  ) stages
  WHERE s.is_nap
  GROUP BY s.id
)
UPDATE sleep_sessions s
SET
  light_mins = n.light_mins,
  deep_mins = n.deep_mins,
  rem_mins = n.rem_mins,
  wake_mins = n.wake_mins,
  total_mins = n.light_mins + n.deep_mins + n.rem_mins,
  updated_at = NOW()
FROM nap_totals n
WHERE s.id = n.id;
