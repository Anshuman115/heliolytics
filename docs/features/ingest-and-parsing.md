# Ingest and parsing

The phone uploads raw band bytes. The server decodes them into typed rows, stores
the immutable upload, and derives the daily, load and readiness rows in the same
transaction.

## Request

`POST /api/ingest` is multipart with three kinds of part:

- `session`: one JSON object with exactly `sessionId`, `startedAt`, `endedAt`,
  `deviceMac` and the optional `batteryPercent`. Timestamps are RFC3339 with an
  offset; a value without a zone is rejected. `sessionId` is printable ASCII up to
  64 characters and `deviceMac` is six upper-case hex pairs separated by colons.
- `catalog`: the catalog JSON. `chunked[]` entries carry `code`, `roundStart` and
  optional `roundSegments[]`; keys the parser does not read, such as `status`,
  `samples`, `bytes` or `file`, are ignored.
- `0xNN_raw.bin`: one part per submitted type. The type comes from the part name,
  never from a filename.

`internal/api/ingest.go` reads the envelope with `NextRawPart`, so no part is ever
spooled to a temporary file. The body is capped at 16 MiB and 40 parts. A
`Content-Transfer-Encoding` other than identity, a nested multipart part, a
duplicate part and an unknown part name are all 400. The request needs `session`,
`catalog` and at least one non-empty type part, and the catalog `sessionId`, when
present, must match the session.

## Transaction

`internal/ingest/ingest.go` runs the whole upload as one transaction:

1. Lock the canonical rows so two uploads cannot interleave their rollups.
2. Parse the catalog, then every submitted type.
3. Write the session row and its exact raw bytes.
4. Write the typed rows. Sleep and workout candidates land in their observation
   tables and the canonical winners are reconciled from the full candidate
   history.
5. Refresh the measured coverage watermarks from the canonical rows.
6. Run the rollups for the affected IST days, the detector for the changed
   intervals, and the readiness computation.
7. Commit and answer with the per-type result.

Any error rolls the transaction back, so a rejected upload leaves no session, no
raw bytes, no samples, no coverage and no daily row. The only side effect that can
survive is the consumed authentication nonce, because authentication completed
before the upload was attempted.

An identical retry of a stored session is accepted and writes nothing new. The
same `sessionId` with different bytes, catalog, envelope or type subset is 409
`session_conflict` and the stored state is untouched; the phone then retries a
per-type refetch under a new session id.

Session envelope timestamps are truncated to UTC microseconds for persistence,
retry comparison, observation ordering and replay. Raw payloads and catalog
timestamps are not rewritten.

Automatic activity recomputation includes every affected existing auto interval
and grows context until missing minutes, workout-overlapping minutes or more than
three consecutive below-cadence minutes reset detection. Reads use bounded chunks,
but chunk edges never truncate activities. Only auto rows are replaced; recorded
workouts and canonical samples are unchanged.

## Types

| Code | Meaning | Parser file |
|---|---|---|
| `0x01` | Activity minute: heart rate and steps | `activity.go`, `activity_hr.go`, `steps_series.go` |
| `0x05` | Workout summary protobuf | `workout.go`, `workout_proto.go` |
| `0x06` | Workout detail blocks | `workout_detail.go`, `workout_detail_timestamps.go` |
| `0x0D` | PAI records | `pai.go` |
| `0x13` | Stress minutes | `series.go` |
| `0x25` | Spot SpO2 | `spo2.go` |
| `0x26` | Sleep SpO2 or breathing placeholders | `spo2.go` |
| `0x2E` | Skin temperature minutes | `temp_samples.go` |
| `0x38` | Respiration records | `series.go` |
| `0x46` | Continuous heart rate seconds | `continuous_hr.go` |
| `0x48` | Main sleep and naps | `sleep.go`, `sleep_nap.go` |
| `0x49` | HRV records | `series.go` |

Two details matter for correctness. A `0x01` step byte of `0xff` means the strap
recorded nothing, while a zero is a measured zero and stays a row. `0x06` only
enriches a `0x05` summary in the same upload; on its own it produces no workout.
`0x26` is retained raw-only: it returns a warning, writes no SpO2 row and never
advances SpO2 coverage. Real `0x25` rows are the only measured SpO2 source.

A summary-only workout can displace a detailed winner. Summary uploads therefore
refresh both workout coverage watermarks, including clearing detail coverage when
no detailed winner remains. The response still lists only submitted types.

## Anchors and timestamps

Minute types use the catalog's `(byteOffset, roundStart)` pairs. The first offset
must be 0, later offsets must be strictly increasing, every offset must be inside
the payload and aligned to the type's stride, and every `roundStart` must be a
whole minute in an offsetless IST wall clock. A type without an anchor fails with
`missing_anchor`; there is no `fetchEnd` fallback and no inferred anchor.

Record timestamps are absolute epochs inside the record, or minutes after the
anchor. Day keys are computed in IST. A sleep row carries the day key of the
device's sleep midnight, and a corrected or later observation moves it.

## Results

The response carries `sessionId`, `complete`, type results and warnings. Each type
result has `status`, `rows`, `dropped`, `trailingBytes`, `warnings` and
`dataThrough`. `status` is `accepted`, `partial` or `raw_only`; `complete` is false
only when a type reports `partial`. `dataThrough` is the greatest canonical
watermark for that type after the commit, never the parser's optimistic
`ProcessedThrough`.

## Daily domain

A `daily_metrics` row exists only while a canonical row supports that IST day:
a sample, a canonical sleep session, an activity or PAI. Unknown metrics in a
supported day stay null. A day nothing supports is removed, even if raw bytes or a
discarded candidate still mention it, and the dependent load and readiness rows
are recomputed before the removal. Corrections therefore shrink the domain as well
as change values. Readiness gaps stay virtual: the API never generates zero or
`no_data` rows up to today.

## No reparse route

There is no reparse endpoint and no in-place replay. Raw bytes are retained so a
parser change can be validated offline: `cmd/rebuild` replays a disposable test
database into a fresh one, and it refuses production resources and any non-empty
target. It is not shipped in the production image and is not a recovery path.

## Checks for code changes

Parser tests use `internal/parse/testdata/`; the committed June set is a real
sensor capture with independently read Zepp rows, and a missing fixture fails the
test instead of skipping it. Store and ingest tests need the disposable database
from `internal/testutil`. `internal/integration` drives real signed HTTP through
the production mux over that database.

Key files: `internal/api/ingest.go`, `internal/parse/parse_batch.go`,
`internal/parse/catalog_validation.go`, `internal/ingest/ingest.go`,
`internal/ingest/rebuild.go`, `internal/rollup/run.go`, `internal/store/coverage.go`.

Validation: not run for this revision.
