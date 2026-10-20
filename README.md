# Heliolytics API

Go backend for a personal wearable dashboard. It accepts raw band uploads, parses
them, stores the results in PostgreSQL with TimescaleDB, and serves the phone and
web clients.

## What the service does

- Ingests twelve band data types: activity minutes, workout summaries and details,
  PAI, stress, spot and sleep SpO2, skin temperature, respiration, continuous HR,
  main sleep with naps, and HRV.
- Retains every accepted raw upload so a parser change can reprocess the stored
  bytes.
- Writes daily summaries from canonical stored rows inside the ingest transaction.
  A day exists only while a stored observation supports it, unknown metrics stay
  null instead of zero, and a discarded observation never creates a day.
- Stores computed readiness for each IST day together with its components and
  status. A day without enough inputs reports that status instead of a guessed
  score.
- Publishes eleven measured coverage codes. `0x26` is retained raw-only and never
  counts as SpO2 data.
- Authenticates two fixed credentials with HMAC-signed requests: `phone` may read
  and write, `web` may only read. There are no user accounts and no third-party AI
  service.
- Takes no historical import. A fresh database receives health and profile data
  only through signed app uploads, never through a dump, export or rebuild.

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
| [Storage and schema](docs/features/storage-and-schema.md) | Tables, schema v4 and retention |
| [Ingest and parsing](docs/features/ingest-and-parsing.md) | Upload handling and per-type parsers |
| [Authentication](docs/features/auth-and-security.md) | Signing, nonces, credential scopes and configuration |
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
Database integration tests need `TEST_DATABASE_URL` (the disposable maintenance
database `heliolytics_test_admin`), `HELIOLYTICS_TEST_DB_CONTAINER_ID` and
`HELIOLYTICS_TEST_DB_IMAGE`. Each test creates its own database and applies
`schema.sql`. Tests fail, never skip, when a database setting or fixture is
missing.

Before a release, run `govulncheck ./...` and `govulncheck -mode=binary` on the
built Linux ARM64 binary. Both must report no vulnerabilities.

## Local notes

Architecture, schema-design, deployment and agent-instruction notes live in the
local-only `docs/local/` directory and are not part of this repository.

[Apache License 2.0](LICENSE).

Validation: not run for this revision.
