#!/bin/bash
set -e

# Build frontend
echo "Building frontend..."
cd frontend && pnpm install && pnpm run build && cd ..

# Start backend server with go run
echo "Starting backend server..."
go run . serve --repository-backend=memory --config=frontend/e2e/fixtures/config.test.toml --no-auth=U000000000 --addr=127.0.0.1:18080 --graphiql=false &
BACKEND_PID=$!

# Function to cleanup on exit
cleanup() {
  echo "Stopping backend server (PID: $BACKEND_PID)..."
  kill $BACKEND_PID 2>/dev/null || true
  wait $BACKEND_PID 2>/dev/null || true
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
cd frontend
BASE_URL=http://localhost:18080 pnpm exec playwright test || TEST_EXIT_CODE=$?

# Show summary
echo ""
echo "========================================="
echo "E2E Test Results"
echo "========================================="

# Cleanup will be called automatically via trap
exit ${TEST_EXIT_CODE:-0}
