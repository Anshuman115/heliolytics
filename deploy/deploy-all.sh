#!/usr/bin/env bash
# Rebuilds the API and web from clean main checkouts.
set -euo pipefail

script_dir=
script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
backend_dir=
backend_dir="$(cd -- "$script_dir/.." && pwd)"
web_dir="$backend_dir/../heliolytics-web"
env_file="$script_dir/.env"
compose_file="$script_dir/docker-compose.yml"

if (($# > 0)); then
  printf 'usage: %s\n' "$0" >&2
  exit 2
fi

[[ -f "$env_file" ]] || { printf 'missing %s\n' "$env_file" >&2; exit 1; }
[[ -d "$web_dir/.git" ]] || { printf 'missing web checkout: %s\n' "$web_dir" >&2; exit 1; }

check_main() {
  local dir="$1" label="$2"
  if [[ "$(git -C "$dir" rev-parse --abbrev-ref HEAD)" != "main" ]]; then
    printf '%s checkout is not on main\n' "$label" >&2
    exit 1
  fi
  if [[ -n "$(git -C "$dir" status --porcelain)" ]]; then
    printf '%s checkout has uncommitted changes\n' "$label" >&2
    exit 1
  fi
}

check_main "$backend_dir" backend
check_main "$web_dir" web

set_env() {
  local key="$1" value="$2" tmp
  tmp="$(mktemp)"
  sed "s|^${key}=.*|${key}=${value}|" "$env_file" >"$tmp"
  chmod 600 "$tmp"
  mv -- "$tmp" "$env_file"
}

set_env HELIOLYTICS_COMMIT "$(git -C "$backend_dir" rev-parse HEAD)"
set_env HELIOLYTICS_WEB_COMMIT "$(git -C "$web_dir" rev-parse HEAD)"
set_env HELIOLYTICS_BUILT_AT "$(date -u +%Y-%m-%dT%H:%M:%SZ)"

"$script_dir/validate-env.sh" --project deploy --env-file "$env_file" --compose-file "$compose_file"

docker compose --project-name deploy --env-file "$env_file" --file "$compose_file" \
  up -d --build --no-deps api web
