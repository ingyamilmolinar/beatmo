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
mapfile -t TEST_FILES < <(printf '%s\n' "${TEST_FILES[@]}" | sort)

TOTAL=${#TEST_FILES[@]}

if [[ $TOTAL -eq 0 ]]; then
    echo '{"include":[]}'
    exit 0
fi

# Performance tests get a runner each (timing on a shared 4-vCPU runner means
# nothing); the rest are packed BATCH_SIZE to a job.
declare -a PERF_FILES=() OTHER_FILES=()
for f in "${TEST_FILES[@]}"; do
    if is_retry "$f"; then PERF_FILES+=("$f"); else OTHER_FILES+=("$f"); fi
done

json='{"include":['
batch_index=0
add_batch() {
    ((batch_index++)) || true
    [[ $batch_index -gt 1 ]] && json="$json,"
    json="$json{\"index\":$batch_index,\"tests\":\"$1\"}"
}
for f in "${PERF_FILES[@]}"; do add_batch "$f"; done
for ((i = 0; i < ${#OTHER_FILES[@]}; i += BATCH_SIZE)); do
    add_batch "${OTHER_FILES[*]:i:BATCH_SIZE}"
done
json="$json]}"

echo "$json"
