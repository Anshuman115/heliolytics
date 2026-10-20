-- Fresh database only; initialization must not seed health or profile rows.

CREATE EXTENSION IF NOT EXISTS timescaledb;
CREATE EXTENSION IF NOT EXISTS btree_gist;


CREATE TABLE sync_sessions (
  session_id   TEXT PRIMARY KEY,
  device_mac   TEXT NOT NULL,
  started_at   TIMESTAMPTZ NOT NULL,
  ended_at     TIMESTAMPTZ NOT NULL,
  battery_pct  SMALLINT CHECK (battery_pct BETWEEN 0 AND 100),
  catalog      JSONB NOT NULL,
  received_at  TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  CHECK (ended_at >= started_at)
);

CREATE TABLE raw_blobs (
  session_id  TEXT NOT NULL REFERENCES sync_sessions(session_id),
  type_code   TEXT NOT NULL CHECK (type_code ~ '^0x[0-9A-F]{2}$'),
  sha256      BYTEA NOT NULL CHECK (octet_length(sha256) = 32),
  payload     BYTEA NOT NULL,
  PRIMARY KEY (session_id, type_code)
);
CREATE INDEX raw_blobs_type_sha ON raw_blobs (type_code, sha256);


CREATE TABLE heart_rate_samples (
  sampled_at   TIMESTAMPTZ NOT NULL,
  source       TEXT NOT NULL CHECK (source IN ('0x01', '0x46')),
  bpm          SMALLINT NOT NULL CHECK (bpm BETWEEN 25 AND 250),
  session_id   TEXT NOT NULL REFERENCES sync_sessions(session_id),
  observed_at  TIMESTAMPTZ NOT NULL,
  day_key      DATE GENERATED ALWAYS AS ((sampled_at AT TIME ZONE 'Asia/Kolkata')::date) STORED,
  PRIMARY KEY (sampled_at, source)
);

CREATE TABLE step_samples (
  sampled_at   TIMESTAMPTZ NOT NULL PRIMARY KEY,
  steps        SMALLINT NOT NULL CHECK (steps BETWEEN 0 AND 400),
  session_id   TEXT NOT NULL REFERENCES sync_sessions(session_id),
  observed_at  TIMESTAMPTZ NOT NULL,
  day_key      DATE GENERATED ALWAYS AS ((sampled_at AT TIME ZONE 'Asia/Kolkata')::date) STORED
);

CREATE TABLE temperature_samples (
  sampled_at   TIMESTAMPTZ NOT NULL PRIMARY KEY,
  celsius      NUMERIC(5,2) NOT NULL CHECK (celsius >= 0 AND celsius <= 50),
  on_wrist     BOOLEAN NOT NULL,
  session_id   TEXT NOT NULL REFERENCES sync_sessions(session_id),
  observed_at  TIMESTAMPTZ NOT NULL,
  day_key      DATE GENERATED ALWAYS AS ((sampled_at AT TIME ZONE 'Asia/Kolkata')::date) STORED,
  CHECK (NOT on_wrist OR (celsius >= 30 AND celsius <= 40))
);

CREATE TABLE hrv_samples (
  sampled_at   TIMESTAMPTZ NOT NULL PRIMARY KEY,
  rmssd_ms     REAL NOT NULL CHECK (rmssd_ms > 0 AND rmssd_ms <= 300),
  session_id   TEXT NOT NULL REFERENCES sync_sessions(session_id),
  observed_at  TIMESTAMPTZ NOT NULL,
  day_key      DATE GENERATED ALWAYS AS ((sampled_at AT TIME ZONE 'Asia/Kolkata')::date) STORED
);

CREATE TABLE stress_samples (
  sampled_at   TIMESTAMPTZ NOT NULL PRIMARY KEY,
  stress       SMALLINT NOT NULL CHECK (stress BETWEEN 0 AND 100),
  session_id   TEXT NOT NULL REFERENCES sync_sessions(session_id),
  observed_at  TIMESTAMPTZ NOT NULL,
  day_key      DATE GENERATED ALWAYS AS ((sampled_at AT TIME ZONE 'Asia/Kolkata')::date) STORED
);

CREATE TABLE resp_samples (
  sampled_at   TIMESTAMPTZ NOT NULL PRIMARY KEY,
  br_min       REAL NOT NULL CHECK (br_min >= 4 AND br_min <= 40),
  session_id   TEXT NOT NULL REFERENCES sync_sessions(session_id),
  observed_at  TIMESTAMPTZ NOT NULL,
  day_key      DATE GENERATED ALWAYS AS ((sampled_at AT TIME ZONE 'Asia/Kolkata')::date) STORED
);

CREATE TABLE spo2_samples (
  sampled_at   TIMESTAMPTZ NOT NULL,
  source       TEXT NOT NULL CHECK (source = '0x25'),
  pct          SMALLINT NOT NULL CHECK (pct BETWEEN 70 AND 100),
  mode         TEXT NOT NULL CHECK (mode IN ('auto', 'manual')),
  session_id   TEXT NOT NULL REFERENCES sync_sessions(session_id),
  observed_at  TIMESTAMPTZ NOT NULL,
  day_key      DATE GENERATED ALWAYS AS ((sampled_at AT TIME ZONE 'Asia/Kolkata')::date) STORED,
  PRIMARY KEY (sampled_at, source)
);


CREATE TABLE sleep_sessions (
  started_at        TIMESTAMPTZ PRIMARY KEY,
  ended_at          TIMESTAMPTZ NOT NULL,
  day_key           DATE NOT NULL,
  is_nap            BOOLEAN NOT NULL,
  score             SMALLINT CHECK (score BETWEEN 0 AND 100),
  total_mins        SMALLINT NOT NULL CHECK (total_mins >= 0 AND total_mins <= 1440),
  deep_mins         SMALLINT NOT NULL CHECK (deep_mins >= 0),
  rem_mins          SMALLINT NOT NULL CHECK (rem_mins >= 0),
  light_mins        SMALLINT NOT NULL CHECK (light_mins >= 0),
  wake_mins         SMALLINT NOT NULL CHECK (wake_mins >= 0),
  stages            JSONB NOT NULL,
  stages_truncated  BOOLEAN NOT NULL,
  observed_at       TIMESTAMPTZ NOT NULL,
  session_id        TEXT NOT NULL REFERENCES sync_sessions(session_id),
  CHECK (ended_at > started_at),
  CHECK (deep_mins + rem_mins + light_mins = total_mins),
  EXCLUDE USING gist (tstzrange(started_at, ended_at) WITH &&)
);
CREATE INDEX sleep_sessions_day ON sleep_sessions (day_key);

CREATE TABLE activities (
  started_at    TIMESTAMPTZ NOT NULL,
  kind          TEXT NOT NULL CHECK (kind IN ('workout', 'auto')),
  ended_at      TIMESTAMPTZ NOT NULL,
  sport_type    SMALLINT NOT NULL,
  sport_name    TEXT NOT NULL,
  calories      SMALLINT CHECK (calories BETWEEN 0 AND 10000),
  avg_hr        SMALLINT CHECK (avg_hr BETWEEN 25 AND 250),
  max_hr        SMALLINT CHECK (max_hr BETWEEN 25 AND 250),
  min_hr        SMALLINT CHECK (min_hr BETWEEN 25 AND 250),
  steps         INT CHECK (steps >= 0),
  has_detail    BOOLEAN NOT NULL,
  observed_at   TIMESTAMPTZ NOT NULL,
  session_id    TEXT NOT NULL REFERENCES sync_sessions(session_id),
  day_key       DATE GENERATED ALWAYS AS ((started_at AT TIME ZONE 'Asia/Kolkata')::date) STORED,
  PRIMARY KEY (started_at, kind),
  CHECK (ended_at > started_at)
);
CREATE INDEX activities_day ON activities (day_key);

CREATE TABLE sleep_observations
  (LIKE sleep_sessions INCLUDING DEFAULTS INCLUDING CONSTRAINTS);
ALTER TABLE sleep_observations ADD PRIMARY KEY (session_id, started_at);
ALTER TABLE sleep_observations ADD FOREIGN KEY (session_id)
  REFERENCES sync_sessions(session_id);
CREATE INDEX sleep_observations_overlap ON sleep_observations
  USING gist (tstzrange(started_at, ended_at, '[)'));

CREATE TABLE workout_observations
  (LIKE activities INCLUDING DEFAULTS INCLUDING CONSTRAINTS INCLUDING GENERATED);
ALTER TABLE workout_observations ADD PRIMARY KEY (session_id, started_at);
ALTER TABLE workout_observations ADD FOREIGN KEY (session_id)
  REFERENCES sync_sessions(session_id);
ALTER TABLE workout_observations ADD CHECK (kind = 'workout');
CREATE INDEX workout_observations_overlap ON workout_observations
  USING gist (tstzrange(started_at, ended_at, '[)'));


CREATE TABLE pai_scores (
  day_key      DATE PRIMARY KEY,
  score        SMALLINT NOT NULL CHECK (score BETWEEN 0 AND 1000),
  sampled_at   TIMESTAMPTZ NOT NULL,
  observed_at  TIMESTAMPTZ NOT NULL,
  session_id   TEXT NOT NULL REFERENCES sync_sessions(session_id)
);


CREATE TABLE daily_metrics (
  day_key               DATE PRIMARY KEY,
  steps                 INT CHECK (steps >= 0),
  avg_hr                REAL CHECK (avg_hr BETWEEN 25 AND 250),
  resting_hr            REAL CHECK (resting_hr BETWEEN 25 AND 250),
  hrv_rmssd_ms          REAL CHECK (hrv_rmssd_ms > 0 AND hrv_rmssd_ms <= 300),
  resp_rate             REAL CHECK (resp_rate >= 4 AND resp_rate <= 40),
  spo2_avg              REAL CHECK (spo2_avg BETWEEN 70 AND 100),
  spo2_min              SMALLINT CHECK (spo2_min BETWEEN 70 AND 100),
  spo2_sleep_avg        REAL CHECK (spo2_sleep_avg BETWEEN 70 AND 100),
  spo2_sleep_min        SMALLINT CHECK (spo2_sleep_min BETWEEN 70 AND 100),
  spo2_n                INT NOT NULL DEFAULT 0 CHECK (spo2_n >= 0),
  stress_avg            REAL CHECK (stress_avg BETWEEN 0 AND 100),
  skin_temp_night_c     REAL CHECK (skin_temp_night_c >= 0 AND skin_temp_night_c <= 50),
  skin_temp_dev_c       REAL CHECK (skin_temp_dev_c >= -20 AND skin_temp_dev_c <= 20),
  sleep_score           SMALLINT CHECK (sleep_score BETWEEN 0 AND 100),
  sleep_mins            SMALLINT CHECK (sleep_mins >= 0),
  sleep_deep_mins       SMALLINT CHECK (sleep_deep_mins >= 0),
  sleep_rem_mins        SMALLINT CHECK (sleep_rem_mins >= 0),
  sleep_light_mins      SMALLINT CHECK (sleep_light_mins >= 0),
  sleep_wake_mins       SMALLINT CHECK (sleep_wake_mins >= 0),
  time_in_bed_mins      SMALLINT CHECK (time_in_bed_mins >= 0),
  sleep_efficiency_pct  SMALLINT CHECK (sleep_efficiency_pct BETWEEN 0 AND 100),
  nap_count             SMALLINT NOT NULL DEFAULT 0 CHECK (nap_count >= 0),
  workout_count         SMALLINT NOT NULL DEFAULT 0 CHECK (workout_count >= 0),
  auto_activity_count   SMALLINT NOT NULL DEFAULT 0 CHECK (auto_activity_count >= 0),
  workout_calories      INT CHECK (workout_calories >= 0),
  trimp                 REAL CHECK (trimp >= 0 AND trimp <= 100000),
  pai_score             SMALLINT CHECK (pai_score BETWEEN 0 AND 1000),
  readiness             SMALLINT CHECK (readiness BETWEEN 0 AND 100),
  readiness_components  JSONB,
  updated_at            TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  CHECK (sleep_mins IS NULL OR sleep_deep_mins IS NULL OR sleep_rem_mins IS NULL
         OR sleep_light_mins IS NULL
         OR sleep_deep_mins + sleep_rem_mins + sleep_light_mins = sleep_mins),
  CHECK (time_in_bed_mins IS NULL OR sleep_mins IS NULL OR sleep_wake_mins IS NULL
         OR time_in_bed_mins = sleep_mins + sleep_wake_mins)
);


CREATE TABLE coverage (
  type_code     TEXT PRIMARY KEY CHECK (type_code ~ '^0x[0-9A-F]{2}$'),
  data_through  TIMESTAMPTZ NOT NULL,
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);

CREATE TABLE profile (
  singleton   BOOLEAN PRIMARY KEY DEFAULT true CHECK (singleton),
  name        TEXT,
  birth_date  DATE CHECK (birth_date > DATE '1900-01-01'),
  sex         TEXT CHECK (sex IN ('male', 'female')),
  height_cm   NUMERIC(4,1) CHECK (height_cm >= 50 AND height_cm <= 250),
  weight_kg   NUMERIC(4,1) CHECK (weight_kg >= 20 AND weight_kg <= 300),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);

CREATE TABLE auth_nonces (
  kid         TEXT NOT NULL,
  nonce       TEXT NOT NULL CHECK (nonce ~ '^[0-9a-f]{32}$'),
  expires_at  TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (kid, nonce)
);
CREATE INDEX auth_nonces_expiry ON auth_nonces (expires_at);


CREATE VIEW canonical_day_keys AS
SELECT day_key FROM heart_rate_samples
UNION SELECT day_key FROM step_samples
UNION SELECT day_key FROM temperature_samples
UNION SELECT day_key FROM hrv_samples
UNION SELECT day_key FROM stress_samples
UNION SELECT day_key FROM resp_samples
UNION SELECT day_key FROM spo2_samples
UNION SELECT day_key FROM sleep_sessions
UNION SELECT day_key FROM activities
UNION SELECT day_key FROM pai_scores;


SELECT create_hypertable('heart_rate_samples', by_range('sampled_at', INTERVAL '90 days'));
ALTER TABLE heart_rate_samples SET (
  timescaledb.compress,
  timescaledb.compress_segmentby = 'source',
  timescaledb.compress_orderby = 'sampled_at DESC'
);
SELECT add_compression_policy('heart_rate_samples', INTERVAL '30 days');

SELECT create_hypertable('step_samples', by_range('sampled_at', INTERVAL '90 days'));
ALTER TABLE step_samples SET (
  timescaledb.compress,
  timescaledb.compress_orderby = 'sampled_at DESC'
);
SELECT add_compression_policy('step_samples', INTERVAL '30 days');

SELECT create_hypertable('temperature_samples', by_range('sampled_at', INTERVAL '90 days'));
ALTER TABLE temperature_samples SET (
  timescaledb.compress,
  timescaledb.compress_orderby = 'sampled_at DESC'
);
SELECT add_compression_policy('temperature_samples', INTERVAL '30 days');

SELECT create_hypertable('hrv_samples', by_range('sampled_at', INTERVAL '90 days'));
ALTER TABLE hrv_samples SET (
  timescaledb.compress,
  timescaledb.compress_orderby = 'sampled_at DESC'
);
SELECT add_compression_policy('hrv_samples', INTERVAL '30 days');

SELECT create_hypertable('stress_samples', by_range('sampled_at', INTERVAL '90 days'));
ALTER TABLE stress_samples SET (
  timescaledb.compress,
  timescaledb.compress_orderby = 'sampled_at DESC'
);
SELECT add_compression_policy('stress_samples', INTERVAL '30 days');

SELECT create_hypertable('resp_samples', by_range('sampled_at', INTERVAL '90 days'));
ALTER TABLE resp_samples SET (
  timescaledb.compress,
  timescaledb.compress_orderby = 'sampled_at DESC'
);
SELECT add_compression_policy('resp_samples', INTERVAL '30 days');

SELECT create_hypertable('spo2_samples', by_range('sampled_at', INTERVAL '90 days'));
ALTER TABLE spo2_samples SET (
  timescaledb.compress,
  timescaledb.compress_segmentby = 'source',
  timescaledb.compress_orderby = 'sampled_at DESC'
);
SELECT add_compression_policy('spo2_samples', INTERVAL '30 days');


CREATE FUNCTION public.rebuild_database_identity()
RETURNS TABLE(system_identifier text, database_oid oid, database_name text)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog
AS $identity$
  SELECT c.system_identifier::text, d.oid, d.datname::text
  FROM pg_catalog.pg_control_system() AS c
  CROSS JOIN pg_catalog.pg_database AS d
  WHERE d.datname = pg_catalog.current_database()
$identity$;
REVOKE ALL ON FUNCTION public.rebuild_database_identity() FROM PUBLIC;


DO $grants$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'heliolytics_app') THEN
    RETURN;
  END IF;

  GRANT SELECT, INSERT, UPDATE ON
    heart_rate_samples, step_samples, temperature_samples, hrv_samples,
    stress_samples, resp_samples, spo2_samples, sleep_sessions, activities,
    pai_scores, daily_metrics, coverage, profile, auth_nonces
    TO heliolytics_app;

  GRANT SELECT, INSERT ON
    sync_sessions, raw_blobs, sleep_observations, workout_observations
    TO heliolytics_app;

  GRANT DELETE ON
    auth_nonces, sleep_sessions, activities, coverage, daily_metrics
    TO heliolytics_app;

  GRANT SELECT ON canonical_day_keys TO heliolytics_app;
END
$grants$;
