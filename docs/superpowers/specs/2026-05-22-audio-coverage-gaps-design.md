# Audio business-logic coverage push — Design

**Date:** 2026-05-22
**Branch:** `add-core-node-types`
**Baseline coverage:** `internal/audio` 85.4% / project 80.4% (from `make coverage-go`).
**Target:** `internal/audio` ≥ 92% / project ≥ 82%.

## Motivation

`make coverage-go` shows a cluster of zero-coverage *production* functions in `internal/audio` that sit on real user-visible flows: the master LUFS readout that feeds the Levels tab, the user-recipe Save/Save-As/dirty-button path that feeds the Synth tab, and the per-instrument trigger-timestamp clock that drives the Synth-tab pulse glow. None of those are exercised by the existing tests, even though every other supporting primitive (`LUFSIntegrator`, `RecordVoiceTrigger`, `updateRecipeDefaults`) is well covered. The gap is in the public wrappers and the dirty-state helpers — exactly the surface UI code calls.

The audio package also carries a handful of small smells that were tolerated because they sat next to "covered" code: a dead `triggerLastEver` field, a process-global `lastSetEQ` documented as kept only for legacy UI tests, and an `abs64Diff` helper that duplicates `math.Abs`. We pick those up in the same PRs that add the tests, so the cleanup ships against the test that proves it.

A separate, smaller surface — the `SetParam` switches on the insert effects — is partially covered (28–60% per effect). One table-driven test against the `InsertEffectCatalog()` raises every switch into the high-90s and self-extends to new effects added later.

## Non-goals

- `channels.go` `ProcessBlock` / `ProcessInsertsBlockLocal` / `ProcessEQBlockLocal` partial coverage. Real-time DSP that the parity suite already exercises indirectly; deserves its own plan with its own risk profile.
- `internal/ui` Synth/Chain panel zone gaps (~50 zero-coverage funcs across the newest audio-panel surfaces). Needs ebitenstub harness work and belongs in a UI-specific plan.
- `internal/eventlogger` 71.2% — narrative pipeline plan, separate concern.
- WASM / browser-only paths. All new tests run in the fast `-tags test` path.

## Phase 1 — Master LUFS pipeline

### Production functions in scope

- `internal/audio/loudness.go:30 EnsureMasterLUFS` (0%)
- `internal/audio/loudness.go:44 FeedMasterLUFS` (0%)
- `internal/audio/loudness.go:58 MasterLUFSShortTerm` (0%)
- `internal/audio/loudness.go:69 ResetLoudnessForTest` (0%)

### Background

`internal/analyzer/service.go` reads master loudness via a `MasterLUFSShortTerm` callback wired through `analyzer.Config` (so `analyzer` doesn't import `audio`). The engine's master push site in `engine_stop.go` feeds samples via `FeedMasterLUFS`. The atomic-bits publish pattern (line 24, `masterLUFSBits atomic.Uint64`) lets the analyzer goroutine read without the integrator mutex.

The underlying DSP (`LUFSIntegrator`, `KWeightingFilter`) is covered (88–93% per function). What is not tested is:

1. Lazy init via `Ensure` and the idempotent re-call.
2. The `bits == 0` short-circuit returning `-120` when no audio has been fed.
3. The cross-goroutine atomic publish path under `-race`.
4. The sample-rate-change branch that re-creates the integrator.

### Tests to add — `internal/audio/loudness_master_test.go`

| Test | What it asserts |
|---|---|
| `TestEnsureMasterLUFS_IdempotentAtSameRate` | After two `EnsureMasterLUFS(48000)` calls with `FeedMasterLUFS` between them, the published short-term value is preserved across the second `Ensure`. |
| `TestEnsureMasterLUFS_RateChangeResets` | `EnsureMasterLUFS(48000)` → feed → `EnsureMasterLUFS(44100)` re-creates the integrator (verified via `MasterLUFSShortTerm() == -120` after the rate change). |
| `TestMasterLUFSShortTerm_BeforeFeedReturnsFloor` | Fresh process state via `ResetLoudnessForTest`; `MasterLUFSShortTerm()` returns `-120` exactly (the `bits == 0` short-circuit). |
| `TestFeedMasterLUFS_KnownTone` | Feed 3 s of a 997 Hz tone at -20 dBFS at 48 kHz; assert `MasterLUFSShortTerm()` ∈ `[-24, -18]` LUFS-S. The wide window accommodates K-weighting's shelf gain near 1 kHz (the canonical BS.1770 coefficients give somewhere between 0 and +2 dB depending on exact frequency vs. the 1681 Hz shelf corner) plus the `-0.691` LUFS offset. The test is a sanity check, not a BS.1770 conformance test — that lives in `LUFSIntegrator`'s existing direct tests. |
| `TestFeedMasterLUFS_NoIntegratorNoOp` | After `ResetLoudnessForTest` + clearing `masterLUFSCurrent` via a fresh process state, `FeedMasterLUFS` is a no-op (no panic, value stays `-120`). Tested by *not* calling Ensure. |
| `TestMasterLUFSShortTerm_CrossGoroutineRace` | Goroutine A loops `FeedMasterLUFS(block)`; goroutine B loops `_ = MasterLUFSShortTerm()`. Run for 200 ms. With `go test -race` clean and the published value strictly increases above `-120` once feed catches up. |

### Discipline notes

- Each test starts with `t.Cleanup(func() { ResetLoudnessForTest() })`.
- The race test needs a small atomic counter on B's loop so the test asserts B actually performed reads (avoids a no-op pass under aggressive compiler reordering).
- Tone fixture: deterministic generator inline (no fixture files), 3 s × 48 kHz = 144 000 samples, one allocation.

### Refactors in same PR

None. The API is clean.

## Phase 2 — User recipe overrides

### Production functions in scope

- `internal/audio/recipe_overrides.go:28 SetUseUserRecipeOverrides` (0%)
- `internal/audio/recipe_overrides.go:149 UpdateRecipeDefaultsAndInvalidate` (0%)
- `internal/audio/recipe_overrides.go:180 RegisterUserRecipeFromBase` (0%)
- `internal/audio/recipe_overrides.go:297 InstrumentParamsDiffer` (0%)
- `internal/audio/recipe_overrides.go:318 abs64Diff` (0%)
- `internal/audio/recipe_overrides.go:74 ApplyUserRecipeOverrides` — currently 88.9%; lift to ~95% by covering the missing nil-src branch.

### Background

These functions implement the RecipeDoc model from the unified-architecture work (see `project_recipe_doc_unified_architecture` memory). The user-visible flows that go through them:

1. **Synth-tab Save button** → `UpdateRecipeDefaultsAndInvalidate(recipeID, overlay)` mutates registered defaults *and* invalidates the voice cache for every bound instrument, so the next trigger hears the new sound.
2. **Synth-tab Save-As button** → `RegisterUserRecipeFromBase` builds a `RecipeDoc` with copied `ParamDefs` and a seed map, registers it via `RegisterPluginRecipeFromDoc`, and returns the doc bytes for persistence.
3. **Save-button dirty indicator** → `InstrumentParamsDiffer` is the UI's cheap "do we have anything to persist?" query.
4. **Bootstrap (browser + desktop)** → flips `SetUseUserRecipeOverrides(true)` once, then `ApplyUserRecipeOverrides(userprefs.Store)` reloads persisted overrides into the registry.

The audio↔userprefs boundary is the `UserRecipeSource` interface (line 48). Tests can pass a stub that returns canned `map[string]map[string]float64` and `map[string][]byte` payloads — no fixture files needed.

### Tests to add — `internal/audio/recipe_overrides_actions_test.go`

| Test | What it asserts |
|---|---|
| `TestUseUserRecipeOverridesRoundTrip` | `Swap…ForTest(true)` returns the prior value; `UseUserRecipeOverrides()` reflects the swap; nested `t.Cleanup` restoration works. |
| `TestApplyUserRecipeOverrides_NilSource` | Gate on, `src == nil` → returns nil without panic and without mutating any registered recipe defaults. |
| `TestUpdateRecipeDefaultsAndInvalidate_HappyPath` | Register a base recipe, bind instrument "kick" to it via `BindInstrumentRecipe`, swap `voiceCacheInvalidate` with a counter (existing pattern at `recipe_overrides_test.go:98`), call with `{"cutoff": 1234}`, assert (a) `RecipeRegistrations()[id].Params[k].Default == 1234`, (b) counter recorded exactly one invocation with `instID == "kick"`, (c) return value is `true`. |
| `TestUpdateRecipeDefaultsAndInvalidate_NoChangeReturnsFalse` | Pre-set defaults equal the overrides; call returns `false`; counter shows zero invocations. |
| `TestRegisterUserRecipeFromBase_HappyPath` | Returns `RecipeDoc` with `Origin == OriginUser`, `len(ParamDefs) == len(base.Params)`, `ParamSeed` contains only the finite values from the input, `NewRecipe(newID)` returns non-nil and is callable. |
| `TestRegisterUserRecipeFromBase_UnknownBase` | Returns a typed error matching `"base recipe .* is not registered"`. |
| `TestRegisterUserRecipeFromBase_StripsNonFiniteSeed` | Input seed contains `NaN`, `+Inf`, `-Inf`, and a finite value; returned `ParamSeed` contains only the finite key. |
| `TestInstrumentParamsDiffer_Cases` | Table-driven: (empty overlay → false), (key absent from defaults → true), (overlay equal to default within ε → false), (overlay differing → true), (empty instID or recipeID → false). |

### Refactors in same PR

- **`abs64Diff` → `math.Abs`.** One-line replacement at the single call site; delete the helper. Net change: -7 lines, +1 import (already present via `math` in surrounding files in this package, but `recipe_overrides.go` currently does not import `math` — added).
- **`isFiniteParam` body → `!math.IsNaN(v) && !math.IsInf(v, 0)`.** Keep the function name as the documented boundary sanitiser; only replace the hand-rolled NaN/Inf comparison. Preserves the comment's intent ("the check is one line") while using the stdlib semantics. Behavior identical for all IEEE-754 inputs.

## Phase 3 — Trigger timestamps + channel EQ response + passThrough

### Production functions in scope

- `internal/audio/trigger_timestamps.go:40 LastTriggerAt` (0%)
- `internal/audio/trigger_timestamps.go:50 SinceLastTrigger` (0%)
- `internal/audio/trigger_timestamps.go:60 ResetTriggerTimestamps` (0%)
- `internal/audio/eq_state.go:63 GetChannelEQResponse` (0%)
- `internal/audio/insert_effects.go:115 passThrough.Reset` (0%)
- `internal/audio/insert_effects.go:116 passThrough.SetParam` (0%)
- `internal/audio/fx_waveshaper.go:58 waveshaper.Reset` (0%, build-tagged `test || js`)

### Background

`SinceLastTrigger(instID)` is called by the Synth-tab pulse glow renderer (`drawSynthTab` in `synth_panel_zone.go`) — every frame, for every section card, to compute a 200 ms decay glow. `LastTriggerAt` is the underlying read; `RecordVoiceTrigger` (the writer) is already covered by the engine's Play* tests.

`GetChannelEQResponse(id, n, lo, hi)` is called by the Synth-tab filter preview pane (`drawSynthPreviewPane`) to overlay the active channel's biquad cascade on the recipe knob cards.

`passThrough` is the fallback returned by `NewEffectProcessor` when the requested `EffectType` is not registered. Today no test exercises that branch.

### Tests to add

**`internal/audio/trigger_timestamps_test.go` (four tests, sub-tests where natural):**

| Test | What it asserts |
|---|---|
| `TestLastTriggerAt_UnknownIDZero` | `LastTriggerAt("nope")` returns `time.Time{}.IsZero() == true`. |
| `TestSinceLastTrigger_UnknownIDFar` | `SinceLastTrigger("nope") >= 10*time.Hour`. |
| `TestRecordVoiceTriggerThenRead` | `RecordVoiceTrigger("kick")` then `LastTriggerAt("kick")` returns a time within the last 1 s; `SinceLastTrigger("kick") < 1 * time.Second`. |
| `TestResetTriggerTimestamps` | After Record + Reset, `LastTriggerAt` returns zero again. |

Each test starts with `t.Cleanup(audio.ResetTriggerTimestamps)`.

**`internal/audio/eq_response_channel_test.go` (three tests):**

| Test | What it asserts |
|---|---|
| `TestGetChannelEQResponse_BadInputs` | `numPoints <= 0` → nil; unknown channelID → nil. |
| `TestGetChannelEQResponse_BasicShape` | After `SetChannelEQ("ch", 48000, []EQBand{{Hz: 1000, GainDB: 6}})`, response has 32 points, frequencies are log-spaced (monotonic in log), magnitude at 1 kHz is positive (boost). |
| `TestGetChannelEQResponse_DefaultSampleRate` | After `SetChannelEQ("ch", 0, …)` (sample rate falls back to 48000), response is non-nil and identical to the explicit-48000 case. |

**`internal/audio/insert_effects_passthrough_test.go` (one consolidated test):**

| Test | What it asserts |
|---|---|
| `TestPassThroughFallback` | `NewEffectProcessor(EffectSlot{Type: "__unregistered__"}, 48000)` returns a `*passThrough`. Verify: `ProcessSample(0.5) == 0.5`; `Reset()` and `SetParam("anything", 1.0)` do not panic; `ProcessBlockBuf(in, out, 8)` copies `in[:8]` into `out[:8]`. |

**Add to existing `fx_*_test.go` (one new sub-test):** `TestWaveshaperReset` calls `Reset()` and asserts no panic. (Stateless effect, so the assertion is structural — the test exists for coverage.) Group with the existing waveshaper test in `fx_waveshaper_test.go` if present; otherwise create one.

### Refactors in same PR

- **Delete `triggerLastEver`** from `trigger_timestamps.go:17`. The field is written by `RecordVoiceTrigger` but never read by any caller (verified via `grep -rn triggerLastEver src/go`). Removes one unsynchronised wall-clock write per trigger. Also delete its reset in `ResetTriggerTimestamps`.
- **Retire `lastSetEQ`** (`eq_state.go:28`) and `LastSetEQ()` (`eq_state.go:34`):
  - Add a public `audio.ChannelEQSnapshot(id string) ChannelEQ` that returns a copy of `lookupChannelEQ(id)` exposed via a new public `ChannelEQ` struct (mirrors `eqRecord` fields).
  - Migrate the four callers identified in the audit:
    - `internal/audio/insert_effects_helpers_test.go:88–96` (3 sites in one test).
    - `internal/ui/eq_export_import_test.go:70`.
    - `internal/ui/drumview_eq_controls_test.go:24,38`.
  - Each caller switches from `audio.LastSetEQ()` (most-recent global) to `audio.ChannelEQSnapshot("main")` (or whichever channel ID the test was implicitly assuming). The migration makes the test's intent visible; the global goes away.
  - Delete `LastSetEQ`, `lastSetEQ`, and the comment block calling out the legacy. Update `recordChannelEQ` to drop the `lastSetEQ = rec` line.
- **Net delta:** `eq_state.go` shrinks by ~10 lines; one process-global removed; four test sites become per-channel-explicit.

## Phase 4 — Effect SetParam table coverage

### Production functions in scope (all `internal/audio/fx_*.go SetParam`)

- `fx_compressor.go:86` 28.6%
- `fx_tape.go:135` 33.3%
- `fx_pitchshift.go:151` 40.0%
- `fx_ringmod.go:66` 40.0%
- `fx_phaser.go:87` 50.0%
- `fx_autowah.go:104` 50.0%
- `fx_gate.go:68` 55.6%
- `fx_flanger.go:106` 57.1%
- `fx_tremolo.go:65` 60.0%
- (others ≥70%, included for free)

### Background

Every effect implements `InsertEffect.SetParam(name string, value float64)` as a switch on the parameter name. The `InsertEffectCatalog()` already returns the full set of valid `(EffectType, []EffectParamDef)` pairs that any production code paths will ever pass. A single table-driven test that walks the catalog catches every switch arm.

### Test to add — `internal/audio/fx_setparam_table_test.go`

One test: `TestEffectSetParamCatalogCoverage`.

For every `(effectType, params)` in `InsertEffectCatalog()`:

1. `eff := NewEffectProcessor(EffectSlot{Type: effectType, Enabled: true}, 48000)`
2. For each `def` in `params`:
   - `eff.SetParam(def.Name, def.Min)`; assert `ProcessSample(0.1)` returns finite.
   - `eff.SetParam(def.Name, def.Max)`; assert finite.
   - `eff.SetParam(def.Name, (def.Min+def.Max)/2)`; assert finite.
3. `eff.SetParam("__unknown__", 0)`; assert finite (covers default branch).
4. `eff.Reset()`; assert finite.

Each effect runs inside `t.Run(string(effectType), …)` so failures attribute clearly. The test self-extends to any new effect added later — registering a new effect with the catalog automatically opts it in.

### Refactors

None. Purely additive.

### Expected coverage impact

The aggregate `fx_*.go SetParam` weighted average rises from ~52% to ~95%. The non-`SetParam` paths in those files are already 75–95% covered by the existing per-effect tests; this fills the remaining gap.

## Verification & coverage delta

After each phase:

```bash
cd /home/ymolinar/Repos/beatmo/src/go && \
  ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -race ./internal/audio/...
```

After Phase 3 (because of UI test migration):

```bash
cd /home/ymolinar/Repos/beatmo/src/go && \
  ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/...
```

End-to-end gate:

```bash
make coverage-go
.tools/go/bin/go tool cover -func=coverage/go.out | tail -1
```

**Pass criteria:** total ≥ 82%, `internal/audio` ≥ 92%, no new goleak-detected goroutines, `-race` clean.

## Risks and mitigations

| Risk | Mitigation |
|---|---|
| `LastSetEQ` migration breaks a UI test path I didn't find with grep | Run `./internal/ui/...` after the migration commit; the migration is staged in its own commit so revert is one `git revert`. |
| LUFS race test is flaky on CI | Bound the test by sample count, not wall time; the cross-goroutine assertion is "B observed at least one value > -120", not a tight numeric bound. |
| Deleting `triggerLastEver` affects a future consumer | Field is package-private and unread; no API change. If a future consumer needs "last trigger across all instruments", it can compute it from the existing `triggerByID` map. |
| `math.Abs` for `abs64Diff` changes semantics for ±0 | `math.Abs` returns `+0` for `-0`, same as the hand-rolled branch on negative. Behavior identical. |
| `isFiniteParam` change classifies values >1e308 differently | `math.IsInf(v, 0)` is exactly the IEEE-754 ±Inf check. The current `v > 1e308 \|\| v < -1e308` admits subnormal-finite values around the boundary that the standard check rejects. The change is *stricter* (catches denormalised-Inf-by-overflow), so any test that exercised the existing boundary still passes; production data never carries such values. |

## Open questions

None — clarifications resolved during brainstorming:

- **Scope:** audio business logic only (confirmed via AskUserQuestion).
- **Refactor appetite:** opportunistic (confirmed).
- **Phasing:** four phases as designed (confirmed).
