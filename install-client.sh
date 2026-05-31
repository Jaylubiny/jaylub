#!/bin/bash

# Exit immediately if a command exits with a non-zero status
set -e

DOMAIN="jaylub.com"
SSH_HOST="ssh.$DOMAIN"
USER_SSH_CONFIG="$HOME/.ssh/config"

echo "🌐 Setting up Cloudflare SSH access for $SSH_HOST..."

# 1. Detect OS and install cloudflared
OS_TYPE="$(uname -s)"

if [ "$OS_TYPE" = "Darwin" ]; then
    echo "🍏 Mac OS detected."
    if ! command -v cloudflared >/dev/null 2>&1; then
        if ! command -v brew >/dev/null 2>&1; then
            echo "❌ Homebrew is required but not installed. Please install it from https://brew.sh/ or install cloudflared manually."
            exit 1
        fi
        echo "📦 Installing cloudflared via Homebrew..."
        brew install cloudflared
    else
        echo "✅ cloudflared is already installed."
    fi

elif [ "$OS_TYPE" = "Linux" ]; then
    echo "🐧 Linux OS detected."
    if ! command -v cloudflared >/dev/null 2>&1; then
        if command -v apt-get >/dev/null 2>&1; then
            echo "📦 Downloading and installing latest cloudflared .deb package..."
            ARCH=$(dpkg --print-architecture)
            if [ "$ARCH" = "amd64" ]; then
                DEB_URL="https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-amd64.deb"
            elif [ "$ARCH" = "arm64" ] || [ "$ARCH" = "aarch64" ]; then
                DEB_URL="https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-arm64.deb"
            else
                DEB_URL="https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-arm.deb"
            fi
            
            wget -q --show-progress -O /tmp/cloudflared.deb "$DEB_URL"
            sudo dpkg -i /tmp/cloudflared.deb
            rm -f /tmp/cloudflared.deb
        else
            echo "⚠️ Non-Debian/Ubuntu system detected. Please install 'cloudflared' using your system's package manager."
            exit 1
        fi
    else
        echo "✅ cloudflared is already installed."
    fi
else
    echo "❌ Unsupported operating system for this script: $OS_TYPE"
    exit 1
fi

# 2. Setup the local SSH Config file safely
echo "📂 Configuring your local SSH settings..."
mkdir -p "$HOME/.ssh"
touch "$USER_SSH_CONFIG"

# Check if the host configuration already exists so we don't duplicate it
if grep -q "Host $SSH_HOST" "$USER_SSH_CONFIG"; then
    echo "ℹ️ SSH config rules for $SSH_HOST already exist. Skipping configuration setup."
else
    echo "📝 Appending Cloudflare Proxy rules to $USER_SSH_CONFIG..."
    cat << EOF >> "$USER_SSH_CONFIG"

Host $SSH_HOST
    ProxyCommand cloudflared access ssh --hostname %h
EOF
fi

# Fix permissions just to make SSH happy
chmod 600 "$USER_SSH_CONFIG"

echo "🎉 Setup Complete!"
echo "----------------------------------------------------"
echo "You can now connect to the server by running:"
echo "👉  ssh jaylub@$SSH_HOST"
echo "----------------------------------------------------"
