# Ingest and parsing

Source review: 7 September 2026. Describes the local code, not a live deployment check.

Flutter uploads raw bytes. Go turns them into typed observations, then daily rows.

## HTTP boundary

`POST /api/v1/ingest` accepts multipart parts named `session`, `catalog` and
`0xNN_raw.bin`. The session identifies the upload; the catalog describes type
metadata and per-page `roundSegments` anchors.

`internal/api/ingest.go` caps the full body at 128 MiB with `http.MaxBytesReader`.
`ParseMultipartForm` uses a 32 MiB file-memory threshold and can spill to temporary
files. That threshold is not the whole handler's peak memory budget: blobs are
subsequently read into memory for parsing. Incomplete file reads reject the upload.

## Processing

1. `ParseBlobs` decodes the catalog, validates anchors and dispatches raw types.
2. `Aggregate` groups parsed records by destination table. It does not compute
   daily summaries.
3. `WriteBatch` starts a transaction, writes session/raw/parsed rows, then runs
   `internal/rollup` against those stored rows. Calculated recovery runs last.
4. Commit returns success. A normal ingest write or rollup error rolls back the
   transaction, including raw/session rows.

## Type map

| Code | Meaning | Main parser files under `internal/parse` |
|---|---|---|
| `0x01` | Minute activity, steps, HR, automatic sessions | `activity.go`, `activity_hr.go`, `activity_sessions.go` |
| `0x05`, `0x06` | Workout summary and detail | `workout.go`, `workout_proto.go`, `workout_detail.go` |
| `0x0D` | PAI | `pai.go` |
| `0x13` | Stress | `stress.go` |
| `0x25`, `0x26` | Spot and sleep SpO2 | `spo2.go` |
| `0x2E` | Skin temperature | `temperature.go`, `temp_samples.go` |
| `0x38` | Breathing | `series.go` |
| `0x39` | Device readiness | `readiness.go` |
| `0x3A` | Resting HR | `resting_hr.go` |
| `0x46` | Continuous HR | `continuous_hr.go` |
| `0x48` | Main sleep and naps | `sleep.go`, `sleep_nap.go` |
| `0x49` | HRV | `hrv.go` |

The parsed workout detail does not imply a separate workout-HR series endpoint.
Consult the stored fields and client usage before promising a new chart.

## Timestamps

Round-relative data uses each page's `(byteOffset, roundStart)` pair. A single
legacy `roundStart` applies only if no segment list is supplied. Offsets must be
in bounds, stride-aligned and strictly increasing. Anchor timestamps must be
valid and plausible. Invalid metadata rejects ingest before database writes.

Activity can use `fetchEnd` only when no device anchor exists and the complete
derived range passes validation. An unusable fallback rejects the ingest.
Day keys use IST conventions; the sleep parser assigns a session's day. A date
label and a timestamp are different values.

## Coverage and replay

Coverage returns `dataThrough`, `lastIngestAt`, `hasData` and fourteen type
watermarks. The phone plans fetches from this server state, with overlap and
fallback windows. It does not hold durable sync bookmarks.

`POST /api/v1/reparse` selects the latest stored session, not all retained history.
It is disabled unless `REPARSE_ENABLED=true`; the additional `X-Reparse-Secret`
check applies only when a non-empty `REPARSE_SECRET` is configured. Use a separate
secret when enabling it. The handler has a pre-ingest reset step, so do not extend
the normal-ingest atomicity claim to the entire reparse handler.

## Checks to use when code changes

Catalog and parser tests cover input handling. Transaction/rollup tests cover
writes and summaries. Dump replay needs local captured inputs; DB tests need a
test database. A suite that skips those inputs does not verify them.

Key files: `internal/api/ingest.go`, `internal/parse/pipeline.go`,
`internal/parse/aggregate.go`, `internal/parse/write_batch.go`,
`internal/store/tx.go`, `internal/store/coverage.go`.
