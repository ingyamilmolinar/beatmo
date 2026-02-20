# Beatmo Beat Sequencer

Beatmo is a graph-driven grid sequencer — a drum machine where nodes on a 2D grid
form a directed graph that drives beat patterns in real time. Each node represents a
potential drum hit; edges define traversal paths that determine when instruments fire.
Runs as a native desktop app (Go/Ebiten) or in the browser via WebAssembly + WebAudio.

## Features

- **Node types** — Regular, Invisible, Silent, and Mute nodes with per-node parameters (volume, pitch, duration, logic, groove)
- **Logic kinds** — Probability, skip-every-N, every-N-triggers, and conditional triggers (trigger-if-prev-skipped, trigger-if-prev-triggered)
- **42 instruments** — Percussion synthesis (snare, kick, hihat, tom, clap, cowbell, crash, ride, rimshot, shaker, sidestick), FM synth (bass, bell, lead, epiano, pluck), plus post-processed variants
- **10-band parametric EQ** — Per-instrument and master channel, with 4th-order Linkwitz-Riley crossovers
- **6 insert effects** — Distortion, delay, reverb, chorus, bitcrusher, filter — C block-processing with AudioWorklet on WASM
- **Send effects** — Delay and reverb send buses with per-instrument wet/dry control
- **Stereo panning** — Per-instrument equal-power pan law
- **Dynamic compressor** — Master channel limiter (threshold -6 dB, ratio 4:1)
- **Voice anti-pop** — Fade-in/fade-out envelopes on every voice to prevent clicks
- **Mobile/touch support** — Gesture detection, inertial scrolling, responsive layout, portrait and landscape
- **Connect mode** — Click-to-connect edge building for quick circuit wiring
- **Timeline** — Immutable playback history with JSON import/export (schema v1)

## Project Structure (Key Packages & Files)
- `src/go/cmd/beatmo.go` — App entrypoint; wires engine, UI, audio. Desktop enables optional `PPROF=1` profiling.
- Core runtime
  - `src/go/core/beat` — Simple BPM scheduler with jitter‑resistant SetBPM and `Progress()`.
  - `src/go/core/model` — Graph model (nodes/edges, traversal, loop detection, per‑node params: volume/pitch/duration/logic/groove).
  - `src/go/core/engine` — Engine loop, tick events, and concurrency‑safe Predictor (audible/visible/triggered look‑ahead with background precompute).
- UI (Ebiten)
  - `src/go/internal/ui` — Game loop + subsystems: camera, grid/edge caches, DrumView, import/export, perf logging, JS bridges.
    - `game_*.go` — Game runtime split by ownership (input, graph edits, update loop, draw paths, sequencer/audio scheduling, parity). Entry glue lives in `game.go`.
    - `drumview_*.go` — DrumView split by ownership (layout, menus, update/draw, caches, EQ/waveform). Base types live in `drumview.go`.
    - `import.go` / `export.go` — JSON schema v1 import/export (bpm, subdiv, instruments, nodes, params).
    - `js_exports_*.go` — Browser harness hooks (graph edits, perf stats, predictor/timeline queries, etc.).
- Audio backends
  - `src/go/internal/audio` — Desktop mixer (oto) + WASM WebAudio bridge, instrument channels + master volume, batch playback, WAV registration.
    - Desktop engine is split across `engine_*.go` (play scheduling, instrument registry, mixer).
    - The mixer uses a **3-phase block processing** design (`engine_stop.go`) to prevent shared biquad EQ state corruption: (1) render voices → per-instrument buffers, (2) per-instrument channel EQ → master buffer, (3) master EQ → output.
    - **Per-instrument insert effects** (distortion, delay, reverb, chorus, bitcrusher, filter) with C block-processing implementations and Go test fallbacks.
    - **Stereo panning** per instrument channel (equal-power pan law).
  - `src/c` — Miniaudio glue, DSP, and reusable audio modules.
    - `drums.c` — Percussion synthesis (snare, kick, hihat, tom, clap, cowbell, etc.)
    - `fmsynth.c` — 4-operator FM synth (bass, bell, lead, epiano, pluck) using wavetable oscillator
    - `effects.c` — Send effects (delay, reverb)
    - `insert_fx.c` — 6 insert effect types with block-based `_process(in, out, N)` API
    - `wavetable.c` — Band-limited wavetable oscillator with guard-point interpolation
    - `adsr.c` — ADSR envelope generator (linear/exponential curves)
    - `lfo.c` — LFO module (sine, triangle, saw, square, random sample-and-hold)
    - `pan.c` — Equal-power stereo panning
- Browser bridge
  - `src/js/audio.js` — WebAudio implementation with reusable volume buses, channel graph, `playSoundsBatch`, and sample loader.
  - `src/js/drums.single.js` — Emscripten single‑file DSP (generated; do not edit by hand).
- Assets
  - `src/go/internal/assets` — Default demo JSON and embedded WAVs (synced from `assets/wav` via `make sync-wav`).

## Environment setup
Install system packages and Node dependencies needed for the real Ebiten
library and Chromium tests:

```sh
sudo make dependencies
```

The script installs X11, ALSA and OpenGL libraries as well as runs `npm ci` and
`npx playwright install --with-deps chromium` so browser tests can run.

Use the bundled toolchain to avoid version drift:

```sh
export GO=$(pwd)/.tools/go/bin/go
```

## Testing

Unit tests can run in two modes. For the fast, stubbed Ebiten path use the
alternate module file:

```sh
cd src/go && go test -tags test -modfile=go.test.mod -timeout 20s ./...
```

Or run the full Makefile suite (C lib + WASM + Go + browser harnesses):

```sh
make test
```

If you have a working X11 setup (or run under `xvfb-run`) you can test against
the real Ebiten library:

```sh
make test-real
```

For convenience, there is also a headless wrapper that runs the stubbed suite
under `xvfb-run`:

```sh
make test-xvfb
```

### Audio mixer/EQ tests

Fast biquad corruption and channel isolation tests (stubbed Ebiten, no X11):

```sh
cd src/go && go test -tags test -modfile=go.test.mod ./internal/audio/... -run "Biquad|Channel|ProcessBlock"
```

Desktop mixer tests with real voices and EQ (requires X11 or `xvfb-run`):

```sh
cd src/go && xvfb-run -a go test ./internal/audio/... -run "Mixer"
```

Browser mixer EQ parity test:

```sh
GO=$(pwd)/.tools/go/bin/go node src/js/mixer_eq_parity.browser.test.js
```

### Cross-platform parity tests

Run the Go golden-file generator and then the WASM comparison:

```sh
make test-parity
```

This loads shared JSON fixtures (`src/go/internal/assets/parity_fixture_*.json`)
into both the native Go predictor and the WASM predictor, then verifies
bit-identical outputs for `VisibleAt`, `AudibleAt`, and `TriggeredAt` over a
64-beat horizon.

### E2E workflow tests

Multi-step scenario tests exercising the full user workflow (build circuit,
play, live-edit, export, import, verify):

```sh
# Browser (3 scenarios: lifecycle, BPM stress, row-add-during-playback)
GO=$(pwd)/.tools/go/bin/go node src/js/e2e_workflow.browser.test.js

# Go (same 3 scenarios with stubbed Ebiten)
cd src/go && go test -tags test -modfile=go.test.mod -run TestE2E ./internal/ui
```

### Real-input browser tests

Tests under `src/js/real_input_*.browser.test.js` exercise the UI via actual
Playwright mouse events (clicks, drags, shift-drags) instead of direct JS API
calls. They use helpers in `real_input_actions.js` which translate grid
coordinates to screen positions via the `gridToScreen(i, j)` JS export.

## Debugging
The UI and game layers now emit verbose logs describing user interactions and
internal state changes. Run the game from the repository root and check the
console output for messages prefixed with `[MODULE/... ]` (e.g., `[GAME]`, `[DRUMVIEW]`,
`[PERF/ENGINE]`).

- Go tests default to **silent logging**. To surface runtime logs, either pass
  `-test.v` or opt in via `TEST_LOG=1 make test` (or `BEATMO_TEST_LOG=1`), with
  `TEST_LOG_LEVEL` / `BEATMO_TEST_LOG_LEVEL=TRACE|DEBUG|INFO|ERROR` to set
  verbosity. `make test` and `make test-debug` stay quiet for passing tests by
  default.

### Headless / Playwright browser tests
Browser harnesses live in `src/js/*.browser.test.js` (79 files). Run them with the bundled
Go (absolute path) so the WASM build step succeeds regardless of cwd:

```sh
GO=$(pwd)/.tools/go/bin/go node src/js/mute_logic.browser.test.js
GO=$(pwd)/.tools/go/bin/go node src/js/logic_sync.browser.test.js
```

Run all browser tests or filter by pattern:

```sh
make test-browser                          # All browser tests
make test-browser-filter FILTER=parity     # Filter by name
make test-browser-list                     # List tests that would run
```

Notes:
- Playwright auto-starts a tiny HTTP server per test; ports are randomized.
- On WASM runs, parity panics are **disabled by default**; set `PARITY_WASM_FATAL=1`
  if you need fatal parity during debugging.

### Gotchas for writing browser E2E tests

- **JS export names**: Playback controls are `startPlay()` / `stopPlay()`, NOT
  `start()` / `stop()`. Optional chaining (`?.()`) silently returns `undefined`
  for missing functions, so typos cause silent no-ops.
- **Settling after `importJSON()`**: After importing a JSON fixture mid-test,
  always call `forceDraw()` and wait 300-500ms before clicking buttons or
  reading state. The layout needs multiple frames to recalculate button rects.
- **Prefer API calls for non-input features**: Use `startPlay()`/`stopPlay()`
  instead of `clickPlayBtn()`/`clickStopBtn()` when the test focus is not
  button interaction (e.g., BPM stress, row operations). Real mouse clicks are
  fragile after mid-test imports.
- **Canvas pixel reads**: Ebiten uses WebGL; direct `readPixels` after buffer
  swap doesn't work. Use `canvasPixelAt(page, x, y)` from
  `real_input_actions.js` which screenshots + decodes via offscreen canvas.
- **State leaks between scenarios**: If a test has multiple scenarios, ensure
  each stops playback properly. A silent no-op stop means the next
  `startPlay()` toggles playback off instead of on.
- **Test stub EQ is not real filtering**: Under `-tags test`, `NewEQProcessor`
  returns a `gainProcessor` (amplitude scaling only), not biquad filters. Tests
  that need actual frequency-dependent filtering must use `makeBiquad()`
  directly and pass it to `SetChannelProcessors()` — the `*biquad` type
  implements the `Processor` interface.
- **Mobile audio unlock**: `audio.js` uses multi-event unlock listeners
  (`touchstart`, `touchend`, `pointerdown`, `mousedown`, `keydown`) with a silent
  buffer play trick for iOS. Synth samples are pre-rendered to raw `Float32Array`
  without creating an AudioContext; `AudioBuffer` is created lazily on first play.
  For mobile emulation tests, use `cdpTap()` from `touch_cdp_helpers.js`.

## Performance instrumentation and tests
- Enable periodic UI perf logs with `PERF_LOG=1` when running Beatmo. It prints every ~2 seconds:
  fps, Update/Draw average/max (ms), audio queue latency and Go→JS audio call timings.
- Engine ticker jitter logs (debug level only) as: `[PERF/ENGINE] ticker avg=.. max=.. count=..`.
- Browser audio bridge emits debug events; inspect via `window.getAudioDebug()` in devtools.

## Contributing Notes
- Keep patches scoped to subsystems; avoid crossing UI/engine/audio boundaries without strong rationale.
- Never commit generated artifacts (e.g., `build/`, `src/js/main.wasm`, `src/js/drums.single.js` unless intentionally regenerated).
- Prefer adding tests in `src/go/internal/ui` with `-tags test` for fast headless runs; engine/model packages use regular tests.
- For web harness tests, run with `GO=$(pwd)/.tools/go/bin/go node src/js/<name>.browser.test.js`.

### Quick perf checks
- Desktop (stubbed Ebiten): `cd src/go && go test -tags test ./internal/ui -run PerfCounters`
- Browser (Update-only harness): `GO=$(pwd)/.tools/go/bin/go node src/js/perf.browser.test.js`
- Browser (full render using main.wasm): `GO=$(pwd)/.tools/go/bin/go node src/js/perf_e2e.browser.test.js`

### Debugging parity / scheduler issues
- Desktop parity fatal follows `PARITY_WATCH` / `PARITY_FATAL`. On WASM, fatals
  are off unless you set `PARITY_WASM_FATAL=1|true|panic`.
- A one-beat grace is applied when the sequencer just scheduled a beat
  (`idx == seqNextIdxs[row]-1`) to avoid false mismatches during redraw.
- Predictor is the single source of truth for DrumView; schedulers call
  `Predictor.Ensure(idx+1)` before parity bookkeeping.

### Concurrency gotcha
`lastTriggeredByRow` is mutex-protected. In tests, use:
`setLastTriggeredForTest`, `lastTriggeredForTest`, or
`lastTriggeredRowSnapshotForTest` on `Game` instead of touching the map
directly, otherwise sequencer goroutines can race and crash.

### Input system & touch on mobile
The UI reads input through function variables `cursorPosition` and
`isMouseButtonPressed` in `input.go` — **not** direct Ebiten calls. On WASM,
Ebiten does not map touch events to mouse, so these functions are wrapped with a
touch-to-mouse override (`touch.go`). When a single touch is active, all
existing mouse-based handlers (DrumView, Camera, Splitter, Editor) see touch
coordinates transparently.

Key points:
- `globalTouchState.Update()` must run **before** any `cursorPosition()` reads
  each frame. `updateTouchOverride()` runs immediately after to set the
  frame-level override state.
- Taps in the drum area use a 2-frame injection cycle (`injectTouchTap`)
  because the touch has already ended by the time the tap gesture fires.
- Multi-touch gestures (pinch zoom, two-finger pan) are handled directly as
  gesture events and do **not** go through the mouse override.
- `SetInputForTest` automatically disables the touch override so test mocks
  work without interference.
- See `CLAUDE.md` for the full input flow diagram and platform behavior table.

### Audio mixer architecture
The desktop mixer processes audio in 3 phases to prevent biquad EQ state
corruption. Biquad filters are stateful and expect continuous input —
interleaving unrelated voice signals through shared filters caused distortion.
The fix sums all voices per-instrument first, then applies EQ to the coherent
sum. See `CLAUDE.md` for the full pipeline diagram.

### Insert effects
Six per-instrument insert effects (distortion, delay, reverb, chorus,
bitcrusher, filter) are implemented in C (`src/c/insert_fx.c`) with
block-based processing. Desktop uses CGo bridges; tests use pure Go fallbacks
via build tags (`test || js`). On WASM, an AudioWorklet
(`src/js/insert_fx_worklet.js`) runs the C effects via Emscripten, falling back
to WebAudio node graphs if worklet init fails. The `BlockProcessor` interface
allows the channel mixer to call each effect once per block instead of per
sample, reducing CGo overhead.

### DSP modules
Reusable C building blocks shared across synths and effects:
- **Wavetable oscillator** (`wavetable.c`) — guard-point interpolation,
  band-limited waveform generation (sine, saw, square, triangle). Replaces
  `sin()` calls in `fmsynth.c`.
- **ADSR envelope** (`adsr.c`) — state-machine with linear/exponential curves.
- **LFO** (`lfo.c`) — 5 shapes via wavetable, plus random sample-and-hold.
- **Stereo panning** (`pan.c`) — equal-power pan law with block processing.

### Audio debugging (desktop)
The mixer has environment variables for isolating audio issues:

| Variable | Purpose |
|----------|---------|
| `TEST_TONE=1` | Output 440Hz sine, bypassing synth (test driver) |
| `TEST_VOICE=1` | Replace synth with simple sine voices |
| `BYPASS_CHANNEL_PROC=1` | Skip EQ/volume processing |
| `BYPASS_HEADROOM=1` | Skip per-voice 0.25× headroom |
| `AUDIO_CLIP_DEBUG=1` | Log clipping events |
| `DEBUG_MIXER=1` | Log mixer workBuf min/max values |
| `SINGLE_VOICE=1` | Limit to one voice at a time |

Typical workflow:
```bash
TEST_TONE=1 make run              # Verify driver works
BYPASS_CHANNEL_PROC=1 make run    # Isolate channel processing
AUDIO_CLIP_DEBUG=1 make run       # Check for clipping
```

See `CLAUDE.md` for full audio debugging documentation.

## Git hooks
Enable the optional pre-commit hook so every commit formats the code, runs the tests with the stubbed Ebiten module and builds the wasm binary:

```sh
git config core.hooksPath .githooks
```
