# Metrics API

The route table lives in `internal/api/routes.go`. The router resolves the exact
escaped path, then the method, before any authentication or body work. It never
redirects and never canonicalizes a slash or dot segment, so
`GET /api/daily-metrics/` is a 404.

Successful responses are JSON objects with named fields. Errors are JSON too:
`{"error":{"code":"...","message":"..."}}`. The message comes from a fixed table,
never from the request or the database.

## Routes

| Method and path | Auth | Range | Response |
|---|---|---|---|
| `GET /health` | public | none | Text `ok`. It never touches the database |
| `GET /version` | public | none | `commit` and `builtAt`, null when the build injected neither |
| `GET /api/daily-metrics` | signed | 366 days | `days`: one C3.3 row per stored IST day |
| `GET /api/sleep` | signed | 366 days | `days`: `dayKey`, `main`, `naps` |
| `GET /api/activities` | signed | 366 days | `activities`: workouts and derived auto activities |
| `GET /api/metrics/hr` | signed | 31 days | `days`: compact heart rate blocks |
| `GET /api/metrics/temperature` | signed | 31 days | `days`: compact temperature blocks |
| `GET /api/metrics/series` | signed | 31 days | `days`: compact vital blocks |
| `GET /api/metrics/coverage` | signed | no parameters | `hasData`, `lastIngestAt`, `dataThrough`, `types` |
| `PUT /api/profile` | `phone` only | none | 204 with an empty body |
| `POST /api/ingest` | `phone` only | none | The C2 per-type result |

An unknown path returns 404 `not_found`. A known path with any other method,
including `HEAD`, returns 405 `method_not_allowed` and lists the accepted methods
in `Allow`.

No other route exists. Workout lists, activity sessions, HRV points, resting HR
points, VO2 max, recovery, strain, daily health scores and reparse are not served;
clients read the same values from the daily row, the sleep route, the activity
route or the series route.

## Date ranges

`from` and `to` must appear together as `YYYY-MM-DD` and each may appear exactly
once. Anything else is 400 `invalid_date`. An inverted range is 400
`invalid_range`. A span above the route cap is 400 `range_too_large`. Omitting
both means the last 30 IST days, ending today.

## Day rows

Every key is present. A metric the store does not hold is null, never zero.
`spo2Samples`, `napCount`, `workoutCount` and `autoActivityCount` are counts and
always numbers.

`readiness` and `readinessComponents` come from the stored row. `readinessStatus`
is the `status` field inside those components, and `readinessBand` is derived from
the stored score (`good` at 67 or more, `fair` at 34 to 66, `low` at 33 or less).
Clients never derive the band themselves. A day whose inputs were insufficient
keeps its status and a null score.

Only stored IST days appear. A calendar gap is not a row, so clients draw gaps as
gaps and never as zero or `no_data` days. The same rule applies to the sleep,
activity and series routes.

Compact blocks carry `dayKey`, `startTime`, second `offsets` and `values`. Clients
pair each offset with its value to rebuild a timestamp. This expands timestamps,
never raw blobs.

## Coverage

`types` has exactly the eleven measured codes: `0x01`, `0x05`, `0x06`, `0x0D`,
`0x13`, `0x25`, `0x2E`, `0x38`, `0x46`, `0x48`, `0x49`. Each value is the canonical
watermark of stored rows, or null before the first one. `0x26` is raw-only, so it
never appears. `lastIngestAt` is the latest committed session and `dataThrough` is
the greatest canonical watermark.

## Writes

`PUT /api/profile` replaces the whole profile. All five keys are required and
an explicit null clears that field: `name`, `birthDate`, `sex`, `heightCm`,
`weightKg`. Values are bounded (birth date after 1900-01-01 and not after today,
50..250 cm, 20..300 kg, sex `male` or `female`). A shape failure is 400
`invalid_body` and a semantic failure is 422 `invalid_profile`. The 204 is sent
only after the transaction commits, and a changed birth date or sex recomputes
every stored load and readiness row in that same transaction.

`POST /api/ingest` accepts at most 16 MiB, 40 parts, and requires the `session`
and `catalog` parts plus one non-empty `0xNN_raw.bin` part. An unknown or
duplicate part is 400. A structural type failure rejects the whole upload with 422
and writes nothing. `0x26` returns `raw_only`.

## Request path

Logging, then the request timeout, then panic recovery, then the router, then the
guard: IP limit, MAC, permission, key limit, body slot, bounded read and hash,
route preparation, nonce, handler. `/health` and `/version` skip the guard
entirely.

## Source files

- `internal/api/routes.go`: the exact route table and guard assembly.
- `internal/api/metrics_common.go`: range rules, JSON writer, error mapping.
- `internal/api/metrics_coverage.go`: the measured coverage shape.
- `internal/api/ingest.go` and `internal/api/profile.go`: the write envelopes.
- `internal/store/metrics_list.go`: stored rows to JSON models.

See [the request walkthrough](go-request-walkthrough.md) for a line-by-line
example.

Validation: not run for this revision.
