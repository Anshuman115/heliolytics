# Storage and schema

Source review: 7 September 2026. Describes the local code, not a live deployment check.

PostgreSQL stores the data. TimescaleDB adds time partitioning to eight sample
tables. sqlc turns selected SQL queries into Go functions.

## Current tables

| Table | Holds |
|---|---|
| `sync_sessions` | Upload metadata and ingest time |
| `raw_type_blobs` | Original bytes for each session and type |
| `daily_metrics` | One summary row per day |
| `sleep_sessions` | Nights, naps and stage JSON in `stages_json` |
| `workouts` | User-started activities, summary/detail coverage flags |
| `activity_sessions` | Activities inferred from minute records |
| `profiles` | Personal profile, used as one stored profile |
| `raw_pai_scores` | Parsed daily PAI values |
| `raw_readiness` | Parsed device readiness values |
| `heart_rate_samples` | HR with `0x01` / `0x46` source identity |
| `step_samples` | Minute steps |
| `temperature_samples` | Skin temperature |
| `hrv_samples` | HRV |
| `spo2_samples` | SpO2 with spot/sleep source identity |
| `stress_samples` | Stress |
| `resp_samples` | Breathing rate |
| `rhr_samples` | Resting HR |

The eight tables ending in `_samples` above are hypertables on `sampled_at`.
The current schema has neither `health_samples` nor `sleep_stages`.

## Why keep several levels?

Raw blobs allow parsing work to be revisited. Parsed observations power detailed
charts. Daily rows answer day-screen requests without loading all samples.
The phone may cache parsed responses; it does not own this canonical history.

Normal ingest writes original blobs, parsed rows and daily rollups in one
transaction. Rollups read stored observations, so an upload containing only one
signal does not replace the entire day with that partial upload.

## Coverage

`coverage.go` reports the latest stored observation/session end and last ingest.
`coverage_types.go` maps all fourteen fetched codes to their own watermarks.
Workout summary/detail and spot/sleep SpO2 have separate source coverage.
Sleep coverage uses stage end times when available, otherwise elapsed sleep plus
wake time. `0x48` coverage uses main sleep, not a later nap.

A latest timestamp is a sync-planning watermark. It does not certify that every
minute before it is present.

## SQL and migrations

Edit `sql/queries/*.sql`, then regenerate `internal/store/db`. Handwritten pgx
queries also exist in `internal/store` and `internal/rollup`.

| Migration | Purpose |
|---|---|
| `007_sleep_stages.sql` | Add wake minutes, nap flag, stage JSON to sleep sessions |
| `008_step_samples.sql` | Add minute steps |
| `009_computed_readiness.sql` | Add fallback recovery column |
| `010_split_health_samples_and_profiles.sql` | Split vitals and add profiles/raw scores |
| `011_heart_rate_source_type.sql` | Activity/continuous HR source identity |
| `012_spo2_source_type.sql` | Spot/sleep SpO2 source identity |
| `013_repair_sleep_stage_times.sql` | Repair existing sleep timing |
| `014_workout_source_coverage.sql` | Separate summary/detail flags |

`schema.sql` initializes an empty DB volume. Numbered migrations are not run by
container startup. Inspect an existing DB's shape and applied changes before
choosing migrations; some scripts assume older tables exist.

There is no automated raw-blob retention, owner export or history deletion route.
`deploy/reset-db.sh` deletes the full database volume. It is not a migration tool.
