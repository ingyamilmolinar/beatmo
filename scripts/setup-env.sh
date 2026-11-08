#!/usr/bin/env bash
# Installs what `make test` (native, the fast default) and `make ci-local`
# (CI's jobs in CI's images) need, at the versions ci/versions.env pins.
# Run as yourself: `make dependencies` (sudo is asked for only where needed).
# Idempotent: a tool already at its pin is left alone; `make doctor` is the check.
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"
# shellcheck disable=SC1091
. ci/versions.env
tools="$root/.tools"

if [[ "$(id -u)" -eq 0 && -n "${SUDO_USER:-}" ]]; then
  # `sudo make dependencies` (the old spelling): do the user-level part as the user.
  exec sudo -u "$SUDO_USER" -H "$0" "$@"
fi
SUDO=""; [[ "$(id -u)" -eq 0 ]] || SUDO="sudo"
user="$(id -un)"

step() { printf '\n== %s\n' "$*"; }
have() { command -v "$1" >/dev/null 2>&1; }
fetch() { # url sha256 dest
  curl -fsSLo "$3" "$1"
  echo "$2  $3" | sha256sum -c - >/dev/null
}

install_mac() {
  have brew || { echo "Install Homebrew (https://brew.sh/) first."; exit 1; }
  brew install pkg-config glfw node emscripten podman ffmpeg cppcheck
  echo "macOS: native targets only match CI loosely; use \`podman machine init && podman machine start\`, then \`make ci-local\`."
}

install_debian() {
  have apt-get || { echo "apt-get not found: this script supports Debian/Ubuntu and macOS."; exit 1; }
  step "system packages (the build image's, plus rootless podman)"
  $SUDO apt-get update
  # Keep in step with ci/images/build.Dockerfile; libgtk-3-dev is the desktop
  # file dialog, which the image never builds.
  $SUDO apt-get install -y --no-install-recommends \
    build-essential pkg-config ca-certificates curl git xz-utils python3 \
    cppcheck xvfb xauth ffmpeg zstd \
    libasound2-dev libgl1-mesa-dev libgl1-mesa-dri libglx-mesa0 \
    libx11-dev libxcursor-dev libxrandr-dev libxinerama-dev libxi-dev libxxf86vm-dev \
    xorg-dev libgtk-3-dev libegl1 libegl-mesa0 \
    podman uidmap slirp4netns fuse-overlayfs dbus-user-session

  step "rootless podman for $user"
  for f in /etc/subuid /etc/subgid; do
    if ! grep -q "^$user:" "$f" 2>/dev/null; then
      $SUDO usermod --add-subuids 100000-165535 --add-subgids 100000-165535 "$user"
      break
    fi
  done
  # A CI runner has 4 CPUs and 16 GB; rootless podman can only impose that
  # when systemd delegates the cpu and memory controllers to the user.
  dropin=/etc/systemd/system/user@.service.d/delegate.conf
  if [[ ! -f "$dropin" ]] && have systemctl; then
    $SUDO mkdir -p "$(dirname "$dropin")"
    printf '[Service]\nDelegate=cpu cpuset io memory pids\n' | $SUDO tee "$dropin" >/dev/null
    $SUDO systemctl daemon-reload
    echo "cgroup delegation enabled: log out and back in once so podman can apply CPU/memory limits"
  fi

  if ! google-chrome --version 2>/dev/null | grep -q "${CHROME_VERSION%-*}"; then
    step "Google Chrome ${CHROME_VERSION}"
    deb="$(mktemp --suffix=.deb)"
    fetch "https://dl.google.com/linux/chrome/deb/pool/main/g/google-chrome-stable/google-chrome-stable_${CHROME_VERSION}_amd64.deb" "$CHROME_SHA256" "$deb"
    $SUDO apt-get install -y --allow-downgrades "$deb"
    rm -f "$deb"
  fi
}

install_pinned_tools() {
  mkdir -p "$tools"
  if [[ "$("$tools/go/bin/go" env GOVERSION 2>/dev/null)" != "go$GO_VERSION" ]]; then
    step "Go $GO_VERSION -> .tools/go"
    tgz="$(mktemp)"; fetch "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" "$GO_SHA256" "$tgz"
    rm -rf "$tools/go"; tar -C "$tools" -xzf "$tgz"; rm -f "$tgz"
  fi
  if ! "$tools/golangci-lint" --version 2>/dev/null | grep -q "version $GOLANGCI_LINT_VERSION"; then
    step "golangci-lint $GOLANGCI_LINT_VERSION -> .tools/golangci-lint"
    tgz="$(mktemp)"; fetch "https://github.com/golangci/golangci-lint/releases/download/v${GOLANGCI_LINT_VERSION}/golangci-lint-${GOLANGCI_LINT_VERSION}-linux-amd64.tar.gz" "$GOLANGCI_LINT_SHA256" "$tgz"
    tar -xzf "$tgz" -O "golangci-lint-${GOLANGCI_LINT_VERSION}-linux-amd64/golangci-lint" > "$tools/golangci-lint"
    chmod +x "$tools/golangci-lint"; rm -f "$tgz"
  fi
  path_hint=()
  if [[ "$(node --version 2>/dev/null)" != "v$NODE_VERSION" ]]; then
    if [[ "$("$tools/node/bin/node" --version 2>/dev/null)" != "v$NODE_VERSION" ]]; then
      step "Node $NODE_VERSION -> .tools/node"
      txz="$(mktemp)"; fetch "https://nodejs.org/dist/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-x64.tar.xz" "$NODE_SHA256" "$txz"
      rm -rf "$tools/node"; mkdir -p "$tools/node"; tar -C "$tools/node" --strip-components=1 -xJf "$txz"; rm -f "$txz"
    fi
    path_hint+=("$tools/node/bin")
    export PATH="$tools/node/bin:$PATH"
  fi
  if ! emcc --version 2>/dev/null | head -1 | grep -q " $EMSDK_VERSION "; then
    if [[ ! -x "$tools/emsdk/upstream/emscripten/emcc" ]] || \
       ! "$tools/emsdk/upstream/emscripten/emcc" --version 2>/dev/null | head -1 | grep -q " $EMSDK_VERSION "; then
      step "emsdk $EMSDK_VERSION -> .tools/emsdk"
      rm -rf "$tools/emsdk"
      git clone -q --depth 1 --branch "$EMSDK_VERSION" https://github.com/emscripten-core/emsdk.git "$tools/emsdk"
      (cd "$tools/emsdk" && ./emsdk install "$EMSDK_VERSION" && ./emsdk activate "$EMSDK_VERSION")
    fi
    path_hint+=("$tools/emsdk" "$tools/emsdk/upstream/emscripten")
    export PATH="$tools/emsdk:$tools/emsdk/upstream/emscripten:$PATH"
  fi
  if ((${#path_hint[@]})); then
    hint="$(IFS=:; echo "${path_hint[*]}")"
    echo "Add the pinned tools to your PATH (e.g. in ~/.profile): export PATH=\"$hint:\$PATH\""
  fi
}

install_js() {
  step "npm packages + Playwright browsers (chromium, firefox, webkit)"
  (cd src/js && npm ci)
  (cd src/js && $SUDO env "PATH=$PATH" npx playwright install-deps chromium firefox webkit)
  (cd src/js && npx playwright install chromium firefox webkit)
}

pull_ci_images() {
  have podman || have docker || return 0
  local rt; rt="$(command -v podman || command -v docker)"
  step "CI images (make ci-local pulls them too; this just warms them)"
  for kind in build browser; do
    ref="$(scripts/ci/image-tag.sh "$kind")"
    "$rt" image inspect "$ref" >/dev/null 2>&1 || "$rt" pull "$ref" \
      || echo "could not pull $ref (unpublished ci/ change?): make ci-local builds it locally"
  done
}

case "$OSTYPE" in
  linux-gnu*) install_debian; install_pinned_tools; install_js; pull_ci_images ;;
  darwin*) install_mac; (cd src/js && npm ci && npx playwright install chromium firefox webkit) ;;
  *) echo "Unsupported OS: $OSTYPE"; exit 1 ;;
esac

step "make doctor (host tools vs ci/versions.env)"
make -s doctor || echo "Some host tools differ from the pins (see above)."
echo "Done. Native: make test. CI-identical, in CI's images: make ci-local (make ci-local-list for the jobs)."
