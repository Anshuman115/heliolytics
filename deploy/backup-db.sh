#!/usr/bin/env bash
# Backup succeeds only after the off-host copy and read-back hash match.
set -euo pipefail
umask 0077

usage() {
  printf 'usage: %s [--source-container <id>]\n' "$0" >&2
  printf 'requires HELIOLYTICS_DEPLOY_DIR, BACKUP_DIR, BACKUP_DESTINATION and optional BACKUP_KEEP_DAYS\n' >&2
}

source_container=
while (($# > 0)); do
  case "$1" in
    --source-container)
      [[ $# -ge 2 ]] || { usage; exit 2; }
      source_container="$2"
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
  printf 'backup failed: %s\n' "$1" >&2
  exit 1
}

for tool in docker ssh scp flock sha256sum date find; do
  command -v "$tool" >/dev/null 2>&1 || fail "required tool missing: $tool"
done

deploy_dir="${HELIOLYTICS_DEPLOY_DIR:-}"
backup_dir="${BACKUP_DIR:-}"
destination="${BACKUP_DESTINATION:-}"
keep_days="${BACKUP_KEEP_DAYS:-30}"

[[ -n "$deploy_dir" ]] || fail 'HELIOLYTICS_DEPLOY_DIR is required'
[[ -d "$deploy_dir" ]] || fail 'HELIOLYTICS_DEPLOY_DIR does not exist'
[[ -f "$deploy_dir/docker-compose.yml" && -f "$deploy_dir/.env" ]] \
  || fail 'canonical deploy compose file or env file is missing'

[[ -n "$backup_dir" ]] || fail 'BACKUP_DIR is required'
[[ "$backup_dir" == /* ]] || fail 'BACKUP_DIR must be an absolute path'
mkdir -p -- "$backup_dir"
chmod 700 -- "$backup_dir"

[[ -n "$destination" ]] || fail 'BACKUP_DESTINATION is required, for example alias:/absolute/backup/directory'
remote_alias="${destination%%:*}"
remote_dir="${destination#*:}"
if [[ "$remote_alias" == "$destination" || -z "$remote_alias" || -z "$remote_dir" ]]; then
  fail 'BACKUP_DESTINATION must be alias:/absolute/directory'
fi
[[ "$remote_alias" =~ ^[A-Za-z0-9][A-Za-z0-9._-]*$ ]] || fail 'destination alias has unsupported characters'
[[ "$remote_dir" =~ ^/[A-Za-z0-9._/-]+$ ]] || fail 'destination path must be a plain absolute path'
[[ "$remote_dir" != *..* ]] || fail 'destination path must not contain parent segments'
[[ "$remote_dir" != */ ]] || fail 'destination path must not end with a slash'

[[ "$keep_days" =~ ^[0-9]+$ ]] || fail 'BACKUP_KEEP_DAYS must be an integer'
((10#$keep_days >= 1)) || fail 'BACKUP_KEEP_DAYS must be at least 1'

exec 9>"$backup_dir/.lock"
flock -n 9 || fail 'another backup holds the lock'

resolve_container() {
  local candidate="$1" service project
  service="$(docker inspect --format '{{index .Config.Labels "com.docker.compose.service"}}' "$candidate" 2>/dev/null)" \
    || fail 'source container cannot be inspected'
  project="$(docker inspect --format '{{index .Config.Labels "com.docker.compose.project"}}' "$candidate" 2>/dev/null)" \
    || fail 'source container labels cannot be read'
  [[ "$service" == "db" ]] || fail 'source container is not the compose db service'
  if [[ "$project" != "deploy" && ! "$project" =~ ^heliolytics-d01-[a-z0-9][a-z0-9-]*$ ]]; then
    fail 'source container project is neither deploy nor a heliolytics-d01-* sandbox'
  fi
  local mounted
  mounted="$(docker inspect --format \
    '{{range .Mounts}}{{if eq .Destination "/var/lib/postgresql/data"}}{{.Type}} {{.Name}}{{end}}{{end}}' \
    "$candidate")"
  [[ "$mounted" == "volume ${project}_pgdata" ]] \
    || fail 'source container does not mount the expected database volume'
  printf '%s' "$candidate"
}

if [[ -n "$source_container" ]]; then
  db_id="$(resolve_container "$source_container")"
else
  db_id="$(docker ps \
    --filter 'label=com.docker.compose.project=deploy' \
    --filter 'label=com.docker.compose.service=db' \
    --format '{{.ID}}')"
  [[ -n "$db_id" ]] || fail 'no running database container found for project deploy'
  [[ "$(printf '%s\n' "$db_id" | wc -l | tr -d ' ')" == "1" ]] \
    || fail 'more than one database container matches project deploy'
  db_id="$(resolve_container "$db_id")"
fi

[[ "$(docker inspect --format '{{.State.Running}}' "$db_id")" == "true" ]] || fail 'source container is not running'

stamp="$(date -u +%Y%m%dT%H%M%SZ)"
base="heliolytics-${stamp}"
dump_path="$backup_dir/${base}.dump"
sha_path="$dump_path.sha256"
info_path="$dump_path.info"

partial_dump="$backup_dir/.${base}.dump.partial"
if [[ -e "$dump_path" ]]; then
  fail 'a backup with this timestamp already exists'
fi

# The password is read from the container environment, never from argv.
if ! docker exec "$db_id" sh -c \
  'PGPASSWORD="$POSTGRES_PASSWORD" pg_dump -h 127.0.0.1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" --format=custom --no-owner --no-acl' \
  >"$partial_dump"; then
  rm -f -- "$partial_dump"
  fail 'pg_dump did not complete'
fi
if [[ ! -s "$partial_dump" ]]; then
  rm -f -- "$partial_dump"
  fail 'dump file is empty'
fi
if ! docker exec -i "$db_id" pg_restore --list <"$partial_dump" >/dev/null 2>&1; then
  rm -f -- "$partial_dump"
  fail 'dump archive is not readable with pg_restore'
fi
mv -- "$partial_dump" "$dump_path"

sha="$(sha256sum -- "$dump_path" | cut -d' ' -f1)"
printf '%s  %s\n' "$sha" "$(basename -- "$dump_path")" >"$sha_path"

server_version="$(docker exec -i "$db_id" sh -c \
  'PGPASSWORD="$POSTGRES_PASSWORD" psql -h 127.0.0.1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Atq' \
  <<<"SHOW server_version;" 2>/dev/null || printf 'unknown')"
timescale_version="$(docker exec -i "$db_id" sh -c \
  'PGPASSWORD="$POSTGRES_PASSWORD" psql -h 127.0.0.1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Atq' \
  <<<"SELECT extversion FROM pg_extension WHERE extname = 'timescaledb';" 2>/dev/null || printf 'unknown')"
image_ref="$(docker inspect --format '{{.Config.Image}}' "$db_id")"
image_id="$(docker inspect --format '{{.Image}}' "$db_id")"
source_commit="${HELIOLYTICS_COMMIT:-unknown}"

{
  printf 'created_utc=%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  printf 'dump_basename=%s\n' "$(basename -- "$dump_path")"
  printf 'dump_sha256=%s\n' "$sha"
  printf 'postgres_version=%s\n' "$server_version"
  printf 'timescaledb_version=%s\n' "$timescale_version"
  printf 'image_ref=%s\n' "$image_ref"
  printf 'image_id=%s\n' "$image_id"
  printf 'source_commit=%s\n' "$source_commit"
  printf 'schema=heliolytics\n'
} >"$info_path"

for artifact in "$dump_path" "$sha_path" "$info_path"; do
  [[ -f "$artifact" ]] || fail 'backup artifact missing before copy'
  chmod 600 -- "$artifact"
done

scp_options=(-q -o BatchMode=yes -o StrictHostKeyChecking=yes)
for artifact in "$dump_path" "$sha_path" "$info_path"; do
  name="$(basename -- "$artifact")"
  if ! scp "${scp_options[@]}" -- "$artifact" "$remote_alias:$remote_dir/$name.partial"; then
    fail "off-host copy failed; the local dump is kept at $dump_path"
  fi
done
for artifact in "$dump_path" "$sha_path" "$info_path"; do
  name="$(basename -- "$artifact")"
  if ! ssh -o BatchMode=yes -o StrictHostKeyChecking=yes -- "$remote_alias" \
    "mv -f -- '$remote_dir/$name.partial' '$remote_dir/$name'"; then
    fail "off-host rename failed; the local dump is kept at $dump_path"
  fi
done

remote_sha="$(ssh -o BatchMode=yes -o StrictHostKeyChecking=yes -- "$remote_alias" \
  "sha256sum -- '$remote_dir/$(basename -- "$dump_path")'" 2>/dev/null | cut -d' ' -f1)"
if [[ "$remote_sha" != "$sha" ]]; then
  fail "off-host hash mismatch; the local dump is kept at $dump_path"
fi

prune_local() {
  local file candidate name
  while IFS= read -r file; do
    if [[ -z "$file" ]]; then
      continue
    fi
    candidate="$(basename -- "$file")"
    if [[ "$candidate" == "$(basename -- "$dump_path")" ]]; then
      continue
    fi
    name="${candidate%.dump}"
    rm -f -- "$backup_dir/$name.dump" "$backup_dir/$name.dump.sha256" "$backup_dir/$name.dump.info"
  done < <(find "$backup_dir" -maxdepth 1 -type f -name 'heliolytics-*.dump' -mtime "+$keep_days" -print)
}

prune_remote() {
  local listing name
  listing="$(ssh -o BatchMode=yes -o StrictHostKeyChecking=yes -- "$remote_alias" \
    "find '$remote_dir' -maxdepth 1 -type f -name 'heliolytics-*.dump' -mtime +$keep_days -print" 2>/dev/null || true)"
  while IFS= read -r entry; do
    if [[ -z "$entry" ]]; then
      continue
    fi
    name="$(basename -- "$entry")"
    if [[ ! "$name" =~ ^heliolytics-[0-9]{8}T[0-9]{6}Z\.dump$ ]]; then
      continue
    fi
    if [[ "$entry" == *..* ]]; then
      continue
    fi
    ssh -o BatchMode=yes -o StrictHostKeyChecking=yes -- "$remote_alias" \
      "rm -f -- '$remote_dir/$name' '$remote_dir/$name.sha256' '$remote_dir/$name.info'" || true
  done <<<"$listing"
}

prune_local
prune_remote

printf 'backup complete: %s\n' "$(basename -- "$dump_path")"
printf 'sha256: %s\n' "$sha"
printf 'off-host: %s:%s\n' "$remote_alias" "$remote_dir"
