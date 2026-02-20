#!/bin/bash
# Auto-discovers and runs browser tests with parallel execution support.
# Timing-sensitive tests (SEQUENTIAL_TESTS) run one-at-a-time after the
# parallel batch finishes, to avoid CPU contention.
#
# Usage: ./scripts/run-browser-tests.sh [--filter PATTERN] [--no-fail-fast]
#                                       [--jobs N] [--sequential] [--list]
#                                       [--tests "file1.js file2.js ..."]
#
# Options:
#   --filter PAT   Only run tests matching pattern (e.g., "touch" or "audio")
#   --no-fail-fast Continue running tests even after failures
#   --jobs N       Number of parallel test jobs (default: $BROWSER_JOBS or 4)
#   --sequential   Run ALL tests one at a time (equivalent to --jobs 1)
#   --list         List tests that would run without executing them
#   --tests LIST   Explicit space-separated list of test filenames to run
#                  (skips auto-discovery)
#
# Environment:
#   GO              Path to Go binary (default: auto-detect)
#   BROWSER_JOBS    Default parallelism level (overridden by --jobs)
#   WASM_PREBUILT   Set to 1 to skip WASM builds in test files (auto-set when
#                   invoked via make test-browser)
#   BROWSER_TEST_PORT_BASE  Port base for deterministic port assignment (auto-set)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Source test configuration
source "$SCRIPT_DIR/browser-test-config.sh"

# Parse arguments
FILTER=""
FAIL_FAST=true
LIST_ONLY=false
JOBS="${BROWSER_JOBS:-4}"
EXPLICIT_TESTS=""

while [[ $# -gt 0 ]]; do
    case $1 in
        --filter)
            FILTER="$2"
            shift 2
            ;;
        --no-fail-fast)
            FAIL_FAST=false
            shift
            ;;
        --jobs)
            JOBS="$2"
            shift 2
            ;;
        --sequential)
            JOBS=1
            shift
            ;;
        --list)
            LIST_ONLY=true
            shift
            ;;
        --tests)
            EXPLICIT_TESTS="$2"
            shift 2
            ;;
        -h|--help)
            echo "Usage: $0 [--filter PATTERN] [--no-fail-fast] [--jobs N] [--sequential] [--list] [--tests LIST]"
            echo ""
            echo "Options:"
            echo "  --filter PAT   Only run tests matching pattern"
            echo "  --no-fail-fast Continue running tests even after failures"
            echo "  --jobs N       Number of parallel test jobs (default: \$BROWSER_JOBS or 4)"
            echo "  --sequential   Run ALL tests one at a time (equivalent to --jobs 1)"
            echo "  --list         List tests that would run without executing them"
            echo "  --tests LIST   Explicit space-separated list of test filenames to run"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Export final parallelism level so test processes can read it
export BROWSER_JOBS="$JOBS"

# Auto-detect GO if not set
if [[ -z "${GO:-}" ]]; then
    if [[ -x "$REPO_ROOT/.tools/go/bin/go" ]]; then
        GO="$REPO_ROOT/.tools/go/bin/go"
    else
        GO="go"
    fi
fi
export GO

# Find all browser test files
cd "$REPO_ROOT/src/js"

# Collect test files
declare -a TEST_FILES=()

if [[ -n "$EXPLICIT_TESTS" ]]; then
    # Use explicitly provided test list (from CI matrix or manual invocation)
    for test_file in $EXPLICIT_TESTS; do
        if [[ -f "$test_file" ]]; then
            TEST_FILES+=("$test_file")
        else
            echo "WARNING: test file not found: $test_file" >&2
        fi
    done
else
    for test_file in *.browser.test.js; do
        [[ -f "$test_file" ]] || continue

        test_name="$test_file"

        # Apply filter if specified
        if [[ -n "$FILTER" ]] && [[ ! "$test_name" =~ $FILTER ]]; then
            continue
        fi

        # Check if test should be skipped
        if should_skip "$test_name"; then
            echo "SKIP: $test_name (configured to skip)"
            continue
        fi

        TEST_FILES+=("$test_file")
    done

    # Sort tests alphabetically for consistent ordering
    IFS=$'\n' TEST_FILES=($(sort <<<"${TEST_FILES[*]}")); unset IFS
fi

# Split into parallel and sequential lists
declare -a PARALLEL_FILES=()
declare -a SEQUENTIAL_FILES=()

for test_file in "${TEST_FILES[@]}"; do
    if is_sequential "$test_file"; then
        SEQUENTIAL_FILES+=("$test_file")
    else
        PARALLEL_FILES+=("$test_file")
    fi
done

# List mode - just print tests and exit
if [[ "$LIST_ONLY" == true ]]; then
    echo "Browser tests that would run:"
    echo ""
    for test_file in "${TEST_FILES[@]}"; do
        env_vars=$(get_test_env "$test_file")
        suffix=""
        if is_sequential "$test_file"; then
            suffix="  [sequential]"
        fi
        if [[ -n "$env_vars" ]]; then
            echo "  $test_file  [$env_vars]$suffix"
        else
            echo "  $test_file$suffix"
        fi
    done
    echo ""
    echo "Total: ${#TEST_FILES[@]} tests (${#PARALLEL_FILES[@]} parallel, ${#SEQUENTIAL_FILES[@]} sequential)"
    echo "Parallelism: $JOBS jobs"
    exit 0
fi

# Tell test files to skip WASM builds (pre-built by Makefile targets)
export WASM_PREBUILT=1

TOTAL=${#TEST_FILES[@]}
PASSED=0
FAILED=0
declare -a FAILED_TESTS=()

# Create temp directory for per-test logs
LOG_DIR=$(mktemp -d)
trap "rm -rf $LOG_DIR" EXIT

echo "========================================"
echo "Running $TOTAL browser tests (${#PARALLEL_FILES[@]} parallel @ $JOBS jobs, ${#SEQUENTIAL_FILES[@]} sequential)"
echo "========================================"
echo ""

# Helper: build env command for a test
build_test_cmd_env() {
    local test_name="$1"
    local port_base="$2"
    local env_vars
    env_vars=$(get_test_env "$test_name")

    local record_env=""
    if [[ "${LLM_RECORD:-}" == "1" ]]; then
        local session_name="${test_name%.browser.test.js}"
        record_env="LLM_RECORD=1 LLM_RECORD_NAME=$session_name"
        if [[ -n "${LLM_RECORD_INTERVAL:-}" ]]; then
            record_env="$record_env LLM_RECORD_INTERVAL=$LLM_RECORD_INTERVAL"
        fi
    fi

    echo "GO=$GO WASM_PREBUILT=1 BROWSER_TEST_PORT_BASE=$port_base $env_vars $record_env"
}

# Track global test index for port assignment
GLOBAL_IDX=0

# ──────────────────────────────────────────────────────────────────────
# Phase 1: Parallel tests
# ──────────────────────────────────────────────────────────────────────
PARALLEL_COUNT=${#PARALLEL_FILES[@]}
STOP_EARLY=false

if [[ $PARALLEL_COUNT -gt 0 ]]; then
    echo "── Phase 1: ${PARALLEL_COUNT} parallel tests ──"
    echo ""

    if [[ "$JOBS" -le 1 ]]; then
        # Sequential fallback for --sequential flag
        for i in "${!PARALLEL_FILES[@]}"; do
            test_file="${PARALLEL_FILES[$i]}"
            test_name="$test_file"
            port_base=$((10000 + GLOBAL_IDX * 100))
            ((GLOBAL_IDX++)) || true
            env_vars=$(get_test_env "$test_name")

            echo "----------------------------------------"
            echo "[$((PASSED + FAILED + 1))/$TOTAL] $test_name"
            if [[ -n "$env_vars" ]]; then
                echo "  ENV: $env_vars"
            fi
            echo "----------------------------------------"

            cmd_env=$(build_test_cmd_env "$test_name" "$port_base")
            cmd="env $cmd_env node $test_file"

            if eval "$cmd"; then
                echo "PASS: $test_name"
                ((PASSED++)) || true
            else
                echo "FAIL: $test_name"
                ((FAILED++)) || true
                FAILED_TESTS+=("$test_name")

                if [[ "$FAIL_FAST" == true ]]; then
                    STOP_EARLY=true
                    break
                fi
            fi
            echo ""
        done
    else
        BATCH_START=0

        while [[ $BATCH_START -lt $PARALLEL_COUNT ]] && [[ "$STOP_EARLY" == false ]]; do
            BATCH_END=$((BATCH_START + JOBS))
            if [[ $BATCH_END -gt $PARALLEL_COUNT ]]; then
                BATCH_END=$PARALLEL_COUNT
            fi

            BATCH_SIZE=$((BATCH_END - BATCH_START))
            echo "── Batch $((BATCH_START / JOBS + 1)): tests $((BATCH_START+1))-$BATCH_END of $PARALLEL_COUNT (parallel) ──"

            # Launch batch
            declare -a BATCH_PIDS=()
            declare -a BATCH_FILES=()
            declare -a BATCH_LOGS=()

            for i in $(seq $BATCH_START $((BATCH_END - 1))); do
                test_file="${PARALLEL_FILES[$i]}"
                test_name="$test_file"
                env_vars=$(get_test_env "$test_name")
                port_base=$((10000 + GLOBAL_IDX * 100))
                ((GLOBAL_IDX++)) || true
                log_file="$LOG_DIR/${test_name}.log"

                # When LLM_RECORD=1, auto-set session name
                record_env=""
                if [[ "${LLM_RECORD:-}" == "1" ]]; then
                    session_name="${test_name%.browser.test.js}"
                    record_env="LLM_RECORD=1 LLM_RECORD_NAME=$session_name"
                    if [[ -n "${LLM_RECORD_INTERVAL:-}" ]]; then
                        record_env="$record_env LLM_RECORD_INTERVAL=$LLM_RECORD_INTERVAL"
                    fi
                fi

                # Launch in background
                env GO="$GO" WASM_PREBUILT=1 BROWSER_TEST_PORT_BASE="$port_base" \
                    $env_vars $record_env \
                    node "$test_file" > "$log_file" 2>&1 &

                BATCH_PIDS+=($!)
                BATCH_FILES+=("$test_file")
                BATCH_LOGS+=("$log_file")
            done

            # Wait for batch to complete
            BATCH_FAILED=false
            for j in "${!BATCH_PIDS[@]}"; do
                pid="${BATCH_PIDS[$j]}"
                test_file="${BATCH_FILES[$j]}"
                log_file="${BATCH_LOGS[$j]}"

                if wait "$pid"; then
                    echo "  PASS: $test_file"
                    ((PASSED++)) || true
                else
                    echo "  FAIL: $test_file"
                    ((FAILED++)) || true
                    FAILED_TESTS+=("$test_file")
                    BATCH_FAILED=true
                fi
            done

            unset BATCH_PIDS BATCH_FILES BATCH_LOGS

            if [[ "$BATCH_FAILED" == true ]] && [[ "$FAIL_FAST" == true ]]; then
                STOP_EARLY=true
            fi

            BATCH_START=$BATCH_END
            echo ""
        done
    fi
fi

# ──────────────────────────────────────────────────────────────────────
# Phase 2: Sequential tests (timing-sensitive)
# ──────────────────────────────────────────────────────────────────────
SEQ_COUNT=${#SEQUENTIAL_FILES[@]}

if [[ $SEQ_COUNT -gt 0 ]] && [[ "$STOP_EARLY" == false ]]; then
    echo "── Phase 2: ${SEQ_COUNT} sequential tests (timing-sensitive) ──"
    echo ""

    for i in "${!SEQUENTIAL_FILES[@]}"; do
        test_file="${SEQUENTIAL_FILES[$i]}"
        test_name="$test_file"
        port_base=$((10000 + GLOBAL_IDX * 100))
        ((GLOBAL_IDX++)) || true
        env_vars=$(get_test_env "$test_name")

        echo "----------------------------------------"
        echo "[$((PASSED + FAILED + 1))/$TOTAL] $test_name  [sequential]"
        if [[ -n "$env_vars" ]]; then
            echo "  ENV: $env_vars"
        fi
        echo "----------------------------------------"

        cmd_env=$(build_test_cmd_env "$test_name" "$port_base")
        cmd="env $cmd_env node $test_file"

        if eval "$cmd"; then
            echo "PASS: $test_name"
            ((PASSED++)) || true
        else
            echo "FAIL: $test_name"
            ((FAILED++)) || true
            FAILED_TESTS+=("$test_name")

            if [[ "$FAIL_FAST" == true ]]; then
                break
            fi
        fi
        echo ""
    done
fi

# Print summary
echo "========================================"
echo "SUMMARY"
echo "========================================"
echo "Passed: $PASSED"
echo "Failed: $FAILED"
echo "Total:  $TOTAL"
echo "Jobs:   $JOBS (parallel phase)"

if [[ $FAILED -gt 0 ]]; then
    echo ""
    echo "Failed tests:"
    for t in "${FAILED_TESTS[@]}"; do
        echo "  $t"
    done

    # Print logs of failed tests
    echo ""
    echo "========================================"
    echo "FAILED TEST LOGS"
    echo "========================================"
    for t in "${FAILED_TESTS[@]}"; do
        log_file="$LOG_DIR/${t}.log"
        if [[ -f "$log_file" ]]; then
            echo ""
            echo "──── $t ────"
            # Show last 50 lines of failed test log
            tail -n 50 "$log_file"
            echo "──── end $t ────"
        fi
    done

    exit 1
fi

echo ""
echo "All tests passed!"
exit 0
