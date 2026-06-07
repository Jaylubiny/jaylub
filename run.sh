#!/usr/bin/env bash
set -euo pipefail

BIN_DIR=".bin"
BIN_PATH="$BIN_DIR/jaylub-server"
PID_FILE="server.pid"
LOG_FILE="server.log"

mkdir -p "$BIN_DIR"

echo "Building full server..."
go build -o "$BIN_PATH" ./cmd/server

echo "Starting full server..."
setsid "$BIN_PATH" > "$LOG_FILE" 2>&1 < /dev/null &
SERVER_PID=$!
echo "$SERVER_PID" > "$PID_FILE"

echo "Server PID: $SERVER_PID"
echo "PID file: $PID_FILE"
echo "Log file: $LOG_FILE"
echo "Full server is running in the background."
echo "Terminal is free to use."
