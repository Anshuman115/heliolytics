# Feature — Storage & schema

PostgreSQL + TimescaleDB. Schema in `schema.sql`, incremental changes in
`migrations/`.

## Tables

| Table | Holds |
|---|---|
| `sync_sessions` | One row per upload — the provenance record |
| `raw_type_blobs` | **The raw strap bytes as uploaded** |
| `daily_metrics` | Per-day rollups (steps, calories, scores) |
| `sleep_sessions` | Nights + naps |
| `sleep_stages` | Per-stage spans within a session |
| `workouts` | User-started workouts |
| `activity_sessions` | Auto-detected sessions |
| `heart_rate_samples` | Continuous HR — **hypertable** |
| `step_samples` | Per-minute steps — **hypertable** |
| `temperature_samples` | Skin temperature — **hypertable** |
| `health_samples` | Per-minute vitals series — **hypertable** |

## Why `raw_type_blobs` exists

This is the design decision that makes everything else recoverable.

The server keeps the raw bytes forever, not just the parsed rows. When a parser bug
is found, `/api/v1/reparse` replays stored blobs through the fixed code — no need to
ask the strap for data it may have already rotated away.

It also means the phone can stay dumb: it uploads and forgets, holding no health
data at all.

## Hypertables

Four sample tables are Timescale hypertables partitioned on `sampled_at`:

```sql
SELECT create_hypertable('heart_rate_samples', 'sampled_at', if_not_exists => TRUE);
SELECT create_hypertable('step_samples',       'sampled_at', if_not_exists => TRUE);
SELECT create_hypertable('temperature_samples','sampled_at', if_not_exists => TRUE);
SELECT create_hypertable('health_samples',     'sampled_at', if_not_exists => TRUE);
```

These are the per-minute/per-second tables — they grow without bound while the
rollup tables stay small. Range queries over `sampled_at` are the dominant read
pattern, which is exactly what chunk pruning accelerates.

The rollup tables (`daily_metrics`, `sleep_sessions`, `workouts`) are plain
Postgres — a few rows per day doesn't justify partitioning.

## Coverage query

`internal/store/coverage.go` computes `dataThrough` as `MAX(ts)` across a `UNION ALL`
of every sample and session table — using end times, not start times, for sessions:

```sql
SELECT started_at + make_interval(secs => duration_sec) FROM workouts
UNION ALL SELECT started_at + make_interval(mins => total_mins) FROM sleep_sessions
```

A workout that started before the last sync but ended after it must count as covered
through its **end**, or the next sync re-fetches it.

## sqlc

`sql/queries/*.sql` → generated `internal/store/db/*.sql.go`.

**Never hand-edit `internal/store/db/`.** Edit the `.sql` and regenerate — hand edits
vanish on the next run.

Some store code (`coverage.go`, `readiness.go`) is hand-written pgx rather than sqlc,
where the query is dynamic or the shape doesn't suit codegen.

## Migrations

`migrations/` holds numbered incremental changes:

| File | Adds |
|---|---|
| `007_sleep_stages.sql` | Per-stage spans |
| `008_step_samples.sql` | Step sample hypertable |
| `009_computed_readiness.sql` | Server-computed readiness column |

`schema.sql` is the from-scratch definition; migrations carry an existing DB
forward. Both must end at the same shape.

## Idempotency

Writes upsert (`metrics_upsert.go`). Re-uploading the same sync session is safe —
required, because a fetch can be re-run after a partial failure.
`row_validate.go` rejects impossible rows before insert.

## Reset

`deploy/reset-db.sh` drops and recreates. Destructive — dev only.
