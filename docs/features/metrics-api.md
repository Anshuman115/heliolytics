# Feature — Metrics API

The read side. Everything the app and web dashboard draw comes from here.

## Routes

`internal/api/routes.go` builds two muxes. `/health` is **outside** the auth chain
(load balancers need it unauthenticated); everything else is behind
HMAC + rate limiting.

| Route | Handler | Returns |
|---|---|---|
| `GET /health` | `metrics_common.go` | Liveness + DB check. **Unauthenticated** |
| `GET /api/v1/metrics/days` | `metrics_days.go` | Per-day rollups (steps, calories, scores) |
| `GET /api/v1/metrics/sleep` | `metrics_sleep.go` | Sleep sessions + stages |
| `GET /api/v1/metrics/temperature` | `metrics_temperature.go` | Skin temperature series |
| `GET /api/v1/metrics/series` | `metrics_series.go` | Per-minute vitals series |
| `GET /api/v1/metrics/hr` | `metrics_hr.go` | Continuous HR samples |
| `GET /api/v1/metrics/workouts` | `metrics_workouts.go` | Workout summaries + detail |
| `GET /api/v1/metrics/activity-sessions` | `metrics_activity_sessions.go` | Auto-detected sessions |
| `GET /api/v1/metrics/coverage` | `metrics_coverage.go` | Ingest watermarks — see [ingest-and-parsing.md](ingest-and-parsing.md) |
| `POST /api/v1/ingest` | `ingest.go` | Upload raw session |
| `POST /api/v1/reparse` | `reparse.go` | Replay stored blobs (off by default) |

## Middleware chain

Order matters — outermost first:

```go
RequestLog(
  RateLimit(rl)(
    HMACToken(cfg.SigningSecret)(apiMux),
  ),
)
```

Rate limiting sits **outside** HMAC deliberately: an unauthenticated flood is
rejected before spending CPU on signature verification.

## Payload shape and the two-tier split

The app splits reads into light and heavy tiers (see the app's
`home-and-rings.md`). `days` / `sleep` / `workouts` / `activity-sessions` are light
and load eagerly. `series` / `hr` / `temperature` are per-minute and load lazily
only on detail screens.

If you add a field to a light endpoint, check it isn't minute-resolution — that's
how the eager payload quietly grows.

## Store layer

`internal/store` wraps pgx. Queries live in `sql/queries/*.sql` and are compiled by
**sqlc** into `internal/store/db/*.sql.go`.

**Do not hand-edit `internal/store/db/`** — it's generated. Change the `.sql` file
and re-run sqlc.

`pgconv.go` handles Postgres ↔ Go type conversion (nullable times, numerics).
`metrics_list.go` / `metrics_types.go` shape API responses; `metrics_upsert.go`
handles the write path.

## Key files

| File | Role |
|---|---|
| `internal/api/routes.go` | Mux + chain assembly |
| `internal/api/metrics_common.go` | Shared handler helpers, `/health` |
| `internal/store/store.go` | Store construction |
| `internal/store/db/` | **Generated** — sqlc output |
| `sql/queries/` | Hand-written SQL, sqlc input |
