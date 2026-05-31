#!/usr/bin/env bash
set -e

echo "Starting Go server..."

go run ./cmd/server &
SERVER_PID=$!

echo "Starting notify service..."

echo "Server PID: $SERVER_PID"

echo "Services are running in background."
echo "Terminal is free to use."

# Optional: keep script alive if you want
# wait $SERVER_PID $NOTIFY_PID
