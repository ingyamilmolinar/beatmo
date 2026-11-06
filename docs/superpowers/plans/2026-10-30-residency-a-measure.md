# Residency A — Measure First: Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** see every audio resource the game holds and load it predictably in real Chrome, then record today's breaking points, before any behaviour changes.

**Architecture:** audio.js counts its own resources (`audioResourceStats()`); Go keeps a ledger of its side and merges the JS counters on read (`resourceLedger()`). A load harness adds calibrated burners for the main thread, the render workers, the audio thread and memory. One load-step runner raises one dimension at a time until a gate fails. Its first runs, on today's code, are the baseline that Plans B–D are judged against.

**Tech Stack:** Go 1.26 (js/wasm + desktop), audio.js / AudioWorklet / Web Workers, Playwright + CDP in real Chrome.

**Spec:** `docs/superpowers/specs/2026-10-25-instrument-residency-and-resource-budgets-design.md` (§2, §7.2, §9, §10 phase 0). The plans run in order: **A (this) → B (audio bundles + Save As) → C (residency + switch rule) → D (caps + governor)**.

## Global Constraints

- Never stage or commit (user rule). Never `git stash`. Use `.tools/go/bin/go`.
- Never serve the shared `src/js/main.wasm` in a timing run: build a private wasm (`GOOS=js GOARCH=wasm .tools/go/bin/go build -o <scratch>/main.wasm ./cmd` from `src/go`) and serve it with `WASM_FILE` + `WASM_PREBUILT=1` (the private-farm recipe is in memory `reference_private_browser_test_farm`).
- Timing gates are enforced only in real Chrome (`TEST_PLATFORM=chrome-local TEST_HEADED=1`). Headless runs enforce the structural gates only: counts, bytes, missed and dropped notes.
- Before a timing run, read `/proc/loadavg`. Discard a step whose 1-min load exceeds `MAX_LOAD` (default 15) before or after it, as `scripts/audio_lane_ab.mjs` does.
- No behaviour change in this plan; every existing suite stays as it is.
- A new Go JS export gets a `wasm_bridge_smoke.browser.test.js` catalogue entry. A JS-only global goes in `jsNativeCatalogueEntries` (`internal/ui/js_exports_catalogue_drift_test.go`). Test seams are `__`-prefixed (exempt).
- Tests never read reference audio.

## Review Focus

1. **A run on a busy machine** (other sessions building) reports false breaking points. Expected: the step is marked discarded and re-run once, never counted. Test: Task 4 `loadGate` unit cases.
2. **A burner that does not load what it claims.** Expected: before any step, the harness measures each thread on a fixed job and refuses to run when the measured slow-down is more than ±25 % off the profile's figure. Test: Task 3 `load_harness.browser.test.js`.
3. **Reading the ledger creates resources** (the bug class §3 lists). Expected: calling `resourceLedger()` / `audioResourceStats()` before the AudioContext exists returns zeros and builds no chain. Test: Task 2.
4. **The ledger is not free.** Expected: one `resourceLedger()` call costs ≤ 1 ms of main thread, and the bridge fetches JS counters at most twice a second. Test: Task 2.
5. **Headless numbers mistaken for real ones.** Expected: the runner stamps `platform` and `headed` into every result, and the baseline report rejects a headless result for a timing row. Test: Task 4 output schema check.

---

### Task 1: audio.js resource counters

**Files:** Modify `src/js/audio.js`. Modify `internal/ui/js_exports_catalogue_drift_test.go` (`jsNativeCatalogueEntries` += `audioResourceStats`). Modify `src/js/wasm_bridge_smoke.browser.test.js` (catalogue entry, `returns: {kind:"object", keys:[…]}`). Test: Create `src/js/resource_stats.browser.test.js`.

**Interfaces — Produces:** `window.audioResourceStats() → { renderEntries, renderBytes, rawRenderEntries, rawRenderBytes, playedVariants, channelChains, dspNodesLive, analysers, laneVoices, activeSources, instrumentBlocks, blockBytes, samplePCMIds, samplePCMBytes, workerPending, audioQueue }` (numbers).
- `renderBytes` counts `data.length × 4`, plus `buffer.length × numberOfChannels × 4` when an AudioBuffer exists.
- `dspNodesLive` = created − disposed (count `_dspDispose` calls).
- `playedVariants` comes from a counter incremented and decremented where `playedCache` entries are made and dropped (a WeakMap cannot be enumerated).

- [ ] Test: `resource_stats.browser.test.js` (headless OK, real app):
  - before the first gesture, every field is 0, except `instrumentBlocks` / `blockBytes` (boot pushes blocks), and `channelChains` is unchanged by the call;
  - after `startPlaybackWhenReady` + 3 s, `renderEntries > 0`, `renderBytes ≥ Σ data bytes` (cross-checked by iterating `renderCache` in-page), and `laneVoices + activeSources > 0`;
  - after `addInsertEffect(<row-0 instrument>, "delay")`, `dspNodesLive` rises by exactly 1.
- [ ] Implement; run the test; run `wasm_bridge_smoke.browser.test.js` and `go test -tags test -run TestJSExportCatalogueDrift ./internal/ui/` — both pass.

### Task 2: Go ledger + `resourceLedger()`

**Files:**
- Create `internal/audio/ledger.go` (no build tags): classes and the Go-side counts.
- Create `internal/audio/ledger_wasm.go`: fetch JS counters with a 500 ms TTL cache, mirroring `analyzer_snapshot_cache.go`.
- Create `internal/ui/js_exports_ledger.go` (export `resourceLedger`).
- Test: `internal/audio/ledger_test.go` (`-tags test`); smoke catalogue entry.

**Interfaces — Produces:**
- `type LedgerClass string` with consts `LedgerDocs`, `LedgerBlocks`, `LedgerChannels`, `LedgerInsertChains`, `LedgerEQRecords`, `LedgerSampleEdits`, `LedgerSamplePCM`, `LedgerVoiceCache`, `LedgerVoiceLastSamples`, `LedgerMixerSlots`, `LedgerAnalyzerSlots`, `LedgerGoHeap`.
- `type LedgerEntry struct{ Count, Bytes int64 }`.
- `type Ledger struct{ Go map[LedgerClass]LedgerEntry; JS map[string]float64; JSAgeMS int64 }`.
- `func ResourceLedger() Ledger`. Every count is read through a lookup that never creates: use `lookupChannel`, not `ensureChannel`, and read the maps directly under their locks.
- JS export `resourceLedger()` → `{go:{class:{count,bytes}}, js:{…}, jsAgeMs}`.

- [ ] Tests:
  - `TestLedgerReadCreatesNothing`: `ResourceLedger()` twice; `chanMgr` channel count and the docStore size are unchanged.
  - `TestLedgerCountsMove`: `SetInsertEffects` on one id → `LedgerInsertChains.Count` +1; `RegisterSamplePCM` 48 000 frames → `LedgerSamplePCM.Bytes` ≥ 192 000.
  - Browser: `resourceLedger()` before the gesture leaves `audioGraphStats().channelChains` unchanged; 100 calls take ≤ 100 ms in total (in-page `performance.now`).
- [ ] Implement; `go test -tags test ./internal/audio/ -run Ledger`, `make js-audit`, smoke test pass.

### Task 3: Load harness (predictable load)

**Files:**
- Create `src/js/load_harness.js` (Node side, used by benches).
- Create `src/js/load_burner_worklet.js` (`beatmo-load-burner` AudioWorkletProcessor).
- Modify `src/js/synth_render_worker.js`: a `{burn: fraction}` message installs a `setInterval(…, 10)` busy loop of `fraction × 10 ms`.
- Modify `src/js/audio.js`: test seams `window.__setRenderWorkerBurn(f)`, `window.__startAudioBurner(f, moduleUrl)` / `__stopAudioBurner()`, and `window.__ballastMB(n)`, which holds `n` MB of ArrayBuffers. `__startAudioBurner` is handed the burner module's URL by `load_harness.js`. audio.js never names `load_burner_worklet.js`, so the deploy manifest never ships it.
- Modify `internal/ui` (a wasm-only test seam `__goBallastMB(n)` that keeps `n` MB live in Go).
- Test: Create `src/js/load_harness.browser.test.js`.

**Interfaces — Produces** (from `load_harness.js`):
- `LOAD_PROFILES`:
  - `laptop: {cpu:1, workerBusy:0, audioBusy:0, deviceMemoryGB:null}`
  - `midPhone: {cpu:4, workerBusy:0.5, audioBusy:0.5, deviceMemoryGB:4}`
  - `lowPhone: {cpu:6, workerBusy:0.65, audioBusy:0.65, deviceMemoryGB:2}`
- `async function applyLoadProfile(page, name) → {stop(), calibration:{mainX, workerX, audioX}}`.
  - Uses CDP `Emulation.setCPUThrottlingRate` for `cpu`, an init script overriding `navigator.deviceMemory`, and the seams above.
  - `calibration` = measured slow-down of a fixed 20 ms busy job on each thread relative to the unloaded run.

- [ ] Test: `load_harness.browser.test.js` (real Chrome when `TEST_PLATFORM=chrome-local`, else Chromium) asserts, per profile, `|calibration.workerX − 1/(1−workerBusy)| ≤ 25 %`, the same for `audioX`, and `mainX ≥ 0.75 × cpu`. Also: `applyLoadProfile` then `stop()` returns every figure to within 10 % of 1.
- [ ] Implement; run the test under `chrome-local` headed and `chromium-local`; record both outputs in the task notes.

### Task 4: Load-step runner

**Files:**
- Create `src/js/webaudio_load_steps.browser.test.js`.
- Create `src/js/load_gate.js`: pure, unit-tested in-file with `node load_gate.js --self-test`.
- Modify `scripts/browser-test-config.sh` (add to `SEQUENTIAL_TESTS`).
- Modify `Makefile`: `bench-load` target with `DIMENSION=`, `PROFILE=`, `STEPS=`.

**Interfaces — Consumes:** Task 3 `applyLoadProfile`; `startPlaybackWhenReady`, `startPlaybackProbe`, `stopPlaybackProbe`, `probeSummaryFields` (`audio_probe_bench_helpers.js`); Task 2 `resourceLedger()`.

**Interfaces — Produces:**
- Env:
  - `LOAD_DIMENSION ∈ {instruments, instrumentsFx, library, switch, preview, edit, effects, nodes, bpm}`
  - `LOAD_PROFILE` (default `laptop`), `LOAD_STEPS` (comma list overriding the defaults), `LOAD_SECS` (default 12 per step), `MAX_LOAD` (default 15).
- Default steps:
  - `instruments` and `instrumentsFx` (each row with an EQ + 2 effects): 4, 8, 12, 16, 24, 32, 48, 64.
  - `library`: 100, 500, 1 000, 2 000, 5 000 user instruments seeded into localStorage before boot.
  - `switch`: 1, 2, 4, 8 switches/s.
  - `effects`: 0, 2, 4, 8, 16 per row.
  - `nodes`: 100, 250, 500, 1 000, 2 000.
  - `bpm`: 90, 120, 160, 200, 240.
  - `edit`: the live-edit phases at 1×, 2×, 4× rate.
  - `preview`: menu picks at 1, 2, 4 per second.
- `load_gate.js`:
  - `export function stepPasses(report, ledger, perf, opts) → {pass, reasons[]}`.
  - `export function loadOk(l0, l1, max) → boolean`.
- Gates (spec §9.3):
  - `notes.missed == 0`, `notes.dropped == 0`, `arrivedLate == 0`;
  - `startLateMs.max ≤ 1000/48000` (one sample);
  - `late.over20ms == 0`, until Task 5 replaces it with `lagVsIdealMs.max ≤ HEARD_TOLERANCE_MS`;
  - no page error;
  - `perfStats().fpsAvg ≥ 20`.
  - The lateness gates are skipped (reported) when not headed `chrome-local`.
- Output: `bench-results/load_<dimension>_<profile>.json` → `{platform, headed, profile, calibration, machineLoad:[…], steps:[{value, pass, reasons, probe:probeSummaryFields, ledger, perf}], breakingPoint}`. `breakingPoint` is the first failing value, or null.

- [ ] `node src/js/load_gate.js --self-test` covers:
  - a clean report passes;
  - each gate fails alone with its reason;
  - `loadOk(3, 16, 15) === false`;
  - a headless run skips the lateness gates.
- [ ] Implement; smoke run `LOAD_DIMENSION=instruments LOAD_STEPS=4,8 LOAD_SECS=6` headless → the file is written with two steps; it is the default when the suite runs the test.

### Task 5: Baseline on today's code

**Files:**
- Create `bench-results/residency_baseline_2026-10-30.md`.
- Create `internal/audio/voice_warm_cost_native_test.go` (`!test && !js`): `BenchmarkWarmInstrumentDesktop` — `WarmInstrument` for 10 roster ids × `DefaultWarmPitches`, reported per voice.

- [ ] With a private wasm, real Chrome headed: run every dimension × {laptop, midPhone, lowPhone} via `make bench-load`.
- [ ] Re-measure the spec §2 table on the post-10-27 pipeline, adding N idle channels that carry EQ + 2 effects (100, 500).
- [ ] Run the desktop benchmark.
- [ ] Write the report:
  - every breaking point;
  - the ledger at each step;
  - the measured desktop warm cost (it settles the 0.3 vs 24 ms question);
  - cold pick-to-sound p50/p95, which decides spec §5.6 against the proposed 50 ms;
  - one line per dimension on what gave first: lateness, missed notes, FPS or memory.
- [ ] Measure the heard-onset tolerance (spec §9.3). On the laptop profile, idle, real Chrome, take the probe's `detectMs.p99` over the `instruments` 12-row step. Set `export const HEARD_TOLERANCE_MS = Math.ceil(detectMs.p99 + 5)` in `load_gate.js` (5 ms = the voice fade-in, `CHAIN_SPEC.fadeInMs`). The constant's comment cites the run. Switch the lateness gate to it and re-run the self-test.
- [ ] Add a "Plan D inputs" section holding:
  - the low-phone largest passing step for `instruments`, `effects` and `nodes`;
  - each profile's peak ledger bytes per class at the 12-row reference circuit.
