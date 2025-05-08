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
DO $$
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
$$;"

# Run all migrations in order
echo "Running all migrations..."
for f in migrations/*.sql; do
  echo "Applying migration $f"
  cat "$f" | docker exec -i qhronos_db psql -U postgres -d qhronos_test
done

# List all internal packages
PKGS=$(go list ./internal/...)

PASS_COUNT=0
FAIL_COUNT=0
ALL_TEST_OUTPUT=""
FAIL_OUTPUTS=""

START_TIME=$(date +%s)

# Accept optional test filter argument
TEST_FILTER="$1"

for PKG in $PKGS; do
  echo -e "\n=== Discovering subtests in $PKG ==="
  TEST_FILES=$(find $(go list -f '{{.Dir}}' $PKG) -name '*_test.go')
  echo "  Found test files: $TEST_FILES"
  TESTS_TO_RUN=()
  > /tmp/test_names.txt
  for file in $TEST_FILES; do
    parent_lines=$(grep -nE '^func Test[A-Za-z0-9_]*\(' "$file")
    echo "    Scanning $file for test functions..."
    IFS=$'\n' read -rd '' -a parent_lines_arr <<<"$parent_lines"
    parent_count=${#parent_lines_arr[@]}
    for ((i=0; i<parent_count; i++)); do
      line="${parent_lines_arr[$i]}"
      parent_line=$(echo "$line" | cut -d: -f1)
      parent_name=$(echo "$line" | sed -E 's/^.*func (Test[A-Za-z0-9_]*)\(.*$/\1/')
      if (( i+1 < parent_count )); then
        next_parent_line=$(echo "${parent_lines_arr[$((i+1))]}" | cut -d: -f1)
        subtest_lines=$(sed -n "$((parent_line+1)),$((next_parent_line-1))p" "$file")
      else
        subtest_lines=$(tail -n +$((parent_line+1)) "$file")
      fi
      # Extract subtest names cleanly
      subtests=$(echo "$subtest_lines" | grep -oE 't.Run\("[^"]+"' | sed -E 's/t.Run\("([^"]+)"/\1/')
      if [ -z "$subtests" ]; then
        echo "$parent_name" >> /tmp/test_names.txt
        echo "      Found test: $parent_name"
      else
        while read -r sub; do
          [ -n "$sub" ] && echo "$parent_name/$sub" >> /tmp/test_names.txt && echo "      Found subtest: $parent_name/$sub"
        done <<< "$subtests"
      fi
    done
  done
  TESTS_TO_RUN=()
  while read -r testname; do
    # If TEST_FILTER is set, only add matching tests
    if [ -z "$TEST_FILTER" ] || [[ "$testname" == *"$TEST_FILTER"* ]]; then
      TESTS_TO_RUN+=("$testname")
    fi
  done < /tmp/test_names.txt
  if [ ${#TESTS_TO_RUN[@]} -eq 0 ]; then
    echo "No test or subtest functions found in $PKG, skipping."
    continue
  fi
  TOTAL=${#TESTS_TO_RUN[@]}
  for i in "${!TESTS_TO_RUN[@]}"; do
    test="${TESTS_TO_RUN[$i]}"
    NUM=$((i+1))
    # Trim leading/trailing whitespace
    TRIMMED_TEST=$(echo "$test" | xargs)
    if [[ "$TRIMMED_TEST" == */* ]]; then
      PARENT=$(echo "$TRIMMED_TEST" | cut -d'/' -f1)
      SUBTEST=$(echo "$TRIMMED_TEST" | cut -d'/' -f2-)
      # Escape regex special characters
      REGEX_SAFE_PARENT=$(printf '%s' "$PARENT" | sed -e 's/[]\\^$.|?*+(){}[]/\\&/g')
      REGEX_SAFE_SUBTEST=$(printf '%s' "$SUBTEST" | sed -e 's/[]\\^$.|?*+(){}[]/\\&/g')
      RUN_PATTERN="^${REGEX_SAFE_PARENT}/${REGEX_SAFE_SUBTEST}$"
    else
      # Escape regex special characters
      REGEX_SAFE_TEST=$(printf '%s' "$TRIMMED_TEST" | sed -e 's/[]\\^$.|?*+(){}[]/\\&/g')
      RUN_PATTERN="^${REGEX_SAFE_TEST}$"
    fi
    TEST_START=$(date +%s)
    echo -ne "Running [$NUM/$TOTAL]: $TRIMMED_TEST in $PKG ... "
    TEST_OUTPUT=$(go test -v -run "$RUN_PATTERN" $PKG 2>&1)
    TEST_END=$(date +%s)
    TEST_DURATION=$((TEST_END-TEST_START))
    if echo "$TEST_OUTPUT" | grep -q '^--- PASS'; then
      PASS_COUNT=$((PASS_COUNT+1))
      echo "PASS (${TEST_DURATION}s)"
    else
      FAIL_COUNT=$((FAIL_COUNT+1))
      echo "FAIL (${TEST_DURATION}s)"
      FAIL_OUTPUTS="$FAIL_OUTPUTS\n\n===== FAILED TEST: $TRIMMED_TEST in $PKG =====\n$TEST_OUTPUT"
    fi
    # Do not print test output unless it fails
  done
  # Optionally, you can add a small sleep here if needed
  # sleep 0.1
done

END_TIME=$(date +%s)
TOTAL_DURATION=$((END_TIME-START_TIME))
echo "Passed: $PASS_COUNT"
echo "Failed: $FAIL_COUNT"
echo "Total elapsed time: ${TOTAL_DURATION}s"

if [ "$FAIL_COUNT" -ne 0 ]; then
  echo -e "\n\n==================== FAILED TEST OUTPUTS ===================="
  echo -e "$FAIL_OUTPUTS"
  exit 1
fi

# Clean up
echo "Cleaning up..."
# docker-compose down (removed) 