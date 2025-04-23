#!/bin/bash

set -e

CONTAINER_ID=$(docker ps -qf "name=qhronos_db")
if [ -z "$CONTAINER_ID" ]; then
    echo "PostgreSQL container is not running. Please start it first using docker-compose up -d postgres redis"
    exit 1
fi

# Wait for PostgreSQL to be ready
for i in {1..30}; do
    if docker exec $CONTAINER_ID pg_isready -U postgres > /dev/null 2>&1; then
        break
    fi
    sleep 1
done
if ! docker exec $CONTAINER_ID pg_isready -U postgres > /dev/null 2>&1; then
    echo "Failed to connect to PostgreSQL after 30 seconds"
    exit 1
fi

# Ensure schema_migrations table exists
MIGRATION_TABLE_EXISTS=$(docker exec $CONTAINER_ID psql -U postgres -d qhronos -tAc "SELECT to_regclass('public.schema_migrations') IS NOT NULL;")
if [ "$MIGRATION_TABLE_EXISTS" != "t" ]; then
    echo "Creating schema_migrations table..."
    docker exec $CONTAINER_ID psql -U postgres -d qhronos -c "CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY, applied_at TIMESTAMP DEFAULT now());"
fi

MODE=$1
COUNT=$2

if [ "$MODE" == "up" ]; then
    echo "Applying migrations..."
    APPLIED=$(docker exec $CONTAINER_ID psql -U postgres -d qhronos -tAc "SELECT version FROM schema_migrations ORDER BY version;")
    APPLIED_VERSIONS=$(echo "$APPLIED" | tr '\n' ' ')
    MIGRATIONS=( $(ls migrations/*.sql | sort) )
    APPLIED_COUNT=0
    for migration in "${MIGRATIONS[@]}"; do
        VERSION=$(basename "$migration" | cut -d'_' -f1)
        if [[ $APPLIED_VERSIONS =~ $VERSION ]]; then
            continue
        fi
        echo "Applying $migration..."
        if ! docker exec -i $CONTAINER_ID psql -U postgres -d qhronos < "$migration"; then
            echo "Failed to apply migration $migration"
            exit 1
        fi
        docker exec $CONTAINER_ID psql -U postgres -d qhronos -c "INSERT INTO schema_migrations (version) VALUES ('$VERSION');"
        ((APPLIED_COUNT++))
        if [ -n "$COUNT" ] && [ "$APPLIED_COUNT" -ge "$COUNT" ]; then
            break
        fi
    done
    echo "Migrations applied: $APPLIED_COUNT"
    exit 0
elif [ "$MODE" == "down" ]; then
    if [ -z "$COUNT" ]; then COUNT=1; fi
    echo "Rolling back $COUNT migration(s)..."
    for ((i=0; i<$COUNT; i++)); do
        VERSION=$(docker exec $CONTAINER_ID psql -U postgres -d qhronos -tAc "SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1;")
        if [ -z "$VERSION" ]; then
            echo "No migrations to roll back."
            exit 0
        fi
        DOWN_FILE="migrations/${VERSION}_down.sql"
        if [ ! -f "$DOWN_FILE" ]; then
            echo "No down migration file found for version $VERSION ($DOWN_FILE). Skipping."
            docker exec $CONTAINER_ID psql -U postgres -d qhronos -c "DELETE FROM schema_migrations WHERE version = '$VERSION';"
            continue
        fi
        echo "Rolling back $DOWN_FILE..."
        if ! docker exec -i $CONTAINER_ID psql -U postgres -d qhronos < "$DOWN_FILE"; then
            echo "Failed to roll back migration $DOWN_FILE"
            exit 1
        fi
        docker exec $CONTAINER_ID psql -U postgres -d qhronos -c "DELETE FROM schema_migrations WHERE version = '$VERSION';"
    done
    echo "Rollback complete."
    exit 0
else
    echo "Usage: $0 up [N] | down [N]"
    exit 1
fi 