#!/usr/bin/env bash
# Rebuilds the API from a clean main checkout.
set -euo pipefail

script_dir=
script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
backend_dir=
backend_dir="$(cd -- "$script_dir/.." && pwd)"
env_file="$script_dir/.env"
compose_file="$script_dir/docker-compose.yml"

if (($# > 0)); then
  printf 'usage: %s\n' "$0" >&2
  exit 2
fi

[[ -f "$env_file" ]] || { printf 'missing %s\n' "$env_file" >&2; exit 1; }

if [[ "$(git -C "$backend_dir" rev-parse --abbrev-ref HEAD)" != "main" ]]; then
  printf 'backend checkout is not on main\n' >&2
  exit 1
fi
if [[ -n "$(git -C "$backend_dir" status --porcelain)" ]]; then
  printf 'backend checkout has uncommitted changes\n' >&2
  exit 1
fi

set_env() {
  local key="$1" value="$2" tmp
  tmp="$(mktemp)"
  sed "s|^${key}=.*|${key}=${value}|" "$env_file" >"$tmp"
  chmod 600 "$tmp"
  mv -- "$tmp" "$env_file"
}

set_env HELIOLYTICS_COMMIT "$(git -C "$backend_dir" rev-parse HEAD)"
set_env HELIOLYTICS_BUILT_AT "$(date -u +%Y-%m-%dT%H:%M:%SZ)"

"$script_dir/validate-env.sh" --project deploy --env-file "$env_file" --compose-file "$compose_file"

docker compose --project-name deploy --env-file "$env_file" --file "$compose_file" \
  up -d --build --no-deps api
