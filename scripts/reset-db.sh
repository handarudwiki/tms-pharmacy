#!/usr/bin/env bash
# Drops every TMS service schema so the services re-create them on next start.
set -euo pipefail
DB="${DATABASE_URL:-postgres://localhost:5432/pharmacy_tms?sslmode=disable}"
psql "$DB" -v ON_ERROR_STOP=1 -q -c "DROP SCHEMA IF EXISTS identity, fleet, shipment, planning, freight CASCADE"
echo "schemas dropped; restart the services to re-run migrations"
