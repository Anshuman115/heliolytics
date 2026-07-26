ALTER TABLE spo2_samples
  ADD COLUMN source_type TEXT
  CHECK (source_type IN ('0x25', '0x26'));

WITH session_sources AS (
  SELECT
    session_id,
    BOOL_OR(type_code = '0x25') AS has_spot,
    BOOL_OR(type_code = '0x26') AS has_sleep
  FROM raw_type_blobs
  WHERE type_code IN ('0x25', '0x26')
  GROUP BY session_id
), resolved AS (
  SELECT
    session_id,
    CASE
      WHEN has_spot AND NOT has_sleep THEN '0x25'
      WHEN has_sleep AND NOT has_spot THEN '0x26'
    END AS source_type
  FROM session_sources
)
UPDATE spo2_samples s
SET source_type = r.source_type
FROM resolved r
WHERE s.source_session_id = r.session_id AND r.source_type IS NOT NULL;

CREATE INDEX idx_spo2_source_time
  ON spo2_samples (source_type, sampled_at DESC);
