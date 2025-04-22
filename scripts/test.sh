#!/bin/bash

set -e

# Function to cleanup on script exit
cleanup() {
    echo "Cleaning up..."
    docker compose down -v
}

# Register cleanup function to be called on script exit
trap cleanup EXIT

echo "Starting PostgreSQL container..."
docker compose up -d postgres

echo "Waiting for PostgreSQL to be ready..."
until docker exec qhronos_db pg_isready -U postgres; do
    echo "PostgreSQL is unavailable - sleeping"
    sleep 1
done

echo "Creating test database..."
docker exec -i qhronos_db psql -U postgres -c "DROP DATABASE IF EXISTS postgres_test;"
docker exec -i qhronos_db psql -U postgres -c "CREATE DATABASE postgres_test;"

echo "Running migrations..."
cat migrations/001_initial_schema.sql | docker exec -i qhronos_db psql -U postgres -d postgres_test

echo "Running tests..."
TEST_DB_HOST=localhost \
TEST_DB_PORT=5433 \
TEST_DB_USER=postgres \
TEST_DB_PASSWORD=postgres \
TEST_DB_NAME=postgres_test \
TEST_DB_SSL_MODE=disable \
go test -v ./... 