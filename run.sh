#!/usr/bin/env bash
set -e

echo "Starting Go server..."

go run ./cmd/server &
SERVER_PID=$!

echo "Starting notify service..."

(
  cd internal/services/
  chmod +x ./notify
  ./notify
) &
NOTIFY_PID=$!

echo "Server PID: $SERVER_PID"
echo "Notify PID: $NOTIFY_PID"

echo "Both services are running in background."
echo "Terminal is free to use."

# Optional: keep script alive if you want
# wait $SERVER_PID $NOTIFY_PID
