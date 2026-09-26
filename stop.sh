#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 1 ]; then
    echo "Usage: $0 <systemd-unit>" >&2
    exit 2
fi

exec systemctl stop "$1"
