# Residency B — Audio Bundles and Save As: Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** each instrument owns one audio bundle (Sampler edit + EQ + insert effects) that its rows point to; a channel and its DSP stages are built from that description only when needed; Save As is the only save; a pick asks the user when two sets of settings conflict.

**Architecture:**
- `audio.AudioBundle` is plain data. One per-instrument store in `internal/audio` is the single source. It replaces `DrumRow`'s EQ/Effects fields and becomes the source behind the audio layer's per-part stores.
- One materialise function applies a bundle to a channel.
- The user library stores the bundle beside the document.
- The UI gets one Save As (save over / save as new / refused) and a pure pick rule with a must-answer card.

**Tech Stack:** Go 1.26 (Ebiten UI, js/wasm), audio.js + the `beatmo-dsp` worklet, Playwright.

**Spec:** `docs/superpowers/specs/2026-10-25-instrument-residency-and-resource-budgets-design.md` §3, §5.2, §5.3, §9.5, §10 phase 1. Runs after Plan A (`2026-10-30-residency-a-measure.md`), whose baseline it is compared against.

## Global Constraints

- Never stage or commit. Never `git stash`. Use `.tools/go/bin/go`. Produce red builds with `go build -overlay`, never by backing up and restoring files.
- **These stay green after every task:**
  - export→import audio identity (`make test-real`: `TestExportImportMasterMixIsByteIdentical*`, `TestEveryBuiltinTemplateRoundTripsByteIdentical`);
  - `xplat_audio_stages.browser.test.js`;
  - `webaudio_live_edit_budget.browser.test.js`.
- The project file's shape does not change. Per-row `sample_edit` / `eq` / `effects` are the instrument's bundle. When two entries of one instrument disagree, the last one wins.
- A flat EQ is ABSENT, never a unity chain (bit-exactness).
- Pan and sends do not exist. Never add a stored setting without a control.
- UI changes are verified by a screenshot scene, opened and inspected, desktop + `MOBILE=1`.
- Every test Game is closed with `t.Cleanup(g.CloseForTest)`. UI behaviour is tested through real input via `Game.Update`.
- No allowlist or known-weak exceptions. A removed JS export loses its smoke-catalogue entry. A new `syscall/js` value passes `make js-audit`.
- Removing a `hooks.Kind` updates `KindAll`, `ActionRegistry`, its eventlogger formatter and `NumNonVerbose`.

## Review Focus

1. **Two rows on one instrument.** Expected: an EQ drag on row 2 changes the curve the EQ panel shows for row 1 too, and both play through it. Test: Task 2 `TestTwoRowsOnOneInstrumentShareTheBundle`.
2. **Undo right after a forking pick.** Expected: the instance is gone, the other row is untouched, and every bundle is back. Test: Task 6 `TestPickAndAnswerAreOneUndoStep` (asserts after undo).
3. **Save As in the Spanish UI on a shipped instrument with a localized name.** Expected: the localized name counts as the same name ("Save over …"). Test: Task 5 `TestSaveAsSameNameMatchesTheLocalizedName`.
4. **Save As during playback.** Expected: no audible change and no lane re-commit, because a library write moves neither `VoiceRevision` nor the bundle. Test: Task 5 `TestSaveAsDoesNotTouchThePlayingSound`.
5. **Instrument-file import into a row that has settings.** Expected: the same pick table and question as the menu. Test: Task 6 `TestInstrumentFileImportFollowsThePickRule`.

---

### Task 1: `AudioBundle` and its store

**Files:**
- Create `internal/audio/audio_bundle.go` (type and methods, no build tags).
- Create `internal/audio/audio_bundle_store.go` (store and partial writers).
- Modify:
  - `effect_chain.go` (`AddInsertEffect`, `RemoveInsertEffect`, `MoveInsertEffect`, `ToggleInsertEffect`, `SetInsertEffectParam`, `SetInsertEffects`, `GetInsertEffects`, `ClearAllInsertEffectsAndNotify` write/read the store);
  - `sample_edit_descriptor.go` (`ApplySampleEditLive`, `SetSampleEdit`, `ClearSampleEdit`, `SampleEditFor`, `HasSampleEdit` use the store; the `sampleEdits` map goes);
  - `eq_state.go`.
- Test: `internal/audio/audio_bundle_test.go` (`-tags test`).

**Interfaces — Produces:**
- `type AudioBundle struct { SampleEdit *SampleEdit; EQ *InstrumentEQ; Inserts []EffectSlot }`. The JSON uses the project-file field names: `sample_edit` (as `SampleEdit.Fields()`), `eq`, `effects`.
- `func (b AudioBundle) Normalized() AudioBundle`: an identity Sampler edit becomes nil; a flat EQ (all gains 0, no muted band, no HPF/LPF) becomes nil; empty inserts become nil.
- `IsNeutral() bool`, `Equal(o AudioBundle) bool` (over `Normalized`), `Clone() AudioBundle`, `Hash() string` (`"sha256:"` + hex of the canonical JSON).
- `Summary() BundleSummary`, with `type BundleSummary struct{ SampleEdit, EQ bool; Effects int }`.
- `var EQBandEdges = [10][2]float64{…}`, moved from `ui.eqBandDefs` (the ui keeps only its use).
- `func (e InstrumentEQ) Bands(sr int) []EQBand`, moved from `ui.buildFullEQBands`.
- Store:
  - `BundleFor(id instrument.ID) AudioBundle`: the circuit entry if any, else the library bundle of `InstrumentLibraryIdentity` (Task 4; neutral until then), else neutral. Always a clone.
  - `HasCircuitBundle(id) bool`.
  - `SetBundle(id, b AudioBundle)`: stores `Normalized`, then applies the change through the existing platform paths. It publishes no hooks event; commit sites keep theirs.
  - `UpdateBundle(id, fn func(*AudioBundle))`.
  - `SetBundleEQ(id, eq *InstrumentEQ)`.
  - `DropBundle(id)`.
  - `ResetBundles()`: clears every circuit entry and the applied state (replaces `ClearAllInsertEffectsAndNotify` + per-channel EQ clears + sample-edit clears).
  - `ActiveInsertCount(id) int`: enabled inserts, no clone; drawn every frame.
  - `BundleRevision(id) uint64`.

- [ ] Tests:
  - `TestBundleNormalizedNeutralEqualHash`: identity edit, flat EQ and empty inserts are neutral; equal bundles hash equally; one changed param changes the hash.
  - `TestBundleJSONUsesProjectFieldNames`.
  - `TestPartialWritersShareOneStore`: `AddInsertEffect` + `SetInsertEffectParam` + `ApplySampleEditLive` + `SetBundleEQ` all show in `BundleFor`; `GetInsertEffects(id)` equals `BundleFor(id).Inserts`; `SampleEditFor` equals `.SampleEdit`.
  - `TestResetBundlesClearsEverything`.
- [ ] Implement. `go test -tags test ./internal/audio/...` and `make test-real` (audio subset) pass with no change to any render.

### Task 2: The UI reads and writes the bundle; `DrumRow` loses its mixer fields

**Files:**
- `internal/ui/drumview.go`: delete `EQGainsDB`, `EQBandMuted`, `HPFEnabled`, `HPFCutoffHz`, `LPFEnabled`, `LPFCutoffHz`, `Effects`; `eqBandDefs` loses its Hz edges.
- `drumview_ctor.go` (row literal; `OnGainChange` 404-407), `drumview_rows.go:121`.
- `drumview_audio_eq.go`: `applyRowEQ`, `clearRowEQ`, `saveZoneBandState`, `loadChannelBandState`, `ensureRowEQ*` (delete), `activeHPF*/LPF*`, `setActiveHPF/LPF`, `toggleEQBandMute`. A row's EQ is `audio.BundleFor(inst).EQ`. Writes go through one wrapper, `(dv) setInstrumentEQ(inst instrument.ID, eq *audio.InstrumentEQ)`, which calls `audio.SetBundleEQ` and re-enables the three analyzers as `applyRowEQ` / `clearRowEQ` do today.
- `drumview_draw_eq_curve.go` (89-90, 185-186, 242), `js_exports_eq_widgets.go:985`.
- `row_rack_zone.go` (FX badge 1457/1612/1617/1710/1714 → `audio.ActiveInsertCount`).
- `drumview_fx_panel.go` (delete `syncFXToRow` and its five calls).
- `export.go` (per-row block reads `BundleFor`).
- `import.go`: `audio.ResetBundles()` before the row loop, replacing 511 plus the per-row EQ clear and sample-edit clear. Per row, `audio.SetBundle(inst.ID, bundleFromEntry(inst))`, so the last entry wins.
- `instrument_export.go` (`buildInstrumentEQ` from the bundle).
- `instrument_import.go`: `applyInstrumentEQToRow` is replaced by `audio.SetBundle(newID, …)` with the file's EQ + effects + sample edit. Task 6 adds the pick rule.
- `kit_binder.go`: unchanged code. A kit swap gives each row its new instrument's own bundle (this plan's call: no carry-over, no question for a whole-kit swap).
- Tests:
  - new `internal/ui/audio_bundle_ui_test.go`;
  - new `internal/ui/mixer_writes_discipline_test.go`;
  - update the files listed under "Test-file counts" in the Plan B notes below.

**Interfaces — Consumes:** Task 1. **Produces:** `(dv *DrumView) setInstrumentEQ(inst instrument.ID, eq *audio.InstrumentEQ)`; `bundleFromEntry(e exportInstrument) audio.AudioBundle` (import.go).

- [ ] Tests:
  - `TestDrumRowHasNoMixerFields`: reflect; none of the seven names exists.
  - `TestUIWritesMixerSettingsOnlyThroughTheBundle`: AST over `internal/ui` non-test files.
    - `audio.SetChannelEQ` and `audio.ClearChannelProcessors` appear only inside `applyMasterEQ`.
    - `audio.SetInsertEffects`, `audio.SetSampleEdit`, `audio.ClearSampleEdit` and `audio.ApplySampleEditLive` appear nowhere. Use `SetBundle` / `UpdateBundle`, or the partial writers that Task 1 made bundle writers.
  - `TestTwoRowsOnOneInstrumentShareTheBundle`: real input; an EQ band drag with row 2 active → `BundleFor(inst).EQ.GainsDB[band]` moved, and selecting row 1 shows the same value in the panel.
  - `TestImportLastEntryWinsForOneInstrument`: a file with two entries for one id carrying different `eq` / `effects` → `BundleFor` equals the second entry's.
  - `TestImportResetsBundlesNotInTheFile`: a bundle set on X, then an import of a file without X → `HasCircuitBundle(X)` is false.
  - `TestKitApplyKeepsEachInstrumentsBundle`.
- [ ] Update the existing tests (`eq_*`, `export_import_*`, `kit_apply`, `fx_panel_lifecycle`, `insert_effects_roundtrip`, `instrument_roundtrip`, `rename_metadata_only`, `row_rack_design_invariants`, `chain_stage_card`, `project_schema_version`, `instrument_wav_locator`) to read `audio.BundleFor`.
- [ ] Run `go test -tags test -modfile=go.test.mod ./internal/ui/ ./internal/audio/...`, then `make test-real`, then `webaudio_live_edit_budget.browser.test.js` (private wasm). All pass. Screenshot `crop_eq_*` and the FX panel scenes (desktop + mobile), opened, unchanged.

### Task 3: Materialise from the bundle; no boot channels; stages only while they do something

**Files:**
- Create `internal/audio/channel_materialise.go` (no build tags).
- Modify:
  - `engine.go` / `engine_wasm.go`: `Register` no longer calls `InstrumentChannel`.
  - `channels.go`: `resetInstrumentChannels` pushes the master volume only; `ChannelVolume` uses `lookupChannel` and returns 1 when absent.
  - `effect_chain.go`: `rebuildChannelProcessors` with no processors removes the channel from `procChannels`.
  - `eq.go` / `eq_wasm.go`.
- Modify `src/js/audio.js`:
  - `getChannelChain(id, create = true)`. `channelVolume` and `channelAnalyzerSnapshot` / `preEQAnalyzerSnapshot` / `synthAnalyzerSnapshot` pass `false` and return their defaults when there is no chain.
  - `updateInsertEffects` with an empty list `_dspDispose`s `fxNode`, sets it to null and rewires.
  - `setChannelEQ` with a null or empty program does the same for `eqNode`.
  - `audioGraphStats` gains `dspNodesLive`.
- Test: `internal/audio/channel_lazy_test.go` (`-tags test`); `internal/audio/channel_stage_native_test.go` (`!test && !js`); Create `src/js/channel_lazy.browser.test.js`.

**Interfaces — Produces:** `func materialiseChannel(id instrument.ID)`. It applies `BundleFor(id)`'s channel side (EQ program or none; the insert chain or none) to the platform. It is the only path `SetBundle`, import and undo use, and Plan C's `Acquire` calls it.

- [ ] Tests:
  - `TestRegisterCreatesNoChannel`.
  - `TestBootCreatesNoInstrumentChannels`: after `ResetInstruments`, `chanMgr.instruments` is empty.
  - `TestChannelVolumeReadCreatesNothing`.
  - `TestMaterialiseIsTheOnlyApplyPath`: AST in `internal/audio` non-test code; `SetChannelEQ` / `rebuildChannelProcessors` are called for instrument channels only from `channel_materialise.go`.
  - Native `TestEmptiedChainLeavesTheMix`: add then remove every effect and flatten the EQ → the channel is not in `procChannels`; the rendered mix is byte-identical to a channel that never had them.
  - Browser `channel_lazy.browser.test.js` (startup demo):
    - after the gesture and warm-up, `channelChains ≤ 1 + rows` (it was 151);
    - after removing every effect and flattening the EQ on row 0, `dspNodesLive` drops by its count;
    - `channelAnalyzerSnapshot("kick-not-in-circuit")` leaves `channelChains` unchanged.
- [ ] Implement; then `make test-real`, `xplat_audio_stages.browser.test.js`, `chain_spec_parity.browser.test.js` and `resource_stats.browser.test.js` pass.

### Task 4: The user library stores bundles; old saved Sampler edits fold in

**Files:**
- `internal/userprefs/instrument_store.go` (+ `_js.go` / `_notjs.go`): `instrumentsDoc.Bundles map[instrument.ID]json.RawMessage "bundles,omitempty"`. The old `sample_edits` section is read once into `Bundles` on load and dropped from the next write; `LoadSampleEdits` / `SaveSampleEdit` are removed from `InstrumentStore`.
- `internal/audio/instrument_library.go`:
  - `InstrumentLibrarySource` gains `LoadInstrumentBundles() (map[instrument.ID][]byte, error)`.
  - `InstrumentLibrarySink` gains `SaveInstrumentBundle(id instrument.ID, b []byte) error` and `DeleteInstrumentBundle(id instrument.ID) error`.
  - `LoadUserInstrumentLibrary` loads bundles. An invalid one gets a single WARN and is skipped, like documents.
- `cmd/beatmo.go` and `internal/ui/js_bootstrap_wasm.go`: replace `ApplySavedSampleEdits` with `audio.FoldLegacySampleEdits(prefs.LoadSampleEdits())`, then `prefs.DeleteSampleEdit(id)` for each folded id.
- Delete `ApplySavedSampleEdits`.
- Tests: `internal/userprefs/instrument_store_test.go`, `internal/audio/instrument_library_test.go`.

**Interfaces — Produces:**
- `func LibraryBundle(id instrument.ID) (AudioBundle, bool)`.
- `func SaveLibraryInstrument(d instrument.Doc, b AudioBundle) error` (document + bundle; the bundle entry is deleted when neutral).
- `func FoldLegacySampleEdits(edits map[instrument.ID]map[string]float64)`. It sets `SampleEdit` on the library bundle of each id that has none, and persists it through the sink.

- [ ] Tests:
  - `TestInstrumentStoreRoundTripsBundles`;
  - `TestInstrumentStoreFoldsItsOldSampleEdits`;
  - `TestLibraryBundleLoadsBesideItsDocument`;
  - `TestFoldLegacySampleEditsIsLossless`: every field is equal and the prefs entry is gone afterwards;
  - `TestBundleForFallsBackToTheLibrary`: no circuit entry → the library bundle; after `SetBundle` → the circuit's.
- [ ] Implement; `go test -tags test ./internal/userprefs/ ./internal/audio/ ./internal/ui/` pass.

### Task 5: Save As is the only save

**Files:**
- Create `internal/ui/instrument_save_as.go`.
- Modify:
  - `synth_panel_zone.go`: drop Save/Reset (tags 912-917, `layoutSynthHeader`, `buildSynthHeaderButtons`, the tag router 2187-2205, hit areas 2499-2520, `synthActionLabel`, mobile `ensureMobileActionButtons` / `MobileActionButtons` → Preview + ⋯, and `synthSaveDirty`).
  - `sampler_panel_zone.go`: drop the sampler-save / sampler-reset buttons, mobile Save, `samplerSave`, `samplerReset`, `samplerSaveAsSuggestion`, `samplerSaveAsConfirmed`.
  - `sampler_state.go`: delete `save` and `saveAs`; `bake` stays for Preview.
  - `action_menu_open.go`: `instrumentActionMenuItems(saveAs func())` → Save As, Export, Import, for both tabs; no Restore shipped.
  - `recipe_save_sink.go`: delete `SaveActiveRecipe`, `saveRecipeForInstrument`, `ResetActiveRecipe`, `resetInstrumentToCircuit`, `resetRecipeForInstrument`, `RestoreShippedActiveInstrument`, `canRestoreShipped`, `restoreShippedForInstrument`, `ResetActiveSampler` and `resetSampleForInstrument`. `SaveActiveRecipeAs` / `SaveActiveRecipeAsFor` fold into `instrument_save_as.go`.
  - `synth_panel_save_as_dialog.go`: one opener, `openSaveAsDialogFor(instID)`, for both tabs. `updateSaveAsDialog` re-classifies every frame; OK is disabled with a note line when the name is empty or taken.
  - `js_exports_synth_recipe.go`: remove `saveActiveRecipe` and `resetActiveRecipe`; `saveActiveRecipeAs` → `SaveActiveInstrumentAs`.
  - `js_exports_eq_widgets.go`: `synthFooterButtonRects` keys = preview, saveAs, more.
  - `i18n`: remove `KeyReset` and `KeyRestoreShipped`; add `KeySaveOver` ("Save over %s"), `KeySaveAsNew` ("Save as new") and `KeyNameTaken` ("%s is another instrument"), with Spanish entries.
  - `internal/audio`: delete `ResetInstrumentToCircuit`, `CircuitInstrumentDoc`, `CircuitInstrumentName`, `SetCircuitInstrumentName`, `DeleteUserLibraryDoc`, `ResetSampleToFactory` / `originalUserSamples`, and `CircuitState.Docs` / `.Names` with their undo capture — keep only what something still reads (`CircuitGeneration` stays for dial relevance).
  - hooks: remove the kinds nothing emits any more.
  - `scene_catalog.go`: delete `synth_action_menu_restore_shipped`; update `synth_save_as_prompt` and `synth_saved_tone_reopened`; add `synth_save_as_new`, `synth_save_as_name_taken`, `sampler_save_as_prompt`.
  - Browser tests: `wasm_bridge_smoke` (catalogue 178-182, 426-428), `synth_save_export_import`, `webaudio_synth_save_persist`, `synth_panel_footer`, `import_synth_render`, `zz_rowrack_sampler_pads` (clicks `sampler-save-as`).
- Tests: new `internal/ui/instrument_save_as_test.go`. Rewrite for Save As, or delete when they only test the removed buttons:
  - `instrument_library_flow_test.go` (the RestoreShipped* tests are deleted);
  - `synth_panel_save_test.go`, `synth_panel_save_persist_test.go`;
  - `synth_save_trumps_circuit_test.go`: `TestUndoAcrossACircuitOpenRestoresResetTarget` is deleted;
  - `synth_save_trumps_startup_real_test.go`;
  - `synth_panel_dirty_indicator_test.go` (deleted with `synthSaveDirty`);
  - `synth_panel_footer_hit_test.go`, `synth_header_fixed_actions_test.go`;
  - `synth_panel_zone_test.go` `TestSynthTab_ResetButtonClearsParams`;
  - `sampler_save_flow_test.go`, `sampler_reset_flow_test.go`, `sampler_edit_descriptor_flow_test.go`, `sampler_layout_test.go`, `sampler_mobile_actions_test.go`, `undo_sample_edit_test.go`;
  - `undo_all_actions_test.go` ("instrument-params-reset");
  - audio `instrument_library_test.go` / `instrument_docs_test.go`;
  - userprefs `sample_edit_store_test.go`.

**Interfaces — Produces:**
- `type saveAsKind int`, with consts `saveAsEmpty`, `saveAsOver`, `saveAsNew`, `saveAsTaken`.
- `func sameInstrumentName(a, b string) bool` (`strings.TrimSpace` + `strings.EqualFold`).
- `func classifySaveAsName(instID instrument.ID, typed string) (saveAsKind, instrument.ID)`. With L = `audio.InstrumentLibraryIdentity(instID, doc)`:
  - empty → `saveAsEmpty`;
  - same name as L's display name (either the plain or the localized `i18n.InstrumentDisplayName`) → `saveAsOver`;
  - same name as any OTHER instrument in `audio.Instruments()` ∪ `audio.LibraryIDs()` → `saveAsTaken`, returning that id;
  - else → `saveAsNew`.
  - So the prefill (the current display name) reads "Save over <name>" for an ordinary instrument and "Save as new" for a renamed copy or fork.
- `func (dv *DrumView) SaveActiveInstrumentAs(name string) instrument.ID`:
  - Over → `audio.SaveLibraryInstrument(circuit doc with ID=L, Origin="", Name=L's name, BundleFor(instID))`, then `EventRecipeSaved`; no undo step.
  - New → today's `SaveActiveRecipeAsFor` steps plus the bundle (mint `NewUserInstrumentID`, `SaveLibraryInstrument`, `RegisterLibraryInstrument`, repoint Origin, name the rows, one undo step), then `EventRecipeCreated`.
  - Taken / Empty → no-op, returns "".

- [ ] Tests, through real input via `Game.Update`, Synth tab and Sampler tab, desktop and mobile where they differ:
  - `TestSaveAsPrefillsTheCurrentName`.
  - `TestSaveAsOverKeepsTheID`: the library entry id is L and holds the doc + bundle. Reopening a circuit that uses L plays the saved document, and its bundle is the circuit's.
  - `TestSaveAsSameNameIgnoresCaseAndSpaces`.
  - `TestSaveAsSameNameMatchesTheLocalizedName` (Spanish).
  - `TestSaveAsNewCreatesAndLeavesTheOriginal`: the original's library bytes are identical.
  - `TestSaveAsRefusesAnotherInstrumentsName`: OK is disabled, Enter does nothing, nothing is written.
  - `TestSaveAsOverAShippedInstrumentLeavesThePack`: `ShippedDoc` is unchanged.
  - `TestSamplerSaveAsBakesNothing`: no new user sample, `LedgerSamplePCM` unchanged, and the library bundle carries the `SampleEdit`.
  - `TestSaveAsDoesNotTouchThePlayingSound`: `VoiceRevision` and `BundleRevision` of the row instrument are unchanged.
  - `TestNoSaveResetOrRestoreShipped`: no save/reset hit-area tags on either tab (desktop + mobile); ⋯ items = [Save As, Export, Import]; no `saveActiveRecipe` / `resetActiveRecipe` globals; neither key is in `i18n.AllKeys()`.
- [ ] Implement. Run `go test -tags test ./...` plus `TestTheHooksRegistry*` / `TestHooks_NumNonVerboseUpdated`. Browser: `synth_save_export_import` (§4b now uses `saveActiveRecipeAs(<same name>)`), `synth_panel_footer`, `webaudio_synth_save_persist`, `zz_rowrack_sampler_pads`, `wasm_bridge_smoke`.
- [ ] Screenshots: `synth_save_as_prompt`, `synth_save_as_new`, `synth_save_as_name_taken`, `sampler_save_as_prompt`, and the Synth and Sampler tab headers. Desktop + `MOBILE=1`, opened; the label, disabled state and note are legible and nothing overlaps.

### Task 6: The pick rule, the question card and instances

**Files:**
- Create `internal/ui/instrument_pick_rule.go` (pure), `internal/ui/instrument_pick.go` (flow) and `internal/ui/bundle_choice_card.go` (a `MenuCard` overlay opened with `menuCardEntry(bundleChoicePortalID, …, true /*mustAnswer*/, onClose)` stacked on `inst-menu`, as in `theme_picker.go:57-63`).
- Modify:
  - `drumview_instrument_helpers.go:223`: the menu `OnSelect` → `pickInstrumentForRow`.
  - `drumview_transport.go`: `AddInstrument` / `CycleInstrument` route through it; `SetInstrument` stays the low-level switch.
  - `js_exports_graph_ui.go:682-698`: `setRowInstrument` applies the rule and returns `"asked"` when it opened the card.
  - `instrument_import.go`: `applyToRow` uses the rule.
  - `i18n`: `KeyBundleChoiceTitle` ("%s has its own settings"), `KeyKeepRowSettings` ("Keep this row's settings"), `KeyKeepRowSettingsCopy` ("Keep this row's settings (a separate copy of %s)"), `KeyUseInstrumentSettings` ("Use %s's").
  - `menu_card_close_behavior_test.go` (a row with `mustAnswer: true`).
  - `scene_catalog.go`: `pick_bundle_question` and `pick_bundle_question_copy`.
- Test: `internal/ui/instrument_pick_test.go`.

**Interfaces — Produces:**
- `type pickDecision int`, with consts `pickUseTarget`, `pickCarryRow`, `pickAsk`.
- `func decidePick(row, target audio.AudioBundle) pickDecision`:
  - row neutral → `pickUseTarget`;
  - row set, target neutral → `pickCarryRow`;
  - `row.Equal(target)` → `pickUseTarget`;
  - else → `pickAsk`.
- `func (dv *DrumView) pickInstrumentForRow(row int, id instrument.ID)`. It decides with row = `BundleFor(rowInst)` and target = `BundleFor(id)`.
- `func (dv *DrumView) applyPick(row int, id instrument.ID, keepRow bool)`. One undo step and one log line (the existing row-instrument event; the answer adds none), applied in this order:
  1. When `keepRow` and another row plays `id`: `fork := nextFreeInstance(id)`, `audio.EnsureInstanceInstrument(fork)`, `SetBundle(fork, rowBundle)`, switch the row to `fork`.
  2. Otherwise, when `keepRow`: `SetBundle(id, rowBundle)`.
  3. Otherwise, when `id` is not yet in the circuit: `SetBundle(id, BundleFor(id))`, which pins the library bundle into the circuit.
  4. Then `SetInstrument`.
- `func nextFreeInstance(base instrument.ID) (instrument.ID, bool)`: the lowest n in 2..9 (`instrument.Instance`) with no row and no document. With none free, the card offers only "Use X's".
- When the last row leaves an instrument, `audio.DropBundle(old)`. Undo restores it, through the snapshot.

- [ ] Tests:
  - `TestPickDecisionTable`: pure; every spec §5.2 table row.
  - `TestPickThroughTheMenuFollowsTheTable`: real input, one case per row; the card appears only for `pickAsk`.
  - `TestPickQuestionCancelChangesNothing`: Esc and an outside tap; the export is byte-identical.
  - `TestKeepOnASharedInstrumentForks`: the other row's id and `BundleFor` are unchanged. Real-build twin `TestForkLeavesTheOtherRowsAudio` (`!test`): `RenderMasterMixOffline` of the other row alone is byte-identical before and after.
  - `TestPickAndAnswerAreOneUndoStep`: after one undo, the instance is gone and every bundle is back.
  - `TestForkLimitOffersOnlyUse`.
  - `TestSavedSampleEditAppliesOnPickNotOnOpen`: a saved bundle with a Sampler edit for a demo instrument; opening the demo gives the circuit's bundle; picking it on a neutral row gives the saved one.
  - `TestSaveAsOnAForkOffersNewUnderItsOwnName`.
  - `TestInstrumentFileImportFollowsThePickRule`.
- [ ] Implement; run the full `-tags test` suite. Screenshots of `pick_bundle_question` and `pick_bundle_question_copy`, desktop + mobile, opened: two choices, each listing what it keeps (for example "Sampler edit, EQ, 2 effects"), nothing clipped.

### Task 7: Docs and full runs

- [ ] Update CLAUDE.md:
  - "Every instrument is ONE document": Save As only; Reset and Restore shipped gone; the library stores the bundle.
  - A new paragraph, "Audio bundles: the instrument owns Sampler edit + EQ + effects", with its guards.
  - "Live edits retune the audio graph…": bundle writers.
  - The export section: the bundle is exported from one source.
- [ ] Update the AGENTS.synth.* mentions of Save/Reset; set the spec status to "approved 2026-10-30; Plan B built"; update the memory note.
- [ ] Runs:
  - `go test -tags test -modfile=go.test.mod ./...`, `make test-real`, `make js-audit`;
  - browser, on a private wasm: `wasm_bridge_smoke`, `synth_save_export_import`, `xplat_audio_stages`, `chain_spec_parity`, `webaudio_live_edit_budget`, `channel_lazy`, `resource_stats`;
  - `make bench-load DIMENSION=instruments` and `instrumentsFx` (laptop + lowPhone), compared with Plan A's baseline: no step regresses, and the idle-chain count is in the report.

**Plan B notes (test-file counts from the code map, for Task 2):** internal/ui — kit_apply 17, drumview_audio_eq_state 16, eq_per_instrument 15, export_import_audio 15, eq_export_import 14, export_import_playback_fidelity 12, export_import_master_audio_identity 12, export_import_master_audio_coverage 10, fx_panel_lifecycle 9, export_import_audio_determinism 6, eq_channel_curve_sync 6, eq_wheel_popup_isolation 5, rename_metadata_only 5, eq_mobile_curve_drag 4, instrument_roundtrip 4, insert_effects_roundtrip 3, eq_curve_drag_exclusive 2, row_rack_design_invariants 2, chain_stage_card 1, instrument_wav_locator 1, project_schema_version 1.
