# Residency C — Lazy Residency and the Switch Rule: Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** an instrument holds a channel only while a row selects or sounds it, holds rendered voices only within a memory budget, and a switch changes the sound at the first note the new instrument can play on time.

**Architecture:**
- A residency manager in `internal/audio` owns the Known / Warm / Live transitions.
- The UI tells it which instruments rows hold. It materialises a channel from the bundle on acquire (Plan B `materialiseChannel`) and releases it after its last voice ends, on both platforms.
- Descriptions reach JS only when an instrument becomes Warm.
- The JS render cache and the desktop voice cache evict least-recently-used non-Live voices under a byte budget.
- A row keeps `selected` (what the UI shows and saves) and `sounding` (what the lane plays). The lane switches a row's sound only once the new voices are ready.

**Tech Stack:** Go 1.26 (desktop + js/wasm), audio.js, Playwright in real Chrome.

**Spec:** `docs/superpowers/specs/2026-10-25-instrument-residency-and-resource-budgets-design.md` §5.1, §5.3–§5.5, §7.3 (late notes), §9.3–§9.5, §10 phases 2–3. Runs after Plans A and B (`2026-10-30-residency-a-measure.md`, `2026-10-30-residency-b-audio-bundles.md`).

## Global Constraints

- Never stage or commit. Never `git stash`. Use `.tools/go/bin/go`. Produce red builds with `go build -overlay`.
- **Stay green after every task:**
  - export→import audio identity (`make test-real`);
  - `xplat_audio_stages.browser.test.js`;
  - `audio_lane_e2e.browser.test.js`;
  - `webaudio_live_edit_budget.browser.test.js`;
  - `TestLaneHeardEqualsLegacyDispatched`.
- No note may be cut, dropped or start late because of a release, an eviction or a switch. That is the gate in every browser test here.
- Timing claims are verified in real Chrome (`TEST_PLATFORM=chrome-local TEST_HEADED=1`) on a private wasm, after a `/proc/loadavg` check. The runner is Plan A's `webaudio_load_steps`.
- Sample PCM (user samples) is description: never evicted while anything references it.
- Every test Game is closed. Real input via `Game.Update` for UI paths.
- No allowlist exceptions.

## Review Focus

1. **Switching a row away while its old instrument still rings** (a long release). Expected: no cut; the old channel is released after its last voice. Test: Task 1 `TestReleaseWaitsForRingingVoices` + `residency.browser.test.js` long-release case.
2. **A→B→A within one note during playback.** Expected: no doubled note; the pending switch is cancelled. Test: Task 4 `TestSwitchBackBeforeReadyCancels` + the `switch` load step at 8/s.
3. **Undo or redo while a switch is pending.** Expected: the pending switch is dropped and sounding equals the restored selection. Test: Task 4 `TestUndoDuringPendingSwitch`.
4. **Import while notes still ring.** Expected: the old circuit's channels are released after their voices end, and no DSP node is orphaned. Test: Task 1 `residency.browser.test.js` import case (`dspNodesLive` returns to the Live count).
5. **A tiny voice budget combined with a switch storm.** Expected: evicted voices re-render cold, yet no note is late, because the switch rule waits for them. Test: Task 3 `voice_budget.browser.test.js` switch case.

---

### Task 1: Residency manager and release on both platforms

**Files:**
- Create `internal/audio/residency.go` (no build tags).
- Create `internal/audio/residency_release_desktop.go` (`!test && !js`).
- Create `internal/audio/residency_release_wasm.go` and `internal/audio/residency_stub.go` (`test`).
- Modify:
  - `channels.go`: `releaseInstrumentChannel`.
  - `engine_mixer.go`: free the `instSlots` entry; a per-channel active-voice count kept by the mixer.
  - `internal/analyzer/service.go`: `UnregisterInstrument(idx int)`.
- Modify `src/js/audio.js`:
  - `window.releaseChannel(id)`: disconnect and dispose `fxNode` / `eqNode`, the analysers and the probe tap, then delete the `channelNodes` entry. When `activeSources` or `laneVoices` still hold a source for `id`, set `chain.releasePending` and finish on the last `ended`.
  - `audioResourceStats` gains `channelsReleasing`.
- Modify `internal/ui`: create `residency_sync.go`, which runs `(g *Game) syncResidency()` every `Update` with a reused slice and calls `SetHeldInstruments` only when the set changed.
- Tests: `internal/audio/residency_test.go`, `internal/ui/residency_sync_test.go`, `internal/audio/residency_native_test.go` (`!test && !js`), `src/js/residency.browser.test.js`.

**Interfaces — Consumes:** Plan B `materialiseChannel(id)`, `BundleFor`.

**Interfaces — Produces:**
- `type ResidencyState int`, with consts `ResidencyKnown`, `ResidencyWarm`, `ResidencyLive`.
- `func Residency(id instrument.ID) ResidencyState`.
- `func SetHeldInstruments(ids []instrument.ID)`: acquire the new ones (`materialiseChannel` + `platformChannelAcquire`) and release the dropped ones (`platformChannelRelease`, deferred while voices ring).
- `func HeldInstruments() []instrument.ID`.
- `func ResidencyTick()`: retries deferred desktop releases; the UI calls it from `syncResidency`.
- Hooks `var platformChannelAcquire, platformChannelRelease func(id channel.ID)`.
- The UI side: held = every row's selected instrument ∪ the instruments rows still sound (Task 4).
- An instrument played with no hold (a test `playSound`) gets a channel ad hoc, which is released at the next sync.

- [ ] Tests:
  - `TestResidencyStateMachine`: Known → Live on hold; Live → Warm on release; re-acquire materialises the same bundle (`BundleFor` unchanged, the platform receives the same chain).
  - `TestReleaseWaitsForRingingVoices` (native): a voice is active → the channel stays until it ends, then the mixer slot and the analyzer slot are free.
  - `TestEvictRebuildIsByteIdentical` (native): render, release, re-acquire, render → identical samples. Added to the export→import identity harness as an evict-rebuild step.
  - `TestRowDeleteReleasesItsInstrument` (real input).
  - `TestImportReleasesWhatTheNewCircuitDoesNotUse`.
  - Browser `residency.browser.test.js` (startup demo, private wasm):
    - switch row 0 through 20 shipped instruments during playback → at every 250 ms sample, `channelChains ≤ 1 + rows + channelsReleasing`;
    - a long-release instrument switched mid-note → the probe shows its note heard to the end (no cut);
    - import of the test fixture while playing → `dspNodesLive` equals the fixture's non-neutral stages 2 s later;
    - throughout: probe `missed == 0`, `dropped == 0`, `late.over20ms == 0`.
- [ ] Implement; run the audio, ui and native suites, then the browser test (headless structural, then chrome-local headed).

### Task 2: Descriptions reach JS lazily; sample PCM loads on first use

**Files:**
- Modify:
  - `internal/audio/instrument_docs_wasm.go`: track the pushed revision per id. `ResetInstrumentDocs` and `SetInstrumentDoc` push only ids already pushed; others are marked stale.
  - `instrument_docs.go`.
  - `voice_warm*.go`: `WarmInstrument` ensures the push first.
  - The wasm `PlayBatch` path: ensure the push for each distinct id in the batch before calling JS.
  - `residency.go`: Acquire ensures the push.
  - `internal/ui/js_exports_synth_recipe.go:240`: the boot push becomes `PushRowInstrumentDocs(rowIDs)`.
  - `user_samples.go` / `ApplySavedSamples`: register availability only; `EnsureInstrumentLoaded` and Acquire load the PCM from the store.
- Tests: `internal/audio/lazy_blocks_test.go` (`-tags test` with a counting platform hook), `internal/ui/lazy_blocks_test.go`.

**Interfaces — Produces:**
- `func EnsureBlockPushed(id instrument.ID)` (a no-op off wasm).
- `func PushRowInstrumentDocs(ids []instrument.ID)`.
- `func EnsureUserSampleLoaded(id instrument.ID) bool`.

- [ ] Tests:
  - `TestBootPushesOnlyRowInstruments`;
  - `TestPlayBatchPushesABlockBeforeItsFirstNote`;
  - `TestAnEditedDocumentRepushesOnlyIfPushed`;
  - `TestUserSamplePCMLoadsOnFirstUse`: after boot, `LedgerSamplePCM.Bytes == 0`; after a pick, it equals the sample.
  - Browser: `resource_stats.browser.test.js` gains "after boot `instrumentBlocks ≤ rows + 1`".
- [ ] Implement. `synth_save_export_import.browser.test.js` and `wasm_bridge_smoke` stay green (they exercise blocks).

### Task 3: Voice memory budget and LRU eviction

**Files:**
- Modify `src/js/audio.js`:
  - `renderCache` records gain `bytes`, `lastUse` (touched in `playedBufferFor` / `processAudioEvent`) and `refs` (counted by `startRenderVoice`, released on `ended`).
  - A running `renderCacheBytes`.
  - `window.setVoiceBudget(bytes)` and `window.setLiveInstruments(ids)`.
  - `evictToBudget()` runs after each insert. It walks records by ascending `lastUse` and skips Live ids, `refs > 0` and `samplePCMIds`; `rawRenderCache` entries of evicted ids go too.
  - Spec §7.2 says Go makes eviction decisions. Here Go owns the POLICY (the budget and the never-evict set) and audio.js applies it to the records only it can see (per-record use times). Mirroring those to Go every frame would cost more than it decides.
  - `audioResourceStats` gains `voiceBudget` and `evictions`.
- Modify `internal/audio`:
  - `voice_budget_wasm.go`: push the budget at boot from `RuntimeProf().VoiceCacheBudgetBytes` (Plan D replaces this with the device budget), and the Live set on every residency change.
  - `voice_cache.go`: `lastSamples` bytes count in the budget and are dropped with their instrument's entries; `SetEvictionGuard(func(instrument.ID) bool)`, which residency sets to "is Live".
- Tests: `internal/audio/voice_cache_budget_native_test.go` (`!test && !js`), `src/js/voice_budget.browser.test.js`.

**Interfaces — Produces:**
- `func SetVoiceBudgetBytes(n int64)` (both platforms).
- `func SetEvictionGuard(fn func(instrument.ID) bool)`.

- [ ] Tests:
  - `TestVoiceCacheNeverEvictsLiveInstruments`;
  - `TestLastSamplesCountInTheBudget`;
  - `voice_budget.browser.test.js`, with a 4 MB budget, switching row 0 through 30 instruments while playing:
    - at every sample, `renderBytes ≤ budget + bytes of Live instruments' records`;
    - `evictions > 0`;
    - no sample PCM is evicted;
    - probe `missed == 0`, `late.over20ms == 0`.
- [ ] Implement; run the native and browser tests.

### Task 4: The switch rule (selected vs sounding)

**Files:**
- Create `internal/ui/row_switch.go`.
- Modify:
  - `drumview_transport.go`: `SetInstrument` creates or cancels a pending switch while playing; when stopped it switches at once.
  - `audio_lane_note.go:107`: `laneNoteAt` reads `soundingAt(row, start)` instead of `Rows[row].Instrument`.
  - `audio_lane_source.go`: `rowNonVoiceKey` includes the pending switch's `to` and `readyAt`.
  - `audio_lane_wire.go`: `advanceSwitches` runs in `laneTick` before the reconcile.
  - `game_played_snapshot.go`: `onDrumRowRemoved` shifts and drops pending switches.
  - `residency_sync.go`: held includes sounding.
  - `import.go`: a real or undo import clears pending switches.
- Modify `internal/audio`: create `voices_ready.go` (desktop: the voice cache holds every (id, pitch) under `docVoiceKey`) and `voices_ready_wasm.go` (JS `voiceCachedFor` per pitch), plus a `test`-tag stub settable by tests.
- Modify `src/js/audio.js`: in `processAudioEvent`, a deferred note whose render lands after `intendedWhen + LateToleranceSec` is dropped and counted (`audioResourceStats().lateDropped`) instead of starting late. Within tolerance it plays (spec §7.3). Go pushes the tolerance with `window.setLaneLateTolerance(sec)` at boot.
- Tests: `internal/ui/row_switch_test.go`, a new case in `src/js/audio_lane_cancel.browser.test.js`.

**Interfaces — Consumes:** Plan B `applyPick` (it calls `SetInstrument`); Task 1 `SetHeldInstruments`.

**Interfaces — Produces:**
- `type pendingSwitch struct{ from, to instrument.ID; pitches []float64; readyAt float64 }` (`readyAt` is NaN until ready).
- `dv.pendingSwitches map[int]*pendingSwitch`.
- `func (dv *DrumView) soundingAt(row int, startSec float64) instrument.ID`: returns `to` when `startSec > readyAt + CancelGuardSec`, else `from`.
- `func (g *Game) advanceSwitches(nowAudioSec float64)`: when `audio.VoicesReady(to, pitches)`, `readyAt = now`. A switch is dropped once `now > readyAt + AudioHorizonSec`.
- `func (dv *DrumView) rowUpcomingPitches(row int, horizonSec float64) []float64`: from the predictor. A switch made while playing warms `to` through the prewarmer's existing `warmPitch(id instrument.ID, pitch float64)` for each of these pitches. Plan D gives these jobs render priority 1.
- `func VoicesReady(id instrument.ID, pitches []float64) bool` (audio).
- A switch to `from` while pending cancels it.

- [ ] Tests, on the lane with a fake clock and stub readiness:
  - `TestSwitchSoundsOldUntilNewIsReady`;
  - `TestSwitchChangesAtFirstNoteAfterReadyPlusGuard`: the note at `readyAt + CancelGuardSec/2` is `from`, the next one is `to`;
  - `TestSwitchBackBeforeReadyCancels`;
  - `TestExportWritesSelectedDuringPendingSwitch`;
  - `TestSwitchWhileStoppedIsImmediate`;
  - `TestRowDeleteDropsItsPendingSwitch`;
  - `TestUndoDuringPendingSwitch`;
  - `TestLaneKeysCoverEveryEditClass` gains the switch class.
  - JS case: a render forced to land 60 ms after its start → not started, `lateDropped == 1`.
- [ ] Implement. `make bench-load DIMENSION=switch` on laptop and lowPhone (chrome-local headed) passes every step's gates. Then `audio_lane_e2e` and `TestLaneHeardEqualsLegacyDispatched`.

### Task 5: Soak, full runs and docs

**Files:** Create `src/js/webaudio_residency_soak.browser.test.js` (`SOAK_MINUTES`, default 3; added to `SEQUENTIAL_TESTS`).

- [ ] The soak cycles switches, menu picks, FX/EQ edits, imports of two fixtures, Save As and undo, sampling `resourceLedger()` every 5 s. Gates:
  - ledger bytes in the last minute ≤ 1.1 × the first minute's;
  - `channelChains ≤ rows + 1 + channelsReleasing`;
  - `dspNodesLive` equals the non-neutral stages of Live instruments;
  - the Go heap plateaus (`perfStats().heapAllocKB` last ≤ 1.2 × first);
  - no probe gate fails.
- [ ] `make bench-load` for every dimension × {laptop, midPhone, lowPhone} on the Plan C build. Write `bench-results/residency_after_c_2026-10-30.md` against Plan A's baseline, flagging any regression. It holds the inputs Plan D needs (its caps are measured on THIS build).
- [ ] Update CLAUDE.md:
  - a new "Instrument residency" paragraph: states, holds, release after the last voice, budgets, guards;
  - the audio-lane paragraph: selected vs sounding, `lateDropped`;
  - the browser-audio-isolation gotchas.
- [ ] Update the memory note.
