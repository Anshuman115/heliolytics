-- Heliolytics PostgreSQL + TimescaleDB schema 
-- Apply on fresh DB via deploy/docker-compose init, or: psql -f schema.sql

CREATE EXTENSION IF NOT EXISTS timescaledb;

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

CREATE TABLE daily_metrics (
  day_key           TEXT PRIMARY KEY,
  steps             INT NOT NULL DEFAULT 0,
  pai_score         INT,
  readiness         INT,
  spo2_avg          INT,
  hrv_rmssd         INT,
  resting_hr        INT,
  max_hr            INT,
  resp_rate_avg     INT,
  stress_avg        INT,
  sleep_score       INT,
  sleep_mins        INT,
  sleep_deep_mins   INT,
  sleep_rem_mins    INT,
  sleep_light_mins  INT,
  temp_avg_c        NUMERIC(4, 1),
  nap_count         INT NOT NULL DEFAULT 0,
  workout_count     INT NOT NULL DEFAULT 0,
  activity_session_count INT NOT NULL DEFAULT 0,
  updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_daily_metrics_updated ON daily_metrics(updated_at DESC);

CREATE TABLE sleep_sessions (
  id               BIGSERIAL PRIMARY KEY,
  sync_session_id  TEXT NOT NULL,
  day_key          TEXT NOT NULL,
  started_at       TIMESTAMPTZ NOT NULL,
  score            INT NOT NULL DEFAULT 0,
  total_mins       INT NOT NULL DEFAULT 0,
  deep_mins        INT NOT NULL DEFAULT 0,
  rem_mins         INT NOT NULL DEFAULT 0,
  light_mins       INT NOT NULL DEFAULT 0,
  wake_mins        INT NOT NULL DEFAULT 0,
  is_nap           BOOLEAN NOT NULL DEFAULT false,
  stages_json      JSONB,
  UNIQUE (started_at, day_key)
);

CREATE TABLE workouts (
  id               BIGSERIAL PRIMARY KEY,
  sync_session_id  TEXT NOT NULL,
  day_key          TEXT NOT NULL,
  started_at       TIMESTAMPTZ NOT NULL,
  sport_type       INT NOT NULL DEFAULT 0,
  sport_name       TEXT,
  duration_sec     INT NOT NULL DEFAULT 0,
  calories         INT,
  avg_hr           INT,
  max_hr           INT,
  UNIQUE (day_key, started_at)
);

CREATE TABLE activity_sessions (
  id               BIGSERIAL PRIMARY KEY,
  sync_session_id  TEXT NOT NULL,
  day_key          TEXT NOT NULL,
  started_at       TIMESTAMPTZ NOT NULL,
  sport_type       INT NOT NULL DEFAULT 0,
  sport_name       TEXT,
  duration_sec     INT NOT NULL DEFAULT 0,
  calories         INT,
  avg_hr           INT,
  max_hr           INT,
  UNIQUE (day_key, started_at)
);

CREATE TABLE temperature_samples (
  id               BIGSERIAL NOT NULL,
  sync_session_id  TEXT NOT NULL,
  day_key          TEXT NOT NULL,
  sampled_at       TIMESTAMPTZ NOT NULL,
  celsius          NUMERIC(4, 1) NOT NULL,
  PRIMARY KEY (sampled_at, id)
);

CREATE INDEX idx_temp_samples_day ON temperature_samples(day_key, sampled_at);

CREATE TABLE health_samples (
  id               BIGSERIAL NOT NULL,
  sync_session_id  TEXT NOT NULL,
  metric           TEXT NOT NULL,
  day_key          TEXT NOT NULL,
  sampled_at       TIMESTAMPTZ NOT NULL,
  value            NUMERIC(6, 2) NOT NULL,
  PRIMARY KEY (sampled_at, id)
);

CREATE INDEX idx_health_samples_day_metric
  ON health_samples(day_key, metric, sampled_at);

SELECT create_hypertable('temperature_samples', 'sampled_at', if_not_exists => TRUE);
SELECT create_hypertable('health_samples', 'sampled_at', if_not_exists => TRUE);
