# Deployment overview

Source review: 7 September 2026. Describes the local code, not a live deployment check.

Read [the deployment guide](../local/deployment.md) for steps.

| Service | Source | Host access |
|---|---|---|
| `db` | `timescale/timescaledb:latest-pg16` | Docker network only |
| `api` | Backend Dockerfile at repo root | `127.0.0.1:8080` by default |
| `web` | Sibling `Heliolytics_Web` | `127.0.0.1:3000` by default |
| `cloudflared` | Tunnel connector | Outbound tunnel |

Compose lives in `deploy/docker-compose.yml`. The API waits for database health;
Web waits for API health. Web accesses Go at `http://api:8080` on the Docker network.

Cloudflare terminates public HTTPS. Public hostname routing is configured outside
these repos. The checked-in Compose file does not prove the live tunnel, DNS or
firewall configuration.

## What the scripts do

- `install.sh` creates `.env` if absent, builds source images and starts the stack.
- `deploy-web.sh` rebuilds only Web. `PULL=1` also pulls its checkout first.
- `reset-db.sh` deletes the named DB volume and starts fresh. Destructive.

Source is built locally by these scripts. Immutable CI images, automatic numbered
migrations, deployment rollback and backups are not implemented by this Compose file.

## Configuration

Go and Web share `HELIOLYTICS_SIGNING_SECRET`. Web separately requires
`HELIOLYTICS_WEB_PASSWORD_HASH` and `HELIOLYTICS_SESSION_SECRET`.
`POSTGRES_PASSWORD` is used by DB and Go; `CLOUDFLARE_TUNNEL_TOKEN` by the connector.
Reparse is disabled by default. `.env.example` is a template, not valid credentials.

Local phone access needs deliberate connectivity: a host-loopback port cannot
be reached by using the host's LAN address. The deployment guide uses Android
USB port forwarding for the local debug case.
