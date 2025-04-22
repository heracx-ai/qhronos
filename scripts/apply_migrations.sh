#!/bin/bash

# Get the container ID
CONTAINER_ID=$(docker ps -qf "name=qhronos_db")

if [ -z "$CONTAINER_ID" ]; then
    echo "PostgreSQL container is not running. Please start it first using ./scripts/db.sh start"
    exit 1
fi

# Wait for PostgreSQL to be ready
echo "Waiting for PostgreSQL to be ready..."
for i in {1..30}; do
    if docker exec $CONTAINER_ID pg_isready -U postgres > /dev/null 2>&1; then
        break
    fi
    echo -n "."
    sleep 1
done

if ! docker exec $CONTAINER_ID pg_isready -U postgres > /dev/null 2>&1; then
    echo "Failed to connect to PostgreSQL after 30 seconds"
    exit 1
fi

# Check if database exists
if ! docker exec $CONTAINER_ID psql -U postgres -lqt | cut -d \| -f 1 | grep -qw qhronos; then
    echo "Creating database qhronos..."
    docker exec $CONTAINER_ID createdb -U postgres qhronos
fi

# Check if migrations have been applied
if docker exec $CONTAINER_ID psql -U postgres -d qhronos -c "SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = 'events')" | grep -q t; then
    echo "Migrations have already been applied"
    exit 0
fi

# Apply migrations
echo "Applying migrations..."
for migration in $(ls migrations/*.sql | sort); do
    echo "Applying $migration..."
    if ! docker exec -i $CONTAINER_ID psql -U postgres -d qhronos < $migration; then
        echo "Failed to apply migration $migration"
        exit 1
    fi
done

echo "Migrations applied successfully!" 