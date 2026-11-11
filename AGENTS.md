# Beatmo

## Overview

**Beatmo** is a graph-driven grid sequencer—a drum machine where nodes on a 2D grid form a directed graph that drives beat patterns in real time. Each node represents a potential drum hit; edges define traversal paths that determine when instruments fire.

**Platforms**: Desktop (Ebiten/Go) and Browser (WASM + WebAudio)

**Toolchain**: Go 1.26, JavaScript/WebAudio, miniaudio C DSP, Emscripten

---

## ⚠️ Git policy — DO NOT stage or commit

**Never run `git add`, `git stage`, or `git commit` in this repo** (this applies to subagents you dispatch too). Leave all changes in the working tree for the human to review and commit themselves. The working branch routinely carries hundreds of files of uncommitted work-in-progress; staging a file you touched also captures that WIP, and isolating your change from it is fragile and can lose the human's work. Verify your work with tests, not commits. Only stage/commit if the human explicitly asks in the current request.

---

## ⚠️ Visual Verification of UI Work (MANDATORY)

**Every UI change — a new feature, a restyle, or a UI bug fix — MUST be verified by rendering a real screenshot scene and looking at the PNG before it is considered done.** Green Go tests are NOT sufficient for UI: the ebitenstub/`-tags test` path does not prove that text, layout, colors, or button/keycap chrome actually render. Shipping UI on unit tests alone has repeatedly left features "unbaked" — e.g. buttons whose labels were completely blank while every unit test passed.

The loop for any UI feature or UI bug:

1. **Scene**: add (or reuse) a scene in `internal/ui/scene_catalog.go` whose `Setup` drives the UI into the exact state under change (open the panel/menu/dialog/picker, seed the data). Mirror the one-liner style: `{Name: "logs_panel", Setup: func(g *Game){ g.drum.notifyError("boom"); g.toggleLogsPanel() }}`. Keep the scene as a regression surface.
2. **Capture**: `cd src/go && CGO_ENABLED=1 ../../.tools/go/bin/go build -o /tmp/beatmo_screenshot ./cmd` (the `cmd` package lives under `src/go`; `.tools` at the repo root) then `xvfb-run -a -s "-screen 0 1280x720x24" /tmp/beatmo_screenshot -log ERROR -scene <name> -screenshot <out.png>` (or `make screenshots-all SCENES=<name>`; `MOBILE=1` there runs the BROWSER mobile pass — for a single native mobile capture use `-mobile -win-w 390 -win-h 844`, portrait required — fine for full-screen scenes, but a `crop_*` scene whose `MobileSetup` exposes a panel only works in the browser pass: natively it crops the wrong surface). A taller window (`-win-w 1400 -win-h 1400` + a matching xvfb screen) shows panels that clip at 720 px.
3. **Look**: open the PNG and confirm the change renders — text present/legible, layout sane, no blank controls, no overlaps, correct colors/active states. Fix and **re-capture until it looks right**.

**Never claim a UI change is complete, and never leave the tree, without a screenshot you have actually opened and inspected.**

## Comments — the code is the spec

A comment says **why**, in one or two sentences (at most 3 lines): a non-obvious choice, the constraint that forced it, or what breaks if someone "simplifies" it. Never what or how.

- **Delete:** restated names or next lines, step narration, banners; history (dates, phases, "it used to", bug stories — git has it); drifting state (measurements, sizes, counts, "as of", "for now"); cross-references ("see foo.go", "Guard: TestX", spec paths); commented-out code.
- **Keep:** the reason for a surprising choice; a hazard ("must not take seqMu: called under it"); an external constraint the code cannot show (a browser bug, a licence rule); a contract the type cannot express (units, ranges, lock ownership).
- Tests follow the same rules: the test name is the spec.
- Directives are not comments (`//go:*`, `//export`, cgo preambles, `//nolint`, eslint, shebangs).
- `make comment-census ARGS=--check` must pass (blocks ≤ 3 lines, no history markers); `--digest` / `--compare` prove a comment edit changed no code.

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
| Cross-platform parity | `make test-parity` |
| Build WASM | `make wasm` (main.wasm only; `make wasm-playtest` builds the `play_ui.wasm` harness, `make wasm-all` both + the audio playtest) |

**Bundled Go**: `.tools/go/bin/go` — always use this to avoid version drift.

---

## Repository Structure

```
src/go/
  cmd/              Entry point (beatmo.go)
  core/
    model/          Graph, nodes, edges, traversal
    beat/           BPM scheduler
    engine/         Engine loop, predictor
  internal/
    ui/             Ebiten UI, DrumView (~1700 .go files incl. tests)
    audio/          Audio engine + synth registry (~500 .go files incl. tests)
    timeline/       Commit ring, history service
    assets/         Embedded JSON (startup demo, parity fixtures, templates) + fonts — no WAVs
    ebitestub/      Stubbed Ebiten for tests
    gamestate/      Transport state machine
    graphruntime/   Graph traversal runtime
    log/            Logger
    utils/          Math helpers

src/js/
  audio.js          WebAudio bridge, render cache
  drums.single.js   Emscripten-compiled C synth
  *.browser.test.js Playwright tests (~126 files)
  index.html        WASM harness

src/c/
  modular.c / modular_stages.c   The modular synth engine — EVERY instrument renders here
                                 (render_modular_p — the ONE entry point; drums, cymbals, FM, melodic)
  drums.c           Shared DSP primitives (softsat/osc_wave), load_audio
  fmsynth.c         fm_render core (FM presets are built in modular_stages.c)
  effects.c         Send delay/reverb        insert_fx.c   Insert effects
  wavetable.c adsr.c noise.c                 Primitives
  miniaudio.c/.h    Audio library

assets/             Sample assets read from disk at runtime (root overridable via BEATMO_ASSETS; not go:embed-ed)
build/              Generated static libraries
```

### File Naming Conventions (Critical for Navigation)

The codebase uses **ownership-based file splitting**. Large modules are split by responsibility:

**Game module** (`game_*.go` — ~120 files incl. tests):
- `game_audio_*.go` — Audio loop, scheduling
- `game_draw_*.go` — Render dispatch, grid pane, helpers
- `game_graph_*.go` — Beat info, node/edge queries
- `game_input_*.go` — Drag, editor, menus
- `game_parity_*.go` — UI/audio consistency checks (8 files)
- `game_sequencer_*.go` — Highlight, schedule, loop

**DrumView module** (`drumview_*.go` — ~90 files incl. tests):
- `drumview_cache_*.go` — Row sprite, layer, stripe caching
- `drumview_draw_*.go` — Controls, layout guides, waveform
- `drumview_instrument*.go` — Instrument selection menus
- `drumview_color_menu.go` — Color wheel picker
- `drumview_audio_eq.go` — EQ controls

**Predictor** (`src/go/core/engine/predictor*.go` — 6 source + tests):
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

**Entry**: `src/go/cmd/beatmo.go` → `ebiten.RunGame(g)`

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

### Input System & Touch-to-Mouse Override

**Problem**: Ebiten on WASM does NOT map touch events to `cursorPosition()` / `isMouseButtonPressed()`. On mobile, those return `(0,0)` / `false`, breaking all 50+ mouse-based handlers (DrumView, Splitter, Camera, Editor, etc.).

**Solution**: Frame-level touch-to-mouse override. When a single touch is active, `cursorPosition()` and `isMouseButtonPressed()` transparently return touch coordinates and `left=true` so every existing mouse handler works without modification.

**Input flow per frame** (`game_update.go`):
```
1. globalTouchState.Update()        ← Poll Ebiten touch IDs, detect gestures
2. Drum-area tap? → injectTouchTap  ← 2-frame press+release for taps (touch already ended)
3. updateTouchOverride()            ← Set touchOverrideActive/X/Y/Left for the frame
4. cursorPosition() / isMouseButtonPressed()  ← Now touch-aware via override
5. Multi-touch gestures handled directly (pinch, two-finger pan, grid tap, long-press)
6. InputDispatcher + Camera + DrumView + Editor  ← All see touch as mouse
```

**Platform behavior**:
| Platform | `cursorPosition()` source | `isMouseButtonPressed()` source |
|----------|--------------------------|-------------------------------|
| Desktop | Ebiten mouse directly | Ebiten mouse directly |
| Mobile WASM (1 touch) | Touch override (primary touch X,Y) | Touch override (`left=true`) |
| Mobile WASM (2+ touches) | Ebiten (falls through) | Ebiten (falls through) |
| Mobile WASM (tap) | 2-frame injection cycle | Injected press then release |

**Key files**:
- `input.go` — Function variables `cursorPosition`, `isMouseButtonPressed` with touch-aware wrappers; `SetInputForTest` for test mocks
- `touch.go` — `TouchState`, `updateTouchOverride()`, `injectTouchTap()`, `resetTouchOverride()`
- `gesture.go` — `detectGesture()`: tap, long-press, single-finger drag, pinch, two-finger pan
- `game_update.go` — Input dispatch order (touch poll → override → cursor read → gesture handlers → dispatcher)
- `game_input_touch.go` — `handleTapInGrid`, `handleTouchLongPress`, `handleTouchPinch`, `handleTouchTwoFingerPan`

---

## Key Concepts

### Predictor as Source of Truth

The engine-owned Predictor is the authoritative answer for "what should fire?" The UI reads via `VisibleAt`/`TriggeredAt`; it never mutates predictor state directly.

### Timeline Immutability

Once a beat is played, its rendered state is frozen:
- `CommitKindPlayback` / `CommitKindImport` for true past
- Past cells never change; edits only affect future
- `preview.BuildRowWindow` follows a single precedence table

### Transport Actions (Play / Pause / Stop / Record)

Every transport action is a **one-frame pulse consumed by `Game.Update()`**, never a synchronous operation. The on-screen button, the JS exports (`startPlay` / `stopPlay`, `js_exports_playback_perf.go`) and the test triggers (`TriggerPlayPause` / `TriggerStop`) all do the same thing: flip a bool (`playPressed` / `stopPressed`). `Game.Update()` consumes it exactly once via `PlayPressed()` / `StopPressed()` (`drumview_transport.go`) and does the real work there.

**`stopPlay()` returning does NOT mean playback stopped** — the next `Update()` frame does that. Same for play/pause.

| Action | Where the work happens | Effect on already-scheduled audio |
|---|---|---|
| **Play** | `game_update.go` (`PlayPressed`, not playing) | `audio.Resume()` + `state.Resume()` |
| **Pause** | `game_update.go` (`PlayPressed`, playing) | bumps `audioGen` **and drains `g.audioCh` inline** |
| **Stop** | `game_update.go` (`StopPressed`) | bumps `audioGen`; **no inline drain** — relies on the gen gate below |
| **Record** | `game_update.go` (`RecordPressed`) | none (see `recording.lifecycle` pool) |

**Stop stops the scheduler, not the sound — by design.** Already-triggered audio is meant to ring out; a Stop that hard-cuts sounding voices clicks. (Same reasoning as the mid-play edit path, which scopes drops per-row + future instead of calling `audio.Stop`.)

The cut line is "not yet handed to the platform layer". `audioGen.Add(1)` makes the audio loop drop every sequencer event that hasn't been dispatched yet — `game_audio_loop.go` drops on `req.gen != curGen`, on a stale per-row `rowGen`, and on `!g.Playing()`. Anything already handed off is left alone:

- **Browser**: events already in audio.js's `audioQueue` finish flushing, and any `BufferSource` already `.start()`ed plays to completion. Nothing clears that queue on Stop — only `pagehide` force-stops `activeSources`.
- **Desktop**: voices already in the mixer decay naturally.

So audio legitimately keeps emitting *after* Stop. Measured under stress (BPM 240 + `buildPerfRect(2,1)`): `isPlaying()` went false at **+263ms** and the last note started at **+1142ms** after `stopPlay()` resolved — those tail events carry `startWhen - ctxAtStart == 0.004` exactly (`AUDIO_MIN_LEAD_SEC`), i.e. overdue backlog being clamped, not pre-scheduled lookahead. This is correct behavior; don't "fix" it.

**Test consequence**: never assume quiescence after `stopPlay()` / `TriggerStop()`. Wait on a condition (`isPlaying() == false` *plus* a stable count), never a bare timeout. In browser tests, never split a mutate-then-observe pair of global audio state across two `page.evaluate` round-trips — the tail lands in the gap and the observe sees it. One evaluate body runs to completion without yielding, so do both inside it:

```js
const { before, after } = await page.evaluate(() => {
  const before = window.getTimingTraces().length;
  window.resetTimingTraces();
  return { before, after: window.getTimingTraces().length };
});
```

This bit `timing_traces.browser.test.js` (green standalone, red under `BROWSER_JOBS=4` where the round-trip gap widens). Reproduce this class of flake by running the test 4× concurrently, not standalone.

### Concurrency Model

- **UI thread**: `Update()` + `Draw()`
- **Background sequencer**: Audio scheduling via goroutine
- **Audio thread** (desktop): Sample generation via Oto
- **WebAudio thread** (browser): Sample playback
- **Guard**: `seqMu` protects structural edits (row add/delete, path changes)

### Parity Verification

Built-in invariant checks ensure UI slate matches predictor state:
- `parityCheck` — Steps vs predictor mismatch
- `parityScan` — Highlight events vs scheduler decisions
- `parityAudio` — Past audio events vs expected triggers
- Grace periods prevent false positives during tight loops

---

## Testing

### Go Tests (~1800 `*_test.go` files)

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
- **Performance**: `game_predictor_throttle_test.go`, `parity_scan_throttle_test.go`, `perf_test.go`
- **E2E workflow**: `e2e_workflow_test.go` (full lifecycle, BPM stress, row-add-during-playback)
- **Render verification**: `render_verify_test.go` (pixel-level and drawRect interception)
- **Cross-platform parity**: `parity_cross_platform_test.go` (golden file generation for WASM comparison)
- **Audio mixer/EQ** (in `src/go/internal/audio/`):
  - `biquad_corruption_test.go` — Biquad state corruption from interleaving, 3-phase coherence, block vs sample parity (fast, `test` tag)
  - `channel_isolation_test.go` — ProcessBlockLocal vs ProcessBlock parent chaining, multi-instrument coherence, accumulation (fast, `test` tag)
  - `mixer_signal_test.go` — Real 3-phase mixer with voices + EQ: multi-voice same instrument, multi-instrument, output capture (`!test` tag, requires xvfb)

### Playwright Browser Tests (~126 files)

```bash
GO=$(pwd)/.tools/go/bin/go node src/js/<name>.browser.test.js
```

**Key Suites** (CLAUDE.md § Playwright Browser Tests keeps the full, current list):
- **Bridge plumbing**: `wasm_bridge_smoke.browser.test.js` (the canonical export catalogue)
- **Performance**: `webaudio_perf.browser.test.js`, `webaudio_perf_e2e.browser.test.js`, `webaudio_pan_stress.browser.test.js`
- **Audio**: `webaudio_smoke.browser.test.js`, `webaudio_drums_consistency.browser.test.js`
- **Real-input (mouse events)**: `e2e_real_input_circuit.browser.test.js`, `e2e_real_input_edge.browser.test.js`, `e2e_real_input_live_edit.browser.test.js`
- **Cross-platform parity**: `xplat_parity.browser.test.js` (WASM vs Go golden comparison), `xplat_audio_compare.browser.test.js`
- **E2E workflow**: `e2e_workflow.browser.test.js` (multi-step scenarios: lifecycle, BPM stress, row operations)
- **Pipeline stages vs desktop**: `xplat_audio_stages.browser.test.js` (played voices, lane stops, every insert effect, every EQ shape, the master, whole circuits — per sample against desktop, in Chromium, Firefox and WebKit)
- **Mobile audio**: `mobile_audio.browser.test.js` (AudioContext unlock via touch, mobile playback output capture, mobile vs desktop parity)

**Targeted test commands**:
```bash
make test-browser                                  # All browser tests (auto-discovered)
make test-browser-filter FILTER=parity             # Filter by name pattern
make test-parity                                   # Go golden files + WASM parity comparison
```

**Test logging**: Default silent. Enable with `TEST_LOG=1`, set `TEST_LOG_LEVEL=TRACE|DEBUG|INFO|ERROR`.

### Real-Input Test Infrastructure

Tests under `src/js/e2e_real_input_*.browser.test.js` exercise the UI via actual Playwright mouse events instead of direct JS API calls. They depend on two helper modules:

- **`real_input_test_helpers.js`** — Low-level: `setupFullWasm()`, `clickAndHold()`, `rectCenter()`, `assertValidRect()`
- **`real_input_actions.js`** — High-level actions: `clickGridCell()`, `shiftDragEdge()`, `rightClickNode()`, `clickPlayBtn()`, `clickStopBtn()`, `clickBpmInc()`, `clickBpmDec()`, `clickAddRow()`, `clickRowMute()`, `clickRowSolo()`, `assertPlaying()`, `assertNodeExists()`, `canvasPixelAt()`, `assertNodeVisible()`

These helpers use JS exports like `gridToScreen(i, j)`, `playBtnRect()`, `nodeIdAt(i, j)` to translate grid coordinates into screen coordinates for mouse events.

### Cross-Platform Parity Fixtures

Five JSON fixture files in `src/go/internal/assets/` define shared test circuits:
- `parity_fixture_simple.json` — 4-node regular loop
- `parity_fixture_probability.json` — Probability logic (p=0.5)
- `parity_fixture_mute.json` — Mute node
- `parity_fixture_multi_row.json` — 3-row circuit (kick/snare/hihat)
- `parity_fixture_node_rules.json` — node-logic rules (its golden `testdata/parity_golden_node_rules.json` is produced by `node_rule_parity_test.go` and checked by `node_logic_parity.browser.test.js`)

Golden files for the first four are generated by `go test -run TestCrossPlatformParity` into `src/go/internal/ui/testdata/parity_golden_*.json`. Browser tests load these and verify bit-identical predictor outputs from WASM.

---

## Debugging

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
| `DEBUG_GEOM=1` | Verbose geometry logs |
| `AUDIO_CLIP_DEBUG=1` | Log audio clipping events |
| `DEBUG_MIXER=1` | Log mixer workBuf values |
| `BYPASS_CHANNEL_PROC=1` | Skip channel processing (EQ, volume) |

See **Audio Debugging (Desktop)** section below for full audio env vars and debugging workflow.

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

**Playback**: `startPlay()`, `stopPlay()`

**Performance**: `resetPerfStats()`, `perfStats()`, `resetAudioScheduleMetrics()`, `getAudioScheduleMetrics()`

---

## Critical Gotchas

### Concurrency & Deadlocks

**seqMu deadlock prevention**: `Game.Update()` holds `seqMu` while calling `drum.Update()`. Any callback from DrumView that calls back into Game code requiring `seqMu` will deadlock. Go's `sync.Mutex` is NOT reentrant.

**Solution**: `onImport` queues data to `pendingImportData`; actual import runs after `seqMu.Unlock()`.

**lastTriggeredByRow is mutex-protected**: Never read/write the map directly in tests. Use `setLastTriggeredForTest` / `lastTriggeredForTest` on `Game` (`game_trigger_cache.go`).

### Input & Touch

**`cursorPosition` / `isMouseButtonPressed` are function variables, not direct Ebiten calls**. They live in `input.go` and wrap Ebiten with touch-override logic. Never call `ebiten.CursorPosition()` directly in UI code — always use the package-level `cursorPosition()`. The raw Ebiten functions are stored as `_ebCursorPosition` / `_ebIsMouseButtonPressed`.

**Touch override is frame-scoped**. `updateTouchOverride()` must run once per frame, after `globalTouchState.Update()` and before any `cursorPosition()` reads. If you reorder input processing in `game_update.go`, you MUST preserve this sequence.

**Tap injection is a 2-frame cycle**. Taps fire after touch ends (`ActiveTouchCount==0`), so the single-touch override can't see them. `injectTouchTap(x, y)` synthesizes frame 0: position+left=true, frame 1: position+left=false. Don't call it during multi-touch.

**`inputForTestActive` suppresses the override**. When `SetInputForTest` is active, `updateTouchOverride()` clears the override so test mocks control input exclusively. Always call `resetTouchOverride()` alongside `SetInputForTest`.

**Multi-touch gestures bypass the override entirely**. Pinch, two-finger pan, and long-press are handled as direct gesture events in `game_update.go`, never mapped to mouse. Only single-finger contact and drum-area taps use the override.

### Predictor Behavior

**Mute gate**: Engine predictor only gates the mute subdivision (`idx+hold+1`), so audio resumes immediately on the following beat. UI/audio mute tests assume this behavior.

### WASM/Playwright

**Absolute GO path required**: When running Node harnesses that call `go build`, pass an absolute path:
```bash
GO=/home/ymolinar/Repos/beatmo/.tools/go/bin/go node src/js/<test>.browser.test.js
```
Relative `.tools/...` can fail because cwd becomes `src/go`.

### Debugging JS/WASM Audio Bugs (hard-won field guide)

Audio in the browser spans **three layers**, and a bug can live in any of them — always localize the layer *before* proposing a fix:

1. **Go (`main.wasm`)** — schedules *what/when* to play (sequencer → `playSoundsBatchFlat`), and owns per-instrument synth params (`audio.SetInstrumentParam`). It does **not** render audio in WASM.
2. **JS bridge (`audio.js`)** — receives each instrument's flattened document (`setInstrumentBlock`), renders one-shots via the C module (`ensureRenderedSample` → `renderVoiceInWorker` off-thread in `synth_render_worker.js`, `renderVoiceSync` as the fallback), and schedules playback (`processAudioEvent` → WebAudio `AudioBufferSourceNode`).
3. **C DSP (`drums.single.js`, emscripten, built from `src/c/*.c`)** — the actual `render_*` / `render_modular_p` sample synthesis.

**Synth vs sample instruments — the single most important distinction.** `RENDER[id]` (a map in `audio.js`) tells you which an instrument is:
- **Synth** (`RENDER[id]` present): its `renderCache[id]` is **re-derivable** — invalidating it just forces a re-render. Safe to delete.
- **Sample** (`registerSamplePCM(id)` ran → `RENDER[id]` **deleted**): its `renderCache[id]` holds the **only copy** of its PCM. Deleting it is **unrecoverable silence** — `processAudioEvent` can't re-render (no `RENDER[id]`, maybe no URL) and falls through to "Unknown sound".
- Check `hasRENDER` in the `[SYNTH-EVT]` log. Real bug found this way: `_updateInstrumentParams` deleted `renderCache[id]` unconditionally on any knob/gen_type edit → a soloed *sample* instrument went permanently silent. Fix gated the delete on `if (RENDER[id])`. Guard: `synth_sample_param_change_keeps_audio.browser.test.js`.

**The "dead until reload" failure mode.** When `ensureRenderedSample` **throws** (e.g. a missing C export in a stale `drums.single.js`, a bad ccall arg) OR returns silence, `processAudioEvent` drops that hit and *re-renders on every subsequent hit* — so a **persistent** problem means the row is silent forever. Defenses now in `audio.js`: a render throw is loud and the instrument's PREVIOUS buffer keeps playing (there is no substitute voice); a silent render shows up as `silent` in the `[SYNTH-DISPATCH] render result {…}` debug log (the old ungated `re-voiced render produced SILENCE` line no longer exists). If you see neither but still get silence, the render *succeeded* and the problem is downstream (routing/solo/mute) or upstream (Go stopped scheduling).

**Diagnostic flags + logs (all gated/off by default).** In the browser console: `__setSynthDragDebug(true)` — one call now flips the **whole cascade** (Go `[synthdrag]` + JS `[SYNTH-DISPATCH]`/`[SYNTH-UPD]`/`[SYNTH-EVT]`/`[SYNTH-RDY]`/`[SYNTH-ERS]`). The per-frame `buildSynthTab` log is deduped (logs only on transition). Trace the cascade in order: drag (`OnPress`/`OnDrag` Go) → `propagate WRITE` (Go) → `[SYNTH-UPD]` (JS bridge received it) → `[SYNTH-EVT] {cached, hasRENDER}` (live event) → `[SYNTH-DISPATCH]` (a render ran) → `render result {silent}`.

**Stale-asset trap (`make serve`).** `audio.js`/`drums.single.js` are served as source; browsers cache them. Symptom: fresh Go `[synthdrag]` logs appear (new `main.wasm` fetched) but **zero** JS `[SYNTH-*]` logs → you're running cached JS without your fix. `make serve` now sends `no-store` (`scripts/serve_nocache.py`); still **hard-reload** (Ctrl/Cmd+Shift+R). The `[AUDIOJS] loaded build SYNTHDBG-N` banner (bump the marker when editing `audio.js`) is the definitive freshness check.

**Measuring audio in a test (don't trust "didn't throw").** Tap the real graph: `page.addInitScript` a `class TestAC extends AudioContext` that wires a `ScriptProcessorNode` on `destination` into a windowed RMS accumulator (`window.__rmsAccum/__frames`); or use `startOutputCapture()`/`stopOutputCapture()`. A constant **~0.0002 RMS is silence** (the ScriptProcessor noise floor). Use a **relative** floor (e.g. `max(1e-4, baseline*0.05)`), not just an absolute one — a stale ABI can drop RMS ~250× yet a residual fade transient clears a naive absolute floor.

**Instrument carefully — your logging can BE the bug.** A debug `console.log` that references an out-of-scope/TDZ variable (`ReferenceError: Cannot access 'heap' before initialization`, `allocCount is not defined`) throws *inside* the render and silences the voice — masquerading as the very bug you're chasing. Keep diagnostic code side-effect-free and reference only in-scope locals; if a function declares the same `const` name twice in different blocks, rename one (the `heap`/`pheap` footgun).

**Production-faithful harness.** Use `buildMainWasm` + `createServer` + the real `AudioContext` tap (real `main.wasm`, real `index.html`) when the bug is in real playback — not the `play_ui.wasm` playtest harness (different HTML, different code path). `buildMainWasm` now also regenerates `synth_param_abi.gen.js` so the env can't ship a stale ABI.

**Stale `play_ui.wasm` trap.** `play_ui.html` (the playtest harness most `*.browser.test.js` files load) runs `play_ui.wasm`, built from `./internal/ui/playtest` — NOT `main.wasm`. The real tests rebuild it themselves unless `WASM_PREBUILT=1`, but an ad-hoc script against `play_ui.html` silently runs whatever old `play_ui.wasm` is on disk; `make wasm` does not rebuild it (`make wasm-playtest` does). Symptom (2026-09-28): the browser received an instrument's OLD 99-key defaults from Go while the Go source pushed 1296 keys.

**No block, no voice.** Go pushes every instrument's flattened document to JS via `setInstrumentBlock`. An id with no pushed block (a bare audio.js page, or a render before Go's init) renders nothing — there is no bare-voice fallback any more. Bare-page tests install real blocks read from the app (`__instrumentBlockForTest`).

### Tests can pass while testing nothing (verify the precondition!)

The most dangerous test is a **green test that never exercised the behavior**. Real examples from this codebase:
- *(History)* `synth_generator_solo_realinput` drove the knob with a **vertical** drag while synth knobs then only turned on **horizontal** drag. It never moved `gen_type`, so "no foreign-param scramble" passed *vacuously* — the bug it guarded was untested for weeks. (Today the rotary drag is gone — a tap opens the value-wheel popup and the mouse wheel changes the value in place — and the `gen_type` selector was replaced by the per-family `<family>_wave` knob; the test now drives the wheel.)
- Optional chaining (`exportName?.()`) silently returns `undefined` for a missing/renamed export, so the test "passes" having called nothing.
- Asserting RMS `> 1e-4` after a residual fade transient passes even when the instrument is effectively dead.

**Audit rubric for any audio/input browser test:**
1. **Precondition assertion** — does it assert the input *actually changed the target state* before asserting the effect? (e.g. `expect snare_wave 0 → N`, `expect rowSoloed(r) === true`, `expect synthControlRects().length > 0`). If not, the gesture may be a no-op.
2. **Real drive path** — does it use real canvas input (`page.mouse`) on a *real rect* (`synthControlRects()`), with the **correct gesture** for the control (tap / wheel)? Or does it secretly fall back to a JS export that bypasses the UI it claims to test?
3. **Meaningful assertion** — does it measure **real audio output** (RMS/peak from a tap) or real state, vs. merely "the call didn't throw" / "a function exists"?
4. **Relative + absolute floors** for silence; baseline captured and asserted non-silent *first*.
5. **Right harness** — production `main.wasm` for real-playback bugs, not the playtest stub; correct flags enabled.
6. **Teeth** — would it fail if the fix were reverted? When you write a fix, prove it: temporarily revert, confirm RED, restore. (`if (false && …)` / `if (true || …)` toggles are quick teeth checks.)

### Test Stub EQ Behavior

Under the `test` build tag, `NewEQProcessor` in `stub.go` creates a `gainProcessor` (simple amplitude scaling) instead of real biquad filters. This means:
- `NewEQProcessor(sr, EQBand{Kind: EQLowpass, Freq: 1000, Q: 0.707})` does NOT actually filter frequencies in stub mode
- Tests that need real biquad filtering must use `makeBiquad()` directly and pass it to `SetChannelProcessors(id, bq)` — the `*biquad` type satisfies the `Processor` interface
- The multiband processor and `silenceProcessor` DO work in stub mode (for band muting tests)
- Desktop-only mixer tests (`!test` build tag) get the real `eqProcessor` with biquad chains

### Parity Defaults

- **WASM**: Parity stays log-only unless `PARITY_WASM_FATAL=1|true|panic`
- **Desktop**: Honors `PARITY_WATCH`/`PARITY_FATAL` normally
- **During imports**: `g.importing` disables parity scans and clears buffers

### WebAudio Sample Rate & Deferred AudioBuffer

WebAudio contexts often default to 48 kHz. Synth buffers are pre-rendered to raw `Float32Array` at a default 48 kHz sample rate (without creating an AudioContext), then lazily wrapped in `AudioBuffer` on first play via `ensureAudioBuffer()`. WebAudio resamples automatically if the context SR differs. This deferred pattern avoids premature AudioContext creation on mobile, where a context created outside a user gesture starts `suspended` and may not unlock.

### Writing E2E / Functional Tests

#### Go Test Patterns

Standard setup for Go E2E tests in `src/go/internal/ui/`:

```go
func TestMyE2E(t *testing.T) {
    assertDefaultParityState(t)        // ALWAYS reset parity globals first
    withDefaultStart(t, false)         // Disable default start node for clean slate
    g := New(testLogger)
    t.Cleanup(g.CloseForTest)
    g.Layout(800, 600)

    // Build circuit
    a := g.tryAddNode(0, 0, model.NodeTypeRegular)
    b := g.tryAddNode(8, 0, model.NodeTypeRegular)
    g.addEdge(a, b)
    g.addEdge(b, a)
    g.start = a
    g.graph.StartNodeID = a.ID
    g.drum.Rows[0].Origin = a.ID
    g.drum.Rows[0].Node = a

    g.updateBeatInfos()     // MUST call after any circuit change
    g.refreshDrumRow()      // MUST call to update UI state

    // Playback
    g.SetPlayFunc(func(string, float64, ...float64) {}) // Hook audio
    g.SetPlaying(true)
    advanceFrames(g, 30)    // Simulate 30 frames (~0.5s at 60Hz)

    // Export/Import
    data, err := g.drum.exportBytes()
    g2 := New(testLogger); g2.Layout(800, 600)
    g2.Import(data)

    // Query predictor
    g.engine.Predictor.Ensure(64)
    vis := g.engine.Predictor.VisibleAt(row, absIdx)

    // Clean stop
    stopPlaybackForTest(g)
}
```

Key rules:
- Always `assertDefaultParityState(t)` at test start
- Always `g.updateBeatInfos()` + `g.refreshDrumRow()` after circuit changes
- Use `stopPlaybackForTest(g)` for clean stop (resets all sequencer state)
- Use `advanceFrames(g, N)` to tick the game loop
- Never access `lastTriggeredByRow` directly — use thread-safe test helpers

#### Browser Test Patterns

Browser E2E tests can use direct API calls or real mouse events:

```javascript
// Direct API approach (faster, more reliable for non-input features)
await page.evaluate(() => {
    addNode?.(0, 0, "regular");
    addEdgeGrid?.(0, 0, 8, 0);
    updateBeatInfos();   // the export is `updateBeatInfos` — `?.()` on a wrong name silently does nothing
    forceDraw?.();
});
await page.evaluate(() => startPlay?.());
// ... test logic ...
await page.evaluate(() => stopPlay?.());

// Real-input approach (tests actual UI pipeline)
import { clickPlayBtn, clickGridCell, shiftDragEdge } from "./real_input_actions.js";
await clickGridCell(page, 0, 0);
await shiftDragEdge(page, 0, 0, 8, 0);
await clickPlayBtn(page);
```

#### Browser E2E Gotchas

1. **JS export names matter**: `startPlay()` / `stopPlay()` — NOT `start()`/`stop()`. Optional chaining (`?.()`) silently returns `undefined` for missing functions, so typos fail silently.

2. **After `importJSON()`, the UI needs settling time**: Always call `forceDraw()` + `await page.waitForTimeout(300-500)` before interacting with the UI post-import. Button rects may not be valid until the layout recalculates.

3. **Use API calls when testing non-input features**: If the test focus is BPM stress or row operations (not button clicking), use `startPlay()`/`stopPlay()` instead of `clickPlayBtn()`/`clickStopBtn()`. Real mouse clicks are fragile after mid-test imports.

4. **Canvas pixel reading requires screenshot workaround**: Ebiten uses WebGL, so `readPixels` doesn't work after buffer swap. Use the `canvasPixelAt(page, x, y)` helper from `real_input_actions.js`, which takes a Playwright screenshot, decodes the PNG via an offscreen canvas, and reads the pixel.

5. **Cross-scenario state leaks**: If a test has multiple scenarios, ensure each one properly stops playback before starting the next. A silent no-op (like calling `stop()` instead of `stopPlay()`) means the next `startPlay()` toggles playback **off**.

6. **Mobile audio unlock**: `audio.js` registers unlock listeners on 5 event types (`touchstart`, `touchend`, `pointerdown`, `mousedown`, `keydown`) with `{ capture: true }`. Tests that dispatch synthetic events (e.g., `new Event("pointerdown")`) will trigger these listeners. For mobile emulation tests, use CDP trusted touch events via `cdpTap()` from `touch_cdp_helpers.js`.

#### Ebitenstub Rendering Gotchas (Go Pixel Tests)

The stubbed Ebiten at `src/go/internal/ebitestub/ebiten/ebiten.go` has limitations for pixel-level testing:

1. **`SubImage()` returns a pixel-less view**: writes to a subimage are simply discarded and never appear in the parent image. `drawGridPane` uses `screen.SubImage(...)` for node rendering, so pixel reads on the parent `screen` miss node draws entirely. **Glow/signal overlays** draw directly to `screen` and CAN be read.

2. **Use render-safe mode for drawRect interception**: `envRenderSafe` is read ONCE at package init from `RENDER_SAFE` (`game_draw_grid_pane.go`), so `t.Setenv("RENDER_SAFE", "1")` alone has no effect — set the package var (`envRenderSafe = true`, restore in `t.Cleanup`) to force nodes through the `drawRect` function path instead of the sprite cache. Then intercept `drawRect` to capture draw calls:
   ```go
   var rects []drawnRect
   orig := drawRect
   drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
       if filled { rects = append(rects, drawnRect{Rect: r, Color: ...}) }
       orig(dst, r, c, filled)
   }
   defer func() { drawRect = orig }()
   ```

3. **Scaled `DrawImage` copies only the first pixel**: an UNSCALED multi-pixel draw is copied per pixel, but a scaled draw floods the destination rectangle with the source's top-left pixel (the 1×1 stretched `drawRectBlit` pattern). Sufficient for presence/color checks, not for detailed scaled-sprite fidelity.

---

## Import/Export Format

**Project JSON** (`beatmo.json`; exports are named `beatmo-export-<stamp>.json`). Every Beatmo JSON document carries `"schema_version"` from `schema.Version` (`internal/schema`, `0.0.1` until first release); the old integer `"version"` survives only as `LegacyVersion` for migrating pre-versioning files — see CLAUDE.md § Import/Export Format for the three-way accept/reject/migrate rule.
```json
{
  "schema_version": "0.0.1",
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

## Maintenance Tips

### Tooling
- Use `.tools/go/bin/go` for all Go tasks
- Use `.tools/go/bin/gofmt` if system `gofmt` is unavailable
- Keep `src/js/main.wasm` out of commits — use `make wasm` locally

### After Modifying Predictor/DrumView/Caching
```bash
# Go tests
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/timeline ./internal/ui

# Browser tests
GO=$(pwd)/.tools/go/bin/go node src/js/logic_sync.browser.test.js
GO=$(pwd)/.tools/go/bin/go node src/js/circuit_sync.browser.test.js

# Cross-platform parity (regenerates golden files then compares in WASM)
make test-parity

# E2E workflow (full lifecycle, BPM stress, row operations)
GO=$(pwd)/.tools/go/bin/go node src/js/e2e_workflow.browser.test.js
```

### After Modifying Audio Mixer/EQ/Channels
```bash
# Fast biquad corruption + channel isolation tests
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/... -v -run "Biquad|Channel|ProcessBlock"

# All fast audio tests
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/...

# Desktop mixer tests (requires xvfb or display)
cd src/go && xvfb-run -a ../../.tools/go/bin/go test ./internal/audio/... -v -run "Mixer"

# Browser EQ parity
GO=$(pwd)/.tools/go/bin/go node src/js/mixer_eq_parity.browser.test.js
```

### For Perf Regressions
- Compare `perf.browser.test.js` vs `perf_e2e.browser.test.js` outputs
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

---

## Audio Debugging (Desktop)

The desktop audio mixer (`src/go/internal/audio/engine_stop.go`) has extensive debugging capabilities for isolating distortion, clipping, and mixing issues.

### Audio Pipeline Overview — 3-Phase Mixer

The desktop mixer (`engine_stop.go`) uses a 3-phase block processing design to prevent shared biquad EQ state corruption. Each biquad filter is stateful (`x1, x2, y1, y2`) and expects a continuous signal stream — interleaving unrelated voice signals through a shared filter corrupts its state and causes audible distortion (clicks, noise).

```
Phase 1: Render voices → per-instrument buffers (NO EQ)
    For each active voice:
        voice.Sample() → voiceTemp
        voiceTemp *= mixHeadroom (0.18× = VoiceHeadroom, chain_spec.go)
        instBufs[instrumentID] += voiceTemp
    ↓
Phase 2: Per-instrument channel processing → masterBuf
    For each active instrument:
        2a channel.ProcessInsertsBlockLocal(instBuf, postFXBuf)   (volume + insert FX)
        2b channel.ProcessEQBlockLocal(postFXBuf, postEQBuf)     (EQ)
        masterBuf += postEQBuf        (does NOT recurse to parent)
    ↓
Phase 2.5: Send effects — delay/reverb fed from the (pre-fader) instBufs → masterBuf
    ↓
Phase 3: Master channel EQ + compressor → workBuf
    mainChannel.ProcessBlockLocal(masterBuf, workBuf)
    ↓
Output capture (if enabled)
    ↓
Soft-clip above 0.9 (0.9·tanh(x/0.9)) + safety clamp [-1, 1]
    ↓
int16 conversion (×32767)
    ↓
Oto audio output
```

**Why 3 phases?** Each biquad filter chain receives a coherent, continuous signal stream (the summed output of all voices for one instrument) instead of interleaved fragments from unrelated voices. WASM was never affected because WebAudio sums voices at the GainNode level before routing through shared filter nodes.

**Key methods**:
- `ProcessBlockLocal(input, output)` — Applies this channel's volume + EQ, accumulates into output. Does NOT recurse to parent channel.
- `ProcessBlock(input, output)` — Same but DOES recurse to parent (used outside the mixer for chained processing).
- `renderVoiceIntoInstBuf(vs, blockLen)` — Phase 1 helper: renders voice samples with headroom into per-instrument buffer.

### Audio Environment Variables

The test/single-voice vars carry a `BEATMO_` prefix (declared in `internal/env/vars.go`; the old unprefixed names are no longer read). `BYPASS_*`, `AUDIO_CLIP_DEBUG` and `DEBUG_*` are unchanged.

| Variable | Purpose |
|----------|---------|
| `AUDIO_CLIP_DEBUG=1` | Log clipping events (samples exceeding ±1.0 before the soft-clip/clamp) |
| `DEBUG_MIXER=1` | Log mixer workBuf min/max values and voice stats periodically |
| `BYPASS_CHANNEL_PROC=1` | Skip all channel processing (volume, EQ) — isolates mixer vs channel issues |
| `BYPASS_HEADROOM=1` | Skip per-voice headroom attenuation (0.18×) — useful to test if headroom is sufficient |
| `BEATMO_TEST_TONE=1` | Output pure 440Hz sine wave, bypassing all synth — tests Oto/driver |
| `BEATMO_TEST_VOICE=1` | Replace C synth with simple 220Hz sine voices — tests voice/mixer path |
| `BEATMO_TEST_RAW_VOICE=1` | Output first voice's raw samples with minimal processing — tests voice buffer |
| `BEATMO_TEST_RAW_VOICE_FULL=1` | With `BEATMO_TEST_RAW_VOICE`, use full amplitude (1.0×) instead of 0.2× |
| `BEATMO_TEST_RAW_VOICE_NOSCALE=1` | With `BEATMO_TEST_RAW_VOICE`, skip all multiplication (direct to int16) |
| `BEATMO_SINGLE_VOICE=1` | Limit mixer to one voice at a time — tests if multi-voice accumulation causes issues |

### Debugging Workflow

**Step 1: Verify driver works**
```bash
BEATMO_TEST_TONE=1 make run
# Should hear clean 440Hz tone
```

**Step 2: Test voice path**
```bash
BEATMO_TEST_VOICE=1 make run
# Should hear clean 220Hz sine for each drum hit
```

**Step 3: Isolate channel processing**
```bash
BYPASS_CHANNEL_PROC=1 make run
# If this is clean but normal mode is distorted, issue is in EQ/volume processing
```

**Step 4: Check clipping**
```bash
AUDIO_CLIP_DEBUG=1 make run
# Logs how many samples clip per block — if many, headroom may be insufficient
```

**Step 5: Inspect mixer values**
```bash
DEBUG_MIXER=1 make run
# Logs min/max values in workBuf — values outside [-1, 1] before clamp indicate issues
```

### Output Capture API

For programmatic comparison (e.g., Go vs WASM parity):

```go
import "github.com/ingyamilmolinar/beatmo/internal/audio"

audio.StartOutputCapture()
// ... play audio ...
samples := audio.StopOutputCapture()  // []float64, post-headroom, pre-int16

// Or non-blocking:
audio.StartOutputCapture()
// ... play audio ...
current := audio.GetOutputCapture()   // Snapshot without stopping
audio.ClearOutputCapture()            // Clear buffer, keep capturing
```

### Common Issues

**Distortion on multiple simultaneous voices**:
- Headroom is applied per-voice (0.18×) BEFORE accumulation
- If 4+ voices still clip, consider reducing individual voice amplitude in C synth
- **Never process individual voices through shared EQ chains** — this was the root cause of the desktop distortion bug. Always sum voices first (per-instrument), then EQ the sum.

**EQ boost causing clipping**:
- Channel processing happens AFTER headroom but BEFORE accumulation
- Large EQ boosts (+12dB) on multiple voices can still clip
- Use `BYPASS_CHANNEL_PROC=1` to verify, then reduce EQ gains

**Crackling/popping**:
- Check `AUDIO_CLIP_DEBUG=1` for clipping frequency
- Use `DEBUG_MIXER=1` to see if values are reasonable
- May indicate buffer underrun — check system audio latency settings

### Biquad State Corruption (Historical Bug)

The desktop audio had severe distortion because the old `processBlock()` processed each voice individually through shared biquad EQ chains. Biquad filters are stateful (`x1, x2, y1, y2`) and expect continuous input — interleaving unrelated voice signals corrupted filter state. WASM was unaffected because WebAudio sums voices before routing through shared filter nodes. The fix restructured `processBlock()` into the 3-phase pattern described above.

**Test coverage for this bug** (`src/go/internal/audio/`):
- `biquad_corruption_test.go` — Directly reproduces the corruption mechanism and proves the fix works (4 tests)
- `channel_isolation_test.go` — Verifies channel hierarchy and ProcessBlockLocal vs ProcessBlock behavior (3 tests)
- `mixer_signal_test.go` — Exercises the real 3-phase mixer with voices and EQ (`!test` build tag, requires xvfb) (3 tests)
- `src/js/mixer_eq_parity.browser.test.js` — Full WASM+WebAudio pipeline with EQ (3 scenarios)
