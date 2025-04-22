#!/bin/bash

# Get the container ID
CONTAINER_ID=$(docker ps -qf "name=qhronos-postgres")

if [ -z "$CONTAINER_ID" ]; then
    echo "PostgreSQL container is not running. Please start it first using ./scripts/db.sh start"
    exit 1
fi

# Enable pg_cron
echo "Enabling pg_cron extension..."
docker exec $CONTAINER_ID psql -U postgres -d qhronos -c "CREATE SCHEMA IF NOT EXISTS cron;"
docker exec $CONTAINER_ID psql -U postgres -d qhronos -c "CREATE EXTENSION IF NOT EXISTS pg_cron SCHEMA cron;"

echo "pg_cron enabled successfully!" 