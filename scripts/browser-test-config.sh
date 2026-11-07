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
    # UI-load → heard-audio isolation matrix (playback probe): timing is the
    # measurement, so it must not share the CPU with parallel instances.
    "webaudio_audio_isolation_bench.browser.test.js"
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
    # Mobile audio-unlock e2e: iPhone emulation (js/wasm → NumCPU()==1, single
    # WASM thread) boots the full ~29MB app across FOUR fresh contexts, then
    # drives real-gesture unlock (re-suspension recovery) + sequencer playback
    # with output capture. Under parallel jobs the renderer's single main thread
    # is starved, so a fresh page's boot poll (waitForFunction on the audio.js
    # exports) never resolves inside the 30s gate — Scenario 3 times out. Passes
    # reliably one-at-a-time. Same starvation class as mobile_sanity /
    # instrument_params_e2e_real_drag above.
    "mobile_audio_unlock.browser.test.js"
    # Boots the full app in THREE engines (Chromium + Firefox + WebKit) in one
    # process. Three renderers plus the parallel jobs' own browsers starve the
    # single WASM thread, so a fresh page's boot poll (waitForFunction on the Go
    # exports) can miss its gate — same starvation class as mobile_audio_unlock
    # above. It has no timing thresholds of its own; it just needs each engine to
    # finish booting.
    "xplat_browser_export_import.browser.test.js"
    # Cold-load timing of the landing page under CDP network + CPU throttling
    # (landing vs no-landing TTI ratio gate when BASELINE_INDEX is set):
    # parallel browsers would skew the very boot times it measures.
    "landing_load.browser.test.js"
    # The entries below each failed in a 4-job parallel run (2026-10-28/29) and pass
    # alone, every time, on the same tree — each measures real time:
    # Heard-audio gates (playback probe) under an injected main-thread stall,
    # plus an in-page timer edit storm whose edit count is gated: parallel jobs
    # starved the timers (17 edits marked vs 30 alone, floor 20).
    "audio_lane_e2e.browser.test.js"
    # f0 measured from a REAL-TIME output capture of each note: under parallel
    # load the 0.25-beat violin read an octave low (-1204 cents); alone, 0 cents.
    "melodic_duration_pitch.browser.test.js"
    # Output captured through a main-thread ScriptProcessor with a 5 s window
    # (same starvation class as instrument_params_e2e_real_drag above): under
    # parallel load it captured nothing (nonZero=0); alone, ~10k samples.
    "sampler_register_pcm.browser.test.js"
    "sampler_unregister_pcm.browser.test.js"
    # Same main-thread ScriptProcessor capture, of a fixed ~0.83 s window from
    # AudioContext creation, around two COLD voice renders: under parallel load
    # the renders outlasted the window ("missing audio data for multiple
    # beats", make test-real 2026-10-29); 6/6 alone.
    "webaudio_smoke.browser.test.js"
    # CPU + heap profile with HARD scheduling / CPU-share gates (overdue,
    # minLead, audio-flush %, WASM overhead %): the profile is the measurement.
    "webaudio_profile_cdp.browser.test.js"
    # HARD overdue gate during synth-param spam (sibling of
    # webaudio_all_knob_bpm_stress above): 1 note 4 ms late under parallel
    # load; 0 alone.
    "webaudio_synth_param_stress.browser.test.js"
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
    # Real devices on BrowserStack only ("On-demand only — real devices, not
    # CI", its header): with no credentials it exits 1, and with them it opens a
    # paid tunnel per run. Run it by hand (DEVICE_FILTER=iPhone|Pixel optional):
    #   GO=$(pwd)/.tools/go/bin/go node src/js/mobile_import_realdevice.browser.test.js
    "mobile_import_realdevice.browser.test.js"
)

# Function to get env vars for a specific test
get_test_env() {
    local test_name="$1"
    echo "${TEST_ENV[$test_name]:-}"
}

# Function to check if test must run sequentially
is_sequential() {
    local test_name="$1" t
    for t in "${SEQUENTIAL_TESTS[@]}"; do
        if [[ "$t" == "$test_name" ]]; then
            return 0
        fi
    done
    return 1
}

# Function to check if test should be skipped
# Performance tests: the only ones a failure is retried for (BROWSER_TEST_RETRIES,
# default 2), and each gets a CI runner to itself. Correctness tests never retry.
RETRY_TESTS=(
    "audio_lane_e2e.browser.test.js"
    "landing_load.browser.test.js"
    "synth_stage_toggle_ui_bench.browser.test.js"
    "timing_traces.browser.test.js"
    "wasm_load_startup.browser.test.js"
    "webaudio_all_knob_bpm_stress.browser.test.js"
    "webaudio_audio_isolation_bench.browser.test.js"
    "webaudio_bench_startup.browser.test.js"
    "webaudio_bpm_stress.browser.test.js"
    "webaudio_import_cold_bench.browser.test.js"
    "webaudio_instrument_switch_bench.browser.test.js"
    "webaudio_live_edit_budget.browser.test.js"
    "webaudio_pan_stress.browser.test.js"
    "webaudio_perf.browser.test.js"
    "webaudio_perf_e2e.browser.test.js"
    "webaudio_render_latency_bench.browser.test.js"
    "webaudio_stress_complex.browser.test.js"
    "webaudio_synth_param_stress.browser.test.js"
    "webaudio_three_stage_latency.browser.test.js"
    "webaudio_timing_stress.browser.test.js"
    "webaudio_zoom_stress.browser.test.js"
)

is_retry() {
    local test_name="$1" t
    for t in "${RETRY_TESTS[@]}"; do
        if [[ "$t" == "$test_name" ]]; then
            return 0
        fi
    done
    return 1
}

should_skip() {
    local test_name="$1" t
    for t in "${SKIP_TESTS[@]}"; do
        if [[ "$t" == "$test_name" ]]; then
            return 0
        fi
    done
    return 1
}
