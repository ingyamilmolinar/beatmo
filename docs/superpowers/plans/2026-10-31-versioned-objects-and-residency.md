# Versioned objects and simpler residency — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Audio resources follow use without ever touching a sound, switching lives in the audio lane, and instruments/bundles/circuits become versioned content-addressed objects with Save As as the only save.

**Architecture:** Channels are created on first use and released only once silent (Go decides the held set; desktop mixer and audio.js each detect silence where the stages run). The lane keeps the last instrument it committed for a row until the new voice is rendered. A pure `internal/objects` package holds digests, histories and pruning; `audio` exposes a `Bundle` value read from / written to the existing per-instrument stores, and the user library becomes instrument histories in that store.

**Tech Stack:** Go 1.26 (`.tools/go/bin/go`), audio.js / `beatmo-dsp` worklet, Playwright browser tests.

**Spec:** `docs/superpowers/specs/2026-10-31-versioned-objects-and-residency-revisions-design.md` (amends `2026-10-25-instrument-residency-and-resource-budgets-design.md`).

## Global Constraints

- Never stage or commit (repo rule): every "commit" step below is replaced by "leave in the working tree".
- Never cut, shorten or change a sound; never drop or change a user setting without asking.
- The drum view holds no audio switching state.
- Every shipped template, the startup demo and every export import and re-export byte-identically; all export→import audio identity tests stay green unchanged.
- A version id is `sha256:<64 hex>` over Go `encoding/json` of the object, identity fields and effect instance uids excluded; identical natively and under `GOOS=js`.
- Release threshold −90 dBFS (amplitude 3.1623e-5), hold 250 ms, plus the chain's memory bound (Task 3).
- Retention default 20 versions per identity; current, version 0 and anything referenced are never pruned.
- Opening a circuit replaces the DOCUMENT with the user's current saved version (instances matched by `InstrumentLibraryIdentity`); the circuit keeps its own bundle.
- No circuit history and no `circuit_id` in files.
- UI tasks: screenshot scenes, desktop + `MOBILE=1`, opened and inspected (CLAUDE.md).
- Fast tests: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./...`; real build: `make test-real`; browser: `GO=/home/ymolinar/Repos/beatmo/.tools/go/bin/go node src/js/<name>.browser.test.js`.

## Review Focus

1. A switch to an instrument whose render fails or never lands: the row keeps sounding the old instrument (never silent, never late). → Task 4 test `TestLaneSwitchKeepsOldVoiceWhileNewNeverReady`.
2. A row deleted (or a circuit imported) while its channel's reverb/delay is still ringing: the tail rings out. → Task 3 tests `TestReleasedChannelRingsOutDelayTail` (Go) and the browser tail case.
3. Stop / seek mid-switch: the lane forgets `lastHeard` and the next play starts on the selected instrument. → Task 4 test `TestLaneSwitchStateResetsOnStop`.
4. Browser storage full on Save As: the user is told, the old library is intact, rows keep playing. → Task 8 test `TestSaveAsFailureKeepsLibraryAndNotifies`.
5. Undo after a Save As or a pick question answer: one step restores rows and their settings; the library version stays. → Task 9/10 tests.

---

### Task 1: Baseline measurement

**Files:**
- Modify: `src/js/audio.js` (`window.audioGraphStats`), `src/js/audio_probe_bench_helpers.js`, `scripts/audio_lane_ab.mjs`
- Modify: Go `perfStats` export (find with `grep -rn '"perfStats"' src/go/internal/ui`)
- Create: `src/js/residency_baseline.browser.test.js`

**Interfaces:**
- Produces: `audioGraphStats()` gains `liveChannels` (channelNodes minus main), `dspStages` (live fx/eq nodes), `renderEntries`, `renderBytes` (Float32 bytes in `renderCache`), `blocksPushed` (`instrumentBlocks.size`); `perfStats()` gains `mixerChannels`, `registeredInstruments`; helpers export `systemLoadOk(max)` (moved from `audio_lane_ab.mjs`).

- [ ] Step 1: Add the counters; `wasm_bridge_smoke` catalogue already covers both exports (shape asserted there gains the new keys).
- [ ] Step 2: Write `residency_baseline.browser.test.js`: boot → record stats; load a circuit of N instruments (N = 4, 8, 16, 32; half with a reverb+delay insert) through `importProjectJSON`, play 8 s with the probe (`startPlaybackWhenReady`, `startPlaybackProbe`), record late/missed and stats; switch one row every 500 ms for 8 s and record probe lag. Writes `bench-results/residency_baseline_<date>.json`; gates nothing except `probeGateFailures` structural limits; skips (reports) when `systemLoadOk(15)` fails.
- [ ] Step 3: Run it on today's code; keep the JSON as the baseline.

### Task 2: No channel until used

**Files:**
- Modify: `src/go/internal/audio/channels.go` (`resetInstrumentChannels`, `resetInstruments`), `engine.go`/`engine_wasm.go`/`stub.go` (`Register`), `src/js/audio.js` (`channelVolume`, analyser reads)
- Test: `src/go/internal/audio/channel_lazy_test.go`, `wasm_bridge_smoke` assertion

**Interfaces:**
- Produces: `chanMgr.lookup(id) (*Channel, bool)` (no create); JS `getChannelChain(id, create = true)` with read paths passing `false`.

- [ ] Step 1: Test `TestResetAndRegisterCreateNoInstrumentChannel`: after `ResetInstruments()` and `Register(x, …)`, `InstrumentChannelIDs()` is empty; after a voice plays on x (or `SetInsertEffects(x, …)`), it contains x.
- [ ] Step 2: Make reset/Register create nothing (reads use `lookup`; volume of a missing channel = 1). JS reads (`channelVolume`, analyser snapshot, `__testChannelSummary`) don't create.
- [ ] Step 3: Browser smoke: after boot `audioGraphStats().liveChannels === 0`.
- [ ] Step 4: Full fast suite + `xplat_audio_stages` + `webaudio_live_edit_budget` green.

### Task 3: Held set and silence-gated release

**Files:**
- Create: `src/go/internal/audio/residency.go`, `residency_test.go`
- Modify: `channels.go` (quiet tracking in `ProcessBlockLocal`), `engine_mixer.go` (release check), `src/js/audio.js` (held set, voice count per chain, release), `src/js/dsp_worklet.js` (quiet report), `chain_spec.go` (+gen) for the memory bound
- Modify: `src/go/internal/ui` warm-set builder (one function returns the rows' instrument set; call `audio.SetHeldInstruments` when it changes)

**Interfaces:**
- Produces: `audio.SetHeldInstruments(ids []instrument.ID)`; `audio.ChannelHeld(id) bool`; constants `ReleaseQuietDB = -90`, `ReleaseHoldSec = 0.25`, `ChainMemorySec(slots []EffectSlot) float64` (longest buffer any enabled effect can hold — read from `fx_host.c`'s buffer table; EQ-only = 0) exported to JS through `chain_spec.gen.js`; JS `window.setHeldChannels(idsJSON)`.
- Rule (both platforms): a channel not held is released when it has no sounding voice AND its input and output have stayed below the threshold for `ReleaseHoldSec + ChainMemorySec`. Re-held before that cancels the release. Release on desktop drops the channel (its processors rebuild from the insert/EQ stores on next acquire); in JS it disposes stage nodes, disconnects and deletes the chain.

- [ ] Step 1: Tests (`residency_test.go`, real mixer where stubs prove nothing → `!test` file `residency_real_test.go`):
  - `TestUnheldChannelReleasesAfterSilence` — voice ends, no fx → channel gone after hold.
  - `TestReleasedChannelRingsOutDelayTail` — 1000 ms delay, mix 1, feedback 0.7, row unheld right after the note: the master render equals one where the channel stays held, sample for sample, until the tail is below −90 dBFS; channel then gone.
  - `TestReheldBeforeReleaseKeepsChannel` — A→B→A within hold: same `*Channel`, no rebuild.
  - `TestRebuiltChannelEqualsFresh` — release, re-acquire: render equals a freshly built channel.
- [ ] Step 2: Implement Go side; wire `SetHeldInstruments` from the UI where the warm set is rebuilt (import, row add/delete/instrument change).
- [ ] Step 3: JS side: per-chain active voice count (increment at `start`, decrement on `ended`/cancel); `dsp_worklet.js` counts consecutive quiet frames on input+output and, when armed with `{watchQuiet: frames}`, posts `{quiet: true}` once reached (re-armed on any loud frame); chains without stage nodes release when the voice count hits 0 and the hold elapses.
- [ ] Step 4: Browser test `channel_release.browser.test.js`: delay+reverb tail on a row switched away keeps ringing (probe envelope on master equals the never-released arm until silence), then `liveChannels` drops; A→B→A within hold: `dspNodesCreated` unchanged.
- [ ] Step 5: Fast suite, `make test-real` audio subset, `xplat_audio_stages`, live-edit budget green.

### Task 4: The lane owns switching

**Files:**
- Modify: `src/go/internal/ui/audio_lane.go`, `audio_lane_source.go`, `audio_lane_note.go`; `src/go/internal/audio` (`VoiceReady(id, pitch) bool` on every platform)
- Test: `src/go/internal/ui/audio_lane_switch_test.go`

**Interfaces:**
- Consumes: `laneSource` gains `VoiceReady(n laneNote) bool`, `Warm(n laneNote)`, `Retarget(n laneNote, inst instrument.ID) laneNote` (same cell, other instrument, hash recomputed).
- Produces: lane-local `heard []instrument.ID` (last instrument committed per row; cleared by `Reset`) and `switching []bool`.
- Rule: in `noteAt`, a sound note whose `Inst != heard[r]` (heard set) and whose voice is not ready is retargeted to `heard[r]`, `Warm` is requested, `switching[r]=true`; a switching row is re-reconciled every tick until its voice is ready (then the existing Replace swaps not-started notes). `heard[r]` updates on every sound commit/replace.

- [ ] Step 1: Tests with the scripted source: `TestLaneSwitchKeepsOldVoiceUntilReady` (old instrument for every note until ready, then new for notes after guard; no note dropped/late/doubled), `TestLaneSwitchKeepsOldVoiceWhileNewNeverReady`, `TestLaneSwitchStateResetsOnStop`, `TestLaneFirstNoteIsNeverRetargeted` (no `heard` yet → selected instrument).
- [ ] Step 2: Implement; Game source: `VoiceReady` = every layer pitch `audio.VoiceReady`; `Warm` = `audio.PrewarmVoice`.
- [ ] Step 3: Separation guard `TestDrumViewHasNoSwitchState` (reflect over `DrumRow`/`DrumView` fields: nothing named sounding/pending switch). Existing lane suite (`audio_lane_*_test.go`, oracle) green.

### Task 5: Voice-cache byte budget (browser)

**Files:**
- Modify: `src/js/audio.js` (`renderCache` set/get), `src/go/internal/ui/runtime_profile.go` (`VoiceCacheBudgetBytes`, browser 256 MiB with bench citation from Task 1), push via existing init bridge
- Test: `src/js/voice_cache_budget.browser.test.js` (pure page, no Go needed if the helper is exported) or a Node-pure unit over the eviction function

**Interfaces:**
- Produces: `evictRendersOverBudget(budget, protect)` — LRU by `lastUse`, never a held instrument's render (`setHeldChannels` set) and never a render with live refs (scheduled, not ended).

- [ ] Step 1: Test: fill over budget → oldest unprotected evicted, protected kept, total ≤ budget.
- [ ] Step 2: Implement; desktop `globalVoiceCache` already bounded? Check; if unbounded, apply the same budget there with a Go test.

### Task 6: `internal/objects`

**Files:**
- Create: `src/go/internal/objects/objects.go`, `objects_test.go`

**Interfaces:**
- Produces:
  - `type Kind int` over `enum.Names` (`KindBundle`, `KindInstrument`, `KindCircuit`), strict decode.
  - `func Digest(v any) string` — `sha256:` + 64 hex of `json.Marshal(v)`.
  - `type Object struct { Kind Kind; Body json.RawMessage; Refs []string }`
  - `type History struct { Versions []string; Current int }` (`Current` index into `Versions`; -1 none)
  - `type Store struct { Objects map[string]Object; Histories map[string]*History }` with `Put(kind, body any, refs ...string) string`, `Get(digest) (Object, bool)`, `Save(identity string, digest string)` (append, make current; re-saving the current digest is a no-op), `Move(identity, digest) error`, `Current(identity) (string, bool)`, `Prune(limit int, pinned func(identity string) []string)`.
  - JSON round-trip of `Store` is stable.

- [ ] Step 1: Tests: digest stable + identity-independent (caller strips), `Save`/`Move` past and future keep all versions, saving after moving back appends, `Prune` keeps newest N + current + pinned + everything reachable through `Refs`, unreachable objects removed.
- [ ] Step 2: Implement (no game imports).

### Task 7: The bundle value

**Files:**
- Create: `src/go/internal/audio/bundle.go`, `bundle_test.go`, `eq_settings.go` (per-instrument EQ settings store)
- Modify: every `DrumRow` mixer-field reader/writer in `src/go/internal/ui` (EQ panel, FX panel, import/export, undo snapshot, kits, Synth/Sampler), `drumview.go` (fields removed)

**Interfaces:**
- Produces:
  - `type EQSettings struct { GainsDB [10]float64; BandMuted [10]bool; HPF, LPF bool; HPFHz, LPFHz float64 }` with `IsFlat()`.
  - `type Bundle struct { SampleEdit *SampleEdit; EQ *EQSettings; Effects []EffectSlot }` (nil/empty = absent part); `Bundle.Digest() string` via `objects.Digest`.
  - `audio.EQSettingsFor(id) (EQSettings, bool)`, `audio.SetEQSettings(id, s)` (stores + compiles through the existing `BuildEQProgram` path; flat clears, never a unity chain).
  - `audio.CurrentBundle(id) Bundle`, `audio.ApplyBundle(id, b Bundle)`.
  - `type BundleMode int` (`ModeInstrument` default, `ModeCircuit`, `ModeOverride`, `ModeMerge`) over `enum.Names`; `Effective(inst, circ Bundle, m BundleMode) Bundle`.
- Rule: the row reads its instrument's settings; no second copy on `DrumRow`. The project file shape is unchanged (`eq`/`effects`/`sample_edit` per row entry written from `CurrentBundle`).

- [ ] Step 1: Tests: `TestEffectiveEveryModeAndPart` (table), `TestBundleRoundTripThroughStores`, `TestBundleDigestIgnoresEffectUIDs`.
- [ ] Step 2: Implement the store + value.
- [ ] Step 3: Remove the seven `DrumRow` fields, routing every site through `EQSettingsFor`/`SetEQSettings` and the insert-effect store. Existing EQ/FX/undo/import tests pass unchanged in meaning (update only field access).
- [ ] Step 4: Full fast suite, `make test-real` export→import identity set, template round-trip, screenshots of EQ + FX panels unchanged.

### Task 8: Versioned library and open resolver

**Files:**
- Create: `src/go/internal/audio/library_versions.go`, `library_versions_test.go`
- Modify: `instrument_library.go` (userLib backed by `objects.Store`), `instrument_circuit_docs.go` (resolver), `src/go/internal/userprefs/instrument_store.go` (`library` section; `persistLocked` returns error), prefs (retention limit, default 20)
- Modify: `src/go/internal/ui/project_file.go` (`circuitDigest`)

**Interfaces:**
- Produces:
  - `type InstrumentVersion struct { Doc *instrument.Doc; Sample string; Bundle string }` (doc id/origin cleared; `Sample` = PCM sha256 for sample instruments).
  - `audio.SaveInstrumentVersion(id instrument.ID, d instrument.Doc, b Bundle) error` (puts bundle + version, appends to history, prunes, persists; error on persist failure with the store unchanged).
  - `audio.CurrentLibraryVersion(id) (InstrumentVersion, bool)` — only a user-saved current (version 0 = shipped = false).
  - `audio.LibraryBundle(id) (Bundle, bool)`.
  - Store persistence: `InstrumentStore.LoadLibrary() ([]byte, error)`, `SaveLibrary([]byte) error`.
  - Read-in: on first load with no `library` section, every `instruments` entry becomes version 1; a `sample_edits` entry joins that version's bundle only when its id has a saved document. Old sections are kept, never written again.
  - `circuitDigest(pf projectFile) string` (instrument docs replaced by their version digests).
- Rule: `OpenCircuitInstrumentDocs` replaces only the document from `CurrentLibraryVersion`; bundles stay the circuit's.

- [ ] Step 1: Tests: `TestSaveAppendsVersionAndKeepsOld`, `TestOpenUsesNewestDocumentKeepsCircuitBundle` (incl. an instance `kick--2`), `TestPointerOnShippedPlaysCircuitCopy`, `TestReadInKeepsEverySavedInstrument`, `TestSampleEditWithoutDocCreatesNoVersion`, `TestSaveAsFailureKeepsLibraryAndNotifies` (failing backend), `TestVersionDigestSameNativeAndJS` (golden file read by a browser test), `TestCircuitDigestStable`.
- [ ] Step 2: Implement; existing `synth_save_trumps_circuit_test.go` and library tests green.

### Task 9: Save As is the only save

**Files:**
- Modify: `recipe_save_sink.go`, `synth_panel_zone.go`, `sampler_panel_zone.go`, `action_menu_open.go`, Save As dialog files; delete Reset/Restore code paths and their tests/scenes
- Test: `save_as_only_test.go` (real input through `Game.Update`), scenes `synth_save_as_over`, `synth_save_as_new`, `synth_save_as_taken`

**Interfaces:**
- Consumes: `audio.SaveInstrumentVersion`, `CurrentBundle`, `InstrumentLibraryIdentity`.
- Produces: `classifySaveAsName(typed string, editing instrument.ID) saveAsKind` (`saveAsOver` / `saveAsNew` / `saveAsTaken`; trim + case-fold; taken compares display names in every locale).

- [ ] Step 1: Tests: no Save/Reset/Restore button in either tab; prefill = current name; three button states; "over" appends a version (old retained) and rows keep their id; "new" repoints rows (one undo step); taken disables confirm; persist failure notifies.
- [ ] Step 2: Implement by changing `saveRecipeForInstrument` (append version, bundle included) and keeping `SaveActiveRecipeAsFor`.
- [ ] Step 3: Scenes desktop + mobile captured and inspected.

### Task 10: Picks and kit swaps never drop settings

**Files:**
- Modify: instrument-menu pick path, `kit_binder.go`, a new `pick_settings_card.go` (MenuCard, `menuCardEntry` mustAnswer)
- Test: `pick_settings_test.go`, row in `TestEveryMenuClosesTheSameWay`; scenes `pick_settings_question`, `kit_swap_settings_question`

**Interfaces:**
- Consumes: `CurrentBundle`, `LibraryBundle`, `ApplyBundle`.
- Rule (residency spec §5.2 table): row neutral → picked instrument's; row set + picked neutral or equal → carry over; both set and different → question "Keep this row's settings" (adds "(also changes row N)" when another row plays it) / "Use <X>'s"; cancel = nothing changes; pick + answer = one undo step. Kit swap: carry over, one question for the whole swap when any row needs it.

- [ ] Step 1: Tests per table row (real input), cancel, undo, kit swap with and without conflict.
- [ ] Step 2: Implement; scenes desktop + mobile captured and inspected.

### Later (not in this plan)

Caps (residency spec §6) sized from Task 1's numbers after Task 5; the governor only if those runs show late notes it would fix.
