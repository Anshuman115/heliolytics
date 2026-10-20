#!/usr/bin/env bash
# Starts Adminer on 127.0.0.1:ADMINER_PORT. Stop it with: deploy/deploy-adminer.sh stop
set -euo pipefail
# shellcheck source=lib.sh
source "$(dirname -- "${BASH_SOURCE[0]}")/lib.sh"

require_env_file
case "${1:-start}" in
  start) compose --profile tools up -d --no-deps adminer ;;
  stop) compose --profile tools rm --stop --force adminer ;;
  *) die "usage: $0 [start|stop]" ;;
esac
