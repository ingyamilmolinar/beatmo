#!/bin/bash
# Install system and Node dependencies for running real tests and browser tests.
# Run with sudo: `sudo make dependencies`

# Exit on error
set -e

# Function to install packages on Debian/Ubuntu
install_debian() {
  echo "Detected Linux"

  if ! [ -x "$(command -v apt-get)" ]; then
    echo "apt-get not found. Cannot install dependencies."
    exit 1
  fi

  # Base packages required for Ebiten and other tools
  # Add nodejs and npm to ensure they are installed from the same repo as emscripten
  BASE_PACKAGES="build-essential pkg-config libasound2-dev libgl1-mesa-dev xorg-dev libxcursor-dev libxrandr-dev libxinerama-dev libxi-dev libxxf86vm-dev xvfb emscripten nodejs npm libgtk-3-dev"

  echo "Updating package lists..."
  apt-get update

  echo "Installing base dependencies..."
  apt-get install -y $BASE_PACKAGES
}

# Function to install packages on macOS
install_mac() {
  echo "Detected macOS"
  if ! [ -x "$(command -v brew)" ]; then
    echo "Homebrew not found. Please install it from https://brew.sh/ and then run this script again."
    exit 1
  fi
  brew install pkg-config glfw node emscripten
}

# Detect OS and install packages
if [[ "$OSTYPE" == "linux-gnu"* ]]; then
  install_debian
elif [[ "$OSTYPE" == "darwin"* ]]; then
  install_mac
else
  echo "Unsupported OS: $OSTYPE"
  exit 1
fi

# Install JS dependencies
(cd src/js && npm ci && npx playwright install --with-deps chromium)

echo "All dependencies installed successfully."