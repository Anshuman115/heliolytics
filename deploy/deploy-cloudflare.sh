#!/usr/bin/env bash
# Starts or updates the Cloudflare tunnel connector. It runs as its own Compose
# project, so the Heliolytics scripts never stop or recreate it.
set -euo pipefail
# shellcheck source=lib.sh
source "$(dirname -- "${BASH_SOURCE[0]}")/lib.sh"

(($# == 0)) || die "usage: $0"
require_env_file
[[ -n "$(env_get CLOUDFLARE_TUNNEL_TOKEN)" ]] || die "CLOUDFLARE_TUNNEL_TOKEN is empty in $env_file"

export INGRESS_NETWORK="${project}_ingress"
docker network inspect "$INGRESS_NETWORK" >/dev/null 2>&1 \
  || die "network $INGRESS_NETWORK does not exist; run deploy/deploy-all.sh first"

tunnel_dir="$deploy_dir/cloudflared"
files=(--file "$tunnel_dir/docker-compose.yml")
if [[ -f "$tunnel_dir/docker-compose.override.yml" ]]; then
  files+=(--file "$tunnel_dir/docker-compose.override.yml")
fi

docker compose --project-name cloudflared --env-file "$env_file" "${files[@]}" up -d
