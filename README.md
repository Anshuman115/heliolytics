# Heliolytics API

Go server for the Heliolytics platform: ingest raw BLE sessions, parse metrics, serve REST API.

Part of a **3-repo system**:

| Repo | Stack |
|------|-------|
| **Heliolytics** (here) | Go · PostgreSQL · TimescaleDB |
| **Heliolytics_App** | Flutter · BLE |
| **Heliolytics_Web** | Next.js · dashboard |

## Quick start

Requires sibling checkout of `Heliolytics_Web` for the full Docker stack.

```bash
cd deploy
cp .env.example .env   # set HELIOLYTICS_SIGNING_SECRET, HELIOLYTICS_WEB_PASSWORD, POSTGRES_PASSWORD
./install.sh
```

- API (local): `http://127.0.0.1:8080/health`
- Web (local): `http://127.0.0.1:3000/login`
- Public HTTPS: Cloudflare Tunnel (`cloudflared` container) — see `deploy/deployment.md`

See `deploy/deployment.md` for full **dev** and **production** (VPS + domain) steps. See `deploy/DEV.md` for a short local Docker cheat sheet.

## Auth

Clients send `X-Heliolytics-Token: {unix_ts}.{nonce}.{hmac_sha256_hex}` where HMAC payload is `{ts}:{nonce}` signed with `HELIOLYTICS_SIGNING_SECRET`.

- **Flutter app:** stores the same secret as “API key” in Settings
- **Web dashboard:** password login only — signing secret stays in Docker env

Public token minting endpoint removed. `/api/v1/reparse` is disabled unless `REPARSE_ENABLED=true`.

## API only (no web)

```bash
go run ./cmd/server
```

Set `DATABASE_URL` and `HELIOLYTICS_SIGNING_SECRET` in the environment.

## Tests

```bash
go test ./...
```
