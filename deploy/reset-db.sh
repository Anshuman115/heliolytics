#!/usr/bin/env bash
# Guarded volume wipe for one exact database volume. It stops the identified
# database container, removes that container without -v, then removes only the
# validated volume. It never runs compose down, prune or a restart.
set -euo pipefail

usage() {
  printf 'usage: %s --project <name> --volume <volume> --backup-file <path>' "$0" >&2
  printf ' --backup-sha256 <64-hex> [--authorize-production] [--expected-working-dir <path>]\n' >&2
}

script_dir=
script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
backend_dir=
backend_dir="$(cd -- "$script_dir/.." && pwd)"

project=
volume=
backup_file=
backup_sha=
authorize_production=0
expected_working_dir=

while (($# > 0)); do
  case "$1" in
    --project)
      [[ $# -ge 2 ]] || { usage; exit 2; }
      project="$2"
      shift 2
      ;;
    --volume)
      [[ $# -ge 2 ]] || { usage; exit 2; }
      volume="$2"
      shift 2
      ;;
    --backup-file)
      [[ $# -ge 2 ]] || { usage; exit 2; }
      backup_file="$2"
      shift 2
      ;;
    --backup-sha256)
      [[ $# -ge 2 ]] || { usage; exit 2; }
      backup_sha="$2"
      shift 2
      ;;
    --expected-working-dir)
      [[ $# -ge 2 ]] || { usage; exit 2; }
      expected_working_dir="$2"
      shift 2
      ;;
    --authorize-production)
      authorize_production=1
      shift
      ;;
    *)
      printf 'unknown option: %s\n' "$1" >&2
      usage
      exit 2
      ;;
  esac
done

if [[ -z "$project" || -z "$volume" || -z "$backup_file" || -z "$backup_sha" ]]; then
  printf 'project, volume, backup file and backup hash are required\n' >&2
  usage
  exit 2
fi

sandbox_pattern='^heliolytics-d01-[a-z0-9][a-z0-9-]*$'
# Compose names the volume <project>_pgdata, so sandbox volumes carry an underscore.
sandbox_volume_pattern='^heliolytics-d01-[a-z0-9][a-z0-9-]*_pgdata$'
if [[ "$project" == "deploy" ]]; then
  if [[ "$volume" != "deploy_pgdata" ]]; then
    printf 'production project deploy requires volume deploy_pgdata\n' >&2
    exit 1
  fi
  if [[ "$authorize_production" != "1" ]]; then
    printf 'production wipe requires --authorize-production\n' >&2
    exit 1
  fi
elif [[ "$project" =~ $sandbox_pattern ]]; then
  if [[ ! "$volume" =~ $sandbox_volume_pattern ]]; then
    printf 'sandbox project requires a heliolytics-d01-* volume\n' >&2
    exit 1
  fi
else
  printf 'unknown project; only deploy or heliolytics-d01-* is allowed\n' >&2
  exit 1
fi

if [[ ! "$backup_sha" =~ ^[0-9a-f]{64}$ ]]; then
  printf 'backup hash must be 64 lowercase hex characters\n' >&2
  exit 1
fi

if [[ ! -f "$script_dir/.env" || ! -f "$script_dir/docker-compose.yml" ]]; then
  printf 'canonical deploy/.env or deploy/docker-compose.yml is missing\n' >&2
  exit 1
fi
if [[ ! -f "$backend_dir/schema.sql" ]]; then
  printf 'canonical schema.sql is missing\n' >&2
  exit 1
fi

if [[ ! -f "$backup_file" ]]; then
  printf 'backup file not found: %s\n' "$backup_file" >&2
  exit 1
fi
if [[ ! -s "$backup_file" ]]; then
  printf 'backup file is empty\n' >&2
  exit 1
fi

actual_sha=
if command -v sha256sum >/dev/null 2>&1; then
  actual_sha="$(sha256sum -- "$backup_file" | cut -d' ' -f1)"
elif command -v shasum >/dev/null 2>&1; then
  actual_sha="$(shasum -a 256 -- "$backup_file" | cut -d' ' -f1)"
else
  printf 'no sha256 tool available\n' >&2
  exit 1
fi
if [[ "$actual_sha" != "$backup_sha" ]]; then
  printf 'backup hash does not match the expected value\n' >&2
  exit 1
fi

info_file="$backup_file.info"
if [[ ! -f "$info_file" ]]; then
  printf 'backup sidecar %s is missing; the source image cannot be determined\n' "$info_file" >&2
  exit 1
fi
restore_image=
restore_image="$(sed -n 's/^image_ref=//p' "$info_file" | head -n 1)"
if [[ -z "$restore_image" ]]; then
  printf 'backup sidecar has no image_ref line\n' >&2
  exit 1
fi
if ! docker run --rm -i --network none "$restore_image" pg_restore --list \
  <"$backup_file" >/dev/null 2>&1; then
  printf 'backup archive is not readable with %s\n' "$restore_image" >&2
  exit 1
fi

db_id=
db_id="$(docker ps -a \
  --filter "label=com.docker.compose.project=$project" \
  --filter "label=com.docker.compose.service=db" \
  --format '{{.ID}}')"
if [[ -z "$db_id" ]]; then
  printf 'no database container found for project %s\n' "$project" >&2
  exit 1
fi
if [[ "$(printf '%s\n' "$db_id" | wc -l | tr -d ' ')" != "1" ]]; then
  printf 'more than one database container matches project %s\n' "$project" >&2
  exit 1
fi
if [[ "$(docker inspect --format '{{.State.Running}}' "$db_id")" != "true" ]]; then
  printf 'database container is not running; start it before the guarded wipe\n' >&2
  exit 1
fi

working_dir=
working_dir="$(docker inspect --format '{{index .Config.Labels "com.docker.compose.project.working_dir"}}' "$db_id")"
expected_dir="$script_dir"
if [[ -n "$expected_working_dir" ]]; then
  expected_dir="$expected_working_dir"
fi
if [[ "$working_dir" != "$expected_dir" ]]; then
  printf 'database container working directory label does not match the canonical deploy directory\n' >&2
  exit 1
fi

mounted_volume=
mounted_volume="$(docker inspect --format '{{range .Mounts}}{{if eq .Destination "/var/lib/postgresql/data"}}{{.Type}} {{.Name}}{{end}}{{end}}' "$db_id")"
if [[ "$mounted_volume" != "volume $volume" ]]; then
  printf 'database container does not mount volume %s at the data path\n' "$volume" >&2
  exit 1
fi

for writer in api web cloudflared; do
  if [[ -n "$(docker ps \
    --filter "label=com.docker.compose.project=$project" \
    --filter "label=com.docker.compose.service=$writer" \
    --filter status=running \
    --format '{{.ID}}')" ]]; then
    printf 'service %s is still running; stop writers before the wipe\n' "$writer" >&2
    exit 1
  fi
done

mount_users=
mount_users="$(docker ps -a --filter "volume=$volume" --format '{{.ID}}')"
if [[ "$mount_users" != "$db_id" ]]; then
  printf 'another container mounts volume %s; refusing to remove it\n' "$volume" >&2
  exit 1
fi

phrase="DELETE $volume FOR $project"
printf 'This deletes volume %s and the database container for project %s.\n' "$volume" "$project"
printf 'Type exactly: %s\n' "$phrase"
if [[ "$project" == "deploy" ]]; then
  if [[ ! -t 0 ]]; then
    printf 'production wipe needs an interactive terminal\n' >&2
    exit 1
  fi
fi
typed=
IFS= read -r typed || true
if [[ "$typed" != "$phrase" ]]; then
  printf 'confirmation did not match; nothing was changed\n' >&2
  exit 1
fi

docker stop --time 60 "$db_id" >/dev/null
docker rm "$db_id" >/dev/null
docker volume rm "$volume" >/dev/null

printf 'Removed volume %s and its database container.\n' "$volume"
printf 'Next: recreate the empty database with roles, extensions and schema only:\n'
printf '  docker compose --project-name %s --env-file deploy/.env --file deploy/docker-compose.yml up -d --build\n' "$project"
