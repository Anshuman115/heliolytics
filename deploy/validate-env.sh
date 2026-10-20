#!/usr/bin/env bash
# Validate resolved Compose configuration without printing secrets or saving it.
# Does not start services.
set -euo pipefail

usage() {
  printf 'usage: %s --project <name> --env-file <absolute path> --compose-file <absolute path>' "$0" >&2
  printf ' [--expected-web-hash-file <absolute path>] [--node-image <ref>]\n' >&2
}

script_dir=
script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
backend_dir=
backend_dir="$(cd -- "$script_dir/.." && pwd)"

project=
env_file=
compose_file=
expected_hash_file=
node_image="node:24.22.0-alpine3.24@sha256:44fce696f38ba8c987c4db89954051000bbd05ffd8d8659ba15ed4a8602e937f"

while (($# > 0)); do
  case "$1" in
    --project)
      [[ $# -ge 2 ]] || { usage; exit 2; }
      project="$2"
      shift 2
      ;;
    --env-file)
      [[ $# -ge 2 ]] || { usage; exit 2; }
      env_file="$2"
      shift 2
      ;;
    --compose-file)
      [[ $# -ge 2 ]] || { usage; exit 2; }
      compose_file="$2"
      shift 2
      ;;
    --expected-web-hash-file)
      [[ $# -ge 2 ]] || { usage; exit 2; }
      expected_hash_file="$2"
      shift 2
      ;;
    --node-image)
      [[ $# -ge 2 ]] || { usage; exit 2; }
      node_image="$2"
      shift 2
      ;;
    *)
      printf 'unknown option\n' >&2
      usage
      exit 2
      ;;
  esac
done

fail() {
  printf 'FAIL %s: %s\n' "$1" "$2" >&2
  exit 1
}

if [[ -z "$project" || -z "$env_file" || -z "$compose_file" ]]; then
  usage
  exit 2
fi
if [[ "$env_file" != /* || "$compose_file" != /* ]]; then
  fail arguments 'env file and compose file must be absolute paths'
fi
if [[ ! -f "$env_file" ]]; then
  fail env_file 'file not found'
fi
if [[ ! -f "$compose_file" ]]; then
  fail compose_file 'file not found'
fi
if [[ ! -f "$backend_dir/schema.sql" || ! -f "$script_dir/initdb/00_roles.sh" ]]; then
  fail required_files 'schema.sql or deploy/initdb/00_roles.sh is missing'
fi

file_mode() {
  local path="$1" mode
  if mode="$(stat -c '%a' -- "$path" 2>/dev/null)"; then
    printf '%s' "$mode"
    return 0
  fi
  if mode="$(stat -f '%Lp' -- "$path" 2>/dev/null)"; then
    printf '%s' "$mode"
    return 0
  fi
  return 1
}

file_owner() {
  local path="$1" owner
  if owner="$(stat -c '%u' -- "$path" 2>/dev/null)"; then
    printf '%s' "$owner"
    return 0
  fi
  if owner="$(stat -f '%u' -- "$path" 2>/dev/null)"; then
    printf '%s' "$owner"
    return 0
  fi
  return 1
}

env_mode=
env_mode="$(file_mode "$env_file")" || fail env_file 'mode cannot be read'
env_owner=
env_owner="$(file_owner "$env_file")" || fail env_file 'owner cannot be read'
if [[ "$env_mode" != "600" && "$env_mode" != "0600" ]]; then
  fail env_file 'file must be mode 0600'
fi
if [[ "$env_owner" != "$(id -u)" ]]; then
  fail env_file 'file must be owned by the running user'
fi

HELIOLYTICS_COMMIT=
HELIOLYTICS_WEB_COMMIT=
HELIOLYTICS_BUILT_AT=
POSTGRES_PASSWORD=
APP_DB_PASSWORD=
HELIOLYTICS_WEB_API_KEY=
HELIOLYTICS_PHONE_API_KEY=
HELIOLYTICS_SESSION_SECRET=
HELIOLYTICS_WEB_PASSWORD_HASH=
API_PORT=
WEB_PORT=
ADMINER_PORT=
TRUST_PROXY=
TRUSTED_PROXY_CIDRS=
COOKIE_SECURE=
DATA_SUBNET=
INGRESS_SUBNET=
CLOUDFLARED_IP=
RATE_LIMIT_PER_MIN=
RATE_LIMIT_PER_KEY_PER_MIN=
REQUEST_TIMEOUT_SECONDS=
MAX_CONCURRENT_BODY_READS=

seen_keys=" "
while IFS= read -r line || [[ -n "$line" ]]; do
  if [[ -z "${line//[[:space:]]/}" ]]; then
    continue
  fi
  if [[ "$line" == \#* ]]; then
    continue
  fi
  if [[ ! "$line" =~ ^([A-Z][A-Z0-9_]*)=(.*)$ ]]; then
    fail env_file 'line is neither a comment nor KEY=VALUE'
  fi
  key="${BASH_REMATCH[1]}"
  value="${BASH_REMATCH[2]}"
  value="${value%$'\r'}"
  if [[ "$seen_keys" == *" $key "* ]]; then
    fail "$key" 'duplicate key in env file'
  fi
  seen_keys="$seen_keys$key "
  if [[ ${#value} -ge 2 && "$value" == \'*\' ]]; then
    value="${value:1:${#value}-2}"
  fi
  case "$key" in
    HELIOLYTICS_COMMIT) HELIOLYTICS_COMMIT="$value" ;;
    HELIOLYTICS_WEB_COMMIT) HELIOLYTICS_WEB_COMMIT="$value" ;;
    HELIOLYTICS_BUILT_AT) HELIOLYTICS_BUILT_AT="$value" ;;
    POSTGRES_PASSWORD) POSTGRES_PASSWORD="$value" ;;
    APP_DB_PASSWORD) APP_DB_PASSWORD="$value" ;;
    HELIOLYTICS_WEB_API_KEY) HELIOLYTICS_WEB_API_KEY="$value" ;;
    HELIOLYTICS_PHONE_API_KEY) HELIOLYTICS_PHONE_API_KEY="$value" ;;
    HELIOLYTICS_SESSION_SECRET) HELIOLYTICS_SESSION_SECRET="$value" ;;
    HELIOLYTICS_WEB_PASSWORD_HASH) HELIOLYTICS_WEB_PASSWORD_HASH="$value" ;;
    API_PORT) API_PORT="$value" ;;
    WEB_PORT) WEB_PORT="$value" ;;
    ADMINER_PORT) ADMINER_PORT="$value" ;;
    TRUST_PROXY) TRUST_PROXY="$value" ;;
    TRUSTED_PROXY_CIDRS) TRUSTED_PROXY_CIDRS="$value" ;;
    COOKIE_SECURE) COOKIE_SECURE="$value" ;;
    DATA_SUBNET) DATA_SUBNET="$value" ;;
    INGRESS_SUBNET) INGRESS_SUBNET="$value" ;;
    CLOUDFLARED_IP) CLOUDFLARED_IP="$value" ;;
    RATE_LIMIT_PER_MIN) RATE_LIMIT_PER_MIN="$value" ;;
    RATE_LIMIT_PER_KEY_PER_MIN) RATE_LIMIT_PER_KEY_PER_MIN="$value" ;;
    REQUEST_TIMEOUT_SECONDS) REQUEST_TIMEOUT_SECONDS="$value" ;;
    MAX_CONCURRENT_BODY_READS) MAX_CONCURRENT_BODY_READS="$value" ;;
    API_IMAGE | WEB_IMAGE) ;;
    *) ;;
  esac
done <"$env_file"

hex64='^[0-9a-f]{64}$'
sha40='^[0-9a-f]{40}$'
rfc3339_utc='^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$'

require_value() {
  local field="$1" value="$2"
  if [[ -z "$value" ]]; then
    fail "$field" 'value is empty'
  fi
}

require_value HELIOLYTICS_COMMIT "$HELIOLYTICS_COMMIT"
require_value HELIOLYTICS_WEB_COMMIT "$HELIOLYTICS_WEB_COMMIT"
require_value HELIOLYTICS_BUILT_AT "$HELIOLYTICS_BUILT_AT"
require_value POSTGRES_PASSWORD "$POSTGRES_PASSWORD"
require_value APP_DB_PASSWORD "$APP_DB_PASSWORD"
require_value HELIOLYTICS_WEB_API_KEY "$HELIOLYTICS_WEB_API_KEY"
require_value HELIOLYTICS_PHONE_API_KEY "$HELIOLYTICS_PHONE_API_KEY"
require_value HELIOLYTICS_SESSION_SECRET "$HELIOLYTICS_SESSION_SECRET"
require_value HELIOLYTICS_WEB_PASSWORD_HASH "$HELIOLYTICS_WEB_PASSWORD_HASH"
require_value TRUST_PROXY "$TRUST_PROXY"
require_value TRUSTED_PROXY_CIDRS "$TRUSTED_PROXY_CIDRS"
require_value COOKIE_SECURE "$COOKIE_SECURE"
require_value CLOUDFLARED_IP "$CLOUDFLARED_IP"
require_value DATA_SUBNET "$DATA_SUBNET"
require_value INGRESS_SUBNET "$INGRESS_SUBNET"

[[ "$HELIOLYTICS_COMMIT" =~ $sha40 ]] || fail HELIOLYTICS_COMMIT 'must be a full lowercase 40-character SHA'
[[ "$HELIOLYTICS_WEB_COMMIT" =~ $sha40 ]] || fail HELIOLYTICS_WEB_COMMIT 'must be a full lowercase 40-character SHA'
[[ "$HELIOLYTICS_BUILT_AT" =~ $rfc3339_utc ]] || fail HELIOLYTICS_BUILT_AT 'must be RFC3339 UTC'

[[ "$POSTGRES_PASSWORD" =~ $hex64 ]] || fail POSTGRES_PASSWORD 'must be 64 lowercase hex characters'
[[ "$APP_DB_PASSWORD" =~ $hex64 ]] || fail APP_DB_PASSWORD 'must be 64 lowercase hex characters'
[[ "$POSTGRES_PASSWORD" != "$APP_DB_PASSWORD" ]] || fail APP_DB_PASSWORD 'owner and app database passwords must differ'

[[ "$HELIOLYTICS_WEB_API_KEY" =~ ^web\.[0-9a-f]{64}$ ]] || fail HELIOLYTICS_WEB_API_KEY 'must be web.<64 lowercase hex>'
[[ "$HELIOLYTICS_PHONE_API_KEY" =~ ^phone\.[0-9a-f]{64}$ ]] || fail HELIOLYTICS_PHONE_API_KEY 'must be phone.<64 lowercase hex>'
web_secret="${HELIOLYTICS_WEB_API_KEY#*.}"
phone_secret="${HELIOLYTICS_PHONE_API_KEY#*.}"
[[ "$phone_secret" != "$web_secret" ]] || fail HELIOLYTICS_PHONE_API_KEY 'phone and web keys must differ'

[[ "$HELIOLYTICS_SESSION_SECRET" =~ $hex64 ]] || fail HELIOLYTICS_SESSION_SECRET 'must be 64 lowercase hex characters'
[[ "$HELIOLYTICS_SESSION_SECRET" != "$phone_secret" ]] || fail HELIOLYTICS_SESSION_SECRET 'must differ from every API secret'
[[ "$HELIOLYTICS_SESSION_SECRET" != "$web_secret" ]] || fail HELIOLYTICS_SESSION_SECRET 'must differ from every API secret'

# shellcheck disable=SC2016
[[ "$HELIOLYTICS_WEB_PASSWORD_HASH" =~ ^scrypt\$N=32768\$r=8\$p=3\$[0-9a-f]{32}\$[0-9a-f]{64}$ ]] \
  || fail HELIOLYTICS_WEB_PASSWORD_HASH 'must be the full scrypt$N=32768$r=8$p=3$<salt>$<hash> line'

for secret_candidate in "$HELIOLYTICS_COMMIT" "$HELIOLYTICS_WEB_COMMIT" \
  "$HELIOLYTICS_BUILT_AT" "$phone_secret" "$web_secret" "$HELIOLYTICS_SESSION_SECRET" \
  "$POSTGRES_PASSWORD" "$APP_DB_PASSWORD"; do
  case "$secret_candidate" in
    000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f | \
      change-me-db-password | REDACTED-EXAMPLE-PLACEHOLDER | \
      generate-with-openssl-rand-hex-32 | generate-with-npm-run-auth-hash-password)
      fail deployment_secrets 'a documented example or placeholder value is in use'
      ;;
  esac
done

case "$TRUST_PROXY" in
  true | false) ;;
  *) fail TRUST_PROXY 'must be true or false' ;;
esac
case "$COOKIE_SECURE" in
  true | false) ;;
  *) fail COOKIE_SECURE 'must be true or false' ;;
esac
if [[ "$COOKIE_SECURE" == "true" && "$TRUST_PROXY" != "true" ]]; then
  fail COOKIE_SECURE 'secure cookies require trusted proxy mode'
fi

IFS=',' read -r -a cidrs <<<"$TRUSTED_PROXY_CIDRS"
for cidr in "${cidrs[@]}"; do
  if [[ ! "$cidr" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}/32$ && ! "$cidr" =~ ^[0-9a-fA-F:]+/128$ ]]; then
    fail TRUSTED_PROXY_CIDRS 'each entry must be a single-address /32 or /128 CIDR'
  fi
done
proxy_ok=0
for cidr in "${cidrs[@]}"; do
  if [[ "$cidr" == "$CLOUDFLARED_IP/32" || "$cidr" == "[$CLOUDFLARED_IP]/128" ]]; then
    proxy_ok=1
  fi
done
if [[ "$proxy_ok" != "1" ]]; then
  fail TRUSTED_PROXY_CIDRS 'trusted CIDRs must include the configured connector address'
fi

[[ "$DATA_SUBNET" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}/[0-9]{1,2}$ ]] || fail DATA_SUBNET 'must be an IPv4 CIDR'
[[ "$INGRESS_SUBNET" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}/[0-9]{1,2}$ ]] || fail INGRESS_SUBNET 'must be an IPv4 CIDR'
[[ "$CLOUDFLARED_IP" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}$ ]] || fail CLOUDFLARED_IP 'must be an IPv4 address'

check_port() {
  local field="$1" value="$2"
  if [[ -z "$value" ]]; then
    return 0
  fi
  if [[ ! "$value" =~ ^[0-9]+$ ]] || ((10#$value < 1 || 10#$value > 65535)); then
    fail "$field" 'must be a port number'
  fi
}
check_port API_PORT "$API_PORT"
check_port WEB_PORT "$WEB_PORT"
check_port ADMINER_PORT "$ADMINER_PORT"

check_positive() {
  local field="$1" value="$2"
  if [[ -z "$value" ]]; then
    return 0
  fi
  if [[ ! "$value" =~ ^[0-9]+$ ]] || ((10#$value < 1)); then
    fail "$field" 'must be a positive integer'
  fi
}
check_positive RATE_LIMIT_PER_MIN "$RATE_LIMIT_PER_MIN"
check_positive RATE_LIMIT_PER_KEY_PER_MIN "$RATE_LIMIT_PER_KEY_PER_MIN"
check_positive MAX_CONCURRENT_BODY_READS "$MAX_CONCURRENT_BODY_READS"
if [[ -n "$REQUEST_TIMEOUT_SECONDS" ]]; then
  if [[ ! "$REQUEST_TIMEOUT_SECONDS" =~ ^[0-9]+$ ]] \
    || ((10#$REQUEST_TIMEOUT_SECONDS < 1 || 10#$REQUEST_TIMEOUT_SECONDS > 90)); then
    fail REQUEST_TIMEOUT_SECONDS 'must be an integer between 1 and 90'
  fi
fi

if ! command -v docker >/dev/null 2>&1; then
  fail docker 'docker is required for the resolved environment check'
fi

helper="$script_dir/validate-env.mjs"
if [[ ! -f "$helper" ]]; then
  fail required_files 'deploy/validate-env.mjs is missing'
fi

compose_config() {
  # Profiles are enabled so the resolved output still proves the optional
  # services stay behind them. Nothing is built or started here.
  docker compose --project-name "$project" --env-file "$env_file" \
    --profile tools --profile tunnel \
    --file "$compose_file" config --format json
}

helper_source=
if ! helper_source="$(base64 <"$helper" | tr -d '\n')"; then
  fail required_files 'deploy/validate-env.mjs could not be read'
fi

compose_source=
if ! compose_source="$(base64 <"$compose_file" | tr -d '\n')"; then
  fail required_files 'the compose file could not be read'
fi

run_helper() {
  docker run --rm -i --network none --read-only --tmpfs /tmp \
    -e "HELIOLYTICS_VALIDATOR_SOURCE=$helper_source" \
    -e "HELIOLYTICS_VALIDATOR_COMPOSE_SOURCE=$compose_source" "$@" \
    --entrypoint sh "$node_image" -c \
    'printf %s "$HELIOLYTICS_VALIDATOR_SOURCE" | base64 -d > /tmp/validate-env.mjs && exec node /tmp/validate-env.mjs'
}

if ! compose_config | run_helper; then
  exit 1
fi

if [[ -z "$expected_hash_file" ]]; then
  exit 0
fi
if [[ "$expected_hash_file" != /* || ! -f "$expected_hash_file" ]]; then
  fail expected_web_hash_file 'absolute path to the generator output is required'
fi

raw_expected=
IFS= read -r -d '' raw_expected <"$expected_hash_file" || true
if [[ "$raw_expected" == *$'\n' ]]; then
  raw_expected="${raw_expected%$'\n'}"
fi
if [[ ! "$raw_expected" =~ ^scrypt\$N=32768\$r=8\$p=3\$[0-9a-f]{32}\$[0-9a-f]{64}$ ]]; then
  fail expected_web_hash_file 'generator output is not a complete scrypt verifier line'
fi

private_dir=
private_dir="$(umask 077 && mktemp -d)"
trap 'rm -rf "$private_dir"' EXIT
{
  printf 'HELIOLYTICS_WEB_PASSWORD_HASH=%s\n' "$raw_expected"
  printf 'HELIOLYTICS_EXPECTED_WEB_PASSWORD_HASH=%s\n' "$raw_expected"
} >"$private_dir/web-hash.env"
chmod 600 -- "$private_dir/web-hash.env"

if ! compose_config | run_helper --env-file "$private_dir/web-hash.env"; then
  exit 1
fi
