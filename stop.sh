#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 1 ]; then
    echo "Usage: $0 <systemd-unit>" >&2
    exit 2
fi

UNIT="$1"

echo "Stopping service: $UNIT..."
# Dropping 'exec' allows the script to continue after stopping the service
sudo systemctl stop "$UNIT"
