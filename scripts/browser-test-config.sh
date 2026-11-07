#!/bin/bash

# test_name=ENV_VARS
declare -A TEST_ENV

# Performance tests need specific thresholds
TEST_ENV["webaudio_perf.browser.test.js"]="PERF_BROWSER_UPDATE_MAX_MS=0.6 PERF_BROWSER_UPDATE_JITTER_MS=0.15 PERF_BROWSER_FPS_MIN=55"
TEST_ENV["webaudio_perf_e2e.browser.test.js"]=""
TEST_ENV["webaudio_pan_stress.browser.test.js"]=""

# Run alone: each has ms-level timing thresholds or writes bench data, and parallel Playwright instances starve the
# WASM sequencer goroutine.
SEQUENTIAL_TESTS=(
    "webaudio_ready_gate.browser.test.js"
    "webaudio_timing_stress.browser.test.js"
    "webaudio_three_stage_latency.browser.test.js"
    "webaudio_bench_startup.browser.test.js"
    "webaudio_bpm_stress.browser.test.js"
    "webaudio_perf.browser.test.js"
    "webaudio_stress_complex.browser.test.js"
    # Timing is the measurement.
    "webaudio_audio_isolation_bench.browser.test.js"
    # On-grid gates under a CPU throttle: parallel jobs starve the sequencer and the render workers.
    "webaudio_all_knob_bpm_stress.browser.test.js"
    # A real-touch tap registers only while the game loop ticks; starved below ~5 fps it falls in a frame gap.
    "inst_menu_mobile_touch.browser.test.js"
    # Small-target real touches, dropped when the loop is starved.
    "mobile_sanity.browser.test.js"
    # Its baseline plays the sequencer and captures output; starved, the baseline reads silence.
    "instrument_params_e2e_real_drag.browser.test.js"
    # Quiet instruments' analysers read ~0 when their warm-up is starved.
    "template_audio_panel_data.browser.test.js"
    # Four fresh iPhone contexts on one WASM thread: starved, a page's boot poll misses its 30 s gate.
    "mobile_audio_unlock.browser.test.js"
    # Three engines in one process: starved, a page's boot poll misses its gate.
    "xplat_browser_export_import.browser.test.js"
    # Measures boot times; parallel browsers would skew them.
    "landing_load.browser.test.js"
    # Each below measures real time and fails only under parallel load. Heard-audio gates under an injected stall, plus
    # an in-page timer edit storm whose edit count is gated.
    "audio_lane_e2e.browser.test.js"
    # f0 from a real-time output capture; starved, a short note reads an octave low.
    "melodic_duration_pitch.browser.test.js"
    # A 5 s capture window that a starved main thread misses.
    "sampler_register_pcm.browser.test.js"
    "sampler_unregister_pcm.browser.test.js"
    # A fixed ~0.83 s capture window around two cold voice renders that outlast it when starved.
    "webaudio_smoke.browser.test.js"
    # The CPU profile is the measurement (hard scheduling / CPU-share gates).
    "webaudio_profile_cdp.browser.test.js"
    # Hard overdue gate during synth-param spam.
    "webaudio_synth_param_stress.browser.test.js"
)

# Visual tests run via make test-visual. webaudio_fx_chain_stress is a deliberately failing reproducer of choppy audio
# under heavy FX load (the sequencer starved by the draw); run it by hand, and drop it from this list once it is fixed.
SKIP_TESTS=(
    "visual_mobile_parity.browser.test.js"
    "visual_regression.browser.test.js"
    "visual_device_parity.browser.test.js"
    "webaudio_fx_chain_stress.browser.test.js"
    # Real BrowserStack devices only: without credentials it exits 1, with them it opens a paid tunnel. Run by hand.
    "mobile_import_realdevice.browser.test.js"
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
