# Feature — Deploy

Docker Compose stack. Full step-by-step lives in the tracked
`deploy/deployment.md` — this is the shape and the traps.

## Stack

`deploy/docker-compose.yml`, four services:

| Service | Image / build | Port |
|---|---|---|
| `db` | `timescale/timescaledb:latest-pg16` | internal |
| `api` | builds `..` (this repo) | `127.0.0.1:8080` |
| `web` | builds `../../Heliolytics_Web` | `127.0.0.1:3000` |
| `cloudflared` | `cloudflare/cloudflared:latest` | — |

### The sibling-clone requirement

`web` builds from `../../Heliolytics_Web`. **All three repos must be cloned as
siblings under one parent** or compose can't find the web source and the build
fails. This is the most common first-deploy failure.

## Ingress — no open ports

HTTPS terminates at **Cloudflare** (orange proxy). The VPS runs `cloudflared`, which
dials out to establish the tunnel.

There is **no Caddy** and **no open 80/443** on the host. `api` and `web` bind to
`127.0.0.1` only — they are unreachable from the internet except through the tunnel.

> Any doc or note claiming Caddy terminates TLS is stale.

Because Cloudflare is in front, `api` sets `TRUST_PROXY: "true"` so rate limiting
keys off the forwarded client IP rather than the tunnel's. See
[auth-and-security.md](auth-and-security.md) — that flag is only safe *because* a
trusted proxy is genuinely in front.

## Startup ordering

Health-gated, not timing-guessed:

- `api` waits for `db` → `service_healthy`
- `web` waits for `api` → `service_healthy`
- `api` healthcheck: `wget -qO- http://localhost:8080/health`
- `web` healthcheck: `wget -qO- http://127.0.0.1:3000/login` (30 s `start_period`)

`/health` is outside the auth chain precisely so the healthcheck can reach it.

## Secrets

Env only — never committed:

| Var | Used by |
|---|---|
| `HELIOLYTICS_SIGNING_SECRET` | `api` + `web` — **must match** |
| `POSTGRES_PASSWORD` | `db` + `api` DSN |
| `HELIOLYTICS_WEB_PASSWORD` | `web` login |
| `REPARSE_ENABLED` / `REPARSE_SECRET` | `api`, default off |
| `API_PORT` / `WEB_PORT` | Host bind, default 8080 / 3000 |

`web` reaches the API in-network via `API_INTERNAL_URL: http://api:8080` — server-side
only, never exposed to the browser.

## Scripts

| Script | Does |
|---|---|
| `deploy/install.sh` | First-time host setup |
| `deploy/deploy-web.sh` | Rebuild **only** web — db/api stay up, no API downtime |
| `deploy/reset-db.sh` | **Destructive.** Drops and re-applies schema |

## Local dev

`deploy/DEV.md` (local-only) covers the dev loop. `docker compose up` from `deploy/`
with a populated `.env`.

## Data safety

`pgdata` is a named volume — it survives `docker compose down`, but **not**
`down -v`. That flag deletes every ingested night. `raw_type_blobs` means a wipe is
recoverable only if the strap still holds the data or you have a dump.
