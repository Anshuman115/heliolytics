#!/usr/bin/env bash
# Rebuilds and restarts the API only.
set -euo pipefail
# shellcheck source=lib.sh
source "$(dirname -- "${BASH_SOURCE[0]}")/lib.sh"

(($# == 0)) || die "usage: $0"
require_env_file
require_clean_main "$backend_dir" backend
[[ -d "$web_dir/.git" ]] || die "web checkout not found at $web_dir"
record_build api
validate
compose up -d --build --no-deps api
