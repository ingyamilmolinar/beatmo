#!/bin/bash
# Outputs a JSON matrix of browser test batches for GitHub Actions.
#
# Usage: ./scripts/browser-test-matrix.sh [--batch-size N] [--filter PATTERN]
#
# Output format (to stdout):
# {
#   "include": [
#     {"index": 1, "tests": "audio.browser.test.js bpm.browser.test.js ..."},
#     {"index": 2, "tests": "connect_mode.browser.test.js ..."},
#     ...
#   ]
# }
#
# Options:
#   --batch-size N Number of tests per job (default: 4)
#   --filter PAT   Only include tests matching pattern

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Source test configuration
source "$SCRIPT_DIR/browser-test-config.sh"

# Parse arguments
FILTER=""
BATCH_SIZE=4

while [[ $# -gt 0 ]]; do
    case $1 in
        --batch-size)
            BATCH_SIZE="$2"
            shift 2
            ;;
        --filter)
            FILTER="$2"
            shift 2
            ;;
        -h|--help)
            echo "Usage: $0 [--batch-size N] [--filter PATTERN]"
            exit 0
            ;;
        *)
            echo "Unknown option: $1" >&2
            exit 1
            ;;
    esac
done

# Discover test files (same logic as run-browser-tests.sh)
cd "$REPO_ROOT/src/js"

declare -a TEST_FILES=()

for test_file in *.browser.test.js; do
    [[ -f "$test_file" ]] || continue

    test_name="$test_file"

    # Apply filter
    if [[ -n "$FILTER" ]] && [[ ! "$test_name" =~ $FILTER ]]; then
        continue
    fi

    # Skip configured tests
    if should_skip "$test_name"; then
        continue
    fi

    TEST_FILES+=("$test_file")
done

# Sort for consistent ordering
IFS=$'\n' TEST_FILES=($(sort <<<"${TEST_FILES[*]}")); unset IFS

TOTAL=${#TEST_FILES[@]}

if [[ $TOTAL -eq 0 ]]; then
    echo '{"include":[]}'
    exit 0
fi

# Build JSON array of batches
json='{"include":['
batch_index=0

for ((i = 0; i < TOTAL; i += BATCH_SIZE)); do
    batch_end=$((i + BATCH_SIZE))
    if [[ $batch_end -gt $TOTAL ]]; then
        batch_end=$TOTAL
    fi

    # Collect test names for this batch
    tests=""
    for ((j = i; j < batch_end; j++)); do
        if [[ -n "$tests" ]]; then
            tests="$tests ${TEST_FILES[$j]}"
        else
            tests="${TEST_FILES[$j]}"
        fi
    done

    ((batch_index++)) || true

    if [[ $i -gt 0 ]]; then
        json="$json,"
    fi
    json="$json{\"index\":$batch_index,\"tests\":\"$tests\"}"
done

json="$json]}"

echo "$json"
