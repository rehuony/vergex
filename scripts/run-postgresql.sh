#!/bin/sh
# run-postgresql.sh — start PostgreSQL with the settings in .env (called by
# `make run-postgresql`).
#
# This is the same container `make up` uses. docker-compose.yml keeps it on the
# internal network, so the extra compose file below also publishes it for the
# backend and tests running from source.
set -eu

cd "$(dirname "$0")/.."

docker compose -f docker-compose.yml -f - up -d --wait vergex-postgres <<'EOF'
services:
  vergex-postgres:
    ports:
      - "127.0.0.1:${DB_PORT:-5432}:5432"
EOF

# The tests run in a database of their own beside the main one.
docker compose exec -T vergex-postgres sh -c '
	test_db="${POSTGRES_DB}_test"
	psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -lqtA | cut -d"|" -f1 | grep -qx "$test_db" ||
		createdb -U "$POSTGRES_USER" "$test_db"
'
