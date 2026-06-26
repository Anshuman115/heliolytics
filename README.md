# Heliolytics API

**Go ingestion + analytics backend for a self-hosted wearable-health platform.**
It accepts raw Bluetooth session bytes from the mobile client, decodes ~20 binary
health streams into typed time-series, stores them in PostgreSQL/TimescaleDB, computes
daily rollups and a science-backed recovery score, and serves a REST metrics API.

> **This repository is the backend.** It pairs with a Flutter BLE client and a Next.js
> dashboard — see [System architecture](#system-architecture).

---

## Engineering highlights

- **Binary protocol decoding** — per-data-type parsers for the wearable's on-flash
  formats: per-minute activity & heart rate, sleep sessions + stage timelines, HRV,
  SpO₂, skin temperature, respiratory rate, stress, PAI, and workouts (including a
  hand-written protobuf decode for workout summaries). Two distinct timestamp models
  (round-relative vs absolute) are handled explicitly.
- **Idempotent ingest** — per-minute samples are keyed by timestamp, so re-syncing the
  overlap window is a no-op; daily step totals are **recomputed** from per-minute rows
  rather than accumulated, eliminating a double-count class of bug. Verified by tests.
- **Server-authoritative sync** — a per-type **coverage** endpoint tells the thin client
  exactly how far each data type has been ingested, so fetch windows live on the server
  and survive client reinstalls.
- **Science-backed recovery score** — daily readiness from HRV (`ln(RMSSD)`), resting HR,
  sleep, and respiratory rate vs a personal rolling baseline (7-day mean / 60-day SD),
  following HRV-guided-training research (Plews et al.).
  Prefers the device's own readiness when present, falls back to the computed score, and
  gates a provisional score until enough baseline exists. Pure, unit-tested scoring in
  [`internal/readiness`](internal/readiness).
- **Type-safe data layer** — queries generated with `sqlc`; TimescaleDB hypertables for
  high-rate per-minute series, plain tables for daily rollups and sessions.
- **HMAC-authenticated**, Dockerized, deployable to a VPS behind a Cloudflare Tunnel.

---

## System architecture

| Repo | Role | Stack |
|------|------|-------|
| **Heliolytics** (this repo) | Ingest, parse, store, metrics API | Go · PostgreSQL + TimescaleDB · sqlc · Docker |
| **Heliolytics_App** | BLE sync + health UI | Flutter · Dart · Riverpod |
| **Heliolytics_Web** | Web dashboard | Next.js |

```
Flutter client ──POST /api/v1/ingest (raw bytes, HMAC)──►  Go API
                                                             │  parse ~20 binary streams
                                                             ▼
                                              PostgreSQL / TimescaleDB
                                              (per-minute hypertables + daily rollups)
                                                   ╱                      ╲
                                   GET /api/v1/metrics/*            Next.js dashboard
                                   (Flutter app)
```

---

## API surface

| Method | Path | Returns |
|--------|------|---------|
| POST | `/api/v1/ingest` | accept a raw sync session (multipart) → parse & store |
| GET | `/api/v1/metrics/days` | daily rollups (steps, sleep, recovery, vitals) |
| GET | `/api/v1/metrics/series` | per-minute series (stress, HRV, SpO₂, RHR, resp…) |
| GET | `/api/v1/metrics/hr` | per-second/continuous heart rate |
| GET | `/api/v1/metrics/sleep` | sleep sessions + stage timelines |
| GET | `/api/v1/metrics/temperature` | skin-temperature series |
| GET | `/api/v1/metrics/workouts` | workout summaries |
| GET | `/api/v1/metrics/activity-sessions` | auto-detected activity sessions |
| GET | `/api/v1/metrics/coverage` | per-type data-through watermarks (drives client fetch) |
| GET | `/health` | liveness |

`/api/v1/reparse` (replay stored raw blobs through the current parsers) is disabled unless
`REPARSE_ENABLED=true`.

---

## Data model

- **Hypertables** (per-minute / high-rate): `heart_rate_samples`, `step_samples`,
  `temperature_samples`, `health_samples` (stress / HRV / SpO₂ / RHR / resp…).
- **Daily / session tables**: `daily_metrics` (rollups + recovery score),
  `sleep_sessions` (+ stage JSON), `workouts`, `activity_sessions`, plus `sync_sessions`
  and `raw_type_blobs` for replayable raw ingest.

Schema in [`schema.sql`](schema.sql); rationale in
[SCHEMA_DESIGN.md](SCHEMA_DESIGN.md). Queries are `sqlc`-generated into `internal/store/db`.

---

## Tech stack

Go 1.22 · PostgreSQL + TimescaleDB · `sqlc` · `pgx/v5` · Docker / docker-compose ·
Cloudflare Tunnel · HMAC-SHA256 auth

---

## Quick start

Full Docker stack (API + DB + web; requires a sibling `Heliolytics_Web` checkout):

```bash
cd deploy
cp .env.example .env   # set HELIOLYTICS_SIGNING_SECRET, POSTGRES_PASSWORD, HELIOLYTICS_WEB_PASSWORD
./install.sh
```

API only:

```bash
DATABASE_URL=postgres://… HELIOLYTICS_SIGNING_SECRET=… go run ./cmd/server
```

Tests:

```bash
go test ./...    # store/integration tests skip automatically without a DATABASE_URL
```

- API health: `http://127.0.0.1:8080/health`
- Web: `http://127.0.0.1:3000/login`
- Public HTTPS via Cloudflare Tunnel — see [deploy/deployment.md](deploy/deployment.md).

---

## Auth & security

Clients send `X-Heliolytics-Token: {unix_ts}.{nonce}.{hmac_sha256_hex}`, where the HMAC
payload `{ts}:{nonce}` is signed with `HELIOLYTICS_SIGNING_SECRET`.

- **Flutter app** stores the same secret and mints short-lived tokens per request.
- **Web dashboard** uses password login; the signing secret never leaves Docker env.
- No public token-minting endpoint; `reparse` is opt-in only.

See [ARCHITECTURE.md](ARCHITECTURE.md) for the ingest → parse → store → serve pipeline.
