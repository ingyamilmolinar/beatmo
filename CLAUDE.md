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
| Sync WAV embeds | `make sync-wav` |

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

### Parity Verification

Built-in invariant checks ensure UI slate matches predictor state:
- `parityCheck` — Steps vs predictor mismatch
- `parityScan` — Highlight events vs scheduler decisions
- `parityAudio` — Past audio events vs expected triggers
- Grace periods prevent false positives during tight loops

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

### Playwright Browser Tests (57 files)

```bash
GO=$(pwd)/.tools/go/bin/go node src/js/<name>.browser.test.js
```

**Key Suites**:
- **Logic sync**: `logic_sync.browser.test.js`, `circuit_sync.browser.test.js`
- **Performance**: `perf.browser.test.js`, `perf_e2e.browser.test.js`, `pan_stress.browser.test.js`
- **Audio**: `audio_presence.browser.test.js`, `drums_consistency.browser.test.js`

**Test logging**: Default silent. Enable with `TEST_LOG=1`, set `TEST_LOG_LEVEL=TRACE|DEBUG|INFO|ERROR`.

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
