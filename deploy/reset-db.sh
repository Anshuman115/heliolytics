#!/usr/bin/env bash
# Deletes the database volume, then rebuilds and starts the database, API and web.
set -euo pipefail
# shellcheck source=lib.sh
source "$(dirname -- "${BASH_SOURCE[0]}")/lib.sh"

(($# == 0)) || die "usage: $0"
require_env_file
validate

volumes="$(docker volume ls --quiet \
  --filter "label=com.docker.compose.project=$project" \
  --filter label=com.docker.compose.volume=pgdata)"

printf 'This deletes all Heliolytics data in project %s.\n' "$project"
read -r -p 'Type DELETE to continue: ' answer
[[ "$answer" == DELETE ]] || die aborted

compose rm --stop --force web api db
while IFS= read -r volume; do
  if [[ -n "$volume" ]]; then
    docker volume rm "$volume" >/dev/null
  fi
done <<<"$volumes"
compose up -d --build db api web
