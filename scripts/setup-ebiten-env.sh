#!/bin/sh
# Setup dependencies for building Tunkul with the real Ebiten library.
# Run this once on a fresh system. Requires sudo for package installs.
set -e
case "$(uname)" in
    Darwin)
        echo "Detected macOS"
        if ! command -v brew >/dev/null; then
            echo "Homebrew not found. Install it from https://brew.sh first." >&2
            exit 1
        fi
        brew update
        brew install go pkg-config
        ;;
    *)
        echo "Detected Linux"
        sudo apt-get update
        sudo apt-get install -y build-essential libgl1-mesa-dev xorg-dev \
            libasound2-dev libxcursor-dev libxrandr-dev libxinerama-dev libxi-dev \
            libxxf86vm-dev pkg-config
        ;;
esac

echo "Environment ready"
