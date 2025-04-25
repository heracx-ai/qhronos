#!/bin/bash

# Exit on error (disabled to allow all tests to run)
# set -e

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

# List all internal packages
PKGS=$(go list ./internal/...)

PASS_COUNT=0
FAIL_COUNT=0
ALL_TEST_OUTPUT=""
FAIL_OUTPUTS=""

for PKG in $PKGS; do
  echo -e "\n=== Running tests in $PKG ==="
  TESTS=$(go test -list . $PKG | grep '^Test' || true)
  if [ -z "$TESTS" ]; then
    echo "No test functions found in $PKG, skipping."
    continue
  fi
  for test in $TESTS; do
    echo -e "\n--- Running $test in $PKG ---"
    TEST_OUTPUT=$(go test -v -run "^$test$" $PKG)
    echo "$TEST_OUTPUT"
    ALL_TEST_OUTPUT="$ALL_TEST_OUTPUT\n$TEST_OUTPUT"
    if echo "$TEST_OUTPUT" | grep -q '^--- PASS'; then
      PASS_COUNT=$((PASS_COUNT+1))
    else
      FAIL_COUNT=$((FAIL_COUNT+1))
      FAIL_OUTPUTS="$FAIL_OUTPUTS\n\n===== FAILED TEST: $test in $PKG =====\n$TEST_OUTPUT"
    fi
  done
  
  # Optionally, you can add a small sleep here if needed
  # sleep 0.1

done

# Count all '=== RUN' lines (including subtests)
RUN_COUNT=$(echo "$ALL_TEST_OUTPUT" | grep '^=== RUN' | wc -l)
echo -e "\nAll tests completed."
echo "Passed: $PASS_COUNT"
echo "Failed: $FAIL_COUNT"
echo "Total RUN (including subtests): $RUN_COUNT"

if [ "$FAIL_COUNT" -ne 0 ]; then
  echo -e "\n\n==================== FAILED TEST OUTPUTS ===================="
  echo -e "$FAIL_OUTPUTS"
  exit 1
fi

# Clean up
echo "Cleaning up..."
# docker-compose down (removed) 