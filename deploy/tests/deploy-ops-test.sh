#!/usr/bin/env bash
# Behavioral sandbox tests for the deploy helpers.
#
# Linux with Docker only. Every resource this script creates is named
# heliolytics-d01-*, heliolytics-restore-* or a private temp directory. It
# refuses to run against the production project or volume and never reads the
# production env file.
set -uo pipefail

script_dir=
script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
deploy_dir=
deploy_dir="$(cd -- "$script_dir/.." && pwd)"
backend_dir=
backend_dir="$(cd -- "$deploy_dir/.." && pwd)"
web_repo="${HELIOLYTICS_D01_WEB_DIR:-$backend_dir/../heliolytics-web}"

db_image="${HELIOLYTICS_TEST_DB_IMAGE:-timescale/timescaledb:2.30.2-pg16@sha256:6f139d56042989bd35f50ba5986e492cd32e35cc6778c50be2659add298dc09f}"
node_image="${HELIOLYTICS_D01_NODE_IMAGE:-node:24.22.0-alpine3.24@sha256:44fce696f38ba8c987c4db89954051000bbd05ffd8d8659ba15ed4a8602e937f}"

run_id="${HELIOLYTICS_D01_RUN_ID:-$$}"
project="heliolytics-d01-test-${run_id}"
volume="${project}_pgdata"
canary_volume="heliolytics-d01-canary-${run_id}"
db_container="${project}-db"
restore_container="heliolytics-restore-${run_id}"

if [[ "$project" == "deploy" || "$volume" == "deploy_pgdata" ]]; then
  printf 'refusing to run against production names\n' >&2
  exit 1
fi

work_dir=
work_dir="$(mktemp -d)"

# Server-side bind mounts resolve on the Docker host. When the gate runs inside a
# helper container with the docker socket, set HELIOLYTICS_D01_MOUNT_ROOT to the
# host path of this checkout. The staged tree lives inside the checkout for that reason.
mount_root="${HELIOLYTICS_D01_MOUNT_ROOT:-$backend_dir}"
sandbox="$backend_dir/.d01-sandbox"

pass_count=0
fail_count=0
pending_count=0

ok() {
  printf 'ok - %s\n' "$1"
  pass_count=$((pass_count + 1))
}

not_ok() {
  printf 'not ok - %s\n' "$1"
  fail_count=$((fail_count + 1))
}

# Show the tail of captured helper output so a failing row explains itself.
diagnose() {
  local file="$1"
  if [[ -s "$file" ]]; then
    printf 'last 8 lines of captured output:\n' >&2
    tail -n 8 "$file" >&2
  fi
}

pending() {
  printf 'owner-pending - %s\n' "$1"
  pending_count=$((pending_count + 1))
}

cleanup() {
  docker rm -f "$db_container" "$restore_container" >/dev/null 2>&1
  docker volume rm "$volume" "$canary_volume" "${restore_container}-data" >/dev/null 2>&1
  rm -rf "$sandbox"
  rm -rf "$work_dir"
}
trap cleanup EXIT

if ! command -v docker >/dev/null 2>&1; then
  printf 'docker is required\n' >&2
  exit 1
fi
real_docker="$(command -v docker)"

hex_fill() {
  local char="$1" count="$2" out="" i
  for ((i = 0; i < count; i++)); do
    out="$out$char"
  done
  printf '%s' "$out"
}

owner_sql() {
  docker exec -i "$1" sh -c \
    'PGPASSWORD="$POSTGRES_PASSWORD" psql -h 127.0.0.1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1 -Atq'
}

app_sql() {
  docker exec -i "$1" sh -c \
    'PGPASSWORD="$APP_DB_PASSWORD" psql -h 127.0.0.1 -U heliolytics_app -d heliolytics -v ON_ERROR_STOP=1 -Atq'
}

expect_failure() {
  local name="$1"
  shift
  if "$@" >"$work_dir/case.out" 2>"$work_dir/case.err"; then
    not_ok "$name"
  else
    ok "$name"
  fi
}

wait_ready() {
  local container="$1" attempt
  for ((attempt = 0; attempt < 90; attempt++)); do
    if owner_sql "$container" <<<"SELECT 1" >/dev/null 2>&1; then
      return 0
    fi
    sleep 2
  done
  return 1
}

# ---------------------------------------------------------------- sandbox tree
stage_deploy_tree() {
  local target="$1"
  mkdir -p "$target/initdb"
  cp -- "$deploy_dir/docker-compose.yml" "$target/docker-compose.yml"
  cp -- "$deploy_dir/postgresql.conf" "$target/postgresql.conf"
  cp -- "$deploy_dir/pg_hba.conf" "$target/pg_hba.conf"
  cp -- "$deploy_dir/initdb/00_roles.sh" "$target/initdb/00_roles.sh"
  cp -- "$deploy_dir/install.sh" "$target/install.sh"
  cp -- "$deploy_dir/validate-env.sh" "$target/validate-env.sh"
  cp -- "$deploy_dir/validate-env.mjs" "$target/validate-env.mjs"
  cp -- "$deploy_dir/backup-db.sh" "$target/backup-db.sh"
  cp -- "$deploy_dir/reset-db.sh" "$target/reset-db.sh"
  cp -- "$deploy_dir/deploy-web.sh" "$target/deploy-web.sh"
}

sandbox_backend="$sandbox/backend"
sandbox_deploy="$sandbox_backend/deploy"
rm -rf "$sandbox"
mkdir -p "$sandbox_backend"
stage_deploy_tree "$sandbox_deploy"
cp -- "$backend_dir/schema.sql" "$sandbox_backend/schema.sql"
ln -s "$web_repo" "$sandbox/heliolytics-web"
printf 'FROM scratch\n' >"$sandbox_backend/Dockerfile"

hex64_a="$(hex_fill a 64)"
hex64_b="$(hex_fill b 64)"
hex64_c="$(hex_fill c 64)"
hex64_d="$(hex_fill d 64)"
hex64_e="$(hex_fill e 64)"
sha_a="$(hex_fill 1 40)"
sha_b="$(hex_fill 2 40)"
sha_web="$(hex_fill 3 40)"
sha_other="$(hex_fill 4 40)"
hash_value="scrypt\$N=32768\$r=8\$p=3\$$(hex_fill f 32)\$$(hex_fill 9 64)"

write_env() {
  local target="$1"
  {
    printf 'HELIOLYTICS_COMMIT=%s\n' "$sha_a"
    printf 'HELIOLYTICS_WEB_COMMIT=%s\n' "$sha_b"
    printf 'HELIOLYTICS_BUILT_AT=2026-10-20T00:00:00Z\n'
    printf 'POSTGRES_PASSWORD=%s\n' "$hex64_a"
    printf 'APP_DB_PASSWORD=%s\n' "$hex64_b"
    printf 'HELIOLYTICS_API_KEYS=web.%s,phone.%s\n' "$hex64_c" "$hex64_d"
    printf 'HELIOLYTICS_API_KEY=web.%s\n' "$hex64_c"
    printf 'HELIOLYTICS_SESSION_SECRET=%s\n' "$hex64_e"
    printf "HELIOLYTICS_WEB_PASSWORD_HASH='%s'\n" "$hash_value"
    printf 'TRUST_PROXY=true\n'
    printf 'TRUSTED_PROXY_CIDRS=172.28.0.10/32\n'
    printf 'COOKIE_SECURE=true\n'
    printf 'DATA_SUBNET=172.30.0.0/24\n'
    printf 'INGRESS_SUBNET=172.28.0.0/24\n'
    printf 'CLOUDFLARED_IP=172.28.0.10\n'
    printf 'API_PORT=8080\n'
    printf 'WEB_PORT=3000\n'
    printf 'ADMINER_PORT=8081\n'
    printf 'RATE_LIMIT_PER_MIN=120\n'
    printf 'RATE_LIMIT_PER_KEY_PER_MIN=600\n'
    printf 'REQUEST_TIMEOUT_SECONDS=90\n'
    printf 'MAX_CONCURRENT_BODY_READS=2\n'
  } >"$target"
  chmod 600 -- "$target"
}

mkdir -p "$work_dir/env"
valid_env="$work_dir/env/valid.env"
write_env "$valid_env"
cp -- "$valid_env" "$sandbox_deploy/.env"
chmod 600 -- "$sandbox_deploy/.env"

# ------------------------------------------------------------- A. installer
install_dir="$work_dir/install"
mkdir -p "$install_dir"
cp -- "$deploy_dir/install.sh" "$install_dir/install.sh"
cp -- "$deploy_dir/.env.example" "$install_dir/.env.example"
mkdir -p "$work_dir/stub-bin"
marker="$work_dir/docker-was-called"
cat >"$work_dir/stub-bin/docker" <<'STUB'
#!/bin/sh
# Installer rows set DOCKER_MARKER to prove docker is never called.
# Every other row forwards to the real client so preflight checks can run.
if [ -n "${DOCKER_MARKER:-}" ]; then
  : >"$DOCKER_MARKER"
  exit 99
fi
exec "${REAL_DOCKER:?real docker path is not set}" "$@"
STUB
chmod +x "$work_dir/stub-bin/docker"

if DOCKER_MARKER="$marker" PATH="$work_dir/stub-bin:$PATH" bash "$install_dir/install.sh" \
  >"$work_dir/case.out" 2>&1; then
  if [[ -f "$install_dir/.env" && ! -e "$marker" ]]; then
    mode="$(stat -c '%a' "$install_dir/.env" 2>/dev/null || stat -f '%Lp' "$install_dir/.env")"
    if [[ "$mode" == "600" ]]; then
      ok 'installer creates a 0600 template without calling docker'
    else
      not_ok 'installer created the template with the wrong mode'
    fi
  else
    not_ok 'installer either called docker or did not create the template'
  fi
else
  not_ok 'installer failed on a missing env file'
fi

printf 'EXISTING=1\n' >"$install_dir/.env"
chmod 600 -- "$install_dir/.env"
before="$(sha256sum -- "$install_dir/.env" | cut -d' ' -f1)"
if DOCKER_MARKER="$marker" PATH="$work_dir/stub-bin:$PATH" bash "$install_dir/install.sh" \
  >"$work_dir/case.out" 2>&1; then
  after="$(sha256sum -- "$install_dir/.env" | cut -d' ' -f1)"
  if [[ "$before" == "$after" ]]; then
    ok 'installer preserves existing env bytes'
  else
    not_ok 'installer changed an existing env file'
  fi
else
  not_ok 'installer failed when the env file already existed'
fi

# ------------------------------------------------- B. environment validator
validate() {
  "$sandbox_deploy/validate-env.sh" \
    --project "$project" \
    --env-file "${1:-$valid_env}" \
    --compose-file "$sandbox_deploy/docker-compose.yml" \
    --node-image "$node_image"
}

expect_failure 'validator rejects missing arguments' \
  "$sandbox_deploy/validate-env.sh" --project "$project"
expect_failure 'validator rejects an unknown option' \
  "$sandbox_deploy/validate-env.sh" --project "$project" --nonsense x
expect_failure 'validator rejects a relative env path' \
  "$sandbox_deploy/validate-env.sh" --project "$project" \
  --env-file relative.env --compose-file "$sandbox_deploy/docker-compose.yml"

readable_env="$work_dir/env/readable.env"
cp -- "$valid_env" "$readable_env"
chmod 644 -- "$readable_env"
expect_failure 'validator rejects a group-readable env file' validate "$readable_env"

mutate_env() {
  local name="$1" search="$2" replace="$3"
  local target="$work_dir/env/$name.env"
  sed "s|$search|$replace|" "$valid_env" >"$target"
  chmod 600 -- "$target"
  printf '%s' "$target"
}

expect_failure 'validator rejects an empty build SHA' \
  validate "$(mutate_env no_sha '^HELIOLYTICS_COMMIT=.*' 'HELIOLYTICS_COMMIT=')"
expect_failure 'validator rejects a short build SHA' \
  validate "$(mutate_env short_sha '^HELIOLYTICS_COMMIT=.*' 'HELIOLYTICS_COMMIT=abc')"
expect_failure 'validator rejects a non-UTC build time' \
  validate "$(mutate_env bad_time '^HELIOLYTICS_BUILT_AT=.*' 'HELIOLYTICS_BUILT_AT=2026-10-20T00:00:00+05:30')"
expect_failure 'validator rejects equal database passwords' \
  validate "$(mutate_env same_db "APP_DB_PASSWORD=$hex64_b" "APP_DB_PASSWORD=$hex64_a")"
expect_failure 'validator rejects a reused session secret' \
  validate "$(mutate_env same_session "HELIOLYTICS_SESSION_SECRET=$hex64_e" "HELIOLYTICS_SESSION_SECRET=$hex64_c")"
expect_failure 'validator rejects a missing web membership' \
  validate "$(mutate_env no_web "HELIOLYTICS_API_KEY=web.$hex64_c" "HELIOLYTICS_API_KEY=web.$hex64_d")"
expect_failure 'validator rejects a duplicate kid' \
  validate "$(mutate_env dup_kid "web.$hex64_c,phone.$hex64_d" "web.$hex64_c,web.$hex64_d")"
expect_failure 'validator rejects the documented example secret' \
  validate "$(mutate_env example_secret "$hex64_d" '000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f')"
expect_failure 'validator rejects wrong scrypt parameters' \
  validate "$(mutate_env bad_hash 'N=32768' 'N=16384')"
expect_failure 'validator rejects secure cookies without trusted proxy' \
  validate "$(mutate_env no_proxy '^TRUST_PROXY=.*' 'TRUST_PROXY=false')"
expect_failure 'validator rejects a wide trusted CIDR' \
  validate "$(mutate_env wide_cidr '^TRUSTED_PROXY_CIDRS=.*' 'TRUSTED_PROXY_CIDRS=10.0.0.0/8')"

if validate "$valid_env" >"$work_dir/case.out" 2>&1; then
  if grep -q 'environment validation: PASS' "$work_dir/case.out"; then
    ok 'validator passes a complete sandbox environment'
  else
    not_ok 'validator succeeded without its PASS line'
  fi
else
  not_ok 'validator rejected a complete sandbox environment'
  diagnose "$work_dir/case.out"
fi

expected_hash_file="$work_dir/env/expected-hash.txt"
printf '%s\n' "$hash_value" >"$expected_hash_file"
chmod 600 -- "$expected_hash_file"
if "$sandbox_deploy/validate-env.sh" \
  --project "$project" \
  --env-file "$valid_env" \
  --compose-file "$sandbox_deploy/docker-compose.yml" \
  --node-image "$node_image" \
  --expected-web-hash-file "$expected_hash_file" >"$work_dir/case.out" 2>&1; then
  if grep -q 'web hash literal preserved: PASS' "$work_dir/case.out"; then
    ok 'validator preserves the generated web hash byte for byte'
  else
    not_ok 'validator did not report the hash preservation result'
  fi
else
  not_ok 'validator rejected a matching generated hash'
  diagnose "$work_dir/case.out"
fi

truncated_hash="$work_dir/env/truncated-hash.txt"
printf '%s' "${hash_value%????}" >"$truncated_hash"
chmod 600 -- "$truncated_hash"
expect_failure 'validator rejects a truncated generated hash' \
  "$sandbox_deploy/validate-env.sh" \
  --project "$project" \
  --env-file "$valid_env" \
  --compose-file "$sandbox_deploy/docker-compose.yml" \
  --node-image "$node_image" \
  --expected-web-hash-file "$truncated_hash"

if grep -q "$hex64_b" "$work_dir/case.err" "$work_dir/case.out" 2>/dev/null; then
  not_ok 'validator printed a secret value'
else
  ok 'validator printed no secret value'
fi

# ------------------------------------------------------- C. web deploy guard
fake_backend="$sandbox/fake-backend"
fake_deploy="$fake_backend/deploy"
fake_web="$sandbox/fake-web"
mkdir -p "$fake_deploy" "$fake_backend/.git" "$fake_web/.git"
stage_deploy_tree "$fake_deploy"
cp -- "$backend_dir/schema.sql" "$fake_backend/schema.sql"
printf 'FROM scratch\n' >"$fake_backend/Dockerfile"
printf 'FROM scratch\n' >"$fake_web/Dockerfile"
cp -- "$valid_env" "$fake_deploy/.env"
chmod 600 -- "$fake_deploy/.env"

cat >"$work_dir/stub-bin/git" <<'STUB'
#!/bin/sh
args="$*"
case "$args" in
  *"rev-parse --abbrev-ref HEAD"*) printf '%s\n' "${STUB_GIT_BRANCH:-main}" ;;
  *"status --porcelain"*) printf '%s' "${STUB_GIT_STATUS:-}" ;;
  *"rev-parse HEAD"*) printf '%s\n' "${STUB_GIT_HEAD:-}" ;;
  *) exit 1 ;;
esac
STUB
chmod +x "$work_dir/stub-bin/git"

STUB_GIT_BRANCH=main
STUB_GIT_STATUS=
STUB_GIT_HEAD="$sha_web"

web_deploy() {
  PATH="$work_dir/stub-bin:$PATH" \
    REAL_DOCKER="$real_docker" \
    STUB_GIT_BRANCH="$STUB_GIT_BRANCH" \
    STUB_GIT_STATUS="$STUB_GIT_STATUS" \
    STUB_GIT_HEAD="$STUB_GIT_HEAD" \
    "$fake_deploy/deploy-web.sh" \
    --web-commit "$sha_web" --built-at 2026-10-20T00:00:00Z \
    --project "$project" --web-dir "$fake_web" "$@"
}

STUB_GIT_BRANCH=v7
expect_failure 'web helper refuses a v7 checkout' web_deploy --preflight-only
STUB_GIT_BRANCH=main
STUB_GIT_STATUS=' M app/page.tsx'
expect_failure 'web helper refuses a dirty checkout' web_deploy --preflight-only
STUB_GIT_STATUS=
expect_failure 'web helper refuses a SHA mismatch' web_deploy --preflight-only --web-commit "$sha_other"
expect_failure 'web helper refuses a missing build time' \
  "$fake_deploy/deploy-web.sh" --web-commit "$sha_web" --project "$project" --web-dir "$fake_web"

if web_deploy --preflight-only >"$work_dir/case.out" 2>&1; then
  ok 'web helper passes preflight for a clean main checkout'
else
  not_ok 'web helper rejected a clean main checkout'
  diagnose "$work_dir/case.out"
fi

# ------------------------------------------------------- D. reset guards
dummy_dump="$work_dir/dump/backup.dump"
mkdir -p "$work_dir/dump"
printf 'not a real dump\n' >"$dummy_dump"
dummy_sha="$(sha256sum -- "$dummy_dump" | cut -d' ' -f1)"
zero_sha="$(hex_fill 0 64)"

expect_failure 'reset refuses missing arguments' \
  "$sandbox_deploy/reset-db.sh" --project "$project"
expect_failure 'reset refuses an unknown option' \
  "$sandbox_deploy/reset-db.sh" --project "$project" --yes
expect_failure 'reset refuses production without the authorization flag' \
  "$sandbox_deploy/reset-db.sh" --project deploy --volume deploy_pgdata \
  --backup-file "$dummy_dump" --backup-sha256 "$dummy_sha"
expect_failure 'reset refuses a production volume with another project' \
  "$sandbox_deploy/reset-db.sh" --project "$project" --volume deploy_pgdata \
  --backup-file "$dummy_dump" --backup-sha256 "$dummy_sha"
expect_failure 'reset refuses an unknown project' \
  "$sandbox_deploy/reset-db.sh" --project other --volume "$volume" \
  --backup-file "$dummy_dump" --backup-sha256 "$dummy_sha"
expect_failure 'reset refuses a missing backup file' \
  "$sandbox_deploy/reset-db.sh" --project "$project" --volume "$volume" \
  --backup-file "$work_dir/absent.dump" --backup-sha256 "$dummy_sha"
expect_failure 'reset refuses a hash mismatch' \
  "$sandbox_deploy/reset-db.sh" --project "$project" --volume "$volume" \
  --backup-file "$dummy_dump" --backup-sha256 "$zero_sha"
expect_failure 'reset refuses a backup without a sidecar' \
  "$sandbox_deploy/reset-db.sh" --project "$project" --volume "$volume" \
  --backup-file "$dummy_dump" --backup-sha256 "$dummy_sha"

# ------------------------------------------------------ E. backup env guards

expect_failure 'backup refuses a missing deploy directory' \
  env -u HELIOLYTICS_DEPLOY_DIR BACKUP_DIR="$work_dir/backups" \
  BACKUP_DESTINATION=alias:/backups "$sandbox_deploy/backup-db.sh"
expect_failure 'backup refuses a missing backup directory' \
  env -u BACKUP_DIR HELIOLYTICS_DEPLOY_DIR="$sandbox_deploy" \
  BACKUP_DESTINATION=alias:/backups "$sandbox_deploy/backup-db.sh"
expect_failure 'backup refuses a missing destination' \
  env -u BACKUP_DESTINATION HELIOLYTICS_DEPLOY_DIR="$sandbox_deploy" \
  BACKUP_DIR="$work_dir/backups" "$sandbox_deploy/backup-db.sh"
expect_failure 'backup refuses a destination without a host alias' \
  env HELIOLYTICS_DEPLOY_DIR="$sandbox_deploy" BACKUP_DIR="$work_dir/backups" \
  BACKUP_DESTINATION=/absolute/path "$sandbox_deploy/backup-db.sh"
expect_failure 'backup refuses a relative destination path' \
  env HELIOLYTICS_DEPLOY_DIR="$sandbox_deploy" BACKUP_DIR="$work_dir/backups" \
  BACKUP_DESTINATION=alias:relative/path "$sandbox_deploy/backup-db.sh"
expect_failure 'backup refuses a parent segment in the destination' \
  env HELIOLYTICS_DEPLOY_DIR="$sandbox_deploy" BACKUP_DIR="$work_dir/backups" \
  BACKUP_DESTINATION=alias:/backups/../etc "$sandbox_deploy/backup-db.sh"
expect_failure 'backup refuses a zero retention window' \
  env HELIOLYTICS_DEPLOY_DIR="$sandbox_deploy" BACKUP_DIR="$work_dir/backups" \
  BACKUP_DESTINATION=alias:/backups BACKUP_KEEP_DAYS=0 "$sandbox_deploy/backup-db.sh"

# ------------------------------------------- F. fresh init, roles and grants
docker volume create "$canary_volume" >/dev/null
owner_pw="owner-$(hex_fill 5 24)"
app_pw="pa'ss\"w\$rd\\word; DROP TABLE profile;--"
restore_pw="restore-$(hex_fill 6 24)"

docker volume create "$volume" >/dev/null
if docker run -d --name "$db_container" \
  --label "com.docker.compose.project=$project" \
  --label "com.docker.compose.service=db" \
  --label "com.docker.compose.project.working_dir=$sandbox_deploy" \
  -e POSTGRES_USER=heliolytics \
  -e POSTGRES_PASSWORD="$owner_pw" \
  -e POSTGRES_DB=heliolytics \
  -e APP_DB_PASSWORD="$app_pw" \
  -e NO_TS_TUNE=true \
  -v "$volume:/var/lib/postgresql/data" \
  -v "$mount_root/.d01-sandbox/backend/deploy/initdb/00_roles.sh:/docker-entrypoint-initdb.d/00_roles.sh:ro" \
  -v "$mount_root/.d01-sandbox/backend/schema.sql:/docker-entrypoint-initdb.d/01_schema.sql:ro" \
  -v "$mount_root/.d01-sandbox/backend/deploy/postgresql.conf:/etc/postgresql/postgresql.conf:ro" \
  -v "$mount_root/.d01-sandbox/backend/deploy/pg_hba.conf:/etc/postgresql/pg_hba.conf:ro" \
  "$db_image" postgres -c config_file=/etc/postgresql/postgresql.conf >"$work_dir/case.out" 2>&1; then
  if wait_ready "$db_container"; then
    ok 'fresh initialization becomes ready under the mounted SCRAM configuration'
  else
    not_ok 'fresh initialization never became ready'
    docker logs --tail 40 "$db_container" >&2
  fi
else
  not_ok 'sandbox database container failed to start'
  diagnose "$work_dir/case.out"
fi

if owner_sql "$db_container" <<'SQL' >/dev/null 2>&1
DO $do$
DECLARE t text; n bigint;
BEGIN
  FOR t IN SELECT tablename FROM pg_tables WHERE schemaname = 'public' ORDER BY tablename LOOP
    EXECUTE format('SELECT count(*) FROM public.%I', t) INTO n;
    IF n <> 0 THEN RAISE EXCEPTION 'table % has % rows', t, n; END IF;
  END LOOP;
END
$do$;
SQL
then
  ok 'fresh initialization leaves every public application table empty'
else
  not_ok 'fresh initialization left rows behind'
fi

if app_sql "$db_container" <<<"SELECT 1" >/dev/null 2>&1; then
  ok 'app role authenticates with a password containing quotes, dollars and backslashes'
else
  not_ok 'app role could not authenticate with the exact password'
fi

if docker exec "$db_container" sh -c \
  'PGPASSWORD=wrong psql -h 127.0.0.1 -U heliolytics_app -d heliolytics -Atqc "SELECT 1"' >/dev/null 2>&1; then
  not_ok 'app role accepted a wrong password'
else
  ok 'app role rejects a wrong password'
fi

if docker exec "$db_container" sh -c \
  'psql -h 127.0.0.1 -U heliolytics_app -d heliolytics -Atqc "SELECT 1"' >/dev/null 2>&1; then
  not_ok 'app role accepted a passwordless connection'
else
  ok 'app role rejects a passwordless connection'
fi

if docker exec "$db_container" getent hosts ::1 >/dev/null 2>&1; then
  if docker exec "$db_container" sh -c \
    'PGPASSWORD="$APP_DB_PASSWORD" psql -h ::1 -U heliolytics_app -d heliolytics -Atqc "SELECT 1"' >/dev/null 2>&1; then
    ok 'app role authenticates over IPv6 loopback'
  else
    not_ok 'app role failed over IPv6 loopback'
  fi
else
  ok 'IPv6 loopback row not applicable in this sandbox'
fi

if app_sql "$db_container" <<<"CREATE ROLE injected LOGIN" >/dev/null 2>&1; then
  not_ok 'app role created a new role'
else
  ok 'app role cannot create roles'
fi

if app_sql "$db_container" <<<"CREATE TABLE public.injected (x int)" >/dev/null 2>&1; then
  not_ok 'app role created a table'
else
  ok 'app role cannot create tables'
fi

if app_sql "$db_container" <<<"DELETE FROM raw_blobs" >/dev/null 2>&1; then
  not_ok 'app role deleted immutable raw blobs'
else
  ok 'app role cannot delete immutable raw rows'
fi

if app_sql "$db_container" <<<"SELECT count(*) FROM daily_metrics" >/dev/null 2>&1; then
  ok 'app role can read application tables'
else
  not_ok 'app role could not read application tables'
fi

injected_roles="$(owner_sql "$db_container" <<<"SELECT count(*) FROM pg_roles WHERE rolname NOT LIKE 'pg\\_%' AND rolname NOT IN ('heliolytics','heliolytics_app')")"
if [[ "$injected_roles" == "0" ]]; then
  ok 'role initialization injected no extra role'
else
  not_ok 'role initialization created an unexpected role'
fi

owner_sql "$db_container" <<'SQL' >/dev/null 2>&1
INSERT INTO coverage (type_code, data_through)
VALUES ('0x0D', TIMESTAMPTZ '2026-06-01 00:00:00+05:30')
ON CONFLICT (type_code) DO NOTHING;
SQL
docker restart "$db_container" >/dev/null
if wait_ready "$db_container"; then
  marker_rows="$(owner_sql "$db_container" <<<"SELECT count(*) FROM coverage WHERE type_code = '0x0D'")"
  if [[ "$marker_rows" == "1" ]]; then
    ok 'restart preserves data and does not rerun initialization'
  else
    not_ok 'restart lost data or reran initialization'
  fi
else
  not_ok 'database did not come back after restart'
fi

# --------------------------------------- G. dump, failed copy, real restore
if docker cp "$deploy_dir/tests/fixture-ops.sql" "$db_container:/tmp/fixture-ops.sql" >/dev/null 2>&1 \
  && docker exec "$db_container" sh -c \
    'PGPASSWORD="$POSTGRES_PASSWORD" psql -h 127.0.0.1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1 -q -f /tmp/fixture-ops.sql' \
    >"$work_dir/case.out" 2>&1; then
  ok 'synthetic fixture loads and compresses an older chunk'
else
  not_ok 'synthetic fixture failed to load'
  diagnose "$work_dir/case.out"
fi

backup_dir="$work_dir/backups"
mkdir -p "$backup_dir"
mkdir -p "$work_dir/stub-copy"
printf '#!/bin/sh\nexit 1\n' >"$work_dir/stub-copy/scp"
printf '#!/bin/sh\nexit 1\n' >"$work_dir/stub-copy/ssh"
chmod +x "$work_dir/stub-copy/scp" "$work_dir/stub-copy/ssh"

# The sandbox database is not project deploy, so it is named explicitly.
if PATH="$work_dir/stub-copy:$PATH" HELIOLYTICS_DEPLOY_DIR="$sandbox_deploy" \
  BACKUP_DIR="$backup_dir" BACKUP_DESTINATION=heliolytics-d01-missing:/backups \
  "$sandbox_deploy/backup-db.sh" --source-container "$db_container" >"$work_dir/case.out" 2>&1; then
  not_ok 'backup reported success when the off-host copy failed'
else
  produced="$(find "$backup_dir" -maxdepth 1 -type f -name 'heliolytics-*.dump' | head -n 1)"
  if [[ -n "$produced" && -s "$produced" && -f "$produced.sha256" && -f "$produced.info" ]]; then
    ok 'backup fails on a failed copy and keeps a valid local dump'
  else
    not_ok 'backup failed but left no usable local dump'
    diagnose "$work_dir/case.out"
  fi
fi

dump_file="$(find "$backup_dir" -maxdepth 1 -type f -name 'heliolytics-*.dump' | head -n 1)"
if [[ -z "$dump_file" || ! -f "$dump_file" ]]; then
  not_ok 'no dump is available for the restore rehearsal'
  printf '\n%d passed, %d failed, %d owner-pending\n' "$pass_count" "$fail_count" "$pending_count"
  exit 1
fi
dump_sha="$(sha256sum -- "$dump_file" | cut -d' ' -f1)"
if grep -q '^image_ref=' "$dump_file.info"; then
  ok 'backup sidecar records the source image'
else
  not_ok 'backup sidecar is missing the source image'
fi

if PATH="$work_dir/stub-copy:$PATH" HELIOLYTICS_DEPLOY_DIR="$sandbox_deploy" \
  BACKUP_DIR="$backup_dir" BACKUP_DESTINATION=heliolytics-d01-missing:/backups \
  "$sandbox_deploy/backup-db.sh" --source-container "$restore_container" >"$work_dir/case.out" 2>&1; then
  not_ok 'backup accepted a container without the database labels'
else
  ok 'backup refuses a container without the expected labels'
fi

docker volume create "${restore_container}-data" >/dev/null
# Timescale stores the job owner role name as row data, so the restore target
# must create the dump's source owner role before pg_restore runs.
if docker run -d --name "$restore_container" --network none \
  -e POSTGRES_USER=heliolytics -e POSTGRES_PASSWORD="$restore_pw" -e POSTGRES_DB=heliolytics \
  -e NO_TS_TUNE=true \
  -v "${restore_container}-data:/var/lib/postgresql/data" \
  "$db_image" postgres >"$work_dir/case.out" 2>&1; then
  if wait_ready "$restore_container"; then
    owner_sql "$restore_container" \
      <<<"CREATE EXTENSION IF NOT EXISTS timescaledb; SELECT timescaledb_pre_restore();" >/dev/null 2>&1
    if docker exec -i "$restore_container" sh -c \
      'PGPASSWORD="$POSTGRES_PASSWORD" pg_restore -h 127.0.0.1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" --exit-on-error --no-owner --no-acl' \
      <"$dump_file" >"$work_dir/case.out" 2>&1; then
      owner_sql "$restore_container" <<<"SELECT timescaledb_post_restore();" >/dev/null 2>&1
      raw_hex="$(owner_sql "$restore_container" \
        <<<"SELECT encode(payload, 'hex') FROM raw_blobs WHERE session_id = 'fixture-session-1' AND type_code = '0x01'")"
      daily="$(owner_sql "$restore_container" \
        <<<"SELECT steps::text FROM daily_metrics WHERE day_key = DATE '2026-06-01'")"
      compressed="$(owner_sql "$restore_container" \
        <<<"SELECT count(*) FROM timescaledb_information.chunks WHERE is_compressed")"
      if [[ "$raw_hex" == "00ff0d0a7f80" && "$daily" == "42" && "$compressed" -ge 1 ]]; then
        ok 'restored dump keeps raw bytes, daily values and compressed chunks'
      else
        not_ok 'restored dump has unexpected fixture values'
        printf 'raw_payload=%s daily_steps=%s compressed_chunks=%s\n' \
          "$raw_hex" "$daily" "$compressed" >&2
      fi
    else
      not_ok 'pg_restore failed on the produced dump'
      diagnose "$work_dir/case.out"
    fi
  else
    not_ok 'restore container never became ready'
    docker logs --tail 20 "$restore_container" >&2
  fi
else
  not_ok 'restore container failed to start'
  diagnose "$work_dir/case.out"
fi

# ------------------------------------------------ H. guarded sandbox wipe
if printf 'DELETE %s FOR %s\n' "$volume" "$project" | \
  "$sandbox_deploy/reset-db.sh" --project "$project" --volume "$volume" \
    --backup-file "$dump_file" --backup-sha256 "$dump_sha" >"$work_dir/case.out" 2>&1; then
  ok 'authorized sandbox wipe removes the exact volume'
else
  not_ok 'authorized sandbox wipe failed'
  diagnose "$work_dir/case.out"
fi

if docker volume inspect "$canary_volume" >/dev/null 2>&1; then
  ok 'unrelated canary volume survives the wipe'
else
  not_ok 'the wipe removed an unrelated volume'
fi

if docker volume inspect "$volume" >/dev/null 2>&1; then
  not_ok 'the wiped volume still exists'
else
  ok 'the target volume is gone'
fi

if docker ps -a --format '{{.Names}}' | grep -qx "$db_container"; then
  not_ok 'the database container still exists after the wipe'
else
  ok 'the database container is gone after the wipe'
fi

pending 'off-host SCP copy and read-back hash against a real SSH destination'

printf '\n%d passed, %d failed, %d owner-pending\n' "$pass_count" "$fail_count" "$pending_count"
if ((fail_count > 0)); then
  exit 1
fi
