# Heliolytics API

Go backend for a personal wearable dashboard. It accepts raw band uploads,
parses them, stores the results in PostgreSQL, and serves the phone and web UI.

Source review: 7 September 2026. Describes the local code, not a live deployment check.

## Start reading

1. [Architecture](docs/local/ARCHITECTURE.md): follow an upload and a read request.
2. [Learn Go through this backend](docs/features/go-basics.md): syntax with real examples.
3. [Follow a Go request](docs/features/go-request-walkthrough.md): handler, query, JSON.
4. [Metrics API](docs/features/metrics-api.md): exact routes and client usage.
5. [Storage](docs/features/storage-and-schema.md): tables and updates.

## Three repositories

| Repo | Job |
|---|---|
| Heliolytics_App | Pair with the strap, fetch bytes, upload, display server results |
| Heliolytics | Parse bytes, store samples, calculate daily summaries, serve JSON |
| Heliolytics_Web | Fetch from Go on the Next.js server, then display charts |

The phone caches some parsed server responses. It does not decode historical
health blobs or own the server's sync progress.

## What exists

- Fourteen synced band types: activity, workouts, sleep, vitals, PAI and readiness.
- Raw uploads retained for replay; sample upserts avoid duplicate timestamps.
- Daily summaries calculated from stored rows inside the ingest transaction.
- Device readiness preferred over a calculated recovery fallback.
- Signed API requests for a single owner. No user accounts or AI integration.
- A VO2 max endpoint exists but returns no points because no source is implemented.

## Stack

`go.mod` declares Go 1.22 and pgx/v5. HTTP uses the Go standard library.
PostgreSQL with TimescaleDB stores data. sqlc generates part of the query layer.
Docker Compose runs the database, API, web app and tunnel connector.

## Run and verify

Read [deployment](docs/local/deployment.md) before starting a stack. It explains
configuration, phone connectivity, existing database upgrades and secret setup.

For a configured development environment:

```sh
go run ./cmd/server
go test ./...
go vet ./...
```

These are instructions, not results from this documentation review. Database
and dump-based tests may skip when their prerequisites are missing. Inspect skips
before calling an integration suite verified.

## Feature documentation

- [Ingest and parsing](docs/features/ingest-and-parsing.md)
- [Authentication](docs/features/auth-and-security.md)
- [Readiness calculation](docs/features/readiness-score.md)
- [Deployment overview](docs/features/deploy.md)
- [Schema design](docs/local/SCHEMA_DESIGN.md)
- [Engineering rules](CLAUDE.md)

[Apache License 2.0](LICENSE).
