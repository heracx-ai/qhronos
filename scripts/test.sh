#!/bin/bash

# Exit on error
set -e

# Start PostgreSQL and Redis containers
docker-compose up -d postgres redis

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

# Run migrations
echo "Running migrations..."
cat migrations/001_initial_schema.sql | docker exec -i qhronos_db psql -U postgres -d qhronos_test

# Run tests
echo "Running tests..."
go test -v ./...

# Clean up
echo "Cleaning up..."
docker-compose down 