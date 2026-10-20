#!/usr/bin/env bash
# Recreates only web. Production requires clean backend and web checkouts on main.
set -euo pipefail

usage() {
  printf 'usage: %s --web-commit <40-hex> --built-at <rfc3339-utc>' "$0" >&2
  printf ' [--project <name>] [--web-dir <path>] [--env-file <path>]' >&2
  printf ' [--compose-file <path>] [--expected-web-hash-file <path>] [--preflight-only]\n' >&2
}

script_dir=
script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
backend_dir=
backend_dir="$(cd -- "$script_dir/.." && pwd)"

project=deploy
web_dir="$backend_dir/../heliolytics-web"
env_file="$script_dir/.env"
compose_file="$script_dir/docker-compose.yml"
expected_hash_file=
preflight_only=0
web_commit=
built_at=

while (($# > 0)); do
  case "$1" in
    --web-commit)
      [[ $# -ge 2 ]] || { usage; exit 2; }
      web_commit="$2"
      shift 2
      ;;
    --built-at)
      [[ $# -ge 2 ]] || { usage; exit 2; }
      built_at="$2"
      shift 2
      ;;
    --project)
      [[ $# -ge 2 ]] || { usage; exit 2; }
      project="$2"
      shift 2
      ;;
    --web-dir)
      [[ $# -ge 2 ]] || { usage; exit 2; }
      web_dir="$2"
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
    --preflight-only)
      preflight_only=1
      shift
      ;;
    *)
      printf 'unknown option: %s\n' "$1" >&2
      usage
      exit 2
      ;;
  esac
done

if [[ -z "$web_commit" || -z "$built_at" ]]; then
  printf 'both --web-commit and --built-at are required\n' >&2
  usage
  exit 2
fi
if [[ ! "$web_commit" =~ ^[0-9a-f]{40}$ ]]; then
  printf '--web-commit must be a full lowercase 40-character SHA\n' >&2
  exit 2
fi
if [[ ! "$built_at" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$ ]]; then
  printf '--built-at must be RFC3339 UTC such as 2026-10-20T04:00:00Z\n' >&2
  exit 2
fi

for required in "$env_file" "$compose_file"; do
  if [[ ! -f "$required" ]]; then
    printf 'missing required file: %s\n' "$required" >&2
    exit 1
  fi
done

require_clean_main() {
  local label="$1" dir="$2" head
  if [[ ! -d "$dir/.git" ]]; then
    printf '%s checkout is not a git worktree: %s\n' "$label" "$dir" >&2
    return 1
  fi
  if [[ "$(git -C "$dir" rev-parse --abbrev-ref HEAD)" != "main" ]]; then
    printf '%s checkout is not on main; refusing to deploy\n' "$label" >&2
    return 1
  fi
  if [[ -n "$(git -C "$dir" status --porcelain)" ]]; then
    printf '%s checkout has uncommitted changes; refusing to deploy\n' "$label" >&2
    return 1
  fi
  head="$(git -C "$dir" rev-parse HEAD)"
  if [[ "$head" != "$web_commit" && "$label" == "web" ]]; then
    printf 'web HEAD does not match --web-commit; refusing to deploy\n' >&2
    return 1
  fi
  return 0
}

require_clean_main backend "$backend_dir"
require_clean_main web "$web_dir"

if [[ ! -f "$web_dir/Dockerfile" ]]; then
  printf 'web Dockerfile not found in %s\n' "$web_dir" >&2
  exit 1
fi

validator_args=(
  --project "$project"
  --env-file "$env_file"
  --compose-file "$compose_file"
)
if [[ -n "$expected_hash_file" ]]; then
  validator_args+=(--expected-web-hash-file "$expected_hash_file")
fi
"$script_dir/validate-env.sh" "${validator_args[@]}"

compose() {
  docker compose --project-name "$project" --env-file "$env_file" \
    --file "$compose_file" "$@"
}

export HELIOLYTICS_WEB_COMMIT="$web_commit"
export HELIOLYTICS_BUILT_AT="$built_at"

if [[ "$preflight_only" == "1" ]]; then
  printf 'preflight only: source, metadata and environment checks passed\n'
  printf 'would run: docker compose build web\n'
  printf 'would run: docker compose up -d --no-deps web\n'
  exit 0
fi

printf 'building web image\n'
compose build web

printf 'recreating web container only\n'
compose up -d --no-deps web

printf 'web redeployed; db, api and tunnel were not touched\n'
