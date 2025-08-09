#!/bin/sh
# Install system and Node dependencies for running real tests and browser tests.
# Run with sudo: `sudo make dependencies`
set -e
case "$(uname)" in
  Darwin)
    echo "Detected macOS"
    if ! command -v brew >/dev/null; then
      echo "Homebrew not found. Install it from https://brew.sh first." >&2
      exit 1
    fi
    brew update
    brew install go pkg-config node
    (cd src/js && npm ci && npx playwright install --with-deps chromium)
    ;;
  *)
    echo "Detected Linux"
    apt-get update
    apt-get install -y build-essential libgl1-mesa-dev xorg-dev \
      libasound2-dev libxcursor-dev libxrandr-dev libxinerama-dev libxi-dev \
      libxxf86vm-dev pkg-config nodejs xvfb
    (cd src/js && npm ci && npx playwright install --with-deps chromium)
    ;;
esac

echo "Environment ready"
