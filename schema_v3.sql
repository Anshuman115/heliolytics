-- Heliolytics PostgreSQL + TimescaleDB schema v3
-- Fresh DB only: deploy/reset-db.sh (point init at this file on v3 branch)
-- v2 frozen: see schema.sql + SCHEMA_DESIGN.md — do not edit for v3 work

CREATE EXTENSION IF NOT EXISTS timescaledb;

-- ========== INGEST (audit) ==========

CREATE TABLE sync_sessions (
  session_id    TEXT PRIMARY KEY,
  device_mac    TEXT,
  started_at    TIMESTAMPTZ NOT NULL,
  ended_at      TIMESTAMPTZ,
  battery_pct   INT,
  catalog_json  JSONB,
  ingested_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE raw_type_blobs (
  session_id  TEXT NOT NULL REFERENCES sync_sessions(session_id) ON DELETE CASCADE,
  type_code   TEXT NOT NULL,
  byte_len    INT NOT NULL,
  payload     BYTEA NOT NULL,
  PRIMARY KEY (session_id, type_code)
);

-- ========== CANONICAL (natural keys, DATE day_key) ==========

CREATE TABLE daily_metrics (
  day_key                  DATE PRIMARY KEY,
  steps                    INT NOT NULL DEFAULT 0,
  pai_score                INT,
  readiness                INT,
  spo2_avg                 INT,
  hrv_rmssd                INT,
  resting_hr               INT,
  max_hr                   INT,
  resp_rate_avg            INT,
  stress_avg               INT,
  sleep_score              INT,
  sleep_mins               INT,
  sleep_deep_mins          INT,
  sleep_rem_mins           INT,
  sleep_light_mins         INT,
  temp_avg_c               NUMERIC(4, 1),
  nap_count                INT NOT NULL DEFAULT 0,
  workout_count            INT NOT NULL DEFAULT 0,
  activity_session_count   INT NOT NULL DEFAULT 0,
  source_session_id        TEXT,
  updated_at               TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_daily_metrics_updated ON daily_metrics(updated_at DESC);

CREATE TABLE sleep_sessions (
  id                  BIGSERIAL PRIMARY KEY,
  started_at          TIMESTAMPTZ NOT NULL,
  day_key             DATE NOT NULL,
  score               INT NOT NULL DEFAULT 0,
  total_mins          INT NOT NULL DEFAULT 0,
  deep_mins           INT NOT NULL DEFAULT 0,
  rem_mins            INT NOT NULL DEFAULT 0,
  light_mins          INT NOT NULL DEFAULT 0,
  wake_mins           INT NOT NULL DEFAULT 0,
  is_nap              BOOLEAN NOT NULL DEFAULT false,
  stages_json         JSONB,
  source_session_id   TEXT NOT NULL,
  updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (started_at, day_key)
);

CREATE INDEX idx_sleep_day ON sleep_sessions (day_key, started_at DESC);

CREATE TABLE workouts (
  id                  BIGSERIAL PRIMARY KEY,
  started_at          TIMESTAMPTZ NOT NULL,
  day_key             DATE NOT NULL,
  sport_type          INT NOT NULL DEFAULT 0,
  sport_name          TEXT,
  duration_sec        INT NOT NULL DEFAULT 0,
  calories            INT,
  avg_hr              INT,
  max_hr              INT,
  source_session_id   TEXT NOT NULL,
  updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (day_key, started_at)
);

CREATE INDEX idx_workouts_day ON workouts (day_key, started_at DESC);

CREATE TABLE activity_sessions (
  id                  BIGSERIAL PRIMARY KEY,
  started_at          TIMESTAMPTZ NOT NULL,
  day_key             DATE NOT NULL,
  sport_type          INT NOT NULL DEFAULT 0,
  sport_name          TEXT,
  duration_sec        INT NOT NULL DEFAULT 0,
  calories            INT,
  avg_hr              INT,
  max_hr              INT,
  source_session_id   TEXT NOT NULL,
  updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (day_key, started_at)
);

CREATE INDEX idx_activity_day ON activity_sessions (day_key, started_at DESC);

-- Continuous HR (~1/sec) from BLE 0x46
CREATE TABLE heart_rate_samples (
  sampled_at        TIMESTAMPTZ NOT NULL,
  day_key           DATE NOT NULL,
  bpm               SMALLINT NOT NULL CHECK (bpm BETWEEN 30 AND 220),
  source_session_id TEXT NOT NULL,
  PRIMARY KEY (sampled_at)
);

CREATE INDEX idx_hr_day ON heart_rate_samples (day_key, sampled_at);

-- Skin temperature (~1/min) from BLE 0x2E
CREATE TABLE temperature_samples (
  sampled_at        TIMESTAMPTZ NOT NULL,
  day_key           DATE NOT NULL,
  celsius           NUMERIC(4, 1) NOT NULL,
  source_session_id TEXT NOT NULL,
  PRIMARY KEY (sampled_at)
);

CREATE INDEX idx_temp_day ON temperature_samples (day_key, sampled_at);

-- Lower-rate vitals (not HR)
CREATE TABLE health_samples (
  metric            TEXT NOT NULL,
  sampled_at        TIMESTAMPTZ NOT NULL,
  day_key           DATE NOT NULL,
  value             NUMERIC(6, 2) NOT NULL,
  source_session_id TEXT NOT NULL,
  PRIMARY KEY (metric, sampled_at)
);

CREATE INDEX idx_health_day_metric
  ON health_samples (day_key, metric, sampled_at);

SELECT create_hypertable('heart_rate_samples', 'sampled_at', if_not_exists => TRUE);
SELECT create_hypertable('temperature_samples', 'sampled_at', if_not_exists => TRUE);
SELECT create_hypertable('health_samples', 'sampled_at', if_not_exists => TRUE);
