#!/bin/bash

# Exit on error
set -e

# Print commands
set -x

# Wait for PostgreSQL to be ready
echo "Waiting for PostgreSQL to be ready..."
until docker exec qhronos_db pg_isready -U postgres; do
    sleep 1
done

# Wait for Redis to be ready
echo "Waiting for Redis to be ready..."
until docker exec qhronos_redis redis-cli ping; do
    sleep 1
done

# Create test database
echo "Creating test database..."
docker exec qhronos_db psql -U postgres -c "DROP DATABASE IF EXISTS qhronos_test;"
docker exec qhronos_db psql -U postgres -c "CREATE DATABASE qhronos_test;"

# Drop all user-defined functions in the public schema before running migrations
docker exec qhronos_db psql -U postgres -d qhronos_test -c "
DO \$\$
DECLARE
    r RECORD;
BEGIN
    FOR r IN (
        SELECT n.nspname as function_schema,
               p.proname as function_name,
               pg_get_function_identity_arguments(p.oid) as args
        FROM pg_proc p
        JOIN pg_namespace n ON p.pronamespace = n.oid
        WHERE n.nspname = 'public'
          AND p.prokind = 'f'
    )
    LOOP
        EXECUTE 'DROP FUNCTION IF EXISTS ' || r.function_schema || '.' || r.function_name || '(' || r.args || ') CASCADE';
    END LOOP;
END
\$\$;"

# Run migrations
echo "Running migrations..."
cat migrations/001_initial_schema.sql | docker exec -i qhronos_db psql -U postgres -d qhronos_test

# Run tests
go test -v -race ./...

# Clean up
echo "Cleaning up..."
# docker-compose down (removed) 