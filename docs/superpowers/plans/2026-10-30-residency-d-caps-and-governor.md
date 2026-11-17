# Residency D — Caps and the Budget Governor: Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** circuits are bounded by measured caps that every entry point enforces. Memory budgets come from the device class. When audio health drops, the game sheds UI work in priority order and never touches note timing.

**Architecture:**
- A pure package `internal/budget` holds the one table: caps (one value for every device), device classes and per-class budgets. Both `internal/ui` and `internal/audio` read it.
- An audio-health reading combines lane lead, render waits, main-thread stalls and audio-thread load.
- A step-down ladder with hysteresis acts on it: pause background pools → lower the draw rate → simplify visuals.
- Background Go work runs in time slices. The browser's render workers serve renders by priority.

**Tech Stack:** Go 1.26 (desktop + js/wasm, Ebiten), audio.js / AudioWorklet, Playwright in real Chrome.

**Spec:** `docs/superpowers/specs/2026-10-25-instrument-residency-and-resource-budgets-design.md` §6, §7, §9.3, §10 phases 4–5. Runs after Plans A–C. Its numbers come from `bench-results/residency_after_c_2026-10-30.md` (Plan C Task 5), because the caps must be measured on the residency build.

**Not planned:**
- Effect delay compensation (spec §8): the user has not approved it.
- Menu prefetch (spec §5.6): planned separately only if Plan C's report shows cold pick-to-sound p95 above 50 ms.

## Global Constraints

- Never stage or commit. Never `git stash`. Use `.tools/go/bin/go`.
- **Caps:**
  - one value for every device, sized for the **lowPhone** profile;
  - cap = `max(floor, floor(0.8 × largest passing lowPhone step))`;
  - floors: 12 instruments (templates), the most effects on any shipped row, 900 nodes (`TestTemplatesAreReasonablySized`);
  - when the measurement falls below a floor, the floor wins and the report names it as an engine/profile problem;
  - each constant carries a comment citing the report file and its step.
- **Never silent:** the control that would exceed a cap says so, and an import over a cap is refused with a message naming it, before anything resets.
- **Never shed `audio` or `timing`:** the ladder only touches background work, the draw rate and visuals.
- UI changes are verified by an inspected screenshot (desktop + `MOBILE=1`). Real input via `Game.Update`. Every test Game is closed.
- Timing claims are measured in real Chrome on a private wasm after a load check. No allowlists.

## Review Focus

1. **An import exactly at a cap, and one over it.** Expected: at the cap it is accepted; one over is refused with the message, and the current circuit is untouched. Test: Task 1 `TestEveryCapEntryPathRefuses` import rows.
2. **Undo or redo back to a state the caps would refuse as a new import.** Expected: always restored (undo is not an import). Test: Task 1 `TestUndoIsNeverRefusedByACap`.
3. **Recording while the ladder steps down.** Expected: the recording is complete and unaffected. Test: Task 4 `TestStepDownNeverTouchesRecording`.
4. **A background tab** (rAF stops, health reads critical). Expected: on return the ladder climbs back to level 0 after the hysteresis period. Test: Task 4 `TestHealthRecoversAfterABackgroundTab`.
5. **Firefox or Safari, where `navigator.deviceMemory` is undefined.** Expected: device detection falls back to its floor and never crashes or picks the laptop class on a phone. Test: Task 2 `TestDetectDeviceClass` rows.

---

### Task 1: Caps table and every entry point

**Files:**
- Create `internal/budget/caps.go` (pure, no imports beyond std).
- Modify the entry points:
  - `internal/ui/drumview_rows.go` (`AddRow` refuses) and `row_rack_zone.go:329-333`: the Add-instrument strip shows "Limit: N instruments" (`i18n.KeyInstrumentLimit`) and is inert.
  - `js_exports_graph_ui.go:670` (`addRow` returns false).
  - `instrument_pick.go`: forking past the cap → the card offers only "Use X's".
  - `project_file.go`: `validateProjectCaps`, called in `Import` before any reset and skipped while `restoringUndo()`.
  - `internal/audio/effect_chain.go`: `AddInsertEffect` returns -1 at `budget.MaxInsertsPerBundle`.
  - `drumview_fx_panel.go:1056-1098`: the Add Effect row is disabled with `KeyEffectLimit`.
  - `js_exports_insert_effects.go:43`.
  - `game_graph_nodes.go:25`: `tryAddNode` refuses at `budget.MaxNodesPerCircuit` with one `notifyError(KeyNodeLimit)` per gesture.
  - `node_clipboard.go:115`: a paste is all-or-nothing.
  - `path.go:26` and `grid_guide.go:274`.
- Scenes: `caps_add_instrument_limit`, `caps_add_effect_limit`, `caps_node_limit_notice`.
- Tests: `internal/budget/caps_test.go`, `internal/ui/caps_entry_points_test.go`, `cmd/gen-showcase` `TestEveryTemplateFitsTheCaps`.

**Interfaces — Produces:**
- `const MaxInstrumentsPerCircuit`, `MaxInsertsPerBundle`, `MaxNodesPerCircuit`, set from Plan C's report by the rule above.
- `func CircuitInstrumentCount(ids []instrument.ID) int`: distinct ids, instances included.
- `type CapError struct{ Cap string; Limit, Got int }`, with `Error()` returning the user message.
- If `MaxInstrumentsPerCircuit` exceeds `analyzer.Config.MaxInstruments` (32), raise that to match in the same task.

- [ ] Tests:
  - `TestCapsTableIsComplete` / `TestCapsRespectTheirFloors`.
  - `TestEveryCapEntryPathRefuses`, table-driven through real input or the JS exports:
    - add-row button and JS `addRow` at the cap;
    - import at the cap (accepted) and one over (refused, the export byte-identical before and after, `CapError` text shown);
    - a fork pick at the cap;
    - the Add Effect button and JS `addInsertEffect` at the cap;
    - a node tap, a path draw and a paste at the cap;
    - an import with too many nodes.
  - `TestUndoIsNeverRefusedByACap`.
  - `TestEveryTemplateFitsTheCaps`.
- [ ] Implement. Screenshots of the three scenes, desktop + mobile, opened: the limit text is legible, the control visibly inert, nothing overlaps.

### Task 2: Device class and memory budgets

**Files:**
- Create `internal/budget/device.go` and `internal/budget/budgets.go`.
- Create `internal/ui/hardware_info_js.go` (reads `navigator.deviceMemory`, `hardwareConcurrency` and the mobile layout) and `internal/ui/hardware_info_notjs.go` (`runtime.NumCPU`; `/proc/meminfo` `MemTotal` on Linux, 0 elsewhere).
- Modify:
  - `cmd/runtime_defaults_wasm.go`: the flat 1 500 MB limit becomes the device budget, applied after detection with `debug.SetMemoryLimit`; `BEATMO_MEMORY_LIMIT_MB` still wins.
  - `internal/ui/game_new.go`: apply the voice budget (`audio.SetVoiceBudgetBytes`, Plan C), the undo byte limit and the synth-graph cache cap.
  - `internal/ui/undo.go`: `UndoManager.maxBytes` trims the oldest entries until the snapshot bytes are within it, besides `maxDepth` 100.
  - `runtime_profile.go`: `VoiceCacheBudgetBytes` is removed, since the budget now comes from `internal/budget`.
- Tests: `internal/budget/device_test.go`, `internal/ui/budget_apply_test.go`, `internal/ui/undo_bytes_test.go`.

**Interfaces — Produces:**
- `type HardwareInfo struct{ DeviceMemoryGB float64; Cores int; Mobile bool; TotalMemoryMB int64 }`.
- `type DeviceClass int`, with consts `DeviceLowPhone`, `DeviceMidPhone`, `DeviceLaptop`.
- `func Detect(hw HardwareInfo) DeviceClass`. Unknown values fall to the class floor:
  - mobile with memory ≤ 2 GB or unknown, or ≤ 4 cores → LowPhone;
  - other mobile → MidPhone;
  - desktop with ≤ 4 GB or ≤ 2 cores → MidPhone;
  - else Laptop.
- `type Budget struct{ VoiceCacheBytes, UndoBytes, RecordingBytesPerChannel int64; GoMemoryLimitMB int64; SynthGraphCacheEntries int }`.
  - `RecordingBytesPerChannel` replaces the fixed 128 MB `BYTES_PER_CHANNEL_MAX`; Go pushes it in the recording worker's configure message.
  - Analyzer and scope buffers are fixed-size per Live channel and bounded by `MaxInstrumentsPerCircuit`, so they are reported in the ledger, not budgeted.
- `func For(c DeviceClass) Budget`. Each byte budget = 1.5 × that class's peak for the class in Plan C's report at the 12-row reference circuit, rounded up to 4 MB; `GoMemoryLimitMB` = 2 × the peak Go heap, rounded up to 64 MB.
- `func (u *UndoManager) SetMaxBytes(n int64)`.

- [ ] Tests:
  - `TestDetectDeviceClass`, rows including undefined deviceMemory on mobile → LowPhone, unknown desktop → Laptop, 2 cores → MidPhone;
  - `TestBudgetForEveryClass`: complete, and monotonic low ≤ mid ≤ laptop;
  - `TestUndoTrimsByBytes`;
  - `TestStartupAppliesTheBudget`: a fake HardwareInfo sets the voice budget and the memory limit; the env override wins.
- [ ] Extend Plan A's `stepPasses` with the budget gate (spec §9.3 "ledger within budget"): when the page reports a budget, every budgeted class's ledger bytes stay ≤ its budget. Add a self-test case.
- [ ] Implement; run `make bench-load DIMENSION=library` on lowPhone and confirm the ledger stays within the class budget at every step.

### Task 3: Audio health reading

**Files:**
- Create `internal/ui/audio_health.go`.
- Modify:
  - `audio_lane.go`: track committed-through minus now each tick; `MinCommittedLeadSec` over the last 1 s.
  - `internal/audio/engine_stop.go`: time `mixer.Read` against the buffer duration; an atomic EWMA `MixerLoad()` and an `MixerOverruns()` count.
- Modify `src/js/audio.js`:
  - `renderWaitsInHorizon`: notes inside the horizon whose render was not ready at `processAudioEvent`.
  - `audioThreadLoad`: `ctx.renderCapacity` averageLoad where it exists; otherwise a `beatmo-load-meter` worklet, a new `src/js/load_meter_worklet.js`, reporting the fraction of quanta in the last second whose callback gap exceeded 1.5 × the quantum duration. It ships, so add it to `ASSET_FILES` in `scripts/deploy-wasm-gcs.sh` (`TestDeployManifestCoversRuntimeRefs`).
  - Both go into `audioResourceStats`.
- Modify `perfStats` (`js_exports_playback_perf.go`): add `audioHealth` and `stepDown`.
- Tests: `internal/ui/audio_health_test.go`, `internal/audio/mixer_load_native_test.go`, `src/js/audio_health.browser.test.js`.

**Interfaces — Produces:**
- `type AudioHealth struct{ LeadMinSec float64; RenderWaits int; MainStallMaxMS float64; MixerLoad, AudioThreadLoad float64 }`.
- `type HealthLevel int`, with consts `HealthOK`, `HealthStrained`, `HealthCritical`.
- `func (h AudioHealth) Level(horizonSec float64) HealthLevel`:
  - Critical when `LeadMinSec < 0.25×horizon`, or a load ≥ 0.85, or `RenderWaits ≥ 3`;
  - Strained when `LeadMinSec < 0.5×horizon`, or a load ≥ 0.7, or `MainStallMaxMS ≥ 100`, or `RenderWaits ≥ 1`;
  - else OK.
- `func (g *Game) audioHealth() AudioHealth`, refreshed every frame from cached sources (JS stats on Plan A's 500 ms TTL).
- `func (l *audioLane) MinCommittedLeadSec() float64`.

- [ ] Tests:
  - `TestHealthLevelThresholds`: a table that hits each boundary.
  - `TestLaneMinCommittedLead`: fake clock; a 600 ms stall drops the lead accordingly.
  - Native `TestMixerLoadIsMeasured`.
  - `audio_health.browser.test.js` (chrome-local):
    - `__startAudioBurner(0.9)` → `audioThreadLoad ≥ 0.7` within 3 s;
    - the isolation stall injector at [700, 300] → `MainStallMaxMS ≥ 250`;
    - idle → `HealthOK`.
- [ ] Implement.

### Task 4: Step-down ladder, background pause, draw rate, simpler visuals, time slices

**Files:**
- Create `internal/ui/step_down.go`.
- Modify `internal/async/pool.go`: `Pause()` / `Resume()` / `Paused()`. A paused pool keeps accepting `Submit` up to its queue, starts no job, and `Close` still drains.
- Create `internal/async/slicer.go`.
- Modify:
  - `internal/ui/synth_graph_pipeline.go` and `internal/soundview` (the yields go through a `Slicer`);
  - `voice_prewarmer.go`: beyond-horizon jobs carry `background=true` and are re-queued while paused; lane `warmPitch` jobs never pause;
  - `cmd/beatmo.go`: `ebiten.SetScreenClearedEveryFrame(false)` once, so a skipped Draw keeps the last frame;
  - `internal/ui/game_draw*.go`: `Draw` returns early when `skipDraw(frame, level)`;
  - `audio_panel_dispatcher.go`: at level 3, live analyzers and the scope are off, with a "Paused to protect the audio" chip (`i18n.KeyVisualsPaused`) on the audio tabs;
  - node glow is off at level 3.
- Modify `src/js/audio.js`:
  - render-worker priority: one job in flight per worker and a pending queue ordered by `priority`;
  - `renderVoiceInWorker(id, opts, priority)`;
  - priorities: 0 = a note within the horizon, 1 = a pending switch's warm-ups (Plan C, through `warmPitch`), 2 = other warm-ups, 3 = graph renders (`renderDocPCM`);
  - the Go warm bridges pass the priority.
- Scene: `visuals_paused_level3` (desktop + mobile).
- Tests: `internal/ui/step_down_test.go`, `internal/async/pool_pause_test.go`, `internal/async/slicer_test.go`, `src/js/render_priority.browser.test.js`.

**Interfaces — Produces:**
- `type stepDown struct{ level int; badSince, okSince time.Time }`.
- `func (s *stepDown) Update(l HealthLevel, now time.Time) int`. The hysteresis defaults are left to the builder's tuning, starting at:
  - step down one level after 0.5 s Strained, or straight to level 2 when Critical;
  - step up one level after 3 s OK.
- `func skipDraw(frame uint64, level int) bool`: level 2 draws every 2nd frame (30 fps), level 3 every 3rd (20 fps, the floor).
- `func NewSlicer(budget time.Duration) *Slicer`, `func (s *Slicer) Yield()`. The budget is 4 ms at level 0 and 2 ms at level 1+.
- Level 1+ pauses `synth.preview` and the prewarmer's background jobs.

- [ ] Tests:
  - `TestStepDownOrderAndHysteresis`.
  - `TestPausedPoolKeepsItsQueue`, `TestPausedPoolCloseStillDrains`.
  - `TestSlicerYieldsAtItsBudget`.
  - `TestDrawSkipsAtLevelTwoAndThree`: Draw bodies executed over 60 frames = 60 / 30 / 20.
  - `TestStepDownNeverTouchesRecording`: real input, record while forcing level 3 → the recording WAV duration equals the played time and its sample count is complete.
  - `TestHealthRecoversAfterABackgroundTab`: fake clock; 10 s Critical, then OK → level 0 after the hysteresis.
  - `render_priority.browser.test.js`: queue 40 warm-ups, then one horizon render → it completes before at least 30 of the warm-ups.
  - Load runs (chrome-local, lowPhone):
    - `instrumentsFx` and `edit` at every step inside the caps: zero late, missed or dropped notes;
    - `perfStats().stepDown ≥ 1` was reached at least once;
    - FPS ≥ 20.
- [ ] Implement. Screenshot `visuals_paused_level3` (desktop + mobile), opened. Run `visual_regression.browser.test.js` to confirm level-0 drawing is unchanged by `SetScreenClearedEveryFrame(false)`.

### Task 5: Final runs and docs

- [ ] `make bench-load` for every dimension × {laptop, midPhone, lowPhone}, plus the residency soak. Write `bench-results/residency_final_<date>.md` comparing Plan A's baseline, Plan C's report and this build, with breaking points before and after.
- [ ] Update CLAUDE.md:
  - a new "Resource budget" section: the caps table with its sources, device classes, budgets, the health reading, the ladder, render priorities and the guards;
  - the CLAUDE.md doc-drift fix (`drawMinIntervalMS` does not exist; the ladder is the draw-rate control);
  - DESIGN/i18n entries for the new strings.
- [ ] Set the spec status to "built"; update the memory note.
- [ ] Full suites: `go test -tags test -modfile=go.test.mod ./...`, `make test-real`, `make js-audit`, the browser suites this plan and Plans B–C touched.
