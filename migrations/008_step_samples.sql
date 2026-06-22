-- Per-minute step storage so overlapping syncs cannot double-count daily steps.
-- Daily totals are recomputed as SUM(steps) per IST day after ingest.
CREATE TABLE IF NOT EXISTS step_samples (
  sampled_at        TIMESTAMPTZ NOT NULL,
  day_key           DATE NOT NULL,
  steps             SMALLINT NOT NULL CHECK (steps >= 0),
  source_session_id TEXT NOT NULL,
  PRIMARY KEY (sampled_at)
);

CREATE INDEX IF NOT EXISTS idx_step_day ON step_samples (day_key, sampled_at);

SELECT create_hypertable('step_samples', 'sampled_at', if_not_exists => TRUE);
