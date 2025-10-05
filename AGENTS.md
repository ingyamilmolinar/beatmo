## Overview
- Tunkul is a grid-based beat sequencer that walks a directed graph to drive multi-row drum playback.
- Primary targets: native desktop (Ebiten) and WebAssembly (Emscripten + WebAudio).
- Codebase is Go 1.23; high-level systems are graph model, beat scheduler/engine, UI, audio backends, and JS bridges.

## Agent Quick Start
- Install deps once: `sudo make dependencies` (runs `scripts/setup-env.sh`: X11/ALSA/OpenGL, Node, Emscripten, Playwright).
- Run desktop quickly: `make run` or `make run-debug`.
- Fast Go tests (stubbed Ebiten): `cd src/go && go test -tags test -modfile=go.test.mod -timeout 20s ./...`.
- Full suite: `make test` (C lib + WASM + Go + browser tests). Real Ebiten: `make test-real`.
- Single browser harness: `GO=$(pwd)/.tools/go/bin/go node src/js/<name>.browser.test.js`.
- Build WASM only: `make wasm` (outputs `src/js/main.wasm`; do not commit).

## Toolchain & Setup
- Go lives under `.tools/go/bin/go` if the bundled toolchain has been bootstrapped; fall back to the system `go` otherwise (Makefile auto-detects via `GO ?=`).
- Install native + Node dependencies via `sudo make dependencies` (runs `scripts/setup-env.sh`).
- Optional: Emscripten is only required to regenerate `src/js/drums.single.js`. Without it, existing build artifacts are reused.
- WASM runtime uses Go's `wasm_exec.js`; browser harnesses rely on Node 18+.
- First builds may fetch Go modules; Makefile wraps with `($(GO) mod download || true)`.

## Daily Commands
- Desktop run (reuses cached C library and audio assets): `make run` or `make run-debug` (enables verbose logs at DEBUG).
- Headless demo: `xvfb-run make run RUN_ARGS=-demo`.
- Stubbed tests (fast path, headless Ebiten): `cd src/go && go test -tags test -modfile=go.test.mod -timeout 20s ./...`.
- Full Makefile test suite (`make test`): builds C lib + WASM, runs Go stub tests, audio unit tests, then Node browser tests (`audio`, `volume`, `slider_volume`, `import_export`, `timeline_center`, `popup_node`, `zoom_grid`, `perf`, `perf_e2e`). Requires Node and may download modules. Alias: `make tests`.
- Real Ebiten coverage (`make test-real`): runs UI/audio tests under `xvfb-run`, regenerates WASM, executes browser tests, and adds `bpm.browser.test.js`.
- Select JS harness: `GO=$(pwd)/.tools/go/bin/go node src/js/<name>.browser.test.js` for consistency with the bundled toolchain.
- WASM build: `make wasm` (outputs `src/js/main.wasm`; do not commit the artifact).
- Audio asset sync: `make sync-wav` copies `assets/wav` into `src/go/internal/assets/wav`.
- Camera/debug log run: `make run-cam-logs` (sets `DEBUG_GEOM=1` + node draw flags).
- Generated binaries: `src/go/ui.test` is produced by Go; treat it as a build artifact.

## Repository Layout
- `src/go/cmd/tunkul.go` – Entry point for desktop and WASM builds; wires up engine, UI, audio, debug toggles, optional `PPROF=1` server.
- `src/go/core/beat` – Minimal scheduler emitting ticks at BPM with pause/resume support.
- `src/go/core/model` – Graph data model; node/edge definitions, traversal helpers, beat row computation, loop detection; per-node params include logic and groove (delay/rush) settings.
- `src/go/core/engine` – Timing engine that wraps the scheduler, emits `Events`, coordinates audio and UI, and hosts the concurrency-safe `Predictor` for look‑ahead visibility/audibility. Toggle via `USE_ENGINE_PREDICTOR=0` to disable.
- `src/go/internal/ui` – Ebiten game loop plus subsystems: camera, grid rendering, drum view, predictor integration, import/export, groove controls, perf logging, JS bridges. Extensive headless tests live here under `-tags test`.
- `src/go/internal/audio` – Native backend (Oto) and WASM bridge, instrument registry, volume/pitch/duration playback, latency tests, WAV registration/export helpers, and stub implementations for tests.
- `src/go/internal/assets` – Embedded demo JSON (`default_demo.json`) and audio assets (synced via `make sync-wav`).
- `src/go/internal/ebitestub` – No-op Ebiten replacements to enable unit tests without graphics.
- `src/js` – WebAudio bridge (`audio.js`), generated DSP (`drums.single.js`), browser tests, perf harnesses, HTML harnesses.
- `src/c` – Miniaudio-backed C DSP for drums (`drums.c`, `miniaudio.c`), compiled to static lib and JS glue.
- `build` – Generated static libraries.

## Architecture Highlights
### Graph & Traversal (`core/model`)
- `Graph.Edges` map `[2]NodeID` to edge metadata; intermediate grid points become `NodeTypeInvisible` nodes so the UI draws orthogonal segments.
- `CalculateBeatRow` returns `[]BeatInfo` plus loop metadata (`isLoop`, `loopStartIndex`). Callers may further clamp to beat length with `SetBeatLength`.
- Utility functions (`wrapBeatIndexRow`, `getIntermediateGridPoints`, etc.) keep traversal indexes aligned with UI pulses.

### Timing Engine (`core/beat`, `core/engine`)
- Scheduler exposes `Tick`, `Progress`, and BPM setters. It drives a buffered engine loop that emits `Event`s on its goroutine.
- Engine mirrors BPM changes to audio backends, handles start/stop, and feeds the UI.
- `Predictor` maintains per-row visibility/audibility buffers to decouple playback from immediate graph mutations. UI updates row paths via `SetPaths`; predictor grows buffers lazily with `Ensure` and can run background horizon expansion.

### UI (`internal/ui`)
- `Game.New` builds the world: graph view, drum view, audio queue, predictor wiring, import/export, demo bootstrapping (`defaults_notjs.go`, `defaults_js.go`).
- `Game.Update` consumes engine events, advances pulses, handles input (mouse, wheel, keyboard, touch), and schedules audio through `audioCh` (drop‑oldest semantics preserved).
- Camera subsystem (`camera.go`) supports pixel snapping, zoom anchoring, drag pans, split panes, and exports debug instrumentation.
- Rendering path uses caches: grid tiles, edge layer cache, node sprite atlas (`nodesprite.go`), timeline background. Safe/deterministic variants exist for diagnostics (`render_blit.go`, env toggles below).
- Drum view maintains per-row state (instrument, origin beat, mute/solo, probability, groove) and synchronizes predictions (`drumview.go` + numerous regression tests). For deep dives we keep a Playwright trace helper (`scripts/capture_wasm_trace.mjs`) that builds the WASM bundle, plays the perf scenario, and writes `trace/perf_e2e_trace.json` so you can inspect a Chrome trace (top categories are printed in the console).
- RowsLayer caching keeps draw cost manageable. Avoid forcing a full rebuild unless the window bounds change; prefer shifting cached sprites and only repainting rows that actually mutated. When investigating WASM draw spikes, confirm `rowsLayerGen` advances only on real mutations and not every frame.
- JS bridge (`js_exports.go`) exposes hooks for browser tests to trigger playback, adjust BPM, probe UI layout, read timeline geometry, open node menus, export/import JSON, and inspect prediction buffers.

### Audio (`internal/audio`)
- Desktop path wraps Oto mixer and supports volume envelopes, instrument loading, pitch/duration resampling, and WAV registration (`sample_desktop.go`).
- WASM path proxies to `window.playSound`, `window.playSoundParams`, and `window.audioNow`.
- Headless tests use stubs and latency harnesses (`engine_test.go`, `latency_test.go`, `wav_test.go`).
- Audio engine is tightly coupled to scheduler ticks; ensure BPM changes travel through the buffered `bpmCh`.
- Embedded WAVs can auto‑register on startup; see `internal/assets/wav_embed.go` and `AutoLoadEmbeddedWAVs`.

### JavaScript & Browser Harnesses (`src/js`)
- Each `.browser.test.js` script spins up Playwright in headless mode; some (perf harnesses) only execute Go `Update` logic against WASM.
- Additional flows: live edit sync, probability edits, mute logic, zoom/grid behavior. Use `GO=$(pwd)/.tools/go/bin/go node <file>` to run under the bundled toolchain.
- WebAudio bridge (`audio.js`) provides `window.playSound*`, `window.audioNow()`, `window.resumeAudio()`, and a rolling debug buffer; inspect with `window.getAudioDebug()`.

## Testing & Diagnostics
- Primary fast loop: `cd src/go && go test -tags test -modfile=go.test.mod ./...`. This uses Ebiten stubs; demo auto‑start is suppressed in tests via `demo_stub.go`.
- Real Ebiten: `BPM_TIMING_TEST=1 xvfb-run -a go test ./...` exercises actual rendering and ensures audio timing stays in sync.
- Focused packages: `go test ./core/model`, `go test ./core/engine`, `go test ./internal/audio`.
- UI input helpers: `SetInputForTest` injects mouse/keyboard events; various `*_test.go` files demonstrate usage (camera drag/zoom, node interactions, drum edits, import/export flows).
- JS helpers from `js_exports.go`: `startPlay`, `incrementBPM`, `sliderRect`, `timelineRect`, `drumOffset`, `drumLength`, `timelineBeats`, `openNodeMenu`, `nodeMenuAction`, `exportJSON`, `importJSON`, etc.
- Perf counters: `src/go/internal/ui/perf_test.go`, `src/js/perf.browser.test.js`, `src/js/perf_e2e.browser.test.js`.
- Predictive engine coverage: `core/engine/predictor*_test.go` (Go) and `internal/ui/predictor_*_test.go` (UI consistency).
- Groove and timing: `internal/ui/groove_timing_test.go`, `internal/ui/bpm_change_no_jump_test.go`, `internal/ui/multi_prob_sched_vs_drumrow_test.go`.

## Debug & Rendering Toggles (env vars)
- `DEBUG_GEOM=1` – per‑frame geometry logs, camera state, and draw corner dumps.
- `DEBUG_DRAW_NODES=1` – enable verbose node draw logs; implied by `DEBUG_GEOM`.
- `RENDER_SAFE=1` – draw nodes and edges entirely in screen space; bypass caches, transforms, and sprite atlases (slow but definitive).
- `SCREEN_EDGES=1` – screen‑space edges while leaving node sprites active; helpful for cache vs. math mismatches.
- `NO_GRID_DRAW=1` – skip grid background to highlight geometry overlays.
- `NO_EDGE_CACHE=1` – disable edge cache; verifies baseline edges stay visible without cached blits.
- `NO_SPRITE_NODES=1` – render nodes through world‑to‑camera transforms instead of sprite atlas.
- `NO_PIXEL_SNAP=1` – bypass pixel snapping to expose sub‑pixel drift.
- Deprecated: `DRAW_TO_TOP` no longer applicable (top‑pane draws to background subimage).
- Combined deterministic path: `SCREEN_EDGES=1` + `RENDER_SAFE=1`.
- `[FRAME]` log prints active modes for quick confirmation.

## Performance & Instrumentation
- `PERF_LOG=1` – periodic logs (~2s) with FPS, Update/Draw avg/max, audio queue depth, Go→JS call timings, heap stats.
- `PPROF=1` – launches `net/http/pprof` server on `localhost:6060` (desktop build only).
- Engine ticker logs jitter as `PERF engine ticker: avg=.. max=.. count=..`.

### Performance Checklist
- Batching active (WASM): confirm `audio.PlayBatch` path is used (in perfStats, `audioDeq` ≈ half of `audioEnq`) and there is no per‑event console spam. Keep `TUNKUL_AUDIO_DEBUG` off unless debugging.
- Volume buses reused: in the browser, `__audioBusCount()` should be small (≈16 levels per instrument), not growing with the number of triggers.
- Warm caches: call `forceDraw()` after layout so `gridCacheInfo()` reports `tileReady=true` and `cacheReady=true`; let DrumView build row sprites and the `rowsLayer` once before measuring.
- Simple draw on web: enable `setSimpleDraw(true)` for perf runs; the grid remains visible while node/edge overlays are simplified.
- Text caching: use `DrawTextAt` for labels; avoid `ebitenutil.DebugPrintAt` in hot paths and ensure timeline info uses web throttling.
- Predictor horizon: ensure `ensurePredictions()` covers the visible window + lookahead to avoid recomputation thrash.
- Visual clamps: desktop glow radius is clamped (≤ grid pane height/8); web outline highlight uses cheap rings (no large fills).
- Harness load: reduce BPM/shapes/window for perf tests, and call `stopPlay()` when appropriate to end runs deterministically.
- Logs/profiling: use `PERF_LOG=1` to watch FPS, call latencies, and queue metrics; use `PPROF=1` for desktop CPU/mem profiling.
- Audio/HL sync: verify `sampleDurationSec(id)` returns >0 for instruments and that node/timeline highlight duration matches audio (pitch/duration scaling).
- Channels non‑blocking: keep `audioCh`/`bpmCh` buffered and use `sendLatest`/`sendLatestBPM`; avoid blocking sends in hot paths.
- Validate with harnesses: `perf.browser.test.js` (Update‑only ~55 FPS) and `perf_e2e.browser.test.js` (Draw ~20ms, Go→JS callAvg < 0.3ms) as baselines.

#### Quick Commands
- Run stubbed Go tests (fast):
  - `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -timeout 60s ./...`
- WASM Update‑only perf harness (no Draw):
  - `GO=$(pwd)/.tools/go/bin/go node src/js/perf.browser.test.js`
- WASM end‑to‑end perf harness (Update+Draw):
  - `GO=$(pwd)/.tools/go/bin/go node src/js/perf_e2e.browser.test.js`
- Grid + timeline highlight presence (simple draw):
  - `GO=$(pwd)/.tools/go/bin/go node src/js/grid_and_highlights.browser.test.js`
- Node grid highlight (simple draw):
  - `GO=$(pwd)/.tools/go/bin/go node src/js/node_grid_highlight.browser.test.js`
- Highlight duration matches audio (regular + simple):
  - `GO=$(pwd)/.tools/go/bin/go node src/js/highlight_duration.browser.test.js`
- Batch bridge check (WASM):
  - `GO=$(pwd)/.tools/go/bin/go node src/js/batch_audio.browser.test.js`
- WAV bus reuse check (WASM):
  - `GO=$(pwd)/.tools/go/bin/go node src/js/wav_bus.browser.test.js`
- Desktop run with perf logs:
  - `PERF_LOG=1 make run`
- Desktop run with pprof server:
  - `PPROF=1 make run` then visit `http://localhost:6060` (CPU/mem profiles)

### Hot Paths & Current Mitigations (Oct 2025)
- WASM Go→JS bridge:
  - Hot: many small cross‑boundary calls are expensive on the browser main thread.
  - Mitigation: `audio.PlayBatch` (Go) + `window.playSoundsBatch` (JS) reduces calls by batching sound events. Expect `audioDeq` ≈ 1/2 `audioEnq` in `perfStats()`.
  - JS: volume buses reuse `GainNode`s per instrument + quantized volume (16 levels) to avoid per‑voice node creation.

- Rendering (UI) in WASM:
  - Hot: per‑frame grid tiling, edge drawing, text rendering, and row sprites.
  - Mitigations:
    - Grid: tile cache + screen‑space cache with pan pad; reuse unless scale/subdiv changes.
    - Edges: screen‑space cache (`edgeCache`) with pad; reuse unless color/scale changes.
    - Rows: DrumView builds per‑row sprites and composes a `rowsLayer` (visible rows only). Highlights are drawn dynamically on top.
    - Text: cached text sprites via `DrawTextAt`; timeline info throttled on web (`timelineInfoThrottleMS`).
    - Simple draw (web): optional simplified path (no grid skip, but lighter node/edge overlays). Toggle via `setSimpleDraw(true)` in WASM.

- Node highlights (main grid) vs audio:
  - Highlights are time‑based and match the audible duration (sample seconds scaled by pitch/duration rate). Timeline highlight frames and node highlight expiry (wall clock) are derived from the same duration.
  - Web simple draw uses a high‑contrast, cheap outline overlay (two white rings + inner row‑colored ring) for visibility; desktop retains the original sprite + glow path.
  - Desktop glow is clamped to ≤ 1/8 of the grid‑pane height to avoid oversized overdraw at extreme zooms.

### Practical Guidelines
- Prefer batching when scheduling multiple audio events in WASM:
  - Go: collect `audio.BatchParam` and call `audio.PlayBatch` once per frame.
  - JS: `window.playSoundsBatch([{id, vol, pitch, dur, when}, ...])`.

- Avoid excessive logging in perf scenarios: console floods block timers and JS event loop. Keep `[AUDIOJS]` logging off unless debugging.
- Sequencer batching: `scheduleSound` accepts an optional `baseNow` so we only call `audio.Now()` once per frame per row. UI-triggered paths should pass `math.NaN()` to fall back to the old behaviour.

- Use caches:
  - Grid + Edge caches reuse across pans/small transforms with pad; invalidate on scale/subdiv/color sig changes.
- Row sprites + `rowsLayer` reduce per‑frame composition work in DrumView (only highlights and controls draw dynamically). Instrument `DrumView.Draw` when chasing regressions to confirm how many rows get repainted and whether `rowsLayerDirty` flips every frame.
  - Text via `DrawTextAt` to avoid repeated `ebitenutil.DebugPrintAt`.

- Simple draw (web) is enabled by default and intended for perf; it keeps the grid on, simplifies node/edge overlays, and preserves clarity of node/timeline highlights.

### Useful WASM Exports for Perf/Tests
- `setSimpleDraw(bool)`, `forceDraw()` – control simplified rendering and force a frame render for cache warmup.
- `gridCacheInfo()` – inspect tile/cache readiness.
- `nodeHighlightedAt(i,j)`, `nodeIdAt(i,j)` – probe grid highlight state per node.
- `sampleDurationSec(id)` – synth/WAV seconds; used for highlight/audio duration matching.
- Browser perf harnesses: `node src/js/perf.browser.test.js` (Update only) and `node src/js/perf_e2e.browser.test.js` (Update + Draw).
- When modifying render caches/node sprites, ensure edge cache reuse respects layout changes (pane resize, zoom) and that baseline primitives still draw when caches are bypassed (guarded by tests like `edge_cache_blit_test.go` and `edge_visibility_no_cache_test.go`).

## Assets, Import/Export & Defaults
- Default demo graph lives in `internal/assets/default_demo.json`. Desktop build embeds it via `demo_embed.go`; browser build fetches via JS bridge defaults.
- UI import/export handles JSON graphs and WAV paths; tests (`export_import_wav_path_test.go`, `import_*.go`) ensure cross‑platform paths and instrument fallbacks behave. Embedded WAVs auto‑register on startup.
- `tunkul-export.json` captures a reference export for regression testing.

## Generated / Cautions
- Built outputs should not be committed unless intentionally regenerated: `build/libdrums.a`, `src/js/main.wasm`, `src/js/play_ui.wasm`, `src/js/playtest.wasm`, `src/go/ui.test`.
- Browser tests require Playwright dependencies; if unavailable, run Go‑only suites and call out the omission in reviews.
- Optional pre‑commit hook (formats, stubbed tests, `make wasm`): `git config core.hooksPath .githooks`.
- Before sending a PR: run stubbed Go tests, WASM build (`make wasm`), and relevant Node harnesses. Prefer real Ebiten tests (`make test-real`) when graphics/audio changes are involved.

## Agent Tips
- Keep changes scoped to their subsystem; the engine handles scheduling and prediction—graph traversal and audio queuing belong to the UI.
- Preserve buffered channels (`bpmCh`, `audioCh`) and drop‑oldest semantics to avoid deadlocks or stalls.
- When investigating rendering glitches, use toggles progressively: strip layers (`NO_GRID_DRAW`, `NO_EDGE_CACHE`, `NO_SPRITE_NODES`), then force screen‑space (`SCREEN_EDGES`, `RENDER_SAFE`). Use fiducial logs to cross‑check math vs presentation.
- When adding tests under `internal/ui`, import `internal/ebitestub` stubs via `//go:build test` to stay headless.
- For WASM issues, scheduling uses `window.audioNow()`; keep conversions consistent between desktop and web paths. Resume contexts via `window.resumeAudio()` in tests as needed.
- UI caches depend on camera scale and split dimensions—invalidate caches when these change to prevent stale blits.
- Predictor is owned by the engine by default; if you touch prediction paths, consider `USE_ENGINE_PREDICTOR=0` to compare legacy UI prediction vs engine.

## Code Style & Conventions
- Subsystem boundaries
  - `core/engine`: scheduling, events, predictor ownership. Avoid UI/audio queuing here.
  - `internal/ui`: graph traversal to beat rows, UI/input, audio queueing, predictor integration.
  - `core/model`: pure data and helpers; keep playback rules as parameters (logic/groove) not side effects.
- Concurrency & channels
  - Preserve non‑blocking, drop‑oldest behavior on `audioCh` and `bpmCh`. Only the latest BPM matters—mirror `sendLatest`/`sendLatestBPM` patterns.
  - Keep buffered channels; do not introduce blocking sends in hot paths.
- Prediction & graph lifetime
  - Engine owns `Predictor` and listens for node changes. Do not replace `g.graph` pointers at runtime—mutate in place (as `Import` does) to keep references valid.
- Tests
  - Prefer headless tests under `-tags test` using `internal/ebitestub` and `go.test.mod`.
  - Use `SetInputForTest` for UI events; follow patterns in existing `*_test.go` files.
- Changes
  - Keep patches minimal and scoped. Maintain existing style. Update adjacent tests when changing behavior.
  - Never commit generated artifacts: `build/libdrums.a`, `src/js/*.wasm`, `src/go/ui.test`.

## Troubleshooting
- Browser audio not playing: browsers often start AudioContext suspended. Call `window.resumeAudio()` or trigger a pointer event before playback.
- Playwright/browser tests fail: ensure `sudo make dependencies` ran (installs Node modules and Chromium with `npx playwright install --with-deps chromium`).
- DSP module (`drums.single.js`) missing: Emscripten is optional; if not installed, existing artifact is reused. Regenerate with a working `emcc` or keep the committed single-file JS.
- WASM not loading in harness: serve from `src/js` (`make serve`) or run Node harnesses with `GO=$(pwd)/.tools/go/bin/go node src/js/<name>.browser.test.js` to use the bundled Go.
- Engine profiling: set `PPROF=1` for desktop and visit `http://localhost:6060`; enable `PERF_LOG=1` for periodic perf logs.

### Field Note: WASM perf stalls (Oct 2025)
- Symptom: perf harness stalled due to excessive console logging and many Go→JS calls.
- Root cause: per‑event logging and unbatched scheduling overwhelmed the main thread; timers starved.
- Fixes adopted:
  - Batched audio scheduling (`PlayBatch`/`playSoundsBatch`) to reduce crossings.
  - Volume buses to avoid per‑voice `GainNode` creation.
  - Lighter perf harness scenarios and explicit `stopPlay` when appropriate.
  - Forced frame renders (`forceDraw()`) in tests to warm caches deterministically.

### Possible Refactors for Debuggability
- The `seqScheduleTime` hot loop mixes predictor reads, gating logic, highlight bookkeeping, and audio queueing. Splitting the scheduling step (`scheduleSubdivision` / `applyMuteLogic`) would make it easier to probe from both WASM and desktop paths.
- `every_n_*` logic currently lives in both `internal/ui` and `core/engine/predictor`. A small shared helper (even a `logic` package) would prevent subtle divergences like the one that triggered this debug session.
- The browser harnesses repeat the same web-server boilerplate. Extracting a tiny `serveHarness` utility (spin up HTTP, expose `waitForExports`) would keep future tests consistent and shorten the per-test code.

## Docs Notes
- Some docs may reference `make test-mock`. Use `make test` for the full stubbed path, or run stubbed Go tests directly with `cd src/go && go test -tags test -modfile=go.test.mod -timeout 20s ./...`.
