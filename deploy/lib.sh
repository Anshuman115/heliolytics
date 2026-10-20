# shellcheck shell=bash
# Sourced by the deploy scripts.

deploy_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
backend_dir="$(cd -- "$deploy_dir/.." && pwd)"
web_dir="${HELIOLYTICS_WEB_DIR:-$backend_dir/../heliolytics-web}"
env_file="${HELIOLYTICS_ENV_FILE:-$deploy_dir/.env}"
compose_file="$deploy_dir/docker-compose.yml"

die() {
  printf '%s\n' "$1" >&2
  exit 1
}

env_get() {
  local key="$1" line value=
  while IFS= read -r line || [[ -n "$line" ]]; do
    if [[ "$line" == "$key="* ]]; then
      value="${line#*=}"
    fi
  done <"$env_file"
  value="${value%$'\r'}"
  if [[ ${#value} -ge 2 && "$value" == \'*\' ]]; then
    value="${value:1:${#value}-2}"
  fi
  printf '%s' "$value"
}

# Replaces KEY=... with KEY=value. With "fill", only an empty KEY= is replaced.
env_put() {
  local key="$1" value="$2" mode="${3:-set}" line tmp
  tmp="$(mktemp "$env_file.XXXXXX")"
  while IFS= read -r line || [[ -n "$line" ]]; do
    if [[ "$line" == "$key=" || ( "$mode" == set && "$line" == "$key="* ) ]]; then
      printf '%s=%s\n' "$key" "$value"
    else
      printf '%s\n' "$line"
    fi
  done <"$env_file" >"$tmp"
  chmod 600 -- "$tmp"
  mv -- "$tmp" "$env_file"
}

require_env_file() {
  [[ -f "$env_file" ]] || die "missing $env_file; run deploy/install.sh first"
  project="$(env_get COMPOSE_PROJECT_NAME)"
  [[ -n "$project" ]] || die "COMPOSE_PROJECT_NAME is empty in $env_file"
}

compose() {
  docker compose --project-name "$project" --env-file "$env_file" --file "$compose_file" "$@"
}

require_clean_main() {
  local dir="$1" label="$2"
  [[ -d "$dir/.git" ]] || die "$label checkout not found at $dir"
  [[ "$(git -C "$dir" rev-parse --abbrev-ref HEAD)" == main ]] || die "$label checkout is not on main"
  [[ -z "$(git -C "$dir" status --porcelain)" ]] || die "$label checkout has uncommitted changes"
}

# Usage: record_build api|all. A backend-only deploy keeps the recorded web commit.
record_build() {
  local web_mode=fill
  [[ "$1" == all ]] && web_mode="set"
  env_put HELIOLYTICS_COMMIT "$(git -C "$backend_dir" rev-parse HEAD)"
  env_put HELIOLYTICS_WEB_COMMIT "$(git -C "$web_dir" rev-parse HEAD)" "$web_mode"
  env_put HELIOLYTICS_BUILT_AT "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
}

validate() {
  "$deploy_dir/validate-env.sh" --project "$project" --env-file "$env_file" --compose-file "$compose_file"
}
