# Follow one Go request

Read [Go basics](go-basics.md) first.

The dates in the request example are illustrative, not data from a specific account.

## 1. Startup provides dependencies

In `cmd/server/main.go`, configuration is loaded from environment variables. `store.New` creates a pgx connection pool. `api.NewMux(st, cfg)` receives that store and registers handlers. A pool reuses database connections across requests.

This is dependency injection: pass the object the handler needs instead of opening a database connection in every handler. It uses ordinary function arguments and no extra framework.

## 2. A route selects the handler

Actual registration in `internal/api/routes.go`:

```go
apiMux.HandleFunc("/api/v1/daily-metrics", mh.days)
```

The HTTP server routes that path to `mh.days`. Logging, rate limiting and HMAC verification wrap the private mux. Each handler checks the HTTP method itself.

The request looks like:

```text
GET /api/v1/daily-metrics?from=2026-09-01&to=2026-09-01
X-Heliolytics-Token: <fresh signed token>
```

The query after `?` supplies parameters. It is not part of the registered path. A handler reads the request (`r`) and writes the response (`w`). See the [HTTP package](https://pkg.go.dev/net/http).

## 3. The handler requests rows

Excerpt from `internal/api/metrics_days.go`, with logging omitted:

```go
from, to := dayRange(r)
rows, err := h.st.ListDays(r.Context(), from, to)
if err != nil {
    http.Error(w, "db error", http.StatusInternalServerError)
    return
}
writeJSON(w, map[string]any{"days": rows})
```

`dayRange` reads date parameters. When both are not supplied, it uses a default range. The store parses dates and performs the query. The handler does not run BLE, parse a blob or calculate readiness for this read.

`any` can hold a value of any type. Here the map gives JSON a `days` key while `rows` remains a typed slice. `writeJSON` sets the content type and encodes it.

## 4. The store converts database types

`internal/store/metrics_list.go` converts dates to pgx date values, calls generated `db.ListDays`, then builds `store.DayMetric` records for the API.

The SQL source is `sql/queries/daily_metrics.sql`. Two relevant expressions are:

```sql
COALESCE(readiness, computed_readiness) AS readiness
WHERE day_key >= $1 AND day_key <= $2
```

`COALESCE` selects the first non-null value. Device readiness wins over the calculated fallback. `$1` and `$2` are separately supplied parameters, not text pasted into SQL. Both ends of the date range are included.

sqlc generates Go query functions from the SQL. Edit the SQL source, not the resulting files under `internal/store/db`. Handwritten queries also exist outside that generated directory, particularly for rollups and coverage.

## 5. JSON returns to the client

The response has `days`, containing records such as `dayKey`, `steps`, and optional metrics. Flutter turns these into `DayMetric`. Web uses its TypeScript model. The screen formats the values. Go's `Readiness` field and JSON's `readiness` name refer to the same value at different boundaries.

An empty successful list means no rows in that range. An HTTP error means the request failed. These are different states and should remain different in the UI.

## 6. A write uses a different path

`POST /api/v1/ingest` reads multipart files and calls `parse.RunIngest`. `ParseBlobs` makes records. `Aggregate` groups them. `WriteBatch` persists them and calls rollups inside `Store.WithTx`.

A transaction groups related changes: it commits them together or rolls them back on failure. An upsert inserts a new row or updates the row with the same stable key. Rollups calculate daily values from stored observations after those upserts. Calculated recovery runs last because it reads the other daily values.

This normal-ingest transaction does not describe the entire reparse handler. That handler has an additional reset step before it calls the ingest pipeline.

## Contract checks

**Where would you change the JSON name?** In the API model's JSON tag, then in both client contracts. Renaming a Go variable alone does not rename a tagged JSON field.

**Where would you change the date query?** In handwritten SQL or store code, then regenerate sqlc if that query is a sqlc input.

**Would changing a chart fix a wrong parsed timestamp?** No. Trace the timestamp through catalog anchors, parser, stored row and API before changing display code.

Next: [storage](storage-and-schema.md), [readiness](readiness-score.md), [ingest](ingest-and-parsing.md).

Validation: not run for this revision.
