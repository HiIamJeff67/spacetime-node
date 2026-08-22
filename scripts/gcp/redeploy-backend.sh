#!/usr/bin/env bash
set -euo pipefail

# Run on the GCP VM after the repository has been cloned to /opt/spacetime-node.
APP_DIR=${APP_DIR:-/opt/spacetime-node}
ENV_FILE=${ENV_FILE:-.env}
COMPOSE_FILE=${COMPOSE_FILE:-deploy/compose/compose.yaml}
MIGRATION_START=${MIGRATION_START:-000010}

cd "$APP_DIR"

if [[ ! -f "$ENV_FILE" ]]; then
  printf 'Missing %s. Copy .env.example to .env and set the VM secrets first.\n' "$ENV_FILE" >&2
  exit 1
fi

git pull --ff-only origin main
docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" up -d --build

printf 'Waiting for PostgreSQL...\n'
for _ in {1..30}; do
  if docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" exec -T postgres \
    sh -lc 'pg_isready -U "$POSTGRES_USER" -d "$POSTGRES_DB"' >/dev/null 2>&1; then
    break
  fi
  sleep 2
done

if ! docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" exec -T postgres \
  sh -lc 'pg_isready -U "$POSTGRES_USER" -d "$POSTGRES_DB"' >/dev/null 2>&1; then
  printf 'PostgreSQL did not become ready in time.\n' >&2
  exit 1
fi

while IFS= read -r migration; do
  version=${migration%%_*}
  if [[ "$version" < "$MIGRATION_START" ]]; then
    continue
  fi

  printf 'Applying %s\n' "$migration"
  docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" exec -T postgres \
    sh -lc 'psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" -f "/docker-entrypoint-initdb.d/$1"' \
    sh "$migration"
done < <(find migrations/postgres -maxdepth 1 -type f \
  -name '[0-9][0-9][0-9][0-9][0-9][0-9]_*.sql' -exec basename {} \; | sort)

docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" restart \
  recommendation gateway mobility embedding-indexer
docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" ps

printf '\nBackend redeploy complete.\n'
printf 'Run scripts/gcp/smoke-test-beacon.sh to verify the Beacon path.\n'
