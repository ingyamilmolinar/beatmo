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
    "webaudio_three_stage_latency.browser.test.js"
    "webaudio_bench_startup.browser.test.js"
    "webaudio_bpm_stress.browser.test.js"
    "webaudio_perf.browser.test.js"
    "webaudio_stress_complex.browser.test.js"
    # Live-edit latency gates (deferAbsMsP90 / event-loop-gap ceilings + CDP CPU
    # throttle scenario): parallel Playwright instances starve the WASM
    # sequencer and the render worker pool, invalidating the on-grid gates.
    "webaudio_all_knob_bpm_stress.browser.test.js"
    # Real-touch open tap only registers when the WASM game loop is ticking;
    # parallel Playwright instances starve it below ~5fps so the touchStart→End
    # falls in a frame gap and is dropped. Passes reliably one-at-a-time.
    "inst_menu_mobile_touch.browser.test.js"
    # Same root cause: a broad real-CDP-touch mobile suite whose small-target
    # taps (mute, row label) are dropped when the loop is starved by parallel
    # jobs. Warm-up gated internally; run alone so the loop stays responsive.
    "mobile_sanity.browser.test.js"
    # Audio-capture e2e: captures a baseline by playing the sequencer and reading
    # ScriptProcessor output. Under parallel jobs the sequencer goroutine is
    # starved and the baseline reads silence ("Sequencer not playing", peak~2e-4),
    # failing before the drag sweep even starts. Loud baseline (peak~0.26) when
    # run alone — same starvation class as the webaudio_* entries above.
    "instrument_params_e2e_real_drag.browser.test.js"
    # Reads each template instrument's per-instrument WebAudio analyser. Under
    # parallel jobs the analyser warm-up is starved and quiet instruments read
    # ~0 (e.g. kick-tight, hi-hat), false-tripping the wired-vs-unwired check.
    # Run alone so every analyser warms up and reports its true (non-zero) peak.
    "template_audio_panel_data.browser.test.js"
)

# Tests that are known to be flaky or run separately and should be skipped
# Visual tests are run via `make test-visual` instead of `make test-browser`.
#
# webaudio_fx_chain_stress is an INTENTIONAL known-failing reproducer for the
# "choppy/inconsistent audio under heavy FX load" investigation — it fails on
# tight Stage-A scheduler-latency thresholds by design (sequencer goroutine
# starved by Ebiten draw on the single WASM thread; see
# bench-results/choppy_audio_fx_chain_findings_2026-06.md). It is skip-listed
# so it does not red-gate unrelated CI; run it manually with
#   GO=$(pwd)/.tools/go/bin/go node src/js/webaudio_fx_chain_stress.browser.test.js
# Remove it from this list once the root cause is fixed (then it becomes a
# regression gate).
SKIP_TESTS=(
    "visual_mobile_parity.browser.test.js"
    "visual_regression.browser.test.js"
    "visual_device_parity.browser.test.js"
    "webaudio_fx_chain_stress.browser.test.js"
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
