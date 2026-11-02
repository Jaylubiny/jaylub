#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"
umask 077

if ! command -v go >/dev/null 2>&1; then
    echo "Go is required to build Jaylub." >&2
    exit 1
fi

# --- LOAD ENVIRONMENT VARIABLES ---
if [ -f internal/database/.env ]; then
    chmod 600 internal/database/.env
    set -a
    source internal/database/.env
    set +a
fi
# ----------------------------------

if [ -z "${DISCORD_BOT_TOKEN:-}" ]; then
    echo "DISCORD_BOT_TOKEN must be set in the environment or internal/database/.env." >&2
    exit 1
fi

BIN_DIR=".bin"
BIN_PATH="$BIN_DIR/jaylub-server"

mkdir -p "$BIN_DIR"

echo "Obfuscating Jaylive client..."
go run tools/obfuscate_jaylive.go

echo "Building full server..."
go build -o "$BIN_PATH" ./cmd/server

echo "Starting full server..."
exec "$BIN_PATH"
