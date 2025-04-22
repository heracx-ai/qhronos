#!/bin/bash

# Get the container ID
CONTAINER_ID=$(docker ps -qf "name=postgres")

if [ -z "$CONTAINER_ID" ]; then
    echo "PostgreSQL container is not running. Please start it first using ./scripts/db.sh start"
    exit 1
fi

# Check if database exists
if ! docker exec $CONTAINER_ID psql -U postgres -lqt | cut -d \| -f 1 | grep -qw qhronos; then
    echo "Creating database qhronos..."
    docker exec $CONTAINER_ID createdb -U postgres qhronos
fi

# Apply migrations
echo "Applying migrations..."
for migration in $(ls migrations/*.sql | sort); do
    echo "Applying $migration..."
    docker exec -i $CONTAINER_ID psql -U postgres -d qhronos < $migration
done

echo "Migrations applied successfully!" 