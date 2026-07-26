ALTER TABLE workouts
  ADD COLUMN has_summary BOOLEAN NOT NULL DEFAULT false,
  ADD COLUMN has_detail BOOLEAN NOT NULL DEFAULT false;

WITH session_sources AS (
  SELECT
    session_id,
    BOOL_OR(type_code = '0x05') AS has_summary,
    BOOL_OR(type_code = '0x06') AS has_detail
  FROM raw_type_blobs
  WHERE type_code IN ('0x05', '0x06')
  GROUP BY session_id
)
UPDATE workouts w
SET
  has_summary = s.has_summary,
  has_detail = s.has_detail,
  updated_at = NOW()
FROM session_sources s
WHERE w.source_session_id = s.session_id;
