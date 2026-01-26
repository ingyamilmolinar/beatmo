# Tunkul Beat Sequencer

Tunkul is a grid based sequencer written in Go using Ebiten. Nodes placed on the grid
form a graph that drives the drum machine in the bottom pane. The project can run
as a desktop app or compile to WebAssembly.

## Project Structure (Key Packages & Files)
- `src/go/cmd/tunkul.go` — App entrypoint; wires engine, UI, audio. Desktop enables optional `PPROF=1` profiling.
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
  - `src/c` — Miniaudio glue and DSP (`drums.c`, `miniaudio.c`). `make` builds static lib and Emscripten single‑file JS.
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

## Debugging
The UI and game layers now emit verbose logs describing user interactions and
internal state changes. Run the game from the repository root and check the
console output for messages prefixed with `[MODULE/... ]` (e.g., `[GAME]`, `[DRUMVIEW]`,
`[PERF/ENGINE]`).

- Go tests default to **silent logging**. To surface runtime logs, either pass
  `-test.v` or opt in via `TEST_LOG=1 make test` (or `TUNKUL_TEST_LOG=1`), with
  `TEST_LOG_LEVEL` / `TUNKUL_TEST_LOG_LEVEL=TRACE|DEBUG|INFO|ERROR` to set
  verbosity. `make test` and `make test-debug` stay quiet for passing tests by
  default.

### Headless / Playwright browser tests
Browser harnesses live in `src/js/*.browser.test.js`. Run them with the bundled
Go (absolute path) so the WASM build step succeeds regardless of cwd:

```sh
GO=$(pwd)/.tools/go/bin/go node src/js/mute_logic.browser.test.js
GO=$(pwd)/.tools/go/bin/go node src/js/logic_sync.browser.test.js
```

Notes:
- Playwright auto-starts a tiny HTTP server per test; ports are randomized.
- On WASM runs, parity panics are **disabled by default**; set `PARITY_WASM_FATAL=1`
  if you need fatal parity during debugging.

## Performance instrumentation and tests
- Enable periodic UI perf logs with `PERF_LOG=1` when running Tunkul. It prints every ~2 seconds:
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

## Git hooks
Enable the optional pre-commit hook so every commit formats the code, runs the tests with the stubbed Ebiten module and builds the wasm binary:

```sh
git config core.hooksPath .githooks
```
