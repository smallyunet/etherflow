#!/bin/bash
set -e

# Directory of this script
DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
PROJECT_ROOT="$DIR/.."

echo "Starting E2E Environment..."
docker-compose -f "$PROJECT_ROOT/docker-compose.e2e.yml" up -d --wait
sleep 3 # Wait for Anvil to recognize port binding

echo "Running E2E Tests..."
# Pass necessary env vars to the test
export ETHERFLOW_RPC_URL="http://localhost:8545"
export ETHERFLOW_DB_DSN="host=localhost user=etherflow password=password dbname=etherflow_e2e port=5432 sslmode=disable"
export ETHERFLOW_DB_DRIVER="postgres"

# Run tests with e2e tag
go test -v -tags=e2e ./test/e2e/...

# Capture exit code
TEST_EXIT_CODE=$?

echo "Cleaning up..."
docker-compose -f "$PROJECT_ROOT/docker-compose.e2e.yml" down -v

if [ $TEST_EXIT_CODE -eq 0 ]; then
  echo "✅ E2E Tests Passed!"
  exit 0
else
  echo "❌ E2E Tests Failed!"
  exit 1
fi
