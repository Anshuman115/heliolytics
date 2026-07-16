# Feature — Ingest & parsing

The server's core job. The phone uploads raw strap bytes; this turns them into rows.

**All health parsing happens here.** The phone deliberately does none of it — that
split is the platform's central design decision, so parser fixes ship as a server
deploy and reach every install at once, and old raw sessions can be re-parsed.

## Endpoint

`POST /api/v1/ingest` → `internal/api/ingest.go`

Multipart form, 128 MB cap (`r.ParseMultipartForm(128 << 20)`). Carries:

- a **catalog JSON** — which type codes are present, with per-page `roundSegments`
- one **blob per type code** — the raw bytes exactly as the strap sent them

## Pipeline

`parse.RunIngest` (`internal/parse/pipeline.go`) is four lines and reads like its
own diagram:

```go
parsed := ParseBlobs(catalogJSON, blobs, fetchEnd)  // bytes → typed records
agg    := Aggregate(parsed)                          // records → per-day rollups
return WriteBatch(ctx, st, syncSessionID, agg)       // rollups → DB, one txn
```

### 1. ParseBlobs

Dispatches each blob to its type parser (`parse_batch.go` → `catalog.go`):

| Code | Parser | Produces |
|---|---|---|
| `0x01` | `activity.go` | Per-minute HR / steps / activity |
| `0x05`,`0x06` | `protobuf.go` | Workout summary + per-second detail |
| `0x0D` | `pai.go` | PAI scores |
| `0x25`,`0x26` | `spo2.go` | SpO₂ spot + sleep |
| `0x2E` | temperature | Skin temperature |
| `0x38` | resp rate | Sleep respiratory rate |
| `0x39` | readiness | Device-reported readiness |
| `0x3A` | `resting_hr.go` | Resting HR |
| `0x3B` | `activity_session.go` | Auto-detected sessions |
| `0x46` | `continuous_hr.go` | Continuous PPG HR (~1/sec) |
| `0x48`,`0x4E` | `sleep.go`, `sleep_nap.go` | Sleep sessions + naps |
| `0x49` | `hrv.go` | HRV RMSSD |

Timestamps come from the **per-page** `roundSegments` anchors, never a single global
`roundStart`. `ist.go` handles the IST day-key convention — a "day" is an IST
calendar day, not a UTC one, so day boundaries don't split a night.

### 2. Aggregate

`aggregate.go` + `dayacc.go` fold records into per-day accumulators (`DayAcc`).

`mergeSleep` picks a **single best main sleep per day** rather than summing every
session — naps are tracked separately. Overlapping or duplicate sessions from
re-fetched pages collapse instead of double-counting.

### 3. WriteBatch

`pipeline_store.go` → `internal/store`. Upserts, so re-uploading the same session is
idempotent. `row_validate.go` rejects impossible rows before they hit the DB.

## Re-parse

`POST /api/v1/reparse` (`reparse.go`) replays stored raw sessions through the current
parsers. **Disabled by default** — needs `REPARSE_ENABLED=true` and a separate
`REPARSE_SECRET`. It's a foot-gun: it rewrites history for every affected day.

## Coverage — the counterpart

`GET /api/v1/metrics/coverage` (`internal/store/coverage.go`) is what makes the
phone stateless. It returns:

| Field | Meaning |
|---|---|
| `dataThrough` | `MAX(ts)` across every sample/session table |
| `lastIngestAt` | Last successful ingest |
| `hasData` | Anything at all? |
| `types` | Per-type-code watermark |

The phone asks this before every sync and fetches only the gap. **The server is the
sole authority on what's already ingested** — no phone-side bookmarks, so a reinstall
resumes correctly.

## Tests

`pipeline_e2e_test.go`, `dump_integration_test.go`, and `dump_readiness_test.go`
replay real captured dumps. `parse_batch_test.go`, `continuous_hr_test.go`,
`spo2_test.go`, `series_test.go`, `aggregate_test.go` cover units.

Run: `go test ./...`

## Key files

| File | Role |
|---|---|
| `internal/api/ingest.go` | HTTP handler, multipart |
| `internal/parse/pipeline.go` | The 3-step orchestration |
| `internal/parse/parse_batch.go` | Blob → records dispatch |
| `internal/parse/catalog.go` | Type-code registry |
| `internal/parse/aggregate.go` | Records → day rollups |
| `internal/parse/ist.go` | IST day-key rules |
| `internal/store/coverage.go` | Watermark query |
