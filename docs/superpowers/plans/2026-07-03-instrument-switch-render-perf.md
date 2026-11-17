# Instrument-Switch Render Performance Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Eliminate the audible click + multi-second lag that occurs when a row's instrument is switched to a synth instrument (esp. Organ Church) during playback, on both browser (WASM) and desktop.

**Architecture:** Four independent bottlenecks, fixed in risk-ascending order: (B4) stop the synchronous main-thread render that freezes audio; (B2) cut the pathological `render_modular` cost for unison+additive patches from ~500ms to ~30-50ms per voice; (B1) pre-warm a newly-selected instrument's voice cache off-thread on instrument change; (B3) render bursts in parallel via a small worker pool. Each is mirrored on the desktop Go audio path. A tracking bench + gated regression tests lock the wins.

**Tech Stack:** Go 1.23 (`src/go/internal/ui`, `src/go/internal/audio`), C DSP (`src/c/modular.c`, `src/c/modular_stages.c`), JS/WebAudio (`src/js/audio.js`, `src/js/synth_render_worker.js`), Playwright browser benches.

## Global Constraints

- Bundled Go only: `.tools/go/bin/go`. Fast test path: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./...`.
- Browser render/cache/worker logic lives in `audio.js` (served as-is) — it does **not** require a WASM rebuild. Only Go/C changes need `make wasm` / `make` (rebuild `libdrums.a` after C edits, then `drums.single.js` for the worker).
- C `render_modular` output is byte-locked by `native_golden_test.go` (`internal/audio`) and `xplat_audio_compare.browser.test.js`. **B2 intentionally re-bakes organ-church's golden** — regenerate goldens in the same change (documented re-voicing, perceptually identical), never silently.
- Param-deferral contract ([[feedback_next_trigger_param_deferral]]): edits take effect on the next trigger; a warmed voice cache stays valid until a live param edit bumps its generation. Pre-warm must not violate this.
- Every transient async named pool in a test must `t.Cleanup(async.DefaultRegistry().Release(name))`.
- Never sync-render a voice on the audio/UI thread during live playback (the defect this plan removes). Main-thread `renderVoiceSync` is reserved for offline/export or no-context-playing paths.
- Machine-independent regression anchors only: `mainThreadRenders == 0`, `deferredPlays == 0` after warm, render-count ratios. Absolute millisecond ceilings are env-tunable and non-gating unless a STRICT flag is set.

## Verified Evidence (baseline, this machine)

Isolated single-voice render cost (no worker queue), 48kHz: organ-church **~500-670ms**, violin ~30-47ms, cello ~25-30ms, piano-grand ~11-18ms, fm-bass ~11-16ms. Bench: `src/js/webaudio_instrument_switch_bench.browser.test.js` → `bench-results/instrument_switch.json`. Cost driver: organ-church `osc_enabled:0` + 11 additive partials each through `unison_voices:7`+drift = 77 osc/sample × per-sample `pow`+`sin` = ~7.4M `pow` + 7.4M `sin` per render (`modular_stages.c:2206-2274`). Under the single worker + 4s timeout, slow renders fall to the sync main-thread fallback → 2.6-3.5s freeze (`maxGap`).

## File Structure

- `src/js/audio.js` — worker-timeout fallback policy (B4), `warmInstrument` bridge + no-main-render-during-playback guard (B1), worker-pool dispatch (B3). Already modified: `window.__evictRenderCacheForTest` test hook.
- `src/js/synth_render_worker.js` — pooled instances (B3).
- `src/c/modular_stages.c` — `modular_gen_bank_render` control-rate modulation + summed-partials wavetable (B2).
- `src/c/modular.h` — any new helper decls (B2).
- `src/go/internal/audio/*golden*`, `internal/audio/testdata/` — regenerated organ-church goldens (B2).
- `src/go/internal/ui/drumview_transport.go` — `SetInstrument` calls warm hook (B1).
- `src/go/internal/audio/` — WASM warm bridge (`synth_recipe_wasm.go` / new `voice_warm_wasm.go`) + desktop voice-cache warm (`voice_cache.go` / `engine.go`) (B1 both platforms).
- `src/js/webaudio_instrument_switch_bench.browser.test.js` — tracking bench (exists).
- `src/js/instrument_switch_regression.browser.test.js` — new gated regression (B1/B4).
- `src/go/internal/audio/modular_organ_cost_test.go` — Go-side render-cost ceiling test (B2).
- `src/go/internal/ui/instrument_switch_warm_test.go` — warm-on-switch unit test (B1).

---

### Task 1: Stop the main-thread render freeze during playback (B4)

**Why first:** biggest UX win, lowest risk, zero parity impact. The 2.6-3.5s freeze / click is the sync `renderVoiceSync` fallback firing when the worker round-trip exceeds the 4s timeout (or the worker is unavailable) *while audio is playing*. During playback we must never block the main thread; keep the note on the existing on-grid neighbor-pitch fallback (P3) and let the exact render land later.

**Files:**
- Modify: `src/js/audio.js` (the `ensureRenderedSample` worker→sync fallback at ~`3451-3480`, and `renderVoiceInWorker` timeout at ~`1159-1199`).
- Test: `src/js/instrument_switch_regression.browser.test.js` (Create).

**Interfaces:**
- Consumes: `window.__evictRenderCacheForTest(id)`, `window.getRenderLatencyMetrics()` (`.mainThreadRenders`, `.neighborPlays`, `.deferredPlays`), `window.playSoundParams(id,vol,pitch,dur,when)`, `window.__audioCtx`.
- Produces: an `isPlaybackActive()` predicate (or reuse an existing "context running + recent scheduled events" signal) gating the sync fallback; `renderLatencyMetrics` unchanged in shape.

- [ ] **Step 1: Write the failing regression test** — `src/js/instrument_switch_regression.browser.test.js`. Boot `play_ui.html` (WASM_PREBUILT ok), unlock AudioContext, `__evictRenderCacheForTest('organ-church')`, `resetRenderLatencyMetrics()`, then under `Emulation.setCPUThrottlingRate rate:4` play organ-church across 8 pitches at 90ms cadence for 3s. Assert `getRenderLatencyMetrics().mainThreadRenders === 0`. (Model the harness on `webaudio_instrument_switch_bench.browser.test.js`.)

- [ ] **Step 2: Run it, verify it FAILS** — `WASM_PREBUILT=1 GO=$(pwd)/.tools/go/bin/go node src/js/instrument_switch_regression.browser.test.js`. Expected: fails with `mainThreadRenders > 0` (the throttled organ renders hit the sync fallback).

- [ ] **Step 3: Implement the guard** in `audio.js`. In `ensureRenderedSample`, when the worker path throws/times out, only fall to `renderVoiceSync` if **not** actively playing; during playback, reject the ensure so the caller's P1/P3 path keeps the note on-grid via the neighbor buffer and re-queues. Concretely: wrap the sync fallback in `if (!isPlaybackActive()) { …renderVoiceSync… } else { throw err; }`. Define `isPlaybackActive()` as `window.__audioCtx && window.__audioCtx.state === 'running' && (lastScheduledWhen - ctx.currentTime) > -0.5` (a scheduled note within the last ~0.5s), tracked by a module var set in `enqueueAudioEvents`.

- [ ] **Step 4: Run the regression test, verify PASS.** `mainThreadRenders === 0`; `neighborPlays > 0` acceptable (on-grid fallback engaged).

- [ ] **Step 5: Run the bench, confirm no main-thread renders on organ/throttled scenarios.** `WASM_PREBUILT=1 … node src/js/webaudio_instrument_switch_bench.browser.test.js`; in `bench-results/instrument_switch.json` `warm-baseline.mainThreadRenders === 0` and `cold-melodic-4x.mainThreadRenders === 0`, `maxGapMs` collapses from ~2600-3500 to low tens.

- [ ] **Step 6: Commit.** `git add src/js/audio.js src/js/instrument_switch_regression.browser.test.js` — commit message: `fix(audio): never sync-render on main thread during playback (instrument-switch freeze)`.

---

### Task 2: Cut render_modular unison+additive cost (B2) — control-rate modulation

**Why:** organ-church renders ~500ms because `pow(2,cents/1200)` and `sin` for ~3Hz vibrato/drift are evaluated at 48kHz across 77 oscillators. Moving them to control-rate (recompute every `CTRL_STRIDE` samples, hold/linear-interpolate between) cuts `pow`+`sin` call counts ~`CTRL_STRIDE`×. This is the highest-impact single change and is self-contained in one loop.

**Files:**
- Modify: `src/c/modular_stages.c` (`modular_gen_bank_render` unison branch, `~2231-2274`).
- Modify: `src/c/modular.h` (if a `CTRL_STRIDE` const or helper is shared).
- Regenerate: `src/go/internal/audio` native goldens + xplat golden (organ-church re-bake).
- Test: `src/go/internal/audio/modular_organ_cost_test.go` (Create).

**Interfaces:**
- Consumes: existing `modular_gen_bank_render` signature (unchanged).
- Produces: identical output shape; organ-church samples change (intentional re-bake).

- [ ] **Step 1: Write the failing cost test** — `modular_organ_cost_test.go`. Render organ-church via the same native render entry the goldens use; time it; assert `elapsed < organCostCeiling` where `organCostCeiling` is env-tunable (default e.g. 120ms native) and skip-gated unless `MODULAR_COST_STRICT=1`. Also assert the render is non-silent (peak > 0.05) so a "fast because broken" regression fails.

- [ ] **Step 2: Run it, verify it FAILS** under strict — `cd src/go && MODULAR_COST_STRICT=1 ../../.tools/go/bin/go test -run TestOrganRenderCost ./internal/audio/`. Expected: FAIL (elapsed ~300-500ms > ceiling). (Requires `libdrums.a`; build via `make` if needed.)

- [ ] **Step 3: Implement control-rate modulation** in `modular_gen_bank_render`. Introduce `const int CTRL_STRIDE = 32;` Recompute `vib_mult` (line 2242 `pow`) and each voice's `pow(2, total_cents/1200)` (line 2253) only when `(i % CTRL_STRIDE) == 0`, caching into `double vib_mult_c` and `double voice_ratio_c[MAX_UNISON]`; linearly interpolate toward the next control point (or hold — hold is simpler and inaudible at 3Hz mod / 32-sample stride = 0.67ms). Keep `wt_osc_set_freq` calls gated on the same stride. Leave `wt_osc_tick` per-sample.

- [ ] **Step 4: Rebuild C + run the cost test, verify PASS.** `make` (rebuild `libdrums.a`); `cd src/go && MODULAR_COST_STRICT=1 ../../.tools/go/bin/go test -run TestOrganRenderCost ./internal/audio/`. Expected: elapsed well under ceiling, non-silent.

- [ ] **Step 5: Regenerate goldens (intentional re-bake).** Run the native golden updater (per `native_golden_test.go` update flow, e.g. `GOLDEN_UPDATE=1 … test ./internal/audio -run Golden`) and `IFX_GOLDEN_UPDATE=1` for the xplat golden. Inspect the diff is confined to organ-church (and any other `unison_voices>=2` additive patch). Commit the regenerated `testdata/`.

- [ ] **Step 6: Verify parity is internally consistent.** `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/...`; then rebuild `drums.single.js` (`make wasm` or the drums.single build step) and run `GO=$(pwd)/.tools/go/bin/go node src/js/xplat_audio_compare.browser.test.js` — WASM must match the new native golden bit-for-bit.

- [ ] **Step 7: Commit.** `git add src/c/modular_stages.c src/c/modular.h src/go/internal/audio/testdata` — `perf(dsp): control-rate vibrato/drift in modular gen-bank unison (organ ~4x faster, golden re-baked)`.

---

### Task 3: Cut render_modular cost further (B2) — sum additive partials into one wavetable

**Why:** even at control rate, organ-church runs 77 oscillators/sample (11 partials × 7 unison). A static additive spectrum is a single periodic waveform: bake the 11 harmonics into ONE wavetable once, then unison replicates 7 copies of that table (7 osc/sample, not 77). Combined with Task 2 this brings organ into the ~30-50ms band.

**Files:**
- Modify: `src/c/modular_stages.c` (`modular_gen_bank_render` — detect a contiguous run of `source==1` sine slots with harmonic ratios and a shared unison config; synthesize a summed wavetable).
- Modify: `src/c/modular.h` / `src/c/wavetable.*` if a "build additive table" helper is added.
- Regenerate: organ-church goldens again.
- Test: reuse `modular_organ_cost_test.go` (tighten ceiling).

**Interfaces:**
- Consumes: gen-bank slot params (`gen*_source`, harmonic ratio, amp) already parsed in the render.
- Produces: same output shape; organ-church re-baked once more.

- [ ] **Step 1: Tighten the failing ceiling.** In `modular_organ_cost_test.go` drop `organCostCeiling` to e.g. 60ms native. Run strict → FAIL (Task-2 result ~120ms > 60ms).

- [ ] **Step 2: Implement summed-partials wavetable.** At render setup (before the per-sample loop), when the gen-bank holds ≥2 `source==1` sine slots sharing `freq_mode==0` + one unison config, precompute `double add_table[WT_LEN]` = Σ amp_k·sin(2π·ratio_k·φ) over one period, normalize, and drive unison from a single `wt_osc` over `add_table` instead of iterating slots per sample. Preserve per-slot amplitude weighting exactly. Fall back to the existing per-slot path when the pattern doesn't match (non-harmonic ratios, per-slot LFO, noise taps).

- [ ] **Step 3: Rebuild + cost test PASS** (`make`; strict cost test under 60ms, non-silent).

- [ ] **Step 4: Regenerate goldens + xplat verify** (same as Task 2 Steps 5-6). Diff confined to additive-unison patches.

- [ ] **Step 5: Run the browser bench** — organ-church isolated render should now be tens of ms; `bench-results/instrument_switch.json` `cold-melodic` `deferredPlays`/`renderMsMax` collapse toward `cold-cello`.

- [ ] **Step 6: Commit.** `perf(dsp): bake additive organ partials into one wavetable (organ render ~10-15x faster, golden re-baked)`.

---

### Task 4: Pre-warm a newly-selected instrument's voice cache (B1) — WASM

**Why:** with renders now cheap (Tasks 2-3), pre-warming the switched-to instrument's pitch set off-thread before the first note removes the residual cold-miss defers. Satisfies the user's "pre-compute … keep cached until params live-edited" — the existing gen-based staleness already invalidates only on live edits.

**Files:**
- Modify: `src/js/audio.js` (add `window.warmInstrument(id, pitchesJSON)` → dedup-ensure-render each pitch off-thread, non-blocking).
- Create: `src/go/internal/audio/voice_warm_wasm.go` (`//go:build js`) — `WarmInstrument(id string, pitches []int)` calling the JS bridge; and a `!js` stub in existing stub file.
- Modify: `src/go/internal/ui/drumview_transport.go:341` `SetInstrument` — after setting the row instrument, gather the row's node pitch set and call `audio.WarmInstrument(id, pitches)`.
- Test: `src/go/internal/ui/instrument_switch_warm_test.go` (Create); extend `instrument_switch_regression.browser.test.js`.

**Interfaces:**
- Consumes: row node pitch set (from the row's beat infos / graph), `ensureRenderReady(id, pitch)` (existing).
- Produces: `audio.WarmInstrument(id string, pitches []int)`; JS `window.warmInstrument(id, pitchesJSON)`.

- [ ] **Step 1: Failing browser assertion.** In `instrument_switch_regression.browser.test.js` add: after calling `window.warmInstrument('organ-church', JSON.stringify([-12,0,7]))` and waiting for renders to settle, playing those 3 pitches yields `deferredPlays === 0`. Run → FAIL (`warmInstrument` undefined).

- [ ] **Step 2: Implement `window.warmInstrument`** in `audio.js`: parse pitches, for each call `ensureRenderReady(id, isMelodicInstrument(id) ? roundPitchForCache(p) : undefined).catch(()=>{})` (deduped by `pendingRenderEnsures`), non-blocking, off-thread via the worker.

- [ ] **Step 3: Browser assertion PASS.**

- [ ] **Step 4: Failing Go unit test** — `instrument_switch_warm_test.go`: a fake `audio.WarmInstrument` recorder; `SetInstrument("organ-church")` on a DrumView whose selected row has nodes at pitches {−12,0,7} must call `WarmInstrument("organ-church", [−12,0,7])`. Run → FAIL (no call site).

- [ ] **Step 5: Implement.** Add `WarmInstrument` (`voice_warm_wasm.go` + `!js` stub) and the call in `SetInstrument` after `dv.Rows[dv.selRow].Instrument = id`, gathering distinct pitches from the row's nodes (fall back to `[0]` when none/percussive). Run → PASS.

- [ ] **Step 6: Full-suite guard** — `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui ./internal/audio`.

- [ ] **Step 7: Commit.** `feat(audio): pre-warm voice cache on instrument switch (WASM)`.

---

### Task 5: Pre-warm on the desktop Go audio path (B1) — desktop mirror

**Why:** the desktop path has the same no-warm + on-audio-thread render class. Mirror the warm + never-block-audio-thread policy.

**Files:**
- Modify: `src/go/internal/audio/voice_cache.go` / `engine.go` — a `WarmVoices(id, pitches)` that renders into the voice cache on a background `async` pool (not the audio loop), and ensure the audio loop's cache-miss path prefers a nearest cached pitch over a synchronous render.
- Modify: `voice_warm_wasm.go`'s `!js` stub → real desktop impl.
- Test: `src/go/internal/audio/voice_warm_desktop_test.go` (Create).

**Interfaces:**
- Consumes: existing voice-cache render + `async.DefaultRegistry()`.
- Produces: `WarmInstrument` (desktop) populating the cache off the audio goroutine.

- [ ] **Step 1: Failing test** — warming organ-church for pitches {0,7} populates the voice cache (cache lookup hits) without rendering on the audio goroutine; use an `async` named pool with `t.Cleanup(Release)`. Run → FAIL.

- [ ] **Step 2: Implement** `WarmInstrument`/`WarmVoices` submitting renders to a named pool (recipe per CLAUDE.md async section); make the audio-loop miss path use nearest-cached-pitch when available instead of a blocking render. Run → PASS.

- [ ] **Step 3: Desktop record bench sanity** — extend `cmd/beatmo_record_bench.go` (or a targeted bench) with an instrument-switch churn; confirm no audio-thread render stall. Optional strict gate.

- [ ] **Step 4: Full audio suite** — `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/...` and `xvfb-run -a ../../.tools/go/bin/go test ./internal/audio/... -run Mixer`.

- [ ] **Step 5: Commit.** `feat(audio): pre-warm voice cache on instrument switch (desktop) + non-blocking miss path`.

---

### Task 6: Render-worker pool for burst parallelism (B3)

**Why:** a warm-set of N pitches (or several rows switching) still serializes on one worker. A 2-3 worker pool renders the burst in parallel, cutting warm/settle latency ~Nx. Do this last — after Tasks 2-4 it may be optional; the bench decides.

**Files:**
- Modify: `src/js/audio.js` (`renderVoiceInWorker` dispatch → round-robin over a pool; `synthWorkerPending` keyed per worker).
- Modify: `src/js/synth_render_worker.js` (unchanged logic; N instances).
- Test: extend `webaudio_instrument_switch_bench.browser.test.js` assertions / `instrument_switch_regression.browser.test.js`.

**Interfaces:**
- Consumes: existing `renderVoiceInWorker(renderFn, sr, frames, paramArr, postAmp)`.
- Produces: pool of size `POOL_N` (default `min(3, hardwareConcurrency-1)`), round-robin dispatch; metrics `workerQueuePeak` now per-pool max.

- [ ] **Step 1: Failing/again-measuring bench assertion.** Add a "warm-burst" scenario: `warmInstrument('organ-church', 8 pitches)` then measure time-to-all-cached. Assert wall-time ≤ (single-worker time / 1.8) as a coarse parallelism check (env-tunable, strict-gated). Run → FAIL (single worker).

- [ ] **Step 2: Implement the pool** — allocate `POOL_N` workers at module load; `renderVoiceInWorker` picks the least-loaded / round-robin worker; correlate `reqId` per worker. Keep the sync fallback policy from Task 1.

- [ ] **Step 3: Bench assertion PASS**; confirm `xplat_audio_compare` still bit-identical (pooling must not change output).

- [ ] **Step 4: Commit.** `perf(audio): render-worker pool for parallel voice warm-up`.

---

### Task 7: Consolidate gates + docs

**Files:**
- Modify: `Makefile` (add `bench-switch` target running the instrument-switch bench, mirroring `bench-render`).
- Modify: `CLAUDE.md` (bench table + gotcha note that instrument switch pre-warms; the never-sync-render-during-playback rule).
- Modify: `src/go/internal/deploy/manifest_drift_test.go` if new JS assets (none expected; pool reuses `synth_render_worker.js`).

- [ ] **Step 1:** Add `bench-switch: wasm` Makefile target. Run it; commit `bench-results/instrument_switch.json` baseline is regenerated.
- [ ] **Step 2:** Document the switch pre-warm + main-thread policy in CLAUDE.md gotchas.
- [ ] **Step 3:** Final full fast suite + the two browser regression tests + `xplat_audio_compare`.
- [ ] **Step 4: Commit.** `docs+ci: instrument-switch bench target + gotchas`.

---

## Self-Review

**Spec coverage:** B1 (Tasks 4,5), B2 (Tasks 2,3), B3 (Task 6), B4 (Task 1), desktop mirror (Task 5), perf tests (Task 1 regression + bench, Task 2/3 cost test, Task 7 gate), user's "pre-compute + keep cached until live edit" (Task 4, leaning on existing gen staleness). All covered.

**Placeholder scan:** DSP inner-loop code (Tasks 2-3) is described as an exact transformation of named lines rather than fabricated sample values — appropriate for a measurement-driven DSP change where the acceptance criterion is the cost ceiling + parity re-bake, not a hand-authored constant. Test anchors are concrete and machine-independent.

**Type consistency:** `WarmInstrument(id string, pitches []int)` (Go, Tasks 4/5), `window.warmInstrument(id, pitchesJSON)` (JS, Task 4), `window.__evictRenderCacheForTest(id)` (exists), `getRenderLatencyMetrics()` fields `mainThreadRenders`/`deferredPlays`/`neighborPlays`/`workerQueuePeak` (used consistently). Consistent across tasks.

**Sequencing rationale:** B4 first (UX win, no parity risk) → B2/B3-dsp (the dominant number, needs golden re-bake) → B1 warm (now cheap enough to matter) → B3 worker pool (optional scale) → desktop mirror alongside → gates last.
