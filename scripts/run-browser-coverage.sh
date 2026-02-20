#!/bin/bash
# Runs browser tests with Go coverage instrumentation and produces a
# merged coverage profile.
#
# Prerequisites: build WASM with `make wasm-cover` first, or let
# `make coverage-browser` do it for you.
#
# Usage: COVERAGE=1 WASM_PREBUILT=1 GO=... ./scripts/run-browser-coverage.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

GO="${GO:-$REPO_ROOT/.tools/go/bin/go}"
COV_RAW="$REPO_ROOT/coverage/browser-raw"
COV_OUT="$REPO_ROOT/coverage/browser.out"

# Clean previous raw coverage data
rm -rf "$COV_RAW"
mkdir -p "$COV_RAW"

echo "========================================"
echo "Running browser tests with coverage"
echo "========================================"
echo ""

# Run browser tests with COVERAGE=1 so helpers flush data
export COVERAGE=1
export JS_COVERAGE=1
export WASM_PREBUILT=1
export GO

"$SCRIPT_DIR/run-browser-tests.sh" --no-fail-fast "$@" || true

echo ""
echo "========================================"
echo "Processing coverage data"
echo "========================================"

# Check if any coverage data was collected
COV_FILES=$(find "$COV_RAW" -name 'covmeta.*' 2>/dev/null | head -1)
if [[ -z "$COV_FILES" ]]; then
    echo "WARNING: No coverage data collected."
    echo "Make sure the WASM binary was built with 'make wasm-cover'."
    exit 1
fi

# Convert to textfmt
"$GO" tool covdata textfmt -i="$COV_RAW" -o="$COV_OUT"

echo ""
echo "Coverage summary:"
"$GO" tool covdata percent -i="$COV_RAW"

echo ""
echo "Coverage profile written to: $COV_OUT"
echo "Generate HTML report with: make coverage-report"
