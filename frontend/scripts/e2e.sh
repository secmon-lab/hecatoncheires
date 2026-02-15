#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
PID_FILE="$PROJECT_ROOT/tmp/server.pid"

# Kill any existing E2E server
if [ -f "$PID_FILE" ]; then
  OLD_PID=$(cat "$PID_FILE")
  if kill -0 "$OLD_PID" 2>/dev/null; then
    echo "Stopping existing E2E server (PID: $OLD_PID)..."
    kill "$OLD_PID" 2>/dev/null || true
    wait "$OLD_PID" 2>/dev/null || true
  fi
  rm -f "$PID_FILE"
fi

# Build frontend
echo "Building frontend..."
cd "$PROJECT_ROOT/frontend" && pnpm install && pnpm run build && cd "$PROJECT_ROOT"

# Ensure tmp directory exists
mkdir -p "$PROJECT_ROOT/tmp"

# Start backend server with go run
echo "Starting backend server..."
go run . serve --log-level error --repository-backend=memory --config=frontend/e2e/fixtures/config.test.toml --no-auth=U000000000 --addr=127.0.0.1:18080 --graphiql=false &
BACKEND_PID=$!
echo "$BACKEND_PID" > "$PID_FILE"

# Function to cleanup on exit
cleanup() {
  echo "Stopping backend server (PID: $BACKEND_PID)..."
  kill "$BACKEND_PID" 2>/dev/null || true
  wait "$BACKEND_PID" 2>/dev/null || true
  rm -f "$PID_FILE"
}
trap cleanup EXIT

# Wait for server to be ready
echo "Waiting for server to be ready..."
for i in {1..30}; do
  if curl -s http://localhost:18080 > /dev/null 2>&1; then
    echo "Server is ready!"
    break
  fi
  sleep 1
done

# Run E2E tests
echo "Running E2E tests..."
cd "$PROJECT_ROOT/frontend"
BASE_URL=http://localhost:18080 pnpm exec playwright test "$@" || TEST_EXIT_CODE=$?

# Show summary
echo ""
echo "========================================="
echo "E2E Test Results"
echo "========================================="

# Cleanup will be called automatically via trap
exit ${TEST_EXIT_CODE:-0}
