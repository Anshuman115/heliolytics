#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"

if [[ ! -f .env ]]; then
  cp .env.example .env
  echo "Created .env from .env.example — edit HELIOLYTICS_SIGNING_SECRET, HELIOLYTICS_WEB_PASSWORD, POSTGRES_PASSWORD."
fi

echo "Stopping stack and deleting Postgres volume (all data lost)..."
docker compose down -v
echo "Starting fresh DB with schema_v3.sql..."
docker compose up -d --build
echo "Done. Re-sync your strap to populate metrics."
