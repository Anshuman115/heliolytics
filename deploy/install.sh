#!/usr/bin/env bash
# Creates deploy/.env and fills every empty secret. Existing values are never changed.
set -euo pipefail
umask 077
# shellcheck source=lib.sh
source "$(dirname -- "${BASH_SOURCE[0]}")/lib.sh"

(($# == 0)) || die "usage: $0"
command -v openssl >/dev/null || die 'openssl is required'
command -v docker >/dev/null || die 'docker is required'

if [[ ! -f "$env_file" ]]; then
  cp -- "$deploy_dir/.env.example" "$env_file"
  chmod 600 -- "$env_file"
  printf 'created %s\n' "$env_file"
fi

hex() {
  openssl rand -hex 32
}

env_put POSTGRES_PASSWORD "$(hex)" fill
env_put APP_DB_PASSWORD "$(hex)" fill
env_put HELIOLYTICS_WEB_API_KEY "web.$(hex)" fill
env_put HELIOLYTICS_PHONE_API_KEY "phone.$(hex)" fill
env_put HELIOLYTICS_SESSION_SECRET "$(hex)" fill

interactive=0
[[ -t 0 && -t 1 ]] && interactive=1

if grep -q '^HELIOLYTICS_WEB_PASSWORD_HASH=$' "$env_file"; then
  if ((interactive)) && [[ -f "$web_dir/scripts/hash-password.mjs" ]]; then
    printf 'Choose the web dashboard password.\n'
    out_dir="$(mktemp -d)"
    trap 'rm -rf "$out_dir"' EXIT
    docker run --rm -it --network none --user "$(id -u):$(id -g)" \
      -v "$web_dir:/web:ro" -v "$out_dir:/out" -w /web \
      node:24.22.0-alpine3.24@sha256:44fce696f38ba8c987c4db89954051000bbd05ffd8d8659ba15ed4a8602e937f \
      sh -c 'node scripts/hash-password.mjs >/out/hash'
    hash="$(tr -d '\r\n' <"$out_dir/hash")"
    env_put HELIOLYTICS_WEB_PASSWORD_HASH "'$hash'" fill
  else
    printf 'HELIOLYTICS_WEB_PASSWORD_HASH is empty; run deploy/install.sh in a terminal with the web checkout at %s\n' "$web_dir"
  fi
fi

if grep -q '^CLOUDFLARE_TUNNEL_TOKEN=$' "$env_file" && ((interactive)); then
  read -r -s -p 'Cloudflare tunnel token (Enter to skip): ' token
  printf '\n'
  [[ -z "$token" ]] || env_put CLOUDFLARE_TUNNEL_TOKEN "$token" fill
fi

printf '%s is ready.\n' "$env_file"
printf 'Configure the phone app with HELIOLYTICS_PHONE_API_KEY from that file.\n'
printf 'Next: deploy/deploy-all.sh\n'
