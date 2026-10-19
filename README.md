# Heliolytics API

Go backend for a personal wearable dashboard. It accepts raw band uploads, parses
them, stores the results in PostgreSQL with TimescaleDB, and serves the phone and
web clients.

## What the service does

- Ingests fourteen band data types covering activity, workouts, sleep, vitals, PAI
  and readiness.
- Retains raw uploads so a parser change can reprocess stored bytes.
- Writes daily summaries from stored rows inside the ingest transaction.
- Prefers the device readiness value and falls back to a calculated recovery score.
- Authenticates a single owner with HMAC-signed requests. No user accounts, no
  third-party AI service.
- Returns no points from the VO2 max endpoint because no source is implemented.

## Three repositories

| Repo | Responsibility |
|---|---|
| heliolytics-app | Pair with the strap, fetch bytes, upload, display server results |
| heliolytics | Parse bytes, store samples, calculate daily summaries, serve JSON |
| heliolytics-web | Read the API on the Next.js server, then render charts |

The phone caches parsed responses. It does not decode historical health blobs and
does not own sync progress; the server does.

## Read the code

| Document | Read for |
|---|---|
| [Learn Go through this backend](docs/features/go-basics.md) | Language basics with examples from this code |
| [Follow a Go request](docs/features/go-request-walkthrough.md) | Handler, query and JSON response path |
| [Metrics API](docs/features/metrics-api.md) | Routes, parameters and client usage |
| [Storage and schema](docs/features/storage-and-schema.md) | Tables, migrations and retention |
| [Ingest and parsing](docs/features/ingest-and-parsing.md) | Upload handling and per-type parsers |
| [Authentication](docs/features/auth-and-security.md) | Request signing, nonce handling, secret sources |
| [Readiness score](docs/features/readiness-score.md) | Inputs, weights and thresholds |
| [Deployment](docs/features/deploy.md) | Compose services and configuration |

## Stack

The module path is `github.com/Anshuman115/heliolytics`. `go.mod` records Go
1.26.9 as the minimum Go version, requires pgx/v5 v5.9.2, and lists
`golang.org/x/text` v0.41.0 as an indirect requirement. HTTP uses the standard
library. PostgreSQL with TimescaleDB stores the data, sqlc generates part of the
query layer, and Docker Compose runs the database, adminer, the API, the web app
and the tunnel connector.

## Run and verify

The commands below need Go 1.26.9. Running the server also needs the database and
configuration described in `deploy/`.

```sh
go run ./cmd/server
go test -count=1 ./...
go vet ./...
```

`-count=1` disables the test cache, so results are not reused from earlier runs.
Database integration tests need `TEST_DATABASE_URL` pointing at a throwaway
TimescaleDB initialized from `schema.sql`. Raw-fixture tests skip when their dump
files are absent. Skipped tests do not cover those cases.

Before a release, run `govulncheck ./...` and `govulncheck -mode=binary` on the
built Linux ARM64 binary. Both must report no vulnerabilities.

Validation: not run for this revision. Database and dump-based tests skip when
their prerequisites are absent, so read the skip messages from `go test -v`
before treating an integration suite as verified.

## Local notes

Architecture, schema-design, deployment and agent-instruction notes live in the
local-only `docs/local/` directory and are not part of this repository.

[Apache License 2.0](LICENSE).
