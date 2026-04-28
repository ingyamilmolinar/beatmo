#!/bin/bash
# Browser test configuration
# Defines environment variables and special handling for specific tests

# Tests that require special environment variables
# Format: test_name=ENV_VARS
declare -A TEST_ENV

# Performance tests need specific thresholds
TEST_ENV["webaudio_perf.browser.test.js"]="PERF_BROWSER_UPDATE_MAX_MS=0.6 PERF_BROWSER_UPDATE_JITTER_MS=0.15 PERF_BROWSER_FPS_MIN=55"
TEST_ENV["webaudio_perf_e2e.browser.test.js"]=""
TEST_ENV["webaudio_pan_stress.browser.test.js"]=""

# Tests that must run sequentially: hardcoded ms-level timing thresholds with no
# BROWSER_JOBS scaling, or benchmark data output. CPU contention from parallel
# Playwright instances starves the WASM sequencer goroutine and invalidates results.
SEQUENTIAL_TESTS=(
    "webaudio_ready_gate.browser.test.js"
    "webaudio_timing_stress.browser.test.js"
    "webaudio_bench_startup.browser.test.js"
    "webaudio_bpm_stress.browser.test.js"
    "webaudio_perf.browser.test.js"
    "webaudio_stress_complex.browser.test.js"
)

# Tests that are known to be flaky or run separately and should be skipped
# Visual tests are run via `make test-visual` instead of `make test-browser`.
SKIP_TESTS=(
    "visual_mobile_parity.browser.test.js"
    "visual_regression.browser.test.js"
    "visual_device_parity.browser.test.js"
)

# Function to get env vars for a specific test
get_test_env() {
    local test_name="$1"
    echo "${TEST_ENV[$test_name]:-}"
}

# Function to check if test must run sequentially
is_sequential() {
    local test_name="$1"
    for t in "${SEQUENTIAL_TESTS[@]}"; do
        if [[ "$t" == "$test_name" ]]; then
            return 0
        fi
    done
    return 1
}

# Function to check if test should be skipped
should_skip() {
    local test_name="$1"
    for t in "${SKIP_TESTS[@]}"; do
        if [[ "$t" == "$test_name" ]]; then
            return 0
        fi
    done
    return 1
}
