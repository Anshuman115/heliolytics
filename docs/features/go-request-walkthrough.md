# Follow one Go request

Read [Go basics](go-basics.md) first.

The dates in the request example are illustrative, not data from a specific
account.

## 1. Startup provides dependencies

In `cmd/server/main.go`, `config.Load` reads the environment once and returns a
value plus an error. `store.New` creates a pgx connection pool without contacting
the database. Both are passed to the API:

```go
handler := api.NewMux(st, cfg,
    func(ctx context.Context, meta store.SessionMeta, catalog []byte,
        blobs map[string][]byte) (store.IngestResult, error) {
        return ingest.Run(ctx, st, meta, catalog, blobs)
    },
    func(ctx context.Context, profile *store.Profile) error {
        return ingest.UpdateProfile(ctx, st, profile)
    },
)
```

This is dependency injection with ordinary function values. The API package never
opens a database and never imports the ingest service; it calls the two callbacks
it was given. `api.NewMux` panics when the store or either callback is nil, so a
half-wired server fails at startup instead of answering requests badly.

## 2. A route selects the handler

`internal/api/routes.go` holds an exact table keyed by the escaped path and then by
method:

```go
routes := map[string]map[string]routeEntry{
    "/api/v1/daily-metrics": {
        http.MethodGet: {protected: true, handler: http.HandlerFunc(metrics.days)},
    },
}
```

The router runs before authentication. An unknown path is a JSON 404 `not_found`;
a known path with another method is a JSON 405 with an `Allow` header. There is no
`ServeMux`, so `/api/v1/daily-metrics/` is not redirected to the real path.

The request looks like this:

```text
GET /api/v1/daily-metrics?from=2026-09-01&to=2026-09-01
X-Heliolytics-Auth: v2.phone.<ts>.<nonce>.<mac>
X-Heliolytics-Content-SHA256: e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

The query after `?` supplies parameters and is not part of the path. A handler
reads the request (`r`) and writes the response (`w`). See the
[HTTP package](https://pkg.go.dev/net/http).

## 3. The handler requests rows

Excerpt from `internal/api/metrics_days.go`, with logging omitted:

```go
from, to, failure := h.rangeBounds(r, maxDailyRangeDays)
if failure != nil {
    middleware.WriteError(w, failure)
    return
}
rows, err := h.st.ListDays(r.Context(), from, to)
if err != nil {
    writeStoreFailure(w, "days", err)
    return
}
writeJSON(w, http.StatusOK, daysResponse{Days: out})
```

`rangeBounds` reads `from` and `to`, applies the 30-day default and enforces the
cap. `context.Context` comes from the request, so a client that disconnects
cancels the query. The handler does not parse blobs, run the detector or compute
readiness for this read; it reads stored rows.

`any` can hold a value of any type. Here an explicit struct gives JSON its `days`
key while `out` stays a typed slice. `writeJSON` sets the content type, the status
and encodes the value.

## 4. The store converts database types

`internal/store/metrics_list.go` turns the date strings into PostgreSQL dates,
calls the generated `db.ListDailyMetrics`, and maps each row to `store.DayMetric`.
The SQL source is `sql/queries/daily_metrics.sql`:

```sql
SELECT day_key, steps, avg_hr, readiness_components, updated_at
FROM daily_metrics
WHERE day_key >= sqlc.arg(from_day) AND day_key <= sqlc.arg(to_day)
ORDER BY day_key ASC;
```

`sqlc.arg` names become Go parameters, so both ends of the range are supplied
values rather than pasted text, and both are inclusive. sqlc generates the code
under `internal/store/db`; edit the SQL, not the generated files.

Row mapping is where nullability is decided. A nullable column becomes a Go
pointer, and nil means the metric is genuinely missing. A stored readiness score
and its `readiness_components` are copied as they are; the store does not rescore.

## 5. JSON returns to the client

The response has `days`, each with `dayKey`, `steps`, the other metric keys and
`updatedAt`. Every key is present. A missing metric is `null`, a measured zero is
`0`, and the two are different states that the client must keep different.

Only supported days appear, because a daily row exists only while a canonical row
supports it. An empty list means no supported day in that range. That is not the
same as a request error, and the client should not merge the two.

## 6. A write uses a different path

`POST /api/v1/ingest` reads a multipart envelope and calls the injected ingest
callback, which is `ingest.Run`. Inside one transaction it locks the canonical
rows, parses every submitted type, writes the session with its raw bytes, writes
the typed rows, reconciles the sleep and workout candidates, refreshes coverage
and runs the daily rollups.

A transaction commits related changes together or rolls them all back. If the
rollup fails, the session, raw bytes, samples and coverage disappear with it, so a
failed upload leaves no half state. An upsert inserts a row or updates the row
with the same stable key; the rollup then computes daily values from the stored
rows inside the same transaction.

## Contract checks

**Where would you change the JSON name?** In the field tag of the API model, then
in both client contracts. Renaming a Go field alone does not rename the tagged
JSON key.

**Where would you change the date query?** In the SQL source and the store, then
regenerate sqlc if that query is generated input.

**Would changing a chart fix a wrong parsed timestamp?** No. Trace the timestamp
through the catalog anchor, the parser, the stored row and the API before touching
display code.

Next: [storage](storage-and-schema.md), [readiness](readiness-score.md),
[ingest](ingest-and-parsing.md).

Validation: not run for this revision.
