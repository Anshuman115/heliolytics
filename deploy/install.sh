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

if [[ -z "${CLOUDFLARE_TUNNEL_TOKEN:-}" ]] && ! grep -q '^CLOUDFLARE_TUNNEL_TOKEN=.\+' .env 2>/dev/null; then
  echo "Warning: CLOUDFLARE_TUNNEL_TOKEN empty — cloudflared will not connect."
  echo "  Local dev: use http://127.0.0.1:\${API_PORT:-8080} and :\${WEB_PORT:-3000}"
  echo "  Production: set token in .env (see deploy/deployment.md)"
fi

docker compose build
docker compose up -d
echo ""
echo "Heliolytics stack is up."
echo "  API (local): http://127.0.0.1:${API_PORT:-8080}/health"
echo "  Web (local): http://127.0.0.1:${WEB_PORT:-3000}/login"
echo "  Public HTTPS: via Cloudflare tunnel hostnames (see Zero Trust dashboard)"
echo ""
echo "Flutter app API key = HELIOLYTICS_SIGNING_SECRET from deploy/.env"
echo "Web login password  = HELIOLYTICS_WEB_PASSWORD from deploy/.env"
