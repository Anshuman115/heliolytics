#!/usr/bin/env bash
set -euo pipefail

DB_IMAGE='timescale/timescaledb:2.30.2-pg16@sha256:6f139d56042989bd35f50ba5986e492cd32e35cc6778c50be2659add298dc09f'
DB_CONTAINER='heliolytics-ci-db'
DB_URL='postgres://heliolytics:test-local-only@127.0.0.1:5445/heliolytics_test_admin?sslmode=disable'
TMP="${RUNNER_TEMP:-/tmp}"

step() { printf '\n##### %s\n' "$1"; }
fail() { printf 'ERROR: %s\n' "$1" >&2; exit 1; }

created=
cleanup() {
  local rc=$?
  if [ -n "$created" ] && docker container inspect "$DB_CONTAINER" >/dev/null 2>&1; then
    docker rm -f -v "$DB_CONTAINER" >/dev/null 2>&1 || true
  fi
  exit "$rc"
}
trap cleanup EXIT

show_failures() {
  python3 - "$1" <<'PY'
import json
import sys

failed = set()
with open(sys.argv[1], encoding="utf-8") as fh:
    for line in fh:
        try:
            ev = json.loads(line)
        except json.JSONDecodeError:
            continue
        if ev.get("Action") == "fail":
            failed.add((ev.get("Package", ""), ev.get("Test", "")))

with open(sys.argv[1], encoding="utf-8") as fh:
    for line in fh:
        try:
            ev = json.loads(line)
        except json.JSONDecodeError:
            print(line.rstrip())
            continue
        key = (ev.get("Package", ""), ev.get("Test", ""))
        if ev.get("Action") == "output" and key in failed:
            sys.stdout.write(ev.get("Output", ""))
PY
}

run_tests() {
  local name=$1
  shift
  local jsonl="$TMP/$name.jsonl"
  local stderr="$TMP/$name.stderr"
  if go test -json -count=1 "$@" ./... >"$jsonl" 2>"$stderr"; then
    python3 .github/ci/go-test-check.py "$jsonl"
    return 0
  fi
  cat "$stderr" >&2
  show_failures "$jsonl"
  python3 .github/ci/go-test-check.py "$jsonl" || true
  fail "go test $name failed"
}

step "1 disposable test database"
export HELIOLYTICS_TEST_DB_IMAGE="$DB_IMAGE"
if docker container inspect "$DB_CONTAINER" >/dev/null 2>&1; then
  fail "$DB_CONTAINER already exists"
fi

if ! err=$(docker create --name "$DB_CONTAINER" \
  --label heliolytics.test=true \
  --label heliolytics.test.project=ci \
  -e POSTGRES_USER=heliolytics \
  -e POSTGRES_PASSWORD=test-local-only \
  -e POSTGRES_DB=heliolytics_test_admin \
  -p 127.0.0.1:5445:5432 \
  -p '[::1]:5445:5432' \
  -v /var/lib/postgresql/data \
  "$HELIOLYTICS_TEST_DB_IMAGE" 2>&1 >/dev/null); then
  printf '%s\n' "$err" >&2
  if grep -Eqi 'ipv6|ip6|cannot assign|address not available|address family' <<<"$err"; then
    fail "the [::1]:5445:5432 publish failed: enable IPv6 loopback in the Docker daemon"
  fi
  fail "docker create failed (see the daemon error above)"
fi
created=1

docker cp schema.sql "$DB_CONTAINER:/docker-entrypoint-initdb.d/01_schema.sql" || fail "docker cp schema.sql failed"
if ! err=$(docker start "$DB_CONTAINER" 2>&1); then
  printf '%s\n' "$err" >&2
  if grep -Eqi 'ipv6|ip6|cannot assign|address not available|address family' <<<"$err"; then
    fail "the [::1]:5445:5432 publish failed: enable IPv6 loopback in the Docker daemon"
  fi
  fail "docker start failed (see the daemon error above)"
fi

published=$(docker port "$DB_CONTAINER" 5432/tcp)
printf '%s\n' "$published"
grep -q '127.0.0.1:5445' <<<"$published" || fail "the 127.0.0.1:5445:5432 publish is missing"
grep -q '\[::1\]:5445' <<<"$published" || fail "the [::1]:5445:5432 publish is missing: enable IPv6 loopback in the Docker daemon"

ready=
for _ in $(seq 1 90); do
  out=$(docker exec -e PGPASSWORD=test-local-only "$DB_CONTAINER" psql -h 127.0.0.1 -X -U heliolytics \
    -d heliolytics_test_admin -Atc "SELECT to_regclass('public.raw_blobs') IS NOT NULL, to_regclass('public.auth_nonces') IS NOT NULL, EXISTS (SELECT 1 FROM pg_extension WHERE extname='timescaledb');" 2>/dev/null) || out=
  if [ "$out" = 't|t|t' ]; then
    ready=1
    break
  fi
  sleep 2
done
if [ -z "$ready" ]; then
  docker logs "$DB_CONTAINER" 2>&1 | tail -40
  fail "test database did not reach t|t|t"
fi

python3 -c 'import socket; socket.create_connection(("::1", 5445), 5).close()' ||
  fail "nothing accepts connections on [::1]:5445: enable IPv6 loopback in the Docker daemon"

export TEST_DATABASE_URL="$DB_URL"
export HELIOLYTICS_TEST_DB_CONTAINER_ID="$(docker inspect --format '{{.Id}}' "$DB_CONTAINER")"

step "2 sqlc generate and diff"
go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1
SQLC="$(go env GOPATH)/bin/sqlc"
"$SQLC" version
"$SQLC" generate
"$SQLC" diff
git diff --exit-code -- internal/store/db || fail "sqlc output differs from the committed files"
untracked=$(git ls-files --others --exclude-standard -- internal/store/db)
[ -z "$untracked" ] || fail "untracked files in internal/store/db: $untracked"

step "3 gofmt"
unformatted=$(gofmt -l .)
if [ -n "$unformatted" ]; then
  printf '%s\n' "$unformatted"
  fail "gofmt found unformatted files"
fi

step "4 go build and go vet"
go build ./...
go vet ./...

step "5 go test and go test -race"
run_tests go-test
run_tests go-race -race

step "6 govulncheck and binary scans"
VBIN=$(mktemp -d "$TMP/govulncheck.XXXXXX")
GOBIN="$VBIN" go install golang.org/x/vuln/cmd/govulncheck@v1.9.0
"$VBIN/govulncheck" ./...
built_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
go build -ldflags "-X main.commit=${GITHUB_SHA:?GITHUB_SHA must be set} -X main.builtAt=$built_at" -o "$TMP/heliolytics-api" ./cmd/server
go build -o "$TMP/heliolytics-rebuild" ./cmd/rebuild
"$VBIN/govulncheck" -mode=binary "$TMP/heliolytics-api"
"$VBIN/govulncheck" -mode=binary "$TMP/heliolytics-rebuild"

step "7 fuzz smoke"
for target in FuzzParseWorkouts FuzzParseWorkoutDetails FuzzParseBlobs; do
  go test -count=1 ./internal/parse -run '^$' -fuzz "^${target}\$" -fuzztime=20s
done

printf '\nall backend checks passed\n'
