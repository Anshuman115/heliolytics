# Deployment and operations

One host running Docker Compose. The backend checkout and the web checkout sit side
by side:

```text
projects/
  heliolytics/       this repository
  heliolytics-web/   the web dashboard
```

## Quick start

```sh
cd projects/heliolytics
deploy/install.sh          # creates deploy/.env and generates every secret
deploy/deploy-all.sh       # builds and starts db, api and web
deploy/deploy-cloudflare.sh   # optional: starts the Cloudflare tunnel connector
```

`install.sh` asks for the web dashboard password and, optionally, the Cloudflare
tunnel token. It never overwrites a value that is already set, so it is safe to run
again. Configure the phone app with `HELIOLYTICS_PHONE_API_KEY` from `deploy/.env`.

## Scripts

| Script | Does | Leaves alone |
|---|---|---|
| `deploy/install.sh` | Creates `deploy/.env` and fills empty secrets | existing values |
| `deploy/deploy.sh` | Rebuilds and restarts `api` | database, web, tunnel, Adminer |
| `deploy/deploy-all.sh` | Rebuilds and starts `db`, `api` and `web` | data, tunnel, Adminer |
| `deploy/reset-db.sh` | Deletes the database volume, then rebuilds `db`, `api` and `web` | tunnel, Adminer |
| `deploy/deploy-cloudflare.sh` | Starts or updates the tunnel connector | everything else |

`deploy.sh` and `deploy-all.sh` refuse a checkout that is not on a clean `main`.
They write the deployed commits and build time into `deploy/.env`, validate the
configuration, and only then build. `reset-db.sh` asks you to type `DELETE`.

## Services

| Service | Container | Image or build | Port |
|---|---|---|---|
| `db` | `heliolytics-db` | `timescale/timescaledb:2.30.2-pg16` | none |
| `api` | `heliolytics-api` | `Dockerfile` at the repo root | `127.0.0.1:8080` |
| `web` | `heliolytics-web` | `../heliolytics-web/Dockerfile` | `127.0.0.1:3000` |
| `adminer` | `heliolytics-adminer` | `adminer:6.1.2-standalone`, `tools` profile | `127.0.0.1:8081` |
| `cloudflared` | `cloudflared` | `cloudflare/cloudflared:2026.10.1`, own project | none |

`db`, `api`, `web` and `adminer` belong to the Compose project named by
`COMPOSE_PROJECT_NAME` in `deploy/.env` (default `heliolytics`). The database volume
is `<project>_pgdata`. Start Adminer when you need it:

```sh
docker compose --project-name <project> --env-file deploy/.env \
  --file deploy/docker-compose.yml --profile tools up -d adminer
```

Networks:

- `data` is internal and carries API to database traffic. `DATA_SUBNET` must match
  the host lines in `deploy/pg_hba.conf`.
- `application` is internal and carries web to API traffic.
- `ingress` is a bridge network with egress, named `<project>_ingress`. The tunnel
  connector joins it at `CLOUDFLARED_IP`.
- `tools` is a bridge network for the Adminer loopback port.

## Cloudflare tunnel

`deploy/cloudflared/docker-compose.yml` runs the connector as a separate Compose
project named `cloudflared`. One tunnel can therefore serve other applications on
the same host, and no Heliolytics script stops or recreates it.

1. In Cloudflare Zero Trust, create a tunnel and copy its token into
   `CLOUDFLARE_TUNNEL_TOKEN`.
2. Add public hostnames with the origins `http://api:8080` and `http://web:3000`.
3. Run `deploy/deploy-cloudflare.sh` after `deploy/deploy-all.sh` has created the
   ingress network.

To also route services that run directly on the host, copy
`deploy/cloudflared/docker-compose.override.example.yml` to
`deploy/cloudflared/docker-compose.override.yml`, use
`http://host.docker.internal:<port>` as their origin, and allow `INGRESS_SUBNET`
to reach those ports in the host firewall. The override file is not committed.

## Configuration

`deploy/.env` is the only secret file and stays at mode 0600. `deploy/.env.example`
shows the command that generates each value. The deploy scripts run
`deploy/validate-env.sh`, which checks the raw file and the resolved Compose
configuration:

| Variable | Rule |
|---|---|
| `COMPOSE_PROJECT_NAME` | Compose project for `db`, `api`, `web` and `adminer` |
| `HELIOLYTICS_COMMIT`, `HELIOLYTICS_WEB_COMMIT` | full lowercase 40-character SHA, written by the deploy scripts |
| `HELIOLYTICS_BUILT_AT` | RFC3339 UTC, written by the deploy scripts |
| `POSTGRES_PASSWORD`, `APP_DB_PASSWORD` | 64 lowercase hex, distinct |
| `HELIOLYTICS_WEB_API_KEY` | `web.<64 lowercase hex>` |
| `HELIOLYTICS_PHONE_API_KEY` | `phone.<64 lowercase hex>`, distinct |
| `HELIOLYTICS_SESSION_SECRET` | 64 lowercase hex, distinct from both API secrets |
| `HELIOLYTICS_WEB_PASSWORD_HASH` | one single-quoted `scrypt$N=32768$r=8$p=3$<salt>$<hash>` line |
| `TRUST_PROXY`, `TRUSTED_PROXY_CIDRS` | `true` and `<CLOUDFLARED_IP>/32` |
| `COOKIE_SECURE` | `true` |

To change the web password, empty `HELIOLYTICS_WEB_PASSWORD_HASH`, run
`deploy/install.sh`, then `deploy/deploy-all.sh`.

## Health and readiness

`GET /health` returns `200 ok` without touching the database. `GET /version` returns
the commit and build time compiled into the binary.

The compose healthchecks are stronger:

- `db` logs in over loopback as the owner role and runs `SELECT 1`.
- `api` runs `/usr/local/bin/api -healthcheck`, which accepts only `200 ok` from
  loopback `/health` within two seconds.
- `web` fetches its own `/login` route.

None of them proves the schema, grants or ingest readiness. Empty database
initialization does that.

## Database initialization

The database image runs the mounted init files on an empty volume, in order:

1. `deploy/initdb/00_roles.sh` creates `heliolytics_app` with `LOGIN`, no superuser,
   no createdb and no createrole. `APP_DB_PASSWORD` reaches SQL as one quoted
   literal.
2. `schema.sql` creates extensions, tables, indexes, constraints, views, functions
   and grants.

The role exists before `schema.sql` grants anything to it.

Before the first request, every public application table is empty:

```sh
docker exec heliolytics-db sh -c \
  'PGPASSWORD="$POSTGRES_PASSWORD" psql -h 127.0.0.1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Atc \
  "SELECT relname, n_live_tup FROM pg_stat_user_tables ORDER BY relname"'
```

Health data enters only through signed `phone` uploads. Signed `web` reads may create
nonce rows and nothing else.

The entrypoint starts a temporary server for initialization with the initialization
copy of `pg_hba.conf`. The real server then uses the mounted `postgresql.conf` and
`pg_hba.conf`. A missing bind mount fails the start instead of running an empty
schema.

`deploy/postgresql.conf` sets 512MB `shared_buffers`, 1536MB
`effective_cache_size`, 4MB `work_mem`, 128MB `maintenance_work_mem`, 40
connections and four Timescale background workers. TimescaleDB telemetry is off.

## Network and transport threat model

The database has no host port. It is reachable only from containers on the internal
`data` network. `pg_hba.conf` accepts SCRAM logins for the owner and app roles from
loopback and `DATA_SUBNET`, then rejects everything else.

`ssl = off`, so API to database traffic crosses a Docker bridge in plaintext. SCRAM
authenticates but does not encrypt. This is acceptable only for a single host whose
database network is unpublished and whose Docker and host root users are trusted.

## Proxy trust

`TRUST_PROXY=true` and `TRUSTED_PROXY_CIDRS` naming only the connector address. The
API trusts a single `CF-Connecting-IP` header only from that socket peer. A missing,
duplicate or malformed header from the trusted peer returns 400. Other peers are
limited by their socket address with forwarded headers ignored.

This holds only while the API loopback port is reachable from the internet solely
through the connector.

## Images and updates

Pinned references and their registry child digests, resolved on 2026-10-20:

| Role | Reference | linux/arm64 child | linux/amd64 child |
|---|---|---|---|
| Go builder | `golang:1.26.9-alpine@sha256:3082400e369fa24d5fc60bca20edab3f6d604e0c5a690ec66b295eff4dd87ade` | `sha256:dccc7a8faa4dbaabed38c2ebb7a7bd24c342f2b0ee0fc1a0d2bd41b0243513ad` | `sha256:397ecc648150dd585f1734dfcd8d1bb4746664e54464191d77ad0ecffaf38bdb` |
| API runtime | `gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab` | `sha256:06c3c14b7fa252d9f4285242d2d665ce3b51f74b02d3c529464f220f2912aecb` | `sha256:52dcfbabb7457ea47c82f6e13af8c8a4a1d9f7b0145142b3ecab20f2b888411d` |
| Database | `timescale/timescaledb:2.30.2-pg16@sha256:6f139d56042989bd35f50ba5986e492cd32e35cc6778c50be2659add298dc09f` | `sha256:fc16b2459b5faf9f84c010c189cfd77267bbadf1514c3bfd20215eebf41fe5e4` | `sha256:8fc0911ce6c822a54342ba5e35556e0e28e22d1cab35696cfd007fec7de57069` |
| Adminer | `adminer:6.1.2-standalone@sha256:49a7d30e71de97fb18933ffe7d46cc2b343c49fd7a63a7791c8be2548eb65115` | `sha256:9ad6244d96b308f877bee41b4543a4de984084093cfe3252d7a7194218253e11` | `sha256:29642e5502fbfc41e31c3f590fc8ffed6b860fc1ffb86c66341ba07556068306` |
| Cloudflared | `cloudflare/cloudflared:2026.10.1@sha256:a15a3b50b8dabd57ac2090e9484c0247dba9dfa1f0df4d298d4317f9d69eb2d5` | `sha256:6d9f470f8df4161f90df39cd97719cdb8d02a8ade3885ab9b0ec15a495820b79` | `sha256:3c0502d2da137286136b44f5b1d0d0be7030cc4833db9f53cff86beadad6cefa` |
| Web Node | `node:24.22.0-alpine3.24@sha256:44fce696f38ba8c987c4db89954051000bbd05ffd8d8659ba15ed4a8602e937f` | `sha256:5b12456322560dcf9342f7339662bccf8c5665cf0732a8ec8363687aa93c5277` | `sha256:429bdfbc204b4eabc6438e6d34e4e9d8dd2ca1df8289bcd78cdd3c5db387b7a5` |

Updating an image is a deliberate change: pick an exact published tag, resolve the
multi-arch index digest with `docker buildx imagetools inspect`, confirm the
`linux/arm64` child, update compose or the Dockerfile, and rebuild.

The API image compiles only `./cmd/server` and ships only `/usr/local/bin/api`.
Build metadata arrives as `GIT_SHA` and `BUILD_TIME` build args. `/version` reports
them.

## Backups

`deploy/backup-db.sh` dumps the database of the project named in `deploy/.env`,
copies the dump to `BACKUP_DESTINATION` over SSH and verifies its SHA-256. Copy
`deploy/backup.env.example` to `/etc/heliolytics/backup.env` and edit the paths. The
units in `deploy/systemd/` run it nightly; set `User`, `Group` and
`WorkingDirectory` in the service to your deploy account first:

```sh
sudo install -Dm700 deploy/backup-db.sh /usr/local/libexec/heliolytics-backup
sudo install -Dm600 deploy/backup.env.example /etc/heliolytics/backup.env
sudo install -m644 deploy/systemd/heliolytics-backup.service deploy/systemd/heliolytics-backup.timer /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now heliolytics-backup.timer
```

## Tests

`deploy/tests/deploy-ops-test.sh` runs on Linux with Docker. It exercises the
installer, the validator, the script refusals, fresh initialization, role grants,
a backup and a real restore, using only `heliolytics-d01-*` resources.

Validation: `deploy/tests/deploy-ops-test.sh` passed (66 passed, 1 owner-pending SSH
copy) on 2026-10-21.
