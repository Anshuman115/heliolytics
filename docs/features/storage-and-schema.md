# Storage and schema

PostgreSQL stores the data. TimescaleDB adds time partitioning to the seven sample tables. sqlc is the only source of SQL text: every statement lives in `sql/queries/*.sql` and generates `internal/store/db`.

`schema.sql` is the entire schema. It runs once on an empty database and is not an in-place upgrade. There are no migrations, no compatibility views and no seed rows. Initialization creates extensions, tables, indexes, constraints, the ownership view, the identity helper and grants only.

## Tables

| Table | Holds |
|---|---|
| `sync_sessions` | Immutable upload envelope: device, start, end, battery, catalog JSON |
| `raw_blobs` | Exact bytes per session and type, with SHA-256 |
| `heart_rate_samples` | HR with `0x01` or `0x46` provenance, keyed by instant and source |
| `step_samples` | Per-minute steps, including observed zero |
| `temperature_samples` | Celsius with two decimals and the wear proxy flag |
| `hrv_samples` | RMSSD in milliseconds |
| `stress_samples` | Stress score |
| `resp_samples` | Breaths per minute |
| `spo2_samples` | Real `0x25` oximetry with auto or manual mode |
| `sleep_sessions` | Canonical sleep winners, attribution day and end-exclusive stages |
| `activities` | Canonical workouts and auto-detected activities, keyed by start and kind |
| `sleep_observations` | Every sleep candidate ever submitted, immutable |
| `workout_observations` | Every workout candidate ever submitted, immutable |
| `pai_scores` | Device PAI score with its record epoch |
| `daily_metrics` | One row per supported day, written by the rollup |
| `coverage` | One watermark per measured type |
| `profile` | The single nullable profile row |
| `auth_nonces` | Consumed request nonces |

The seven sample tables are hypertables on `sampled_at` with 90-day chunks. Compression runs after 30 days, segmented by `source` for heart rate and SpO2 only. Mutable tables are never compressed.

## Nullability

NULL means missing, never zero. Every numeric column has a domain CHECK, and writers send NULL when a sensor produced nothing. A measured zero is stored as zero. `daily_metrics` counts are NOT NULL because their absence is zero; every average, minimum and score stays nullable.

## Session and raw envelopes

`sync_sessions` and `raw_blobs` are insert-only for the application role. A repeated upload with the same session ID is accepted only when device, start, end, battery, the semantic catalog and every payload byte match. A changed catalog, timestamp, type set or payload returns `ErrSessionConflict`, which the API reports as 409 `session_conflict` with no writes. Raw bytes are always re-parsed; the SHA-256 is integrity metadata, not a parse-skip key.

Raw storage is retain-all until the owner approves a pruning policy. No job deletes raw rows, sessions, samples or candidates.

## Sample ordering

Each sample row stores `observed_at`, the producing sync's `ended_at`, and `session_id`. A write advances the row only when `(observed_at, session_id)` is at least the stored tuple, comparing session IDs bytewise. Newer observations advance lineage even when the value is equal, and that lineage change alone is not a rollup trigger. An older upload can never regress a canonical value.

Duplicate keys inside one caller batch collapse when the value matches and fail as `InvalidRowError` when it does not, before any statement runs. Batches are set-based UNNEST statements of at most 4096 rows.

## Sleep and workout reconciliation

Sleep and workout uploads insert typed candidates into `sleep_observations` and `workout_observations`, then reconcile each overlap-connected component:

1. Expand the component over every candidate that intersects it, including candidates that lost earlier.
2. Sort by `observed_at` descending, session ID bytewise descending, then start and end ascending.
3. Keep a candidate when its half-open interval overlaps no candidate already kept.
4. Delete canonical non-winners first, then upsert the winners.

This keeps rejected history, preserves endpoint adjacency and restores a previously displaced candidate when a newer one arrives. A retried candidate that differs from the stored one returns `ErrSessionConflict`. Auto activities are separate rows: workout reconciliation never deletes them, and auto replacement never touches workouts.

## Canonical change reporting

Every canonical write returns a `Mutation` with the affected IST days, the merged old and new instant intervals and a `Changed` flag. Point samples report `[timestamp, timestamp+1µs)`. Session rows report their full old and new ranges. Lineage-only updates return no days and no intervals, so they never trigger a rollup.

## Daily domain

`canonical_day_keys` unions the seven sample day keys, sleep attribution dates, activity start dates and PAI dates. It exists for tests.

Online code probes support per day with `sampled_at` bounds so Timescale excludes unrelated chunks. `ListCanonicalDayKeys` returns the supported subset, `NormalizeDailyDomainTx` deletes unsupported rows inside the affected set and fails if a supported day has no computed row. Raw sessions, profile, coverage and rejected candidates never create daily support.

## Coverage

`coverage` stores one row per measured type. `RefreshTypeCoverage` recomputes the exact canonical maximum inside the ingest transaction and deletes a watermark with no rows left. Coverage reads touch only `coverage` and `sync_sessions`.

The measured set is `0x01 0x05 0x06 0x0D 0x13 0x25 0x2E 0x38 0x46 0x48 0x49`. Raw-only `0x26` never appears. `0x01` is the maximum of minute steps and `0x01` heart rate. `0x48` is the end of main or nap sleep. `0x0D` is the PAI record epoch, not the upload time.

A watermark is a fetch-planning hint, not proof that every earlier minute exists.

## Identity and rebuilds

`public.rebuild_database_identity()` is a fixed-search-path `SECURITY DEFINER` helper that returns `system_identifier`, the database OID and the database name. PUBLIC execute is revoked and the application role has no grant, so only the owner can read it.

`Store.Identity` maps that row to `DatabaseIdentity`. A rebuild compares the identity of source and target before any mutation, so two DSNs that alias one database through loopback addresses or host names are still refused. Rebuild is test-only tooling: it is absent from the production image, and it writes only to an empty target with the current schema.

## Grants

`schema.sql` grants the application role `SELECT, INSERT, UPDATE` on mutable tables and `SELECT, INSERT` on `sync_sessions`, `raw_blobs`, `sleep_observations` and `workout_observations`. `DELETE` is granted only on `auth_nonces`, `sleep_sessions`, `activities`, `coverage` and `daily_metrics`. The ownership view is select-only and the identity helper is not granted.

Grants are conditional on the role existing, so an isolated test database without `heliolytics_app` still initializes. Role and password creation belong to the deployment role script.

Validation: not run for this revision.
