# Metrics API

Source review: 7 September 2026. Describes the local code, not a live deployment check.

The route registry is `internal/api/routes.go`. Successful responses use named
JSON fields such as `days` or `workouts`; errors generally use HTTP status and plain
text. There is no universal `data` wrapper.

## Routes and actual consumers

Paths below start with `/api/v1` unless shown otherwise. App means the Flutter
client; Web means the Next.js server-side client.

| Method and path | Response / purpose | Current consumer |
|---|---|---|
| `GET /health` | `ok` when the DB responds; unauthenticated | Healthchecks, connection checks |
| `GET /api/v1/daily-metrics` | `days`: daily rollups | App and Web |
| `GET /api/v1/sleep` | `sleep`: sessions and stages | App and Web |
| `GET /api/v1/metrics/workouts` | `workouts`: workout summaries | App and Web |
| `GET /api/v1/metrics/activity-sessions` | `activitySessions`: automatic sessions | App and Web |
| `GET /api/v1/metrics/hr` | `days`: compact HR blocks | App and Web |
| `GET /api/v1/metrics/series` | `days`: compact vital blocks | App and Web |
| `GET /api/v1/metrics/temperature` | `days`: compact temperature blocks | App and Web |
| `GET /api/v1/metrics/coverage` | Ingest and per-type timestamps | App sync, Web status |
| `GET /api/v1/daily-health-scores` | `days`: selected daily health fields | App |
| `GET /api/v1/metrics/hrv` | `points`: sample timestamps and values | Available; no current client call |
| `GET /api/v1/metrics/rhr` | `points`: sample timestamps and values | Available; no current client call |
| `GET /api/v1/metrics/vo2max` | Empty `points`; no data source | Stub; no current client call |
| `GET /api/v1/recovery` | Calculated score and components | Available; no current client call |
| `GET /api/v1/strain` | `paiScore` for a day | Available; no current client call |
| `GET /api/v1/profile` | Stored profile | Available; App does not read it |
| `PUT /api/v1/profile` | Save profile | App best-effort profile upload |
| `POST /api/v1/ingest` | Accept multipart sync | App |
| `POST /api/v1/reparse` | Replay latest stored session | Operator; disabled by default |

The former paths `/api/v1/metrics/days` and `/api/v1/metrics/sleep` are not registered.

## Dates and shapes

Most history routes accept inclusive `from` and `to` date keys. Defaults in
`metrics_common.go` start 30 days before today, or 90 for workouts/auto sessions.
Both endpoints are inclusive; these defaults are not exact row counts.
Recovery and strain require `day=YYYY-MM-DD`.

Compact blocks carry a `startTime`, second `offsets`, and `values`, plus day/metric
identity as applicable. Clients pair each offset with its value and rebuild time.
This expands timestamps, not health blobs. A consumer must follow each endpoint's
actual response shape rather than assume every route returns a flat list.

Profile writes use `height_cm` and `weight_kg`; daily metric JSON uses names such
as `dayKey` and `hrvRmssd`. Profile and daily metric casing are different contracts.

`/recovery` always computes the backend formula. The readiness in daily metrics
prefers the device's score. These can differ; do not label them interchangeable.

## Request path

Logging -> rate limiter -> HMAC verifier -> handler -> store -> JSON.
`/health` bypasses that chain and checks the database.

Flutter fetches day bundles plus separate health tiles; heavy series are requested
on detail screens. Web currently starts eight reads on the server for its dashboard.

## Source files

- `internal/api/routes.go`: exact registrations.
- `internal/api/metrics_common.go`: default ranges, JSON helper, DB health.
- `internal/store/metrics_list.go`: database-to-JSON model conversion.
- `sql/queries`: handwritten sqlc queries; `internal/store/db` is generated.

See [the request walkthrough](go-request-walkthrough.md) for a line-by-line example.
