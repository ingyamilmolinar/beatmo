#!/usr/bin/env bash
# Compares the host's tools with ci/versions.env, so a native `make test` runs
# on what CI runs. Exit 1 on any mismatch.
set -uo pipefail
root="$(cd "$(dirname "$0")/../.." && pwd)"
# shellcheck disable=SC1091
. "$root/ci/versions.env"
bad=0
check() {
  local name="$1" want="$2" got="$3"
  if [[ "$got" == "$want" ]]; then
    printf '  ok    %-14s %s\n' "$name" "$got"
  else
    printf '  DIFF  %-14s want %s, have %s\n' "$name" "$want" "${got:-missing}"; bad=1
  fi
}
go="${GO:-$root/.tools/go/bin/go}"
check go "$GO_VERSION" "$("$go" env GOVERSION 2>/dev/null | sed 's/^go//')"
check emcc "$EMSDK_VERSION" "$(emcc --version 2>/dev/null | head -1 | sed -E 's/.* ([0-9]+\.[0-9]+\.[0-9]+).*/\1/')"
check node "$NODE_VERSION" "$(node --version 2>/dev/null | sed 's/^v//')"
check golangci-lint "$GOLANGCI_LINT_VERSION" "$("${GOLANGCI_LINT:-$root/.tools/golangci-lint}" --version 2>/dev/null | sed -E 's/.*version ([0-9.]+).*/\1/')"
check chrome "${CHROME_VERSION%-*}" "$(google-chrome --version 2>/dev/null | sed -E 's/[^0-9]*([0-9.]+).*/\1/')"
check playwright "$PLAYWRIGHT_VERSION" "$(cd "$root/src/js" && node -p 'require("playwright/package.json").version' 2>/dev/null)"
check debian 12 "$(cut -d. -f1 /etc/debian_version 2>/dev/null)"
exit $bad
