# Deployment and operations

This page is the owner runbook for one Heliolytics host. It describes the code in
this repository. It does not prove the live host state: every owner action below is
unrun until the owner records it.

## Services and names

Run every command from the backend checkout with the canonical paths:

```sh
cd ~/projects/heliolytics
docker compose --project-name deploy --env-file deploy/.env \
  --file deploy/docker-compose.yml <command>
```

| Service | Image or build | Port | Networks | Profile |
|---|---|---|---|---|
| `db` | `timescale/timescaledb:2.30.2-pg16` | none | `data` | default |
| `api` | `Dockerfile` at the repo root | `127.0.0.1:8080` | `data`, `application`, `ingress` | default |
| `web` | sibling `heliolytics-web/Dockerfile` | `127.0.0.1:3000` | `application`, `ingress` | default |
| `adminer` | `adminer:6.1.2-standalone` | `127.0.0.1:8081` | `data`, `tools` | `tools` |
| `cloudflared` | `cloudflare/cloudflared:2026.10.1` | none | `ingress` | `tunnel` |

The compose volume key is `pgdata`, so project `deploy` uses the existing volume
`deploy_pgdata`. Never rename the project or the volume key on the live host.

Local work starts only `db`, `api` and `web`. The tunnel and Adminer start only when
their profile is named:

```sh
docker compose --project-name deploy --env-file deploy/.env \
  --file deploy/docker-compose.yml --profile tools up -d adminer
```

Networks:

- `data` is internal and carries API to database traffic. The subnet is
  `DATA_SUBNET` (default `172.30.0.0/24`) and must match the host lines in
  `deploy/pg_hba.conf`.
- `application` is internal and carries web to API traffic.
- `ingress` is a bridge network with egress. The connector has the static address
  `CLOUDFLARED_IP` (default `172.28.0.10`); `TRUSTED_PROXY_CIDRS` must contain
  exactly that address as a `/32`.
- `tools` is a bridge network so the published Adminer loopback port works.

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
`linux/arm64` child, update compose or the Dockerfile, and rebuild. Never use
`latest`, a floating major tag or an untested Timescale major.

The API image compiles only `./cmd/server` and ships only `/usr/local/bin/api`. It
contains no rebuild CLI, no test seed, no fixtures, no shell and no package tools.
Build metadata arrives as `GIT_SHA` and `BUILD_TIME` build args, not runtime
environment variables, and `/version` reports them.

## Configuration

`deploy/install.sh` copies `deploy/.env.example` to `deploy/.env` with mode 0600 and
stops. It never builds, pulls, starts, generates secrets or contacts the network,
and it never overwrites an existing `.env`.

`deploy/.env.example` is invalid on purpose. Generate the private values yourself:

```sh
openssl rand -hex 32
```

`deploy/validate-env.sh` checks the raw file and then the resolved compose
configuration:

```sh
deploy/validate-env.sh \
  --project deploy \
  --env-file "$PWD/deploy/.env" \
  --compose-file "$PWD/deploy/docker-compose.yml" \
  --expected-web-hash-file "$HOME/web-hash.txt"
```

It prints only `FAIL <field>: <reason>` or one PASS line per check group. It never
prints a value and never saves the resolved configuration. Required shape:

| Variable | Rule |
|---|---|
| `HELIOLYTICS_COMMIT`, `HELIOLYTICS_WEB_COMMIT` | Full lowercase 40-character SHA |
| `HELIOLYTICS_BUILT_AT` | RFC3339 UTC, for example `2026-10-20T04:00:00Z` |
| `POSTGRES_PASSWORD` | 64 lowercase hex, schema owner |
| `APP_DB_PASSWORD` | 64 lowercase hex, distinct from the owner password |
| `HELIOLYTICS_WEB_API_KEY` | `web.<64 lowercase hex>`, passed to the API and web |
| `HELIOLYTICS_PHONE_API_KEY` | `phone.<64 lowercase hex>`, distinct secret, passed only to the API |
| `HELIOLYTICS_SESSION_SECRET` | 64 lowercase hex, unequal to every API secret |
| `HELIOLYTICS_WEB_PASSWORD_HASH` | One line, single-quoted `scrypt$N=32768$r=8$p=3$<salt>$<hash>` |
| `TRUST_PROXY`, `TRUSTED_PROXY_CIDRS` | `true` plus the connector `/32` in production |
| `COOKIE_SECURE` | `true` in production |

The password hash is generated in the web checkout with `npm run auth:hash-password`.
Keep the whole value single-quoted, because Compose expands `$` in unquoted and
double-quoted values. Enter each API key once in the env file. Compose passes both
to the API and only the web key to the web container. The API never receives the
session secret or the web password hash.

## Health and readiness

`GET /health` returns `200 ok` without touching the database. It proves the process
is alive, nothing more.

`GET /version` returns the commit and build time compiled into the binary.

The compose healthchecks are stronger than liveness:

- `db` logs in over loopback as the owner role and runs `SELECT 1`.
- `api` runs `/usr/local/bin/api -healthcheck`, which calls loopback `/health` with a
  two second timeout and accepts only `200 ok`.
- `web` fetches its own `/login` route.

None of them proves the schema, grants or ingest readiness. Empty database
initialization does that.

## Database initialization

The database image runs the mounted init files on an empty volume, in order:

1. `deploy/initdb/00_roles.sh` creates `heliolytics_app` with `LOGIN`, no superuser,
   no createdb, no createrole, and the 30 second statement and 60 second idle
   transaction timeouts. The password arrives through `APP_DB_PASSWORD` in the
   container environment and reaches SQL as one quoted literal.
2. `schema.sql` creates extensions, tables, indexes, constraints, views, functions and
   grants. There are no migrations and no fixtures.

Order matters: the role exists before `schema.sql` grants anything to it.

Before the first app request, every public application table must be empty:

```sh
docker exec deploy-db-1 sh -c \
  'PGPASSWORD="$POSTGRES_PASSWORD" psql -h 127.0.0.1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Atc \
  "SELECT relname, n_live_tup FROM pg_stat_user_tables ORDER BY relname"'
```

Health data enters only through signed `phone` uploads and the profile only through
the signed profile request. Signed `web` reads may create nonce rows and nothing
else. Historical captures, exports and dumps are never imported, seeded, restored or
rebuilt into a fresh production database.

The image entrypoint starts a temporary server for initialization. That temporary
server uses the initialization copy of `pg_hba.conf`, so the role and schema scripts
can log in locally. The real server then starts with
`command: ["postgres", "-c", "config_file=/etc/postgresql/postgresql.conf"]`, which
uses the mounted `postgresql.conf` and `pg_hba.conf`. Both files are read-only bind
mounts and `create_host_path` is false, so a missing file fails instead of starting
with an empty schema.

`deploy/postgresql.conf` bounds the cluster to the container budget: 512MB
`shared_buffers`, 1536MB `effective_cache_size`, 4MB `work_mem`, 128MB
`maintenance_work_mem`, 40 connections, four Timescale background workers, no
statement logging, and 500ms duration logging without bind parameters. TimescaleDB
telemetry is off and the image autotune hook is disabled.

## Network and transport threat model

The database has no host port. It is reachable only from containers on the internal
`data` network, and `pg_hba.conf` accepts SCRAM logins for the owner and app roles
from loopback and `DATA_SUBNET`, then rejects everything else.

`ssl = off`, so API to database SQL and health data cross a Docker bridge in
plaintext. SCRAM authenticates but does not encrypt. This is acceptable only for a
single host whose database network is unpublished and whose Docker and host root
users are trusted. It does not protect against a compromised host, a compromised API
or Adminer container, or any forwarded tunnel into the database.

Owner acceptance of this single-host plaintext boundary is pending. Any remote or
multi-host database requires authenticated verified TLS before it is connected.

## Proxy trust

Production requires `TRUST_PROXY=true` and `TRUSTED_PROXY_CIDRS` naming only the
connector address. The API trusts a single-address `CF-Connecting-IP` header only
from that socket peer. A missing, duplicate or malformed header from the trusted peer
returns 400, and other peers are limited by their socket address with forwarded
headers ignored.

This holds only while the API loopback port is reachable from the internet solely
through the connector. Header presence is never proof of provenance, and the web app
cannot inspect socket peers, so the isolated ingress network is a deployment
requirement rather than a Next.js feature.

## Nightly backup

One systemd oneshot service runs one script:

- `deploy/backup-db.sh` dumps the whole database with `pg_dump --format=custom
  --no-owner --no-acl`, using the database container's own credentials read from its
  environment.
- It writes `heliolytics-<UTC stamp>.dump`, a `.sha256` sidecar and a `.info` file
  with UTC time, PostgreSQL and TimescaleDB versions, the source image reference, and
  the recorded commit.
- It copies all three files to `BACKUP_DESTINATION` as `.partial` names, renames them
  after every transfer succeeds, reads the remote SHA-256 back and compares it.
- Only then it prunes dated bundles older than `BACKUP_KEEP_DAYS` (default 30) in the
  local and remote backup directories. It never follows symlinks and never deletes
  unrelated files.
- If the copy fails it exits nonzero, keeps the local dump for a retry and applies no
  remote retention.

Install steps:

```sh
sudo install -Dm700 deploy/backup-db.sh /usr/local/libexec/heliolytics-backup
sudo install -d -m700 /etc/heliolytics
sudo install -m600 deploy/backup.env.example /etc/heliolytics/backup.env
sudo install -m644 deploy/systemd/heliolytics-backup.service /etc/systemd/system/
sudo install -m644 deploy/systemd/heliolytics-backup.timer /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl start heliolytics-backup.service
sudo systemctl enable --now heliolytics-backup.timer
```

The service runs as the deploy account in the `docker` group, with
`EnvironmentFile=/etc/heliolytics/backup.env`, umask 0077 and a one hour timeout. The
timer fires daily at 03:00 UTC with a 15 minute randomized delay and
`Persistent=true`. Enabling the timer is not backup evidence: check the journal, the
off-host file and its SHA-256 yourself.

`deploy/backup.env.example` expects an existing reachable SSH alias and directory. A
local dump alone is not a backup, and a destination that has never worked is a
configuration error rather than a future hook.

## Manual restore rehearsal

This rehearsal restores a downloaded backup into a separate disposable container. It
is test evidence, never a backfill into production and never a replacement for the
fresh production volume.

```sh
set -eu
# DUMP is freshly downloaded from the configured off-host destination.
# IMAGE is the exact compatible reference from the backup .info file.
# NAME is a unique disposable name such as heliolytics-restore-20261020.
# PRIVATE_RESTORE_ENV is a 0600 file with POSTGRES_USER, POSTGRES_PASSWORD,
# POSTGRES_DB and NO_TS_TUNE=true for the image. POSTGRES_USER must be the
# dump's source owner role, heliolytics, because Timescale job rows store it.
case "$NAME" in heliolytics-restore-*) ;; *) echo 'bad NAME' >&2; exit 1;; esac
(cd "$(dirname "$DUMP")" && sha256sum --check "$(basename "$DUMP").sha256")
docker volume create --label heliolytics.restore=true "$NAME-data"
docker run -d --name "$NAME" --network none --memory 2g --cpus 1.5 \
  --env-file "$PRIVATE_RESTORE_ENV" -v "$NAME-data:/var/lib/postgresql/data" "$IMAGE"
ready=false
for attempt in $(seq 1 60); do
  if docker exec "$NAME" sh -c \
    'PGPASSWORD="$POSTGRES_PASSWORD" psql -h 127.0.0.1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1 -Atqc "SELECT 1"' >/dev/null 2>&1; then
    ready=true
    break
  fi
  sleep 2
done
test "$ready" = true
docker exec "$NAME" sh -c \
  'PGPASSWORD="$POSTGRES_PASSWORD" psql -h 127.0.0.1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1 -c "CREATE EXTENSION IF NOT EXISTS timescaledb; SELECT timescaledb_pre_restore();"'
docker exec -i "$NAME" sh -c \
  'PGPASSWORD="$POSTGRES_PASSWORD" pg_restore -h 127.0.0.1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" --exit-on-error --no-owner --no-acl' < "$DUMP"
docker exec "$NAME" sh -c \
  'PGPASSWORD="$POSTGRES_PASSWORD" psql -h 127.0.0.1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1 -c "SELECT timescaledb_post_restore();"'
docker exec "$NAME" sh -c \
  'PGPASSWORD="$POSTGRES_PASSWORD" psql -h 127.0.0.1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1 -Atc "SELECT extversion FROM pg_extension WHERE extname = '\''timescaledb'\''; SELECT count(*) FROM timescaledb_information.hypertables; SELECT count(*) FROM timescaledb_information.chunks WHERE is_compressed;"'
# Compare representative raw, sample, daily and profile values with the source.
docker rm -f "$NAME"
docker volume rm "$NAME-data"
```

Rules that keep this rehearsal honest:

- The image must be retrievable and compatible with the backup. If it is not, the
  restore is unproven and the wipe is blocked.
- Timescale keeps the job owner role name as row data, so the restore target must
  create the dump's source owner role. Use `POSTGRES_USER=heliolytics`; another
  name fails `pg_restore` on `_timescaledb_config.bgw_job` and blocks the wipe.
- Do not mount an empty production schema before restoring a complete dump.
- Do not use parallel `pg_restore`.
- Stop on every failed command. Cleanup targets only the recorded disposable names.
- Record the dump path, SHA-256 and result privately. There is no receipt system.

## Guarded volume wipe

`deploy/reset-db.sh` removes one exact database volume. It never runs `compose down`,
`down -v` or `volume prune`, and it never restarts the stack.

```sh
deploy/reset-db.sh \
  --project deploy \
  --volume deploy_pgdata \
  --backup-file /home/deploy/tmp/heliolytics-20261020T030000Z.dump \
  --backup-sha256 <64 hex from the downloaded sidecar> \
  --authorize-production
```

It refuses to continue unless all of these hold:

- The project is `deploy` (with `--authorize-production`) or a `heliolytics-d01-*`
  sandbox, and the volume matches that project.
- `deploy/.env`, `deploy/docker-compose.yml` and `schema.sql` exist.
- The backup file is regular, nonempty, matches the expected SHA-256, has a `.info`
  sidecar naming a compatible image, and lists with `pg_restore`.
- Exactly one database container matches the project labels, it is running, its
  `com.docker.compose.project.working_dir` label equals the canonical deploy
  directory (or an explicitly recorded directory), and its
  `/var/lib/postgresql/data` mount is the requested volume.
- No `api`, `web` or `cloudflared` container for the project is running.
- No other container, running or stopped, mounts the volume.
- The operator types `DELETE <volume> FOR <project>` at an interactive prompt.

Sandbox mode accepts the same typed phrase from standard input so the test harness can
exercise the positive path. It still refuses production names.

Run this only after the downloaded backup has been restored and queried in the
disposable rehearsal above. Afterwards the volume is gone and recovery depends on
that tested backup.

## Owner deploy order

1. Record the running image references and IDs, the source PostgreSQL and TimescaleDB
   versions, and the database container's compose working-directory label.
2. Separate the shared Hermes connector, project, routes and firewall first, and
   verify the endpoints the owner still needs.
3. Choose the off-host backup destination, SSH trust and dedicated remote directory.
   Install the backup script, service and 0600 environment file, run the service once
   by hand and confirm the off-host file and hash.
4. Confirm the backend and web checkouts are on `main` and clean, then
   `git pull --ff-only` in each.
5. Keep the old database credentials until the wipe decision. Generate independent
   owner and app database passwords, phone and web API keys, and the web session
   secret. Generate the web password hash, place it single-quoted, and delete the
   temporary hash file after the byte comparison.
6. Build the API and web images with the full SHAs and one UTC build time, then run
   `deploy/validate-env.sh`. Accept or reject the single-host plaintext database
   transport decision recorded above before starting.
7. Stop the API, web and product connector writers. Confirm no sync is active, take
   the final backup, download it, verify the SHA-256 and restore it in the rehearsal.
   Any failure aborts the wipe.
8. Run the guarded wipe, then start the stack and let the database initialize with
   roles, extensions and schema only. Confirm every public application table is
   empty, then let the app perform the first upload.
9. For a later web-only change, `deploy/deploy-web.sh` rebuilds and recreates only the
   web container. It refuses a non-`main`, dirty or mislabelled checkout.
   `--preflight-only` runs every check without building.

Rollback before the wipe is the old image set and the untouched volume. After the
wipe there is no rollback volume, only the tested off-host dump.

## Retention and schema state

`schema.sql` is the only DDL, so an existing volume is never upgraded in place:
production is created fresh and receives health data only from accepted app uploads.

Raw uploads are retained without a limit until the owner approves a backup and pruning
policy. No automatic raw retention or cascade delete exists.

## Sandbox verification

`deploy/tests/deploy-ops-test.sh` runs the behavioral sandbox tests: installer
behavior, validator refusals and passes, web helper guards, backup refusals, fresh
initialization with zero rows, the app role's grants, a dump with a failed off-host
copy, a real restore of that dump into a disposable container, and a guarded sandbox
wipe with an unrelated canary volume. It creates only `heliolytics-d01-*` and
`heliolytics-restore-*` resources and refuses production names.

The off-host SCP copy and read-back hash need a real SSH destination. That row stays
owner-pending until the owner runs it, and the harness prints it separately.

When a row fails, the harness prints the last lines of that row's captured helper
output, so a rejected environment or a refused statement explains itself. No secret
value is printed.

The harness stages its sandbox tree at `.d01-sandbox/` inside the checkout and removes
it on exit, because server-side bind mounts resolve on the Docker host. When it runs
inside a helper container that mounts the Docker socket, set
`HELIOLYTICS_D01_MOUNT_ROOT` to the host path of the checkout so those bind mounts
resolve. The helper image needs Bash, Docker, the Compose plugin, `flock`, `ssh`,
`scp` and `sha256sum`.

`deploy/tests/fixture-ops.sql` is the synthetic restore fixture: one session, one raw
blob, representative samples, a sleep session, a workout, PAI, coverage, one daily row,
a profile and one compressed chunk. It contains no personal identifiers.

## Pending owner operations

Code delivery does not close these rows. Each needs live evidence the owner records
privately.

| ID | Pending owner evidence |
|---|---|
| OPS-1 | Replace any malformed live password hash, confirm the generated hash byte comparison, and complete a real browser login |
| OPS-2 | Rotate the owner and app database passwords, phone and web keys and session secret; update the phone build; confirm old keys and cookies fail |
| OPS-3 | Choose a reachable off-host destination, enable the timer, observe one nightly run, and restore a downloaded dump in the rehearsal |
| OPS-4 | Recreate stale containers, confirm the pre-rename checkout path is gone, and check effective labels, mounts and networks |
| OPS-5 | Build and deploy the ARM64 image from `main`, then scan the assembled image and record its provenance |
| OPS-6 | Add the Cloudflare HTTPS redirect and HSTS rules, then probe both hostnames |
| OPS-7 | Confirm the connector-only ingress subnet, the narrow trusted CIDR and the live rate responses |
| OPS-8 | Record registry digests for every deployed reference and confirm arm64 on the host |
| OPS-9 | Check effective CPU, memory, PID and log caps with `docker inspect` on the live host |
| OPS-10 | Confirm nonroot read-only API and web, restricted database login and SCRAM on local and network paths |
| OPS-11 | Start the stack explicitly after validation; confirm no reparse routes or superseded signing variables remain |
| OPS-12 | Migrate the Hermes connector, routes and firewall before replacing the product connector |
| OPS-13 | Decide on burned-default handling and historical references; no purge is claimed without confirmation |
| OPS-14 | Confirm the build context carries no canary secret and the final image has no extra binaries |
| OPS-15 | Fix existing file modes, keep Adminer on its profile, and update or reboot the host only after recovery is proven |
| OPS-16 | Confirm no obsolete deploy helper or flag remains in the deployed configuration |
| OPS-17 | Confirm fresh initialization leaves zero health and profile rows and that the schema-only entry path is understood |
| OPS-18 | Confirm the published names, paths, environment variables and backup prerequisites match the live host |
| DATA-3 | Record one successful off-host backup and one compatible restore into a separate volume |
| DATA-5 | Verify the live restricted role, SCRAM authentication and the accepted single-host transport decision |
| DATA-16 | Record `SHOW` output for telemetry, duration threshold, parameter limits and statement logging, plus one slow-query duration line |

Validation: not run for this revision.
