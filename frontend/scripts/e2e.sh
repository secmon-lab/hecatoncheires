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

# Function to cleanup on exit
BACKEND_PID=""
cleanup() {
  if [ -n "$BACKEND_PID" ]; then
    echo "Stopping backend server (PID: $BACKEND_PID)..."
    kill "$BACKEND_PID" 2>/dev/null || true
    wait "$BACKEND_PID" 2>/dev/null || true
  fi
  rm -f "$PID_FILE"
}
trap cleanup EXIT

# Try starting backend server with retries on different ports
MAX_ATTEMPTS=3
SERVER_READY=false

for attempt in $(seq 1 $MAX_ATTEMPTS); do
  E2E_PORT=$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1]); s.close()')
  echo "Attempt $attempt/$MAX_ATTEMPTS: starting backend on port $E2E_PORT..."

  go run . serve --log-level error --repository-backend=memory --config=frontend/e2e/fixtures/config.test.toml --no-auth=U000000000 --addr=127.0.0.1:$E2E_PORT --graphiql=false &
  BACKEND_PID=$!
  echo "$BACKEND_PID" > "$PID_FILE"

  # Wait for server to be ready (timeout: 30s)
  for i in {1..30}; do
    if ! kill -0 "$BACKEND_PID" 2>/dev/null; then
      echo "Backend server exited unexpectedly on port $E2E_PORT"
      break
    fi
    if curl -s http://localhost:$E2E_PORT > /dev/null 2>&1; then
      echo "Server is ready on port $E2E_PORT!"
      SERVER_READY=true
      break
    fi
    sleep 1
  done

  if [ "$SERVER_READY" = "true" ]; then
    break
  fi

  # Clean up failed attempt before retrying
  kill "$BACKEND_PID" 2>/dev/null || true
  wait "$BACKEND_PID" 2>/dev/null || true
  BACKEND_PID=""
done

if [ "$SERVER_READY" != "true" ]; then
  echo "ERROR: Server failed to start after $MAX_ATTEMPTS attempts"
  exit 1
fi

# Run E2E tests (timeout: 120s)
echo "Running E2E tests..."
cd "$PROJECT_ROOT/frontend"
BASE_URL=http://localhost:$E2E_PORT pnpm exec playwright test "$@" || TEST_EXIT_CODE=$?

# Show summary
echo ""
echo "========================================="
echo "E2E Test Results"
echo "========================================="

# Cleanup will be called automatically via trap
exit ${TEST_EXIT_CODE:-0}
