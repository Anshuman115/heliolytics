#!/usr/bin/env bash
# Rebuilds and starts the database, API and web. Existing data is kept.
set -euo pipefail
# shellcheck source=lib.sh
source "$(dirname -- "${BASH_SOURCE[0]}")/lib.sh"

(($# == 0)) || die "usage: $0"
require_env_file
require_clean_main "$backend_dir" backend
require_clean_main "$web_dir" web
record_build all
validate
compose up -d --build db api web
