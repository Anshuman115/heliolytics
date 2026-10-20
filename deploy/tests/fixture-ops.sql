-- Synthetic restore-rehearsal fixture for deploy/tests/deploy-ops-test.sh.
--
-- All values are invented. There are no serials, real MACs, free text or
-- personal identifiers. Dates are fixed so dump and restored values compare
-- exactly. Run as the owner role on a freshly initialized schema.

INSERT INTO sync_sessions (session_id, device_mac, started_at, ended_at, battery_pct, catalog)
VALUES (
  'fixture-session-1',
  '02:00:00:00:00:01',
  TIMESTAMPTZ '2026-06-01 00:00:00+05:30',
  TIMESTAMPTZ '2026-06-01 06:00:00+05:30',
  90,
  '{"schemaVersion":1,"sessionId":"fixture-session-1","chunked":[],"unsolicited":[]}'::jsonb
)
ON CONFLICT (session_id) DO NOTHING;

INSERT INTO raw_blobs (session_id, type_code, sha256, payload)
VALUES (
  'fixture-session-1',
  '0x01',
  decode(repeat('ab', 32), 'hex'),
  decode('00ff0d0a7f80', 'hex')
)
ON CONFLICT (session_id, type_code) DO NOTHING;

INSERT INTO heart_rate_samples (sampled_at, source, bpm, session_id, observed_at) VALUES
  (TIMESTAMPTZ '2026-06-01 01:00:00+05:30', '0x01', 58, 'fixture-session-1', TIMESTAMPTZ '2026-06-01 06:00:00+05:30'),
  (TIMESTAMPTZ '2026-06-01 01:01:00+05:30', '0x01', 61, 'fixture-session-1', TIMESTAMPTZ '2026-06-01 06:00:00+05:30'),
  (TIMESTAMPTZ '2026-10-01 01:00:00+05:30', '0x01', 57, 'fixture-session-1', TIMESTAMPTZ '2026-06-01 06:00:00+05:30')
ON CONFLICT (sampled_at, source) DO NOTHING;

INSERT INTO step_samples (sampled_at, steps, session_id, observed_at) VALUES
  (TIMESTAMPTZ '2026-06-01 09:00:00+05:30', 0, 'fixture-session-1', TIMESTAMPTZ '2026-06-01 06:00:00+05:30'),
  (TIMESTAMPTZ '2026-06-01 09:01:00+05:30', 42, 'fixture-session-1', TIMESTAMPTZ '2026-06-01 06:00:00+05:30')
ON CONFLICT (sampled_at) DO NOTHING;

INSERT INTO temperature_samples (sampled_at, celsius, on_wrist, session_id, observed_at) VALUES
  (TIMESTAMPTZ '2026-06-01 01:00:00+05:30', 34.25, true, 'fixture-session-1', TIMESTAMPTZ '2026-06-01 06:00:00+05:30')
ON CONFLICT (sampled_at) DO NOTHING;

INSERT INTO hrv_samples (sampled_at, rmssd_ms, session_id, observed_at) VALUES
  (TIMESTAMPTZ '2026-06-01 01:30:00+05:30', 48.5, 'fixture-session-1', TIMESTAMPTZ '2026-06-01 06:00:00+05:30')
ON CONFLICT (sampled_at) DO NOTHING;

INSERT INTO stress_samples (sampled_at, stress, session_id, observed_at) VALUES
  (TIMESTAMPTZ '2026-06-01 12:00:00+05:30', 21, 'fixture-session-1', TIMESTAMPTZ '2026-06-01 06:00:00+05:30')
ON CONFLICT (sampled_at) DO NOTHING;

INSERT INTO resp_samples (sampled_at, br_min, session_id, observed_at) VALUES
  (TIMESTAMPTZ '2026-06-01 01:45:00+05:30', 14.5, 'fixture-session-1', TIMESTAMPTZ '2026-06-01 06:00:00+05:30')
ON CONFLICT (sampled_at) DO NOTHING;

INSERT INTO spo2_samples (sampled_at, source, pct, mode, session_id, observed_at) VALUES
  (TIMESTAMPTZ '2026-06-01 02:00:00+05:30', '0x25', 96, 'auto', 'fixture-session-1', TIMESTAMPTZ '2026-06-01 06:00:00+05:30'),
  (TIMESTAMPTZ '2026-06-01 02:30:00+05:30', '0x25', 95, 'auto', 'fixture-session-1', TIMESTAMPTZ '2026-06-01 06:00:00+05:30'),
  (TIMESTAMPTZ '2026-06-01 03:00:00+05:30', '0x25', 97, 'auto', 'fixture-session-1', TIMESTAMPTZ '2026-06-01 06:00:00+05:30')
ON CONFLICT (sampled_at, source) DO NOTHING;

INSERT INTO pai_scores (day_key, score, sampled_at, observed_at, session_id)
VALUES (DATE '2026-06-01', 120, TIMESTAMPTZ '2026-06-01 06:00:00+05:30', TIMESTAMPTZ '2026-06-01 06:00:00+05:30', 'fixture-session-1')
ON CONFLICT (day_key) DO NOTHING;

INSERT INTO sleep_sessions (
  started_at, ended_at, day_key, is_nap, score, total_mins, deep_mins, rem_mins,
  light_mins, wake_mins, stages, stages_truncated, observed_at, session_id
) VALUES (
  TIMESTAMPTZ '2026-05-31 23:00:00+05:30',
  TIMESTAMPTZ '2026-06-01 06:00:00+05:30',
  DATE '2026-06-01',
  false,
  78,
  420,
  90,
  110,
  220,
  30,
  '[]'::jsonb,
  false,
  TIMESTAMPTZ '2026-06-01 06:00:00+05:30',
  'fixture-session-1'
)
ON CONFLICT (started_at) DO NOTHING;

INSERT INTO activities (
  started_at, kind, ended_at, sport_type, sport_name, calories, avg_hr, max_hr, min_hr,
  steps, has_detail, observed_at, session_id
) VALUES (
  TIMESTAMPTZ '2026-06-01 07:00:00+05:30',
  'workout',
  TIMESTAMPTZ '2026-06-01 07:45:00+05:30',
  1,
  'Walk',
  210,
  112,
  138,
  88,
  4200,
  true,
  TIMESTAMPTZ '2026-06-01 06:00:00+05:30',
  'fixture-session-1'
)
ON CONFLICT (started_at, kind) DO NOTHING;

INSERT INTO coverage (type_code, data_through)
VALUES ('0x01', TIMESTAMPTZ '2026-06-01 01:01:00+05:30')
ON CONFLICT (type_code) DO NOTHING;

INSERT INTO daily_metrics (day_key, steps, avg_hr, spo2_avg, spo2_min, spo2_n, sleep_score, sleep_mins, pai_score)
VALUES (DATE '2026-06-01', 42, 59.5, 96, 95, 3, 78, 420, 120)
ON CONFLICT (day_key) DO NOTHING;

INSERT INTO profile (name, birth_date, sex, height_cm, weight_kg)
VALUES ('Fixture', DATE '1990-01-01', 'male', 178.0, 74.5)
ON CONFLICT (singleton) DO NOTHING;

-- Compress the older chunk so the restore rehearsal proves compressed data survives.
SELECT compress_chunk(older_chunk)
FROM show_chunks('heart_rate_samples', older_than => INTERVAL '30 days') AS older_chunk;
