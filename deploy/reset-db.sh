#!/usr/bin/env bash
# Wipes the database volume and rebuilds the stack. Defaults to the deploy project.
set -euo pipefail

usage() {
  printf 'usage: %s [--project <name>] [--volume <name>] [--env-file <path>] [--compose-file <path>]\n' "$0" >&2
}

fail() {
  printf '%s\n' "$1" >&2
  exit 1
}

script_dir=
script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"

project=deploy
volume=
env_file="$script_dir/.env"
compose_file="$script_dir/docker-compose.yml"

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
    -h | --help)
      usage
      exit 0
      ;;
    *)
      usage
      exit 2
      ;;
  esac
done

if [[ -z "$volume" ]]; then
  volume="${project}_pgdata"
fi

[[ "$volume" == "${project}_pgdata" ]] || fail "volume must be ${project}_pgdata"
[[ -f "$env_file" ]] || fail "missing env file: $env_file"
[[ -f "$compose_file" ]] || fail "missing compose file: $compose_file"

"$script_dir/validate-env.sh" --project "$project" --env-file "$env_file" --compose-file "$compose_file"

printf 'This deletes volume %s and all data in project %s.\n' "$volume" "$project"
read -r -p 'Type DELETE to continue: ' answer
[[ "$answer" == "DELETE" ]] || fail 'aborted'

compose() {
  docker compose --project-name "$project" --env-file "$env_file" --file "$compose_file" "$@"
}

compose --profile tunnel --profile tools down -v
compose --profile tunnel --profile tools up -d

printf 'reset complete; %s was recreated by the database entrypoint\n' "$volume"
