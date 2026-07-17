-- migrations/010_split_health_samples_and_profiles.sql
-- Kill the tall health_samples table (metric column, mixed signals) in favor
-- of one hypertable per metric — matches the pattern step_samples already
-- uses, and lets each rollup function query its own table without a WHERE
-- metric = '...' filter.
--
-- Also adds profiles (single-row user info) and raw_pai_scores / raw_readiness
-- (device-reported single-value-per-day scores, parsed once from raw blobs,
-- given their own table so rollup can read them back from the DB like every
-- other field instead of writing straight from the parsed payload).

CREATE TABLE hrv_samples (
  sampled_at        TIMESTAMPTZ NOT NULL,
  day_key           DATE NOT NULL,
  value             NUMERIC(6, 2) NOT NULL,
  source_session_id TEXT NOT NULL,
  PRIMARY KEY (sampled_at)
);
CREATE INDEX idx_hrv_day ON hrv_samples (day_key, sampled_at);
SELECT create_hypertable('hrv_samples', 'sampled_at', if_not_exists => TRUE);

CREATE TABLE spo2_samples (
  sampled_at        TIMESTAMPTZ NOT NULL,
  day_key           DATE NOT NULL,
  value             NUMERIC(6, 2) NOT NULL,
  source_session_id TEXT NOT NULL,
  PRIMARY KEY (sampled_at)
);
CREATE INDEX idx_spo2_day ON spo2_samples (day_key, sampled_at);
SELECT create_hypertable('spo2_samples', 'sampled_at', if_not_exists => TRUE);

CREATE TABLE stress_samples (
  sampled_at        TIMESTAMPTZ NOT NULL,
  day_key           DATE NOT NULL,
  value             NUMERIC(6, 2) NOT NULL,
  source_session_id TEXT NOT NULL,
  PRIMARY KEY (sampled_at)
);
CREATE INDEX idx_stress_day ON stress_samples (day_key, sampled_at);
SELECT create_hypertable('stress_samples', 'sampled_at', if_not_exists => TRUE);

CREATE TABLE resp_samples (
  sampled_at        TIMESTAMPTZ NOT NULL,
  day_key           DATE NOT NULL,
  value             NUMERIC(6, 2) NOT NULL,
  source_session_id TEXT NOT NULL,
  PRIMARY KEY (sampled_at)
);
CREATE INDEX idx_resp_day ON resp_samples (day_key, sampled_at);
SELECT create_hypertable('resp_samples', 'sampled_at', if_not_exists => TRUE);

CREATE TABLE rhr_samples (
  sampled_at        TIMESTAMPTZ NOT NULL,
  day_key           DATE NOT NULL,
  value             NUMERIC(6, 2) NOT NULL,
  source_session_id TEXT NOT NULL,
  PRIMARY KEY (sampled_at)
);
CREATE INDEX idx_rhr_day ON rhr_samples (day_key, sampled_at);
SELECT create_hypertable('rhr_samples', 'sampled_at', if_not_exists => TRUE);

CREATE TABLE profiles (
  id          BIGSERIAL PRIMARY KEY,
  name        TEXT,
  age         INT,
  height_cm   NUMERIC(5, 1),
  weight_kg   NUMERIC(5, 1),
  created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE raw_pai_scores (
  day_key           DATE PRIMARY KEY,
  score             INT NOT NULL,
  source_session_id TEXT NOT NULL,
  updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE raw_readiness (
  day_key           DATE PRIMARY KEY,
  score             INT NOT NULL,
  source_session_id TEXT NOT NULL,
  updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

DROP TABLE IF EXISTS health_samples;

-- Two fields for the daily-health-scores tier-2 endpoint: a daily calorie
-- total (summed from workouts, not a resting/BMR estimate) and a daily mean
-- resting-adjacent HR (averaged from the continuous heart_rate_samples
-- table). Both filled by rollup, not by ingest, same rule as every other
-- daily_metrics column.
ALTER TABLE daily_metrics ADD COLUMN IF NOT EXISTS calories_total INT;
ALTER TABLE daily_metrics ADD COLUMN IF NOT EXISTS avg_hr INT;
