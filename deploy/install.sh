#!/usr/bin/env bash
# Creates the private deploy/.env from the template and stops.
# It never builds, pulls, starts or contacts anything, and it never generates secrets.
set -euo pipefail
umask 077

script_dir=
script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
cd -- "$script_dir"

if [[ -f .env ]]; then
  printf 'deploy/.env already exists; leaving its bytes unchanged.\n'
  printf 'Next steps:\n'
  printf '  1. Fill every empty value in deploy/.env; keep the password hash single-quoted.\n'
  printf '  2. Run deploy/validate-env.sh with the canonical project, env and compose files.\n'
  printf '  3. Build and start explicitly: docker compose --project-name deploy up -d --build\n'
  exit 0
fi

if [[ ! -f .env.example ]]; then
  printf 'deploy/.env.example is missing; restore it from the repository.\n' >&2
  exit 1
fi

cp -- .env.example .env
chmod 600 -- .env

printf 'Created deploy/.env mode 0600 from the template.\n'
printf 'Every required value is still empty, so validation will fail until you fill it.\n'
printf 'Next steps:\n'
printf '  1. Generate secrets with openssl rand -hex 32 and the hash with npm run auth:hash-password.\n'
printf '  2. Fill deploy/.env; keep HELIOLYTICS_WEB_PASSWORD_HASH single-quoted as one line.\n'
printf '  3. Run deploy/validate-env.sh with the canonical project, env and compose files.\n'
printf '  4. Build and start explicitly: docker compose --project-name deploy up -d --build\n'
exit 0
