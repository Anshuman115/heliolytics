#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"

if [[ ! -f .env ]]; then
  echo "Creating .env from .env.example — edit secrets before sharing URLs."
  cp .env.example .env
fi

if ! command -v docker >/dev/null; then
  echo "Docker is required. Install: https://docs.docker.com/engine/install/"
  exit 1
fi

docker compose build
docker compose up -d
echo ""
echo "Heliolytics stack is up."
echo "  API:    http://$(hostname -I 2>/dev/null | awk '{print $1}' || echo localhost):${API_PORT:-8080}"
echo "  Web:    http://$(hostname -I 2>/dev/null | awk '{print $1}' || echo localhost):${WEB_PORT:-3000}/login"
echo "  HTTPS:  https://$(hostname -I 2>/dev/null | awk '{print $1}' || echo localhost)"
echo ""
echo "Flutter app API key = HELIOLYTICS_SIGNING_SECRET from deploy/.env"
echo "Web login password  = HELIOLYTICS_WEB_PASSWORD from deploy/.env"
