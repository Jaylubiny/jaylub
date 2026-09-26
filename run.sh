#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# --- LOAD ENVIRONMENT VARIABLES ---
if [ -f internal/database/.env ]; then
    echo "Loading environment variables from internal/database/.env..."
    set -a
    source internal/database/.env
    set +a
fi
# ----------------------------------

BIN_DIR=".bin"
BIN_PATH="$BIN_DIR/jaylub-server"

mkdir -p "$BIN_DIR"

echo "Obfuscating Jaylive client..."
go run tools/obfuscate_jaylive.go

echo "Building full server..."
go build -o "$BIN_PATH" ./cmd/server

echo "Starting full server..."
exec "$BIN_PATH"
