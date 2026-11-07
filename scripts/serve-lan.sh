#!/usr/bin/env bash
# Binds the test server to 0.0.0.0 and prints a LAN URL + QR for the in-page audio probe, to open from a phone.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

if [ -z "${GO:-}" ]; then
  if [ -x ".tools/go/bin/go" ]; then
    GO="$(pwd)/.tools/go/bin/go"
  else
    GO="go"
  fi
fi
export GO

if [ -z "${BEATMO_LAN_PORT:-}" ]; then
  # 8089: deliberately offset from `make serve` (8080) so both can run in
  # parallel without EADDRINUSE. Override via `BEATMO_LAN_PORT=<n> make serve-lan`.
  export BEATMO_LAN_PORT=8089
fi

exec node src/js/serve_lan.js
