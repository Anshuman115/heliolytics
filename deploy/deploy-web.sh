#!/usr/bin/env bash
# Deploy ONLY the web dashboard: rebuild its image and recreate just the `web`
# container. db / api / cloudflared keep running untouched (no downtime for the
# API or the strap sync path).
#
# Usage:
#   ./deploy-web.sh           # build from the current Heliolytics_Web checkout
#   PULL=1 ./deploy-web.sh    # git pull the web repo first, then build
set -euo pipefail
cd "$(dirname "$0")"

if ! command -v docker >/dev/null; then
  echo "Docker is required." >&2
  exit 1
fi

WEB_DIR="../../Heliolytics_Web"
if [[ ! -d "$WEB_DIR" ]]; then
  echo "Web checkout not found at $WEB_DIR (sibling of this repo)." >&2
  exit 1
fi

if [[ "${PULL:-0}" == "1" && -d "$WEB_DIR/.git" ]]; then
  echo "→ Pulling latest web source…"
  git -C "$WEB_DIR" pull --ff-only
fi

echo "→ Building web image…"
docker compose build web

echo "→ Recreating web container (db / api left running)…"
docker compose up -d --no-deps web

echo ""
echo "✓ Web redeployed."
echo "  Local: http://127.0.0.1:${WEB_PORT:-3000}/login"
echo "  Demo:  http://127.0.0.1:${WEB_PORT:-3000}/demo"
echo "  Public: via your Cloudflare tunnel hostname"
