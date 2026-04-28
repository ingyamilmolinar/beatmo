# Tunkul

## Overview

**Tunkul** is a graph-driven grid sequencer—a drum machine where nodes on a 2D grid form a directed graph that drives beat patterns in real time. Each node represents a potential drum hit; edges define traversal paths that determine when instruments fire.

**Platforms**: Desktop (Ebiten/Go) and Browser (WASM + WebAudio)

**Toolchain**: Go 1.23, JavaScript/WebAudio, miniaudio C DSP, Emscripten

---

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
| Build WASM | `make wasm` |
| Capture UI screenshots | `make screenshot` (desktop + browser; output → `screenshots/`) |
| Capture all UI scenes | `make screenshots-all` (all scenes; `SCENES=name` to filter, `MOBILE=1` for mobile pass) |
| Regenerate design tokens | `make gen-design-tokens` (DESIGN.md → `internal/ui/design_*.gen.go`) |
| Install pre-commit hook | `make install-hooks` (rejects commits with stale `*.gen.go`) |

**Bundled Go**: `.tools/go/bin/go` — always use this to avoid version drift.

---

## Repository Structure

```
src/go/
  cmd/              Entry point (tunkul.go)
  core/
    model/          Graph, nodes, edges, traversal
    beat/           BPM scheduler
    engine/         Engine loop, predictor
  internal/
    ui/             Ebiten UI, DrumView (386 files)
    audio/          Desktop audio engine (52 files)
    timeline/       Commit ring, history service
    assets/         Embedded JSON/WAV
    ebitestub/      Stubbed Ebiten for tests
    gamestate/      Transport state machine
    graphruntime/   Graph traversal runtime
    log/            Logger
    utils/          Math helpers

src/js/
  audio.js          WebAudio bridge, render cache
  drums.single.js   Emscripten-compiled C synth
  *.browser.test.js Playwright tests (57 files)
  index.html        WASM harness

src/c/
  drums.c           C synth (snare, kick, hihat, tom, clap, cowbell)
  miniaudio.c/.h    Audio library

assets/             WAV samples (synced to Go embeds)
build/              Generated static libraries
```

### File Naming Conventions (Critical for Navigation)

The codebase uses **ownership-based file splitting**. Large modules are split by responsibility:

**Game module** (`game_*.go` — 61 files):
- `game_audio_*.go` — Audio loop, scheduling
- `game_draw_*.go` — Render dispatch, grid pane, helpers
- `game_graph_*.go` — Beat info, node/edge queries
- `game_input_*.go` — Drag, editor, menus
- `game_parity_*.go` — UI/audio consistency checks (25+ files)
- `game_sequencer_*.go` — Highlight, schedule, loop

**DrumView module** (`drumview_*.go` — 43 files):
- `drumview_cache_*.go` — Row sprite, layer, stripe caching
- `drumview_draw_*.go` — Controls, layout guides, waveform
- `drumview_instrument*.go` — Instrument selection menus
- `drumview_color_menu.go` — Color wheel picker
- `drumview_audio_eq.go` — EQ controls

**Predictor** (`predictor_*.go` — 13 files):
- `predictor.go` — Struct definition
- `predictor_api.go` — Query APIs (VisibleAt, TriggeredAt, AudibleAt)
- `predictor_compute.go` — Ensure, RebaseAt algorithm
- `predictor_logic.go` — evalAudible, shouldTriggerNode
- `predictor_paths.go` — SetPaths, buffer management
- `predictor_background.go` — Background lookahead worker

**Platform suffixes**:
- `*_wasm.go` — WASM-only implementations
- `*_desktop.go` — Desktop-only implementations
- `*_stub.go` — Platform stubs
- `*_test.go` — Tests (colocated with source)

---

## Core Packages

### `src/go/core/model/` — Graph Model

**Key Files**: `graph.go`, `node_types.go`, `graph_traversal.go`, `graph_params.go`

**Node Types**:
- `NodeTypeRegular` — Visible, audible, plays unless gated by logic
- `NodeTypeInvisible` — Not drawn, never plays (path guidance)
- `NodeTypeSilent` — Visible but silent (orthogonal routing)
- `NodeTypeMute` — Visible, gates audio for subsequent nodes

**Node Parameters** (`NodeParams`): Volume (0-1), Pitch, Duration, LogicKind (`probability`, `skip_every_n`, `every_n_triggers`, `trigger_if_prev_skipped`, `trigger_if_prev_triggered`), LogicN, LogicP, GrooveKind, GroovePct

**Traversal**: Edges use orthogonal L-shaped routing. Loop detection captures start index; loops repeat to fill beat length.

### `src/go/core/beat/` — BPM Scheduler

**Key File**: `sched.go`

Fixed-step BPM ticker with phase-preserving `SetBPM()` that maps elapsed fraction to new timeline (prevents audio jumps).

### `src/go/core/engine/` — Engine & Predictor

**Predictor** — The authoritative source of truth for what notes fire:
- `Ensure(horizon)` — Compute predictions up to absolute index
- `VisibleAt(row, abs)` — Should UI render this cell as ON?
- `TriggeredAt(row, abs)` — Did a mute node fire here?
- `AudibleAt(row, abs)` — Would this produce sound?
- `SetPaths(paths, isLoop, loopStart, nodes)` — Update after circuit edits

**Logic Evaluation**: Probability uses deterministic hash on `(row, idx, nodeID)`. Skip/every-N uses per-node trigger counts. Previous-trigger logic checks `lastFiredByRow`.

---

## Architecture

### Game Loop (Ebiten)

**Entry**: `src/go/cmd/tunkul.go` → `ebiten.RunGame(g)`

- `Update()` — Called at 60 TPS (fixed); handles input, sequencer, audio scheduling
- `Draw(screen)` — Called at vsync; renders grid pane, drum pane, overlays
- `Layout(w, h)` — Returns canvas dimensions

**Decoupling**: Update runs at exactly 60Hz regardless of Draw performance.

### Two-Pane Layout

- **Grid Pane (top)**: Node graph visualization, edges with arrows, camera pan/zoom
- **Drum Pane (bottom)**: Timeline editor with drum rows, controls (Play/Stop/BPM/Length/Subdiv), per-row instrument selector, mute/solo, volume, color picker

### Rendering Pipeline

```
Predictor buffers (source of truth)
    ↓
Timeline service (immutable history)
    ↓
preview.BuildRowWindow (pure function, single precedence table)
    ↓
DrumRow.Steps / CellTypes (render inputs)
    ↓
Row sprite cache (GPU texture)
    ↓
Rows layer composite
    ↓
Screen blit + highlights + UI controls
```

### Key Code Paths

**Audio scheduling**: `game_sequencer_schedule.go` → advances `seqNextIdxs[row]`, emits highlight events to `hlCh`

**UI applies highlights**: `game_sequencer_highlight.go` → `applySequencerHighlight()` advances `nextBeatIdxs[row]`, records timeline commits

**Slate rebuild**: `game_refresh_drum_row.go` → `refreshDrumRow()` ensures predictor horizon, builds Steps/CellTypes via `preview.BuildRowWindow`, marks caches dirty

**Graph changes**: `game_graph_update_beat_infos.go` → `updateBeatInfos()` recomputes paths, calls `predictor.SetPaths()`, seeds timeline history

---

## Key Concepts

### Predictor as Source of Truth

The engine-owned Predictor is the authoritative answer for "what should fire?" The UI reads via `VisibleAt`/`TriggeredAt`; it never mutates predictor state directly.

### Timeline Immutability

Once a beat is played, its rendered state is frozen:
- `CommitKindPlayback` / `CommitKindImport` for true past
- Past cells never change; edits only affect future
- `preview.BuildRowWindow` follows a single precedence table

### Concurrency Model

- **UI thread**: `Update()` + `Draw()`
- **Background sequencer**: Audio scheduling via goroutine
- **Audio thread** (desktop): Sample generation via Oto
- **WebAudio thread** (browser): Sample playback
- **Guard**: `seqMu` protects structural edits (row add/delete, path changes)

### Async / Resource-Constrained Goroutines (`internal/async/`)

`async.Pool` + `async.Registry` are the **single canonical API** for any background work that isn't a real-time loop. Spawn raw `go func()` only for the audio/sequencer/BPM real-time loops where pool dispatch latency is unacceptable — every other case goes through here.

**Building blocks:**
- `async.Pool` — bounded workers + bounded queue. `Submit` is non-blocking (returns `ErrBackpressure` on saturation, never blocks the caller); `SubmitBlocking` respects `context.Context`. Workers recover panics. `Close` drains the queue.
- `async.Registry` — process-wide named-pool registry sharing a global worker budget (default `max(6, NumCPU-4)`, reserving threads for Ebiten/oto). `DefaultRegistry()` is the singleton. Use `Get(name, opts)` for first-call lazy creation; subsequent calls reuse. `Release(name)` frees the budget for transient/test pools.
- `async.Scheduler` — heap-backed deadline dispatcher. One timer goroutine no matter how many entries are pending; due jobs are non-blocking-submitted to the underlying pool. Use instead of `go func() { time.Sleep(d); fn() }` whenever pending count could grow with workload.
- `async.Go(name, fn)` — fire-and-forget convenience wrapper; lazy-creates a 1-worker / queue-8 pool in `DefaultRegistry`. Use for one-shot, low-frequency tasks (file dialogs, init hand-offs).

**Standing pools (production):**
| Name | Owner | Workers / Queue |
|---|---|---|
| `recording.lifecycle` | `internal/audio/recording_lifecycle.go` | 1 / 8 — slow file-I/O finalize |
| `eventstream.persist` | `internal/eventstream/sink.go` | 1 / 8 — JSONL writer |
| `hooks.fanout` | `internal/hooks/bus.go` | 2 / 256 — pub/sub delivery |
| `audio.timers` | `internal/ui/game_new.go` | 1 / 64 — backs `Game.audioScheduler` for future-`when` playFn dispatch |
| `ui.dialog` | `internal/ui/select_json_async_desktop.go`, `drumview_ctor.go` | 1 / 8 (defaults) — desktop file pickers |
| `userprefs.persist` | `internal/userprefs/store_notjs.go` | 1 / 8 — favorites file writer (atomic-rename, coalescing latest-write-wins) |

**Recipe** (mirror `recording_lifecycle.go:23-36`): acquire a named pool from `DefaultRegistry().Get(...)` with a private-pool fallback if the registry budget is exhausted, hold the `*Pool` for the lifetime of the subsystem, `Submit` jobs that don't need to block.

**Test discipline:** every test that allocates a transient named pool must `t.Cleanup(func(){ async.DefaultRegistry().Release(name) })`. Packages with goroutine-spawning code use `goleak.VerifyTestMain(m, goleak.IgnoreCurrent())` in `main_test.go` (async, hooks, eventstream, audio, engine), pre-warming long-lived pools before the baseline so only test-introduced leaks trip the check.

**Predictor background:** `Predictor.StartBackground` is single-shot via an atomic CAS gate (`bgRunning`). Concurrent or repeat calls update `targetFn` but spawn at most one worker; `StopBackground` clears the gate so a fresh start works.

**WASM recording — off-thread (NOT a Go pool):** Browser recording uses an AudioWorkletProcessor (`src/js/recording_capture_worklet.js`) on the audio rendering thread + a Web Worker (`src/js/recording_encoder_worker.js`) on its own thread. Capture posts transferable Float32Array batches directly to the worker via a MessageChannel; the main thread is out of the data path. WAV encoding + zip bundling happen entirely in the worker. Hard caps: `BYTES_PER_CHANNEL_MAX=128 MB`, `RECORDING_DURATION_MAX_SEC=1800`, per-channel outbox depth=8 (drop on overflow, counter surfaced via `perfStats().recordingDrops`). The Go-WASM `audio.StopRecording` returns immediately with metadata + an "expected" zip filename; a background goroutine awaits the worker's Blob and triggers the download via `EventRecordStop`. End-to-end test: `src/js/recording_lifecycle.browser.test.js` (formerly `recording.browser.test.js`).

### Parity Verification

Built-in invariant checks ensure UI slate matches predictor state:
- `parityCheck` — Steps vs predictor mismatch
- `parityScan` — Highlight events vs scheduler decisions
- `parityAudio` — Past audio events vs expected triggers
- Grace periods prevent false positives during tight loops

### Event Notification (Two-Tier)

Two distinct mechanisms for "X changed, notify Y":

1. **Async `hooks.Bus`** (`internal/hooks/`) — fire-and-forget pub/sub for *external* observers (telemetry, eventstream, future cross-package consumers). 4-worker pool, panics swallowed, never blocks publisher. Subscribers run **off the UI goroutine** with no ordering guarantees. Helpers live in `internal/ui/event_helpers.go` (`emitNodeAdded`, `emitRowInstrumentChange`, etc.). Use this for anything an external package or test sink might want to observe.

2. **Sync direct calls** for intra-component handoffs that must be visible before the next `Draw()`. Pattern: a private method like `dv.onRowInstrumentChanged(row, oldID, newID)` invoked at every mutation site. Example: `SetInstrument` and the rename closures in `js_exports_graph_ui.go` both call `onRowInstrumentChanged` so the EQ panel's `eqActiveChannel` follows the row when the row's instrument id changes (see `drumview_audio_eq.go`).

**Rules:**
- `hooks.Bus` is **not** a substitute for sync delivery — handlers run on a worker pool, not the publisher's goroutine. Never use it for UI state that must be coherent before the next frame.
- Intra-struct notifications (publisher and consumer in the same struct) use direct method calls, not subscriber slices. A `[]func` subscriber API is justified only across real component boundaries with multiple consumers.
- Sync handlers are reachable from `Game.Update` under `seqMu`; they must not call back into Game paths that re-acquire `seqMu` (deadlock).
- When a state field is mutated at multiple sites (e.g., `Rows[i].Instrument` is changed by `SetInstrument` *and* by the rename flow), every site must call the sync notifier — otherwise some paths leave dependents stale.

---

## Testing

### Go Tests (240+ files)

**Fast Path** (stubbed Ebiten, no X11):
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./...
```

**Real Ebiten** (needs X11):
```bash
cd src/go && xvfb-run -a go test ./...
```

**Key Categories**:
- **Predictor/audio**: `drum_sync_invariant_test.go`, `mute_node_audio_test.go`, `probability_prediction_consistency_test.go`
- **DrumView caching**: `drum_rows_layer_cache_test.go`, `drum_row_cache_test.go`
- **Import/export**: `import_validation_test.go`, `import_button_flow_test.go`
- **Parity**: `parity_*_test.go` (25+ files)
- **Performance**: `game_draw_throttle_test.go`, `perf_test.go`

### Playwright Browser Tests

```bash
GO=$(pwd)/.tools/go/bin/go node src/js/<name>.browser.test.js
```

**Key Suites** (after the JS test suite reduction — see § JS Test Charter below):
- **Bridge plumbing**: `wasm_bridge_smoke.browser.test.js` (the canonical export catalogue), `wasm_bridge_input_sanity.browser.test.js`
- **Cross-platform parity**: `xplat_parity.browser.test.js`, `xplat_audio_compare.browser.test.js`, `xplat_output_capture.browser.test.js`
- **WebAudio**: `webaudio_smoke.browser.test.js`, `webaudio_perf.browser.test.js`, `webaudio_perf_e2e.browser.test.js`, `webaudio_pan_stress.browser.test.js`, `webaudio_bpm_stress.browser.test.js`, `webaudio_drums_consistency.browser.test.js`, `webaudio_mixer_eq_parity.browser.test.js`
- **Worklets / workers**: `worklet_insert_effects.browser.test.js`, `recording_lifecycle.browser.test.js`, `recording_capture.browser.test.js`
- **Real input / e2e**: `e2e_workflow.browser.test.js`, `e2e_real_input_circuit.browser.test.js`, `e2e_real_input_edge.browser.test.js`, `e2e_real_input_live_edit.browser.test.js`, `e2e_node_click.browser.test.js`, `e2e_drum_row_controls.browser.test.js`, `e2e_transport.browser.test.js`, `e2e_timeline_seek.browser.test.js`
- **Touch / mobile**: `touch_gestures.browser.test.js`, `touch_integration.browser.test.js`, `touch_pinch_no_node.browser.test.js`, `touch_device_matrix.browser.test.js`, `touch_dropdown_scroll.browser.test.js`, `mobile_audio.browser.test.js`, `mobile_audio_unlock.browser.test.js`, `mobile_speaker_routing.browser.test.js`, `mobile_file_picker.browser.test.js`, `mobile_native_input.browser.test.js`, `mobile_text_input.browser.test.js`
- **Visual regression**: `visual_regression.browser.test.js`, `visual_device_parity.browser.test.js`, `visual_mobile_parity.browser.test.js`
- **Browser platform**: `browser_media_session.browser.test.js`

**Test logging**: Default silent. Enable with `TEST_LOG=1`, set `TEST_LOG_LEVEL=TRACE|DEBUG|INFO|ERROR`.

### JS Test Charter — what the JS suite owns

**Logic correctness lives in Go.** The JS browser suite owns *only* the Go↔WebAudio/Worklet/Worker/real-input boundaries that Go cannot reach. New tests must justify themselves against this charter or they belong in `internal/ui/` instead.

The JS suite is responsible for, and ONLY for:

1. **WebAudio behavior** — `AudioContext` sample rate, `decodeAudioData`, gain/pan node graphs, hard limiter clipping, anti-pop fade timing, scheduling lead/lag at `AudioContext.currentTime`.
2. **AudioWorklet message protocol** — `recording_capture_worklet.js` and `insert_fx_worklet.js` configure/setParam/data-batch flows; backpressure & port transfer.
3. **Web Worker boundary** — `recording_encoder_worker.js` (WAV/FLAC/ZIP encoding off-thread; Blob/zip download e2e).
4. **WASM↔JS bridge plumbing** — every Go-registered JS export is callable, returns the expected shape, doesn't throw. Tracked in **one** place: `wasm_bridge_smoke.browser.test.js` (the canonical export catalogue).
5. **Cross-platform parity** — WASM-compiled Go produces bit-identical output to native Go for the same inputs (predictor goldens, audio rendering goldens). `xplat_parity.browser.test.js` and `xplat_audio_compare.browser.test.js`.
6. **Real input dispatch through the canvas** — touch, pointer, multi-touch via Playwright/CDP; mobile audio unlock gestures; iOS speaker routing; mobile viewport.
7. **Visual regression** — Playwright canvas screenshot diffing where pixel-level browser rendering matters.
8. **Browser-only platform features** — Media Session API, file picker dialogs, mobile orientation events, browser storage.

**What the JS suite must NOT do (use Go instead):**
- Re-test predictor/timeline/graph/scheduler logic — Go's `core/engine`, `core/model`, `internal/timeline`, `internal/ui` cover it directly with deterministic stubs.
- Re-test UI state machines (dropdowns, sidebar, scroll, orientation, layout) — Go's ebitenstub harness covers them; see `dropdown_*_test.go`, `widget_layout_test.go`, `popup_close_test.go`, `responsive_layout_test.go`.
- Re-test DSP correctness (biquad, compressor, insert effects, EQ) — Go's `internal/audio` (88% native coverage) owns it.
- Re-test JSON import/export round-trips — Go's `import_export_logic_test.go` and friends own it.

**When adding a new JS export in Go:** add an entry to `src/js/wasm_bridge_smoke.browser.test.js`'s catalogue (existence at minimum, callable if safe args exist). Do NOT create a new dedicated `*.browser.test.js` file just to verify the export got registered.

**When reducing a JS test:** the recipe is:
1. Read both the JS test and the candidate Go test that covers the same scenario. Verify the Go test is strictly larger (or equal) in coverage.
2. If Go is partial, write the missing Go test FIRST (in `internal/ui/` or `internal/audio/`).
3. Either delete the JS test outright (preferred when Go fully covers it), or replace its body with a 5–10-line bridge-plumbing assertion using `bridge_smoke_helpers.js` (`assertExportExists`, `callExport`, `assertReturnShape`). Cite the Go test(s) in a top-of-file `COVERED-BY-GO:` block.
4. The bulk-reduction runner `reduced_bridge_smoke.js` was retired after Batch 12; if you need the same shape of stub, write it inline against `bridge_smoke_helpers.js` directly.

**Reference helpers:** `src/js/bridge_smoke_helpers.js` is the canonical source of bridge-plumbing assertions, used by `wasm_bridge_smoke.browser.test.js` (the canonical export catalogue). For the historical reduction pattern, see git history before the Batch 12 cleanup.

---

## Debugging

### Log levels

The `internal/log` package enforces this contract — drift is caught by `internal/log/forbidden_at_info_test.go` (regression guard) and `internal/eventlogger/coverage_test.go` (every hooks.Kind has a formatter).

| Level | Rule |
|-------|------|
| **INFO** | One line per **user action** or **major component lifecycle event**. Never per-frame, never "ignored X because Y", never internal-mechanism breakdowns. Most user-action narrative is emitted by `internal/eventlogger/`, which subscribes to `hooks.Bus` and renders one human-readable line per published event. |
| **WARN** | Degraded-but-functional. Slow paths, parity mismatches when non-fatal, dropped frames, fallbacks taken. Has its own `LevelWarn` threshold — silenceable independent of INFO. |
| **ERROR** | Failure: an operation could not complete. |
| **DEBUG** | Mechanism detail useful for diagnosing a specific subsystem. May fire per event or per pool job, but never per frame. |
| **TRACE** | Firehose. Per-frame, per-cell, per-tick. |

**Output format**: `15:04:05.000 LEVEL [tag] message`. The tag is the first bracketed token of the format string (extracted automatically); when no tag is present the bracketed section is omitted.

### Environment Variables

| Variable | Purpose |
|----------|---------|
| `PPROF=1` | Start pprof server on `localhost:6060` |
| `PERF_LOG=1` | Emit perf logs every ~2s |
| `TIMELINE_TRACE=1` | Trace timeline masking logic |
| `PARITY_FATAL=0\|false` | Disable parity panics |
| `PARITY_WATCH=log\|panic` | Parity mode (desktop) |
| `PARITY_WASM_FATAL=1\|true\|panic` | Enable parity panics on WASM |
| `TEST_LOG=1` | Enable test logging |
| `BEATMO_TEST_LOG=1` | Enable in-process test logger output (alias for the package's runtime gate) |
| `BEATMO_TEST_LOG_LEVEL=TRACE\|DEBUG\|INFO\|WARN\|ERROR\|NONE` | Override log level when test logging is enabled |
| `BEATMO_INFO_LOG=off` | Disable the hooks.Bus → INFO narrative consumer (default: on) |
| `BEATMO_EVENT_LOG=<path>` | Write the full hooks.Bus event stream to a JSONL file (separate from INFO narrative) |
| `BEATMO_EVENT_LOG_VERBOSE=1` | Include verbose kinds (camera pan/zoom, drag-progress) in BOTH the INFO narrative and the JSONL stream |
| `DEBUG_GEOM=1` | Verbose geometry logs |
| `SCOPE_EXPORT=1` | Enable scope export flight recorder (JSONL) |
| `SCOPE_EXPORT_PATH=<path>` | Output file (default `scope_export.jsonl`) |
| `SCOPE_EXPORT_INTERVAL=<secs>` | Snapshot interval (default `2`) |

### JS Exports (WASM)

**State Inspection**:
- `dumpRowState(row)` — Row timeline + predictor state
- `dumpTimelineSegments()` — Full timeline snapshot
- `visibleAt(row, abs)`, `triggeredAt(row, abs)` — Predictor queries
- `rowWindow(row)` — Current row window

**Graph Manipulation**:
- `addNode(i, j, type)`, `deleteNodeGrid(i, j)`
- `addEdgeGrid(i1, j1, i2, j2)`
- `setNodeLogicGrid(i, j, kind, n, p)`

**Playback**: `startPlay()`, `stop()`

**Performance**: `resetPerfStats()`, `perfStats()`, `resetAudioScheduleMetrics()`, `getAudioScheduleMetrics()`

---

## Critical Gotchas

### Concurrency & Deadlocks

**seqMu deadlock prevention**: `Game.Update()` holds `seqMu` while calling `drum.Update()`. Any callback from DrumView that calls back into Game code requiring `seqMu` will deadlock. Go's `sync.Mutex` is NOT reentrant.

**Solution**: `onImport` queues data to `pendingImportData`; actual import runs after `seqMu.Unlock()`.

**lastTriggeredByRow is mutex-protected**: Never read/write the map directly in tests. Use `setLastTriggeredForTest`, `lastTriggeredForTest`, or `lastTriggeredRowSnapshotForTest` on `Game`.

### Predictor Behavior

**Mute gate**: Engine predictor only gates the mute subdivision (`idx+hold+1`), so audio resumes immediately on the following beat. UI/audio mute tests assume this behavior.

### WASM/Playwright

**Absolute GO path required**: When running Node harnesses that call `go build`, pass an absolute path:
```bash
GO=/home/ymolinar/Repos/tunkul/.tools/go/bin/go node src/js/<test>.browser.test.js
```
Relative `.tools/...` can fail because cwd becomes `src/go`.

### Parity Defaults

- **WASM**: Parity stays log-only unless `PARITY_WASM_FATAL=1|true|panic`
- **Desktop**: Honors `PARITY_WATCH`/`PARITY_FATAL` normally
- **During imports**: `g.importing` disables parity scans and clears buffers

### WebAudio Sample Rate

WebAudio contexts often default to 48 kHz. Synth buffers render at `AudioContext.sampleRate`. If you change render lengths/amps, preserve the dynamic SR or you'll get pitch/tempo drift in browsers.

### Runtime Profile (browser/desktop divergence)

`RuntimeProfile` (`internal/ui/runtime_profile.go`) is the single source of truth for every value that differs between browser/WASM and native desktop. Sibling to `LayoutProfile` (which owns mobile↔desktop screen-class divergence). Do **not** add new `runtime.GOOS == "js"` or `runtime.GOARCH == "wasm"` checks in `internal/ui/` — extend `RuntimeProfile` and read from `RuntimeProf()` instead.

**Two builders, one chooser:**
- `browserRuntimeProfile()` and `desktopRuntimeProfile()` (no build tags) hold the values.
- `runtime_profile_js.go` (`//go:build js`) selects the browser builder; `runtime_profile_notjs.go` (`//go:build !js`) selects the desktop builder. The build-tag pair has no test-vs-non-test split, so it compiles cleanly under every `GOOS=js -tags=test` combination.

**Test override:**
```go
restore := SetRuntimeProfileForTest(browserRuntimeProfile())
defer restore()
```
or directly mutate `RuntimeProf().Field` for individual flips.

**Bench-time override:**
- Browser: set `window.__beatmoProfileOverride = { drawMinIntervalMS: 0, ... }` before WASM init. The Playwright harness in `webaudio_bench_startup.browser.test.js` reads `BEATMO_PROFILE_OVERRIDE` (JSON string) and injects it via `page.addInitScript`.
- Desktop: set `BEATMO_*` env vars (e.g. `BEATMO_AUDIO_LOOKAHEAD_SEC`, `BEATMO_DISABLE_NODE_GLOW`).

**Each surviving divergent field must carry a one-line bench citation** in `runtime_profile.go` justifying why the browser and desktop values differ. Knobs without a bench number should be unified across both profiles. See `bench-results/runtime_profile_sweep.md` for the methodology.

---

## Import/Export Format

**JSON Schema v1** (`tunkul.json`):
```json
{
  "version": 1,
  "subdiv": 8,
  "bpm": 120,
  "instruments": [{"name": "Kick", "id": "kick", "kind": "builtin", "volume": 1.0, "origin": 0, "color": "#C87850FF"}],
  "nodes": [{"id": 0, "i": 0, "j": 0, "type": "regular", "inputs": [], "outputs": [], "volume": 1.0, "pitch": 0, "duration": 1.0, "logic_kind": "", "logic_n": 0, "logic_p": 0}],
  "eq": {"gains_db": [0,0,0,0,0,0,0,0,0,0], "bands_hz": [[20,100],[100,200]]}
}
```

---

## Building Circuits for Beats

### Timing Fundamentals

Each node in a circuit represents **one subdivision**. The `subdiv` setting defines how many subdivisions occur per beat:

| subdiv | Subdivisions per beat | Node duration at 90 BPM |
|--------|----------------------|-------------------------|
| 4 | 4 (quarter notes) | 167ms |
| 8 | 8 (eighth notes) | 83ms |
| 16 | 16 (sixteenth notes) | 42ms |
| 32 | 32 (thirty-second notes) | 21ms |

**Key formula**: `loop_period = num_nodes × (60 / BPM / subdiv)` seconds

### Loop Length Guidelines

For a standard rock/pop beat at `subdiv=16`:

| Rhythmic goal | Loop size needed | Hit spacing |
|---------------|------------------|-------------|
| Hit every beat (quarter notes) | 16 nodes | Every 16 subdivisions |
| Hit every 2 beats (half notes) | 32 nodes | Every 32 subdivisions |
| Hit every half beat (8th notes) | 8 nodes | Every 8 subdivisions |
| Hit every quarter beat (16th notes) | 4 nodes | Every 4 subdivisions |

**Common mistake**: Using small loops (4-8 nodes) when you want slower rhythms. A 4-node loop at `subdiv=16` cycles 4 times per beat—way too fast for most patterns!

### Rectangular Loop Shapes

Use simple rectangular shapes for predictable timing. Recommended patterns:

**8×2 Rectangle (16 nodes)** — Good for quarter-note patterns:
```
0 → 1 → 2 → 3 → 4 → 5 → 6 → 7
↑                           ↓
15← 14← 13← 12← 11← 10← 9 ← 8
```

**4×2 Rectangle (8 nodes)** — Good for 8th-note patterns:
```
0 → 1 → 2 → 3
↑           ↓
7 ← 6 ← 5 ← 4
```

### Standard Drum Pattern Recipe

For a basic rock beat at 90 BPM with `subdiv=16`:

| Instrument | Loop size | Hit positions | Result |
|------------|-----------|---------------|--------|
| **Kick** | 16 nodes (8×2) | pos 0 | Every beat (four-on-the-floor) |
| **Snare** | 16 nodes (8×2) | pos 8 | Backbeat (offset half-loop from kick) |
| **Hi-hat** | 8 nodes (4×2) | pos 0, 4 | 8th notes |
| **Tom** | 16 nodes (8×2) | pos 0 | Accent with kick |
| **Clap** | 16 nodes (8×2) | pos 8 | Layer with snare |

### Node Types for Rhythm

- **Regular nodes**: Produce sound when traversed
- **Silent nodes**: Advance timing without sound (use for spacing)
- **Mute nodes**: Gate subsequent audio (for rhythmic breaks)

### Tips

1. **Start with kick and snare**: Get the backbeat working first, then layer other instruments
2. **Use silent nodes for spacing**: A 16-node loop with 1 regular + 15 silent = one hit per beat
3. **Offset snare from kick**: If kick hits at position 0, place snare hit at position `loop_size / 2`
4. **Hi-hat loops can be shorter**: They cycle faster, which is usually what you want
5. **Test at target BPM**: A pattern that sounds good at 60 BPM may be too slow at 120 BPM

### Example: Adjusting for BPM

If your beat sounds too fast at 90 BPM:
- Double your loop sizes (8 nodes → 16 nodes)
- Or halve the `subdiv` setting (16 → 8)

If your beat sounds too slow:
- Halve your loop sizes
- Or double the `subdiv` setting

---

## Design Tokens

`DESIGN.md` is the design-system document; its YAML front matter is normative. The Go runtime mirrors those values in `src/go/internal/ui/theme.go`, `theme_tokens.go`, and `touch_sizes.go`.

Two test guards keep the two sides aligned. Both run in the fast `-tags test` path and gate every PR that touches `internal/ui/`:

| Guard | What it checks |
|---|---|
| `design_md_drift_test.go` (`TestDesignMDDrift`) | Parses DESIGN.md YAML and asserts every named color (26), spacing (13), rounded (4), and alpha (5) token equals its Go constant. RGB-only comparison for tokens whose Go form carries a non-255 alpha. |
| `token_discipline_test.go` (`TestTokenDiscipline`) | Per-file budget for inline `color.RGBA{}` / `color.NRGBA{}` literals in `internal/ui/`. Files above budget fail; **files below budget also fail** — the budget must tighten when literals are removed (ratchet). New literals in non-budgeted files fail. Infrastructure files (`theme.go`, `theme_tokens.go`, `drawing.go`, `icons.go`) are exempt. |
| `design_md_lint_test.go` (`TestDesignMDLintSnapshot`) | Runs `npx @google/design.md@0.1.1 lint DESIGN.md` and pins the warning set to `testdata/design_md_lint.expected.json`. New or removed warnings fail. Skips when `npx` is unavailable. |

### Token-change checklist

- **Adding a color / spacing / rounded / alpha token:** update DESIGN.md YAML, the corresponding Go constant in `theme.go` / `touch_sizes.go`, and the want-map in `design_md_drift_test.go`. All three.
- **Replacing an inline literal with a token:** decrement the per-file count in `token_discipline_test.go`'s `allowedLiteralBudget`. Run the test — it fails if you forget.
- **Adding a new file with literals:** must be added to the budget map *with review* — prefer using existing `Token*()` accessors from `theme_tokens.go` instead.
- **Alpha buckets:** use `WithAlpha(token, AlphaFaint|AlphaSubtle|AlphaMedium|AlphaStrong|AlphaOverlay)` from `theme_tokens.go`. Don't introduce new opacity values; pick the closest bucket and bump the bucket only if you really need a new one.

### Semantic invariants (memorize)

- **Three reds, three roles.** `error` = stop/error **text only**; `mute` = mute button **fill only**; `destructive` = destructive button **fill only**. Never reuse a red variant for a different role; do not introduce a fourth red.
- **`primary` and `on-surface-accent` share a hex** (`#00C8FF`) but have distinct roles (interactive surface vs. text). Use the role-correct accessor.
- **Surface hierarchy nests strictly:** `background` → `surface-1` → `surface-2` → `surface-3`. Never place a lower-level surface inside a higher-level container.

### Forbidden glyphs

`▶`, `▼`, `▲`, `✕`, `≡`, `⏸`, `■`, `↑`, `↓`, `→`, `←` in `internal/ui/` source. Use `IconID` enums + `DrawIcon(dst, IconID, r, col)`. Single-character text labels (M/S/FX/O/X) are permitted; raw chrome glyphs are not. See DESIGN.md "Permitted text-glyph exceptions" for the complete table.

### Validation commands

```bash
# Drift + discipline + lint snapshot (fast)
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod \
  -run 'TestDesignMDDrift|TestTokenDiscipline|TestDesignMDLintSnapshot' ./internal/ui/

# Direct lint (requires npx + network)
npx -y @google/design.md@0.1.1 lint DESIGN.md
```

---

## Maintenance Tips

### Tooling
- Use `.tools/go/bin/go` for all Go tasks
- Use `.tools/go/bin/gofmt` if system `gofmt` is unavailable
- Keep `src/js/main.wasm` out of commits — use `make wasm` locally

### After Modifying Predictor/DrumView/Caching
```bash
# Go tests
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/timeline ./internal/ui

# Browser bridge smoke (canonical export catalogue)
GO=$(pwd)/.tools/go/bin/go node src/js/wasm_bridge_smoke.browser.test.js
# Cross-platform predictor/audio parity
GO=$(pwd)/.tools/go/bin/go node src/js/xplat_parity.browser.test.js
GO=$(pwd)/.tools/go/bin/go node src/js/xplat_audio_compare.browser.test.js
```

### For Perf Regressions
- Compare `webaudio_perf.browser.test.js` vs `webaudio_perf_e2e.browser.test.js` outputs
- Check `perfStats()` after `resetPerfStats()`
- Inspect `dumpRowState` / `dumpTimelineSegments` via browser console

### File Organization
- UI files organized by ownership: `game_*.go`, `drumview_*.go`
- Tests colocated with implementation
- Use deterministic splitters for large files:
  - `go run scripts/split_ui_phase1.go`
  - `go run scripts/split_phase1_longfiles.go`

### Concurrency Debugging
- Run with `-log DEBUG` or `-log TRACE` for lock acquisition logs
- Use `PPROF=1` then: `go tool pprof http://localhost:6060/debug/pprof/goroutine`
- Check for `seqMu` deadlocks: ensure no callbacks from DrumView acquire it
