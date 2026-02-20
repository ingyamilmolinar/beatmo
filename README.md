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

## Quick Start

| Task | Command |
|------|---------|
| Install deps | `sudo make dependencies` |
| Desktop run | `make run` |
| Debug run | `make run-debug` |
| Fast Go tests | `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./...` |
| Full pipeline | `make test` |
| Real Ebiten tests | `make test-real` |
| Single browser test | `GO=$(pwd)/.tools/go/bin/go node src/js/<name>.browser.test.js` |
| Cross-platform parity | `make test-parity` |
| Build WASM | `make wasm` |
| Headless stubbed tests | `make test-xvfb` |
| List browser tests | `make test-browser-list` |
| Sync WAV embeds | `make sync-wav` |
| Go coverage | `make coverage-go` |
| Browser coverage (Go+JS) | `make coverage-browser` |
| JS source coverage | `make coverage-js` |
| All coverage | `make coverage` |
| Coverage HTML reports | `make coverage-report` |
| Coverage graph (DOT/SVG) | `make coverage-graph` |

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

### Cross-platform parity tests

Run the Go golden-file generator and then the WASM comparison:

```sh
make test-parity
```

## Code Coverage

### Go coverage

```sh
make coverage-go              # Go unit test coverage (atomic mode, outputs coverage/go.out)
```

### Browser coverage (Go + JS via WASM)

```sh
make coverage-browser         # Build coverage-instrumented WASM, run all browser tests, collect Go coverage
```

### JS source coverage

```sh
make coverage-js              # V8 coverage report via c8 (requires a prior browser run with JS_COVERAGE=1)
```

### All coverage

```sh
make coverage                 # Run coverage-go + coverage-browser + coverage-js in sequence
```

### Reports & visualization

```sh
make coverage-report          # Generate HTML reports (coverage/go.html, coverage/browser.html)
make coverage-graph           # Generate per-function DOT/SVG/PNG coverage graph (requires graphviz for SVG/PNG)
```

Coverage data lands in `coverage/`. Set `COVERAGE=1` and/or `JS_COVERAGE=1` when
running browser tests manually to collect data outside of `make coverage-browser`.

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

## Performance instrumentation
- Enable periodic UI perf logs with `PERF_LOG=1`. Prints fps, Update/Draw timing, and audio latency every ~2 seconds.

## Environment Variables

### Runtime Configuration

| Variable | Purpose |
|----------|---------|
| `BEATMO_ASSETS=<path>` | Override default assets root directory |
| `BEATMO_CONFIG=<path>` | Load custom JSON circuit as initial demo |
| `BEATMO_DEMO_CONFIG=<path>` | Load custom JSON circuit (takes precedence over `BEATMO_CONFIG`) |
| `BEATMO_ROW_SNAPSHOTS=1` | Enable row snapshot mode |
| `AUDIO_SAMPLE_RATE=<int>` | Override default audio sample rate (default: 44100) |
| `SEQ_TICK_MS=<int>` | Override sequencer tick duration (default: 4ms on WASM) |
| `PERF_FAST_PATH=1` | Enable fast perf path (auto-enabled on WASM) |
| `PERF_BROWSER_UPDATE_MAX_MS=<int>` | Set max Update() duration for browser perf optimization |
| `BPM_TIMING_TEST=1` | Enable BPM timing test mode |

### Profiling

| Variable | Purpose |
|----------|---------|
| `PPROF=1` | Start pprof server on `localhost:6060` |
| `PERF_LOG=1` | Emit perf logs every ~2s |
| `PYROSCOPE_URL=<url>` | Pyroscope profiling backend URL |
| `PYROSCOPE_APP=<name>` | Pyroscope app identifier (default: `beatmo`) |

### Audio Debugging

| Variable | Purpose |
|----------|---------|
| `TEST_TONE=1` | Output pure 440Hz sine wave, bypassing all synth (tests Oto/driver) |
| `TEST_VOICE=1` | Replace C synth with simple 220Hz sine voices (tests voice/mixer path) |
| `TEST_RAW_VOICE=1` | Output first voice's raw samples, bypassing all processing |
| `TEST_RAW_VOICE_FULL=1` | With `TEST_RAW_VOICE`, use full amplitude (1.0×) instead of 0.2× |
| `TEST_RAW_VOICE_NOSCALE=1` | With `TEST_RAW_VOICE`, skip all multiplication (direct to int16) |
| `SINGLE_VOICE=1` | Limit mixer to one voice at a time (tests multi-voice accumulation) |
| `AUDIO_CLIP_DEBUG=1` | Log clipping events (samples exceeding ±1.0 before hard clamp) |
| `DEBUG_MIXER=1` | Log mixer workBuf min/max values and voice stats periodically |
| `BYPASS_CHANNEL_PROC=1` | Skip all channel processing (volume, EQ) — isolates mixer vs channel |
| `BYPASS_HEADROOM=1` | Skip per-voice headroom attenuation (0.25×) |
| `BYPASS_EQ=1` | Skip all EQ/processor processing |
| `BYPASS_NORMALIZE=1` | Skip peak normalization during export |
| `DEBUG_CHANNEL=1` | Log channel processing values |

### UI Rendering & Geometry

| Variable | Purpose |
|----------|---------|
| `RENDER_SAFE=1` | Force drawRect-based rendering instead of sprite cache (for pixel testing) |
| `SCREEN_EDGES=1` | Draw screen-space edge arrows at viewport edges |
| `NO_GRID_DRAW=1` | Skip grid pane drawing entirely |
| `NO_GRID_TILE_CACHE=1` | Disable grid tile caching (force redraw every frame) |
| `NO_PIXEL_SNAP=1` | Disable pixel snapping for geometry diagnostics |
| `NO_EDGE_CACHE=1` | Disable edge cache (force redraw every frame) |
| `NO_SPRITE_NODES=1` | Disable node sprite rendering |
| `DEBUG_GEOM=1` | Verbose geometry logs |
| `DEBUG_DRAW_NODES=1` | Enable detailed node draw logs |
| `BEATMO_DEBUG_INST=1` | Debug logging for instrument menu operations |

### Timeline & Parity

| Variable | Purpose |
|----------|---------|
| `TIMELINE_TRACE=1` | Trace timeline masking logic |
| `TIMELINE_TRACE_ROW=<int>` | Specify which row to trace (default: 0) |
| `DEBUG_HISTORY_SEED=1` | Debug timeline history seeding logic |
| `PARITY_FATAL=0\|false` | Disable parity panics |
| `PARITY_WATCH=log\|panic` | Parity mode (desktop) |
| `PARITY_WASM_FATAL=1\|true\|panic` | Enable parity panics on WASM |
| `PARITY_DUMP_STDERR=1` | Dump parity error messages to stderr |
| `PARITY_SCAN_STRIDE=<int>` | Parity scan stride (cells per scan pass) |

### Test Infrastructure (Go)

| Variable | Purpose |
|----------|---------|
| `BEATMO_TEST_LOG=1` | Enable verbose logging during tests (default silent) |
| `BEATMO_TEST_LOG_LEVEL=<level>` | Test log level: `TRACE`, `DEBUG`, `INFO`, `ERROR` |
| `FONTCACHE_SUBPROCESS=1` | Internal: font cache test subprocess marker |

### Test Infrastructure (Browser / JS)

| Variable | Purpose |
|----------|---------|
| `GO=<path>` | Path to Go binary for WASM builds |
| `BROWSER_JOBS=<int>` | Number of parallel browser test jobs (default: 4) |
| `BROWSER_TEST_PORT_BASE=<int>` | Base port for browser test servers (default: random 8500-9499) |
| `WASM_PREBUILT=1` | Skip WASM rebuild if binary exists |
| `TEST_LOG=1` | Enable page console logging in browser tests |
| `COVERAGE=1` | Enable Go coverage flushing during browser tests |
| `JS_COVERAGE=1` | Enable JS source coverage collection via Playwright |
| `FILTER=<pattern>` | Test filter pattern for `make test-browser-filter` |

### Browser Performance Thresholds

| Variable | Purpose |
|----------|---------|
| `PERF_E2E_FPS_MIN=<float>` | Min FPS for e2e perf test |
| `PERF_E2E_DRAW_MAX_MS=<float>` | Max avg draw time for e2e perf test |
| `PERF_E2E_UPDATE_MAX_MS=<float>` | Max avg update time for e2e perf test |
| `PERF_E2E_AUDIO_CALL_MAX_MS=<float>` | Max avg audio call duration for e2e perf test |
| `PERF_E2E_AUDIO_QLAT_MAX_MS=<float>` | Max audio queue latency for e2e perf test |
| `PERF_E2E_OVERDUE_MAX=<int>` | Max overdue audio events |
| `PERF_E2E_SMALL_LEAD_MAX=<int>` | Max audio events with <4ms lead time |
| `PERF_BROWSER_FPS_MIN=<float>` | Min FPS for browser perf test |
| `PERF_BROWSER_UPDATE_MAX_MS=<float>` | Max avg update time for perf test |
| `PERF_BROWSER_UPDATE_JITTER_MS=<float>` | Update timing jitter tolerance |
| `PAN_STRESS_FPS_MIN=<float>` | Min FPS for pan stress test |
| `PAN_STRESS_DRAW_MAX_MS=<float>` | Max avg draw time for pan stress test |
| `STRESS_COMPLEX_FPS_MIN=<float>` | Min FPS for complex stress test |
| `STRESS_COMPLEX_DRAW_MAX_MS=<float>` | Max avg draw time for complex stress test |
| `STRESS_COMPLEX_INTERVAL_MS=<int>` | Sample interval for stress test |
| `STRESS_COMPLEX_SAMPLES=<int>` | Number of stress test samples |
| `DRUM_EDIT_TIMEOUT_MS=<int>` | Timeout for drum edit changes (default: 350ms) |
| `DRUM_EDIT_CYCLE_MS=<int>` | Cycle wait for drum edits (default: 220ms) |

### Benchmarking

| Variable | Purpose |
|----------|---------|
| `BPM=<int>` | BPM for `make bench` |
| `SECS=<int>` | Benchmark duration in seconds |
| `BPM_LEVELS=<space-separated>` | BPM levels for `bench-desktop.sh` |
| `DURATION=<int>` | Duration per BPM level (default: 15s) |
| `RESULTS_DIR=<path>` | Results output directory (default: `bench-results`) |
| `BENCH_CIRCUIT=<name>` | Circuit for perf_e2e benchmark (`startup` or synthetic) |
| `BENCH_BPM=<int>` | BPM for perf_e2e benchmark (default: 200) |
| `BENCH_DURATION=<int>` | Duration for perf_e2e in ms |

### LLM Recording & Agent

| Variable | Purpose |
|----------|---------|
| `LLM_RECORD=1` | Enable browser recording for LLM tests |
| `LLM_RECORD_NAME=<string>` | Recording session name |
| `LLM_RECORD_INTERVAL=<int>` | Frame capture interval in ms (default: 500) |
| `ANTHROPIC_API_KEY=<key>` | Claude API key for agent/evaluate |
| `MODEL=<model-id>` | Claude model for agent (default: `claude-haiku-4-5-20251001`) |

### Deployment (GCP)

| Variable | Purpose |
|----------|---------|
| `GCP_PROJECT=<id>` | GCP project ID |
| `GCS_BUCKET=<name>` | GCS bucket name |
| `GCS_LOCATION=<region>` | GCS bucket region (default: `us-central1`) |
| `DOMAIN=<domain>` | Domain for static site (default: `beatmo.io`) |
| `DRY_RUN=1` | Preview deployment without changes |

### External Services

| Variable | Purpose |
|----------|---------|
| `BROWSERSTACK_USERNAME` | BrowserStack username for visual/device testing |
| `BROWSERSTACK_ACCESS_KEY` | BrowserStack access key |

## Contributing Notes
- Keep patches scoped to subsystems; avoid crossing UI/engine/audio boundaries without strong rationale.
- Never commit generated artifacts (e.g., `build/`, `src/js/main.wasm`, `src/js/drums.single.js` unless intentionally regenerated).
- Prefer adding tests in `src/go/internal/ui` with `-tags test` for fast headless runs; engine/model packages use regular tests.
- For web harness tests, run with `GO=$(pwd)/.tools/go/bin/go node src/js/<name>.browser.test.js`.

### Input system & touch on mobile
Touch input is transparently mapped to mouse via a frame-level override in `input.go` / `touch.go`, so all mouse-based handlers work on mobile without modification. See the header comment in `src/go/internal/ui/input.go` for the full platform behavior table.

### Audio mixer architecture
The desktop mixer uses a 3-phase block processing design (voices → per-instrument EQ → master EQ) to prevent biquad state corruption. See the header comment in `src/go/internal/audio/engine_stop.go` for the full pipeline description and debugging workflow.

### Audio debugging (desktop)
The mixer supports several env vars for isolating audio issues. See the [Audio Debugging](#audio-debugging) section above for the full variable table.

## Git hooks
Enable the optional pre-commit hook so every commit formats the code, runs the tests with the stubbed Ebiten module and builds the wasm binary:

```sh
git config core.hooksPath .githooks
```
