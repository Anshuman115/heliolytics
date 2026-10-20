#!/usr/bin/env bash
# Creates the restricted application role on first database initialization.
# Runs once, before schema.sql, as the owner role from POSTGRES_USER.
set -euo pipefail

if [[ -z "${APP_DB_PASSWORD:-}" ]]; then
  printf 'role initialization failed: APP_DB_PASSWORD is required\n' >&2
  exit 1
fi
if [[ -z "${POSTGRES_USER:-}" || -z "${POSTGRES_DB:-}" ]]; then
  printf 'role initialization failed: POSTGRES_USER and POSTGRES_DB are required\n' >&2
  exit 1
fi

umask 077
diag_dir=
diag_dir="$(mktemp -d)"
cleanup() {
  rm -rf "$diag_dir"
}
trap cleanup EXIT

# The password reaches psql only through the environment and one quoted literal.
# Diagnostics go to a private file so a failing statement cannot echo it.
if ! psql --no-psqlrc --set ON_ERROR_STOP=1 \
  --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" \
  >"$diag_dir/stdout" 2>"$diag_dir/stderr" <<'SQL'
\getenv app_password APP_DB_PASSWORD
SET log_statement = 'none';
SET log_min_error_statement = 'panic';
DO $do$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'heliolytics_app') THEN
    CREATE ROLE heliolytics_app;
  END IF;
END
$do$;
ALTER ROLE heliolytics_app WITH LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION PASSWORD :'app_password';
ALTER ROLE heliolytics_app SET statement_timeout = '30s';
ALTER ROLE heliolytics_app SET idle_in_transaction_session_timeout = '60s';
SQL
then
  printf 'role initialization failed: see container logs for the generic error\n' >&2
  exit 1
fi

printf 'role initialization complete\n'
