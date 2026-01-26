## Overview

- **Tunkul** is a graph-driven grid sequencer. Each node/edge in the directed graph maps to drum events that are scheduled in real time.
- Targets: **desktop** (Ebiten) and **WASM** (Go WASM + Emscripten DSP + WebAudio).
- Toolchain: **Go 1.23**, JavaScript/WebAudio harnesses, miniaudio C DSP.
- Core pillars:
  - Graph + traversal (`src/go/core/model`).
  - Engine scheduler + predictor (`src/go/core/engine`).
  - Ebiten UI/game loop (`src/go/internal/ui`).
  - Audio backends (`src/go/internal/audio`, `src/js/audio.js`).
  - Playwright browser suites (`src/js/*.browser.test.js`).

---

## Quick Start Commands

| Task | Command | Notes |
|------|---------|-------|
| Install deps | `sudo make dependencies` | Installs X11/ALSA/OpenGL, Node, Emscripten, Playwright. |
| Desktop run | `make run` _(or `make run-debug`)_ | Uses cached C/WAV assets. |
| Headless demo | `xvfb-run make run RUN_ARGS=-demo` | CI smoke test. |
| fast Go tests | `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./...` | Ebiten stubbed renderer. |
| Full pipeline | `make test` | Builds C lib + WASM, runs Go + Playwright harnesses. |
| Real Ebiten | `make test-real` | xvfb + browser suites (adds `bpm.browser.test.js`). |
| Single browser harness | `GO=$(pwd)/.tools/go/bin/go node src/js/<name>.browser.test.js` | Uses bundled Go toolchain. |
| Build WASM only | `make wasm` | Produces `src/js/main.wasm` (do **not** commit). |
| Sync WAV embeds | `make sync-wav` | Mirrors `assets/wav` into Go embeds. |

Bundled Go lives at `.tools/go/bin/go`; the Makefile falls back to system Go when missing. WASM builds rely on Go’s `wasm_exec.js`; Playwright harnesses require **Node 18+**.

---

## Repository Structure (Exhaustive Highlights)

```
src/go/            Go sources (command, core packages, internal subsystems)
  cmd/             Entry points (desktop/WASM)
  core/            Shared logic (beat scheduler, engine, predictor, graph)
  internal/        Ebiten UI, audio backends, assets, utilities
src/js/            WebAudio bridge, Playwright harnesses, WASM HTML
src/c/             Miniaudio DSP (compiled into static libs/WASM)
assets/            WAV samples (synced into Go embeds)
build/             Generated static libraries
```

### `src/go/cmd`
- `tunkul.go` — desktop/WASM entry wiring UI, engine, audio, CLI flags, optional `PPROF=1`.

### `src/go/core`

| Package | Key Files | Description |
|---------|-----------|-------------|
| `beat` | `sched.go`, `sched_test.go`, `scheduler_testshim.go` | BPM ticker, pause/resume, phase-preserving `SetBPM`. |
| `model` | `graph.go`, `graph_params.go`, `graph_traversal.go`, `node_types.go`, `graph_test.go` | Directed graph, traversal helpers, node parameters (volume/pitch/duration/logic/groove), loop detection, JSON export/import. |
| `engine` | `engine.go`, `predictor_*.go`, `predictor_*_test.go`, `engine_close_test.go` | Engine loop, event dispatch, concurrency-safe **predictor** (audible/visible/triggered buffers, `Ensure`, `UpdateNode`, `DeleteNode`, background horizon). |

> **Predictor (`predictor_*.go`) cheat sheet**
> - `Ensure(horizon)` grows buffers and recomputes when `predDirty`.
> - `UpdateNode` / `DeleteNode` refresh cached node parameters.
> - `VisibleAt` / `AudibleAt` / `TriggeredAt` expose per-row state for UI/audio.

### `src/go/internal`

| Directory | Purpose | Notable Files |
|-----------|---------|---------------|
| `assets` | Embedded JSON/WAVs | `default_demo.json`, generated go embeds. |
| `audio` | Desktop & WASM audio engines | Desktop: `engine_*.go`; WASM: `engine_wasm.go`; plus channel mixers, sample loaders. |
| `ebitestub` | No-op Ebiten replacements for tests | Stub renderer/game loop. |
| `gamestate` | Transport state machine | `state.go`. |
| `graphruntime` | Graph traversal runtime | `runtime.go`. |
| `log` | Lightweight logger | `log.go`. |
| `timeline` | Timeline commit service | `service*.go`. |
| `utils` | Shared helpers | `math.go`. |
| `ui` | Ebiten UI, DrumView, predictor integration, import/export, instrumentation, **extensive tests** | See breakdown below. |

#### `src/go/internal/ui` (selected highlights)

| File / Folder | Role |
|---------------|------|
| `game_*.go` | Core runtime split by ownership (input, graph edits, update loop, draw paths, sequencer/audio scheduling, parity). Entry glue lives in `game.go`. |
| `drumview_*.go` | Drum pane split by ownership (layout/menus, update/draw, caches, EQ/waveform). Base types live in `drumview.go`. |
| `timeline_alias.go` | UI-facing timeline types (re-exports `internal/timeline.Snapshot`). Timeline storage lives in `src/go/internal/timeline/service.go`. |
| `audio*.go` | UI-side audio helpers, lookahead tuning, highlight duration math. |
| `import*.go` / `export*.go` | JSON import/export and validation, Playwright shims. |
| `js_exports_*.go` | WASM bridge: exports `dumpRowState`, `dumpTimelineSegments`, `visibleAt`, `triggeredAt`, `setNodeLogicGrid`, `addNode`, `addEdgeGrid`, etc. |
| `game_instrumentation.go` | Shared instrumentation (`rowStateSnapshot`, `dumpRowState`, `dumpTimelineSegments`). |
| `assets/` | UI textures/icons. |
| **Tests** | 70+ files covering caching, audio, predictor, import/export (`drum_sync_invariant_test.go`, `drum_future_cache_test.go`, `game_draw_throttle_test.go`, etc.). |

#### Go Test Buckets
- **Predictor/audio**: `predictor_*_test.go`, `mute_node_audio_test.go`, `probability_prediction_consistency_test.go`.
- **DrumView invariants**: `drum_sync_invariant_test.go`, `drum_future_readd_test.go`, `drum_rows_layer_cache_test.go`.
- **Import/export**: `import_validation_test.go`, `import_button_flow_test.go`, etc.
- **Perf & caching**: `game_draw_throttle_test.go`, `perf_test.go`.

### `src/js`

| File / Area | Purpose |
|-------------|---------|
| `audio.js` | WebAudio queue: event batching, render cache (`ensureRenderedSample`), `resetPerfStats`, `getAudioScheduleMetrics` with 180s history. |
| `browser_test_helpers.js` | Shared Playwright helpers (e.g., fail tests on `recentSchedulerMismatches()`). |
| `drums.single.js` | Generated Emscripten synth DSP. |
| `drums.js` | Core DSP glue used in consistency tests. |
| `index.html`, `play_ui.html` | WASM harness pages for Playwright runs. |
| `wasm_exec.js` | Go WASM runtime shim. |
| **Playwright suites** | Comprehensive coverage: logic/circuit sync (`logic_sync.browser.test.js`, `circuit_sync.browser.test.js`), perf (`perf.browser.test.js`, `perf_e2e.browser.test.js`, `pan_stress.browser.test.js`), UI flows (`live_edit_scenario.browser.test.js`, `import_export.browser.test.js`), audio behaviors (`audio.browser.test.js`, `batch_audio.browser.test.js`, `wav_bus.browser.test.js`). |

### `src/c`
- `drums.c`, `miniaudio.c`, `miniaudio.h` — miniaudio DSP, compiled into static libraries/WASM modules consumed by Go/JS layers.

---

## Predictor, DrumView & Cache Interactions (Current Behavior)

1. **Node/logic update** → `Graph.SetNodeParams` → `Game` node-changed hook:
   - Determine affected row (`rowIndexForNode`).
   - Reset row logic state & predictor contexts (`predDirty`, `pathsDirty`).
   - Notify `engine.Predictor.UpdateNode/DeleteNode`.
   - If idle (`!g.playing`), call `refreshDrumRow()` immediately so DrumView matches predictor state.
2. **Timeline façade** (`internal/timeline.Service`) keeps immutable arrays for past/present/future. DrumView only stitches those slices; timeline commits happen via the service (`RecordCommitKind`, `ReplaceCommit`, `Trim*`, `Snapshot`).
3. **RowsLayer caching**:
   - `rowsLayerDirty` triggers rebuilds; otherwise the layer shifts using cached sprites/pads.
   - Stripe mode (`setRowsLayerStripes`) splits the layer into vertical stripes for WASM perf.
   - Simple-draw mode (default for WASM perf harnesses) skips heavy UI controls.
4. **Perf counters**: `Game` exposes recent timings through `perfStats()` (fps, update/draw averages, `drawGridMS`, `drawDrumMS`, `updateMS`, `refreshMS`, audio queue stats).
5. **Instrumentation**: `dumpRowState`, `dumpTimelineSegments`, `predictorAudibleSnapshot` are available in Go tests and JS harnesses.

---

## Testing & Diagnostics

### Go (`src/go/internal/ui`)
Run `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui` to cover:
- Predictor/audio invariants (`drum_sync_invariant_test.go`, `mute_node_audio_test.go`).
- DrumView caching (`drum_rows_layer_cache_test.go`).
- Import/export flows (`import_validation_test.go`, etc.).
- Perf throttling (`game_draw_throttle_test.go`).

### Browser (`src/js`)

| Harness | Focus | Key Exports / Checks |
|---------|-------|----------------------|
| `logic_sync.browser.test.js` | Probability/logic edits must clear stale steps | `setNodeLogicGrid`, `rowWindow`, `visibleAt`, `triggeredAt`. |
| `circuit_sync.browser.test.js` | Delete/re-add flows keep DrumView/predictor aligned | `addNode`, `deleteNodeGrid`, `rowWindow`. |
| `pan_stress.browser.test.js` | Pan performance (FPS ≥ 6, draw ≤ 20 ms) | `resetPerfStats`, `perfStats`. |
| `perf.browser.test.js` | Update-only performance | `buildPerfRect`, `startPlay`, perf counters. |
| `perf_e2e.browser.test.js` | Update+Draw+Audio perf | `resetPerfStats`, `resetAudioScheduleMetrics`, audio lag quantiles. |
| `drums_consistency.browser.test.js` | Synth vs WebAudio correlation | Captures `__samples` vs DSP reference. |
| `live_edit_scenario.browser.test.js` | Real-time editing (timeline dumps, predictor parity). | 

### Instrumentation & Metrics
- `resetPerfStats()` / `resetAudioScheduleMetrics()` ensure clean baselines before perf tests.
- `getAudioScheduleMetrics()` exposes lead/lag quantiles + history (180 entries).
- `dumpRowState(row)` returns timeline offset/masks and predictor buffers for debugging/tracing.
- `recentSchedulerMismatches()` / `clearSchedulerMismatches()` expose and reset the scheduler-vs-UI mismatch ring in WASM/Playwright harnesses.

### Debug Toggles (Env Vars)
- `PPROF=1` — start pprof server on `localhost:6060` (desktop).
- `TUNKUL_DEMO_CONFIG` / `TUNKUL_CONFIG` — choose a demo JSON file.
- `DEBUG_GEOM=1`, `DEBUG_DRAW_NODES=1` — verbose geometry / per-node draw logs.
- `RENDER_SAFE=1`, `SCREEN_EDGES=1` — render in screen space (debug caches).
- `NO_GRID_DRAW=1`, `NO_GRID_TILE_CACHE=1`, `NO_EDGE_CACHE=1`, `NO_SPRITE_NODES=1`, `NO_PIXEL_SNAP=1` — disable specific caches/draw paths.
- `PERF_LOG=1`, `PERF_FAST_PATH=1`, `PERF_BROWSER_UPDATE_MAX_MS=<any>` — enable perf logging / force fast-path refresh gating.
- `TIMELINE_TRACE=1`, `TIMELINE_TRACE_ROW=<row>`, `DEBUG_HISTORY_SEED=1` — timeline masking/seed tracing.
- `PARITY_FATAL=0|false` / `PARITY_WATCH=log|panic` / `PARITY_WASM_FATAL=1|true|panic` / `PARITY_DUMP_STDERR=1` — parity diagnostics.
- `TUNKUL_ROW_SNAPSHOTS=1|true`, `BPM_TIMING_TEST=1`, `PREVIEW_DEBUG=1` — test/debug-only modes.
- Deterministic combo: `SCREEN_EDGES=1` + `RENDER_SAFE=1`.

---

## Performance Notes

- Simple-draw mode (default for WASM perf harnesses) removes UI chrome, keeping draw ≤ ~15 ms on pan stress tests.
- Predictor background worker throttles lookahead when draw time crosses 10/14/18 ms thresholds.
- `audio.js` re-enqueues synth renders if the cache is cold, preventing silent leading samples in captures.
- Harnesses reset perf counters and audio metrics before measuring, avoiding warm-up artifacts.

### Test Logging
- Default Go test runs are **silent** (logger level `NONE`) so passing tests emit no logs.
- Opt in with `-test.v` or `TEST_LOG=1 make test` / `TUNKUL_TEST_LOG=1` (set `TEST_LOG_LEVEL` / `TUNKUL_TEST_LOG_LEVEL=TRACE|DEBUG|INFO|ERROR` to adjust verbosity). `make test` and `make test-debug` keep logs off unless you explicitly opt in.

---

## DrumView State Pipeline (Predictor → Sequencer → Timeline → UI Slate → Caches)

This is the authoritative reference for how *playback/audio truth* becomes the
final DrumView `Steps`/`CellTypes` UI slate, and how caches stay correct during
live edits.

**Key code paths**
- `src/go/internal/ui/game_*.go` (notably `game_sequencer_schedule.go`, `game_sequencer_highlight.go`, `game_sync_ui.go`, `game_refresh_drum_row.go`, `game_graph_update_beat_infos.go`): `seqScheduleTime`, `applySequencerHighlight`, `syncUIToTime`, `refreshDrumRow`, `updateBeatInfos`.
- `src/go/core/engine/predictor_*.go`: engine predictor buffers (`Ensure`, `SetPaths`, `VisibleAt`, `TriggeredAt`, `AudibleAt`).
- `src/go/internal/ui/preview/window_builder.go`: pure window construction (`preview.BuildRowWindow`) and precedence table.
- `src/go/internal/timeline/service*.go`: commit ring + immutable sidecar (`RecordCommitKind`, `ReplaceCommit`, `SeedFromWindow`, `TrimAfterPathChange`, `UpdateRowSegments`, `Snapshot`).
- `src/go/internal/ui/drumview_*.go` (notably `drumview_cache_*.go`): row sprite + rows-layer caches (`buildRowSprite`, `rowsLayerMaybeRebuild`, `rowsStripesMaybeRebuild`, `markRowDirty`, `markRowsShiftDirty`).

### Glossary (indices & boundaries)
- `abs`: absolute subdivision index (internal time unit). One beat is `grid.MaxDiv()` subdivisions.
- DrumView window: `Offset` (first visible `abs`) + `Length` (window size in subdivisions).
- `nextBeatIdxs[row]`: UI row boundary; `abs < nextBeatIdxs[row]` is considered past for that row.
- `seqNextIdxs[row]`: sequencer row boundary; used alongside `nextBeatIdxs[row]` to define the authoritative past boundary.
- “True past” boundary used by masking/immutability: `pastExclusive = max(nextBeatIdxs[row], seqNextIdxs[row])`.

### Ownership model (who is the source of truth?)
- **Graph / traversal**: `updateBeatInfos()` computes per-row paths (`beatInfosByRow`, loop facts). This is the only place the UI’s notion of “the path” should be rebuilt.
- **Predictor (engine-owned)**: `engine.Predictor` is concurrency-safe and owns predicted buffers:
  - `VisibleAt(row, abs)` for regular nodes (what the UI should show as ON).
  - `TriggeredAt(row, abs)` for mute nodes (mute “fires” even though it’s not audible).
  - `AudibleAt(row, abs)` for audio parity/debug (what would actually produce sound).
- **Predictor snapshots**: legacy UI-owned predictor snapshots were removed; tests/preview read directly from the engine predictor.
- **Timeline (history + masking)**: `timeline.Service` stores commits for *immutability/masking* and instrumentation dumps. It is not what DrumView renders from directly.
- **DrumView slate (render inputs)**: DrumView renders strictly from `drum.Rows[row].Steps` and `drum.Rows[row].CellTypes`. Keeping these correct + invalidating caches is the core responsibility of `refreshDrumRow()`.

### Runtime flow (audio → UI)
1. **Audio scheduling (sequencer truth)**
   - `seqScheduleTime()` advances `seqNextIdxs[row]` based on wall-clock time + applied BPM and schedules audio in small bursts to catch up.
   - It does *not* mutate DrumView rows directly; it emits highlight events onto `hlCh` so the UI thread can apply them safely.

2. **UI thread applies highlights (playhead + history commits)**
   - `Game.Update` drains `hlCh` and calls `applySequencerHighlight(row, idx, BeatInfo)` on the UI thread.
   - This is where `nextBeatIdxs[row] = idx+1` advances, and where timeline history is recorded:
     - `CommitKindPlayback` while playing (immutable history).
     - `CommitKindSeeded` while not playing (speculative bookkeeping).
   - Mute nodes are committed as “on” when triggered so rendering and parity stay consistent.

3. **UI catch-up when rendering lags**
   - `syncUIToTime()` re-anchors pulses/counters to the wall-clock timeline and *freezes newly passed indices* by recording playback commits up to the new target. This makes past immutability robust even if highlight events arrive late.

4. **Slate rebuild (`refreshDrumRow`)**
   - `refreshDrumRow()` is the authoritative “build the UI slate” step. It runs every frame during playback (unless perf-throttled) and is forced immediately by graph edits.
   - High-level responsibilities:
     - Ensure predictor horizon covers `Offset+Length` (and any lookahead needed).
     - If `pathsDirty`, call `engine.Predictor.SetPaths(...)` before reading predictor values so edits don’t render stale state.
     - Reconcile frozen ranges so speculative timeline entries cannot leak forward and mask re-added nodes.
     - Demote “leaked future” immutable commits: any Playback/Import commit at `abs >= pastExclusive` is demoted to `CommitKindReleased` (value/type preserved) so it stops masking future edits.
     - Build fresh `Steps`/`CellTypes` via `buildRowWindow` → `preview.BuildRowWindow` (pure, deterministic).
     - Publish timeline segments for instrumentation (`timeline.UpdateRowSegments`).
     - Compute `rowRenderSig(Steps, CellTypes)` and call `drum.markRowDirty(row)` only when render-relevant slate inputs changed.
   - **Row snapshot safety**: to prevent in-place mutation bugs, `refreshDrumRow` swaps in freshly built slices and reuses scratch buffers (or allocates fresh when `TUNKUL_ROW_SNAPSHOTS=1|true`).

### Single precedence table (window construction)
All window building must follow a single rule order (implemented in `preview.BuildRowWindow`):
1. **Immutable timeline commits** (`CommitKindPlayback`/`CommitKindImport`) for **true past only** (`abs < pastExclusive`).
2. **Freeze preservation**: when the window is stationary, preserve already-rendered past cells from the previous window snapshot (bounded so it can’t mask future edits).
3. **Predictor/live preview** for everything else (future window, or past cells with no immutable history).

Window construction must be timeline-pure. Any timeline mutation (demotion, trim, reconciliation) belongs in `refreshDrumRow()`.

### Edit flows (what must happen on graph changes)
- **Node param/type changes**: `Graph.SetNodeChangedHook` marks `predDirty` + `pathsDirty`, forces refresh under perf fast-path, and calls `drum.markRowDirty(row)` so edits that don’t change `Steps` still redraw immediately.
- **Circuit/path changes** (`updateBeatInfos()`):
  - Recomputes `beatInfosByRow` and loop facts.
  - Calls `engine.Predictor.SetPaths(...)` and optionally `RebaseAt(...)` to keep contexts aligned.
  - Seeds already-rendered history for changed paths (`timeline.SeedFromWindow`) so previously-rendered past stays stable.
  - Trims speculative commits after path edits (`timeline.TrimAfterPathChange`) and clamps `frozenUpToByRow` so future windows can follow the predictor again.
  - Marks affected rows dirty and refreshes immediately.

### DrumView caching layers (render performance & correctness)
DrumView has multiple caching layers; they all ultimately depend on the correctness of `Rows[i].Steps` and `Rows[i].CellTypes`:
- **Per-row sprite cache (`rowCache[i]`)**: a row-height sprite built from `Steps` + `CellTypes` + row color. It supports incremental reuse on small offset shifts by shifting the cached image and redrawing only the uncovered strip.
  - Invalidated via `markRowDirty(i)` (single row) or `markAllRowsDirty()` (full).
  - Offset shifts call `markRowsShiftDirty()` to allow incremental reuse rather than forcing a full rebuild.
- **Rows composite layer (`rowsLayer`)**: a cached composition of all visible row sprites. It also supports small horizontal shift reuse and uses an adaptive pad on WASM to reduce rebuild churn during pans.
- **WASM stripe mode (`rowsStripes`)**: optional vertical-stripe composition for browser perf; same invalidation rules but rebuilds smaller surfaces.
- **Simple-draw mode**: skips heavy UI chrome and uses lightweight highlight sprites; it still depends on the same slate + invalidation rules.

### Invariants (must hold)
- **Past immutability**: once `abs < pastExclusive` (or a cell is a Playback/Import commit), rendered `Steps` + `CellTypes` must never change; edits may only affect `abs >= pastExclusive`.
- **Realtime edits**: any graph edit must reflect in DrumView within ≤1 frame, including cache invalidation (even under perf fast-path).
- **Single precedence table**: window construction must follow the explicit rule order above; do not add scattered overrides.

### Open work (non-blocking)
- Predictor cache struct / mockable interfaces once legacy predictor mirrors are removable.
- Scenario DSL + golden timeline snapshots for regressions.
- Reusable image pool for `*ebiten.Image` row sprites.

### 2025-12-08 Fresh Gotchas (concurrency, mute, Playwright)
- `lastTriggeredByRow` is now mutex-protected. **Never write/read the map directly** in tests; use `setLastTriggeredForTest`, `lastTriggeredForTest`, or `lastTriggeredRowSnapshotForTest` on `Game`. Direct map access will race the sequencer and can panic.
- Predictor mute gate: engine predictor only gates the mute subdivision (`idx+hold+1`) so audio resumes immediately on the following beat; UI/audio mute tests assume this behavior.
- WASM/Playwright builds: when running Node harnesses that call `go build` (e.g., `mute_logic.browser.test.js`), pass an **absolute** GO path: `GO=/home/ymolinar/Repos/tunkul/.tools/go/bin/go node src/js/<test>.browser.test.js`. Relative `.tools/...` can fail because cwd becomes `src/go`.
- Parity on WASM stays log-only unless `PARITY_WASM_FATAL=1|true|panic`; desktop honors `PARITY_WATCH`/`PARITY_FATAL` as before.

---

## Maintenance Tips for Future Agents

- Use `.tools/go/bin/go` for all Go/testing tasks to avoid version drift.
- Use `.tools/go/bin/gofmt` if system `gofmt` is unavailable.
- When splitting long Go files, prefer the deterministic splitters (avoid manual cut/paste):
  - UI: `go run scripts/split_ui_phase1.go`
  - Audio/timeline: `go run scripts/split_phase1_longfiles.go`
- Split by subsystem ownership; avoid micro-files unless they isolate a single hot path (e.g., a giant draw/update function).
- After modifying predictor/DrumView/caching logic, run:
  - `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/timeline ./internal/ui`
  - `GO=$(pwd)/.tools/go/bin/go node src/js/logic_sync.browser.test.js`
  - `GO=$(pwd)/.tools/go/bin/go node src/js/circuit_sync.browser.test.js`
- For perf regressions, compare `perf.browser` vs `perf_e2e` outputs (post `resetPerfStats`).
- Inspect `dumpRowState` / `dumpTimelineSegments` via browser console for timeline anomalies.
- Keep generated WASM (`src/js/main.wasm`) out of commits — use `make wasm` locally only.

### 2025-12-05 Parity & Sequencer Gotchas
- Parity scans now ignore beats before the playhead (`playheadFloor`) but still run while paused; past audio events no longer demand highlights.
- `parityCheck` only runs when parity fatal is enabled (`PARITY_FATAL`) and skips mute nodes; highlight parity lives in `parityScan`.
- `parityCheck` also skips cells when the predictor marks them invisible (logic/probability gated) even if a slate cell exists—this avoids panics when logic mutes a note but the authored slate remains true.
- Past audio events stored in `parityAudio` are ignored once the playhead advances; tests (`parity_past_audio_test`) cover this.
- `audio_missing` parity now gives a short 120 ms grace after a sequencer decision before panicking, so races between decision logging and audio logging don’t produce false positives; after the grace, any missing audio still panics even if the playhead hasn’t advanced.
- Scheduler parity ignores beats strictly older than `playheadFloor()-1` (i.e., more than one step behind the next beat) to avoid past-state panics while still checking the current beat; see `parity_past_scheduler_test`.
- During imports `g.importing` disables parity scans/checks and clears parity buffers (ring/audio/seq decisions); `renderReady` is temporarily false. This prevents import-time refresh parity panics.
- Perf counter test (`TestDesktopPerfCountersCollect`) needs sequencer enabled; leave it on for desktop runs.
- Quick sanity: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui`.
- Sequencer parity expectations come from `parityExpected` (engine predictor); `seqScheduleTime` calls `Predictor.Ensure(idx+1)` before parity bookkeeping.
- ParityCheck allows a one-beat grace: if the sequencer just scheduled idx and slate hasn’t refreshed yet (`idx == seqNextIdxs[row]-1`), the mismatch is ignored to avoid false positives during tight render/refresh loops.
- On WASM/Playwright runs parity fatal is downgraded to log-only by default; set `PARITY_WASM_FATAL=1|true|panic` to re-enable panics. Desktop/CLI still honor `PARITY_WATCH`/`PARITY_FATAL` normally.
- Mute gate: engine predictor now gates only the mute subdivision (`idx+hold+1`), so audio resumes immediately on the following beat; UI mute/audio tests expect this.

## 2025-11-28 WebAudio/WASM Integration Notes (critical)

- **Sample-rate correctness**: WebAudio contexts often default to 48 kHz. We now render synth buffers at `AudioContext.sampleRate` (see `ensureRenderedSample` in `src/js/audio.js`). If you change render lengths/amps, preserve the dynamic SR or you’ll get pitch/tempo drift and “chipmunk” timbre in browsers.
- **Render cache invalidation**: Cached renders include `sr`/`frames`; a mismatched `sr` triggers re-render. Tests read `window.__renderMeta` for verification.
- **Main bus wiring**: `rewireChannel` special-cases the main channel (ingress==gain) to connect `(eq?)->(analyser?)->destination` and avoid feedback/silence. Preserve this when adding EQ/analyser nodes.
- **Analyzer/waveform exports**: `channelAnalyzerSnapshot` now returns `wave` (clamped time-domain) plus `spectrum`. DrumView waveform uses this; JS/Go tests tap it.
- **EQ bands & controls (2025-11-29)**:
  - DrumView EQ panel shows 10 fixed bands (Hz ranges 20–20k) with sliders (±12 dB, 0.1 dB steps). Alternating band backgrounds + Hz labels. Waveform toggle still available.
  - Applying EQ re-attaches the analyser so waveform/spectrum stay live after EQ changes.
  - Go tests: `drumview_eq_columns_test.go`, `drumview_eq_controls_test.go`, `drumview_eq_wave_after_eq_test.go`.
  - JS tests: `eq_panel_columns.browser.test.js`, `eq_controls_wave.browser.test.js`.
  - JS exports: `eqBandsSnapshot()` (values + Hz labels), `eqControlsSnapshot()` (current gains).
- **Tests to run for audio regressions**:
  - `cd src/js && node audio_render_rate.browser.test.js` (validates render SR/duration == AudioContext SR).
  - `cd src/js && node audio_presence.browser.test.js` (WASM harness proves audible waveform on main bus).
  - `cd src/js && node audio_cache.browser.test.js` (render cache semantics).
  - For Go desktop path: `cd src/go && go test ./internal/audio -tags test` (uses Ebiten stubs).
- **Context exposure**: `window.__audioCtx` / `__audioCtxSR` are set for diagnostics; leave in place for Playwright assertions.
- **When debugging “no sound in browser”**: Check (1) AudioContext resumed, (2) render cache built at the correct SR, (3) main bus analyser not creating a loop, (4) wasm module loaded (`drums.single.js`). The presence test spins a mini static server to verify end-to-end.
