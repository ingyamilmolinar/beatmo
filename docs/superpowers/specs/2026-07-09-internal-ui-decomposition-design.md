# internal/ui Decomposition — Architectural Rethink

**Date:** 2026-07-09
**Branch:** add-core-node-types
**Status:** DESIGN — approved shape, awaiting spec review before implementation planning
**Deliverable of this doc:** the target architecture + a 3-phase migration plan. No code changes accompany it.

---

## 1. Problem

The complexity graph (`make go-graph`, `build/go-graph.json`) is unambiguous: **`internal/ui` is the only part of the codebase with a complexity problem, and two God-objects *are* that problem.**

| Metric | Value | Source |
|---|---|---|
| `internal/ui` LOC (native-build AST) | 71,705 across 1087 funcs, 360 types | go-graph.json |
| `Game` method count | **342** `func (g *Game)` | grep verified |
| `DrumView` method count | **529** `func (dv *DrumView)` | grep verified |
| Combined | **871 methods** | — |
| Next-biggest type anywhere else | `Analyzer` — **21 methods** | agent audit |

Every entry in the top-30 cyclomatic/LOC/nesting rankings is a `Game.*` or `DrumView.*` method, except `Predictor.Ensure` (cyc 72), `applyUserStageParamsToModular`, and a handful of declarative data tables — all of which are **essential and already well-factored** (verified: `internal/audio`, `core/engine`, `internal/timeline`, `internal/audio/fingerprint` host no hidden God-object; their biggest types top out at 21–30 cohesive methods).

### The complexity is *breadth*, not *depth*

The single most important diagnostic fact: **the worst offenders are flat, not deeply nested.**

- `Game.initJSGraphUI`: cyc **455**, LOC 2158, **nesting 0** — 150 independent registration closures in one function.
- `Game.initJSEqWidgets`: cyc 193, LOC 988, nesting 0.

High cyclomatic complexity here means *many independent things in one namespace*, not a tangled algorithm. The pain is that ~9 independent subsystems share the `Game` receiver (and ~7 share `DrumView`), coupled by **receiver identity**, not by genuinely shared mutable state.

The few genuinely *deep* functions (`Game.Update` cd 23, `refreshDrumRow` nest 7, `seqScheduleTime` nest 7, `Predictor.Ensure` cyc 72) are either essential deep-modules (leave `Ensure`) or linear-phase sequences amenable to phase extraction (`Update`).

### What the prior campaign did — and did NOT do

A prior refactor (uncommitted, on this branch) anonymously **value-embedded 6 field clusters** into the two God-objects — `gridRenderCache`, `parityTracker`, `inputGestureState` (Game); `rowSpriteCache`, `rowsLayerCache`, `rowsWinCache` (DrumView) — via Go field promotion. This reduced *struct-declaration* LOC (Game 492→375, DrumView 590→513) with **zero call-site churn**.

**But it clustered fields only.** All 871 methods and the flat `g.field` / `dv.field` coupling are untouched. The interface surface was never narrowed. **Real decomposition — relocating methods, hiding state behind interfaces — is entirely open work.** This spec is that work.

---

## 2. Design principles

1. **Narrow the interface, not just the field list.** The win is decoupling: a collaborator you can understand and test without the rest of Game. Field-bag embedding was step zero; this plan relocates methods and introduces capability interfaces.
2. **Zero call-site churn at every step, via a safety ladder** (§4). Each intermediate state compiles and passes the full suite. No big-bang rewrites.
3. **Preserve every concurrency and rendering invariant** (§6). These are load-bearing and guarded by tests; the refactor's success criterion is "same behavior, narrower structure."
4. **Follow the patterns already in the codebase.** The Zone/capability-callback inversion is already shipping for EQ/Chain; the guard-clause phase-extraction is already proven on `parityScan`. This plan generalizes existing wins, it does not invent new machinery.
5. **Each phase is independently shippable and verifiable** — a PR-sized unit that leaves the tree green.

---

## 3. Sequencing rationale (thesis-first)

Order chosen: **Phase 1 PlaybackSlate (hardest) → Phase 2 panel inversion → Phase 3 mechanical flatten.**

Rationale: PlaybackSlate is the thesis of the whole rethink ("the God-objects are separable collaborators, not irreducible cores"). If the seqMu-guarded scheduling core *cannot* be cleanly extracted, that materially changes how much of the rest is worth doing. De-risk the central claim first while the reasoning is fresh, then the medium- and low-risk phases proceed with a proven pattern and a lighter cognitive load.

The three phases are structurally independent (Phase 2 touches DrumView panels; Phase 3 is mechanical/orthogonal), so this ordering costs nothing in coupling — it is purely a risk-sequencing choice.

---

## 4. The migration technique — a 3-rung safety ladder

Applied independently to each God-object extraction. Every rung is compiler- and test-verified before the next.

**Rung 1 — Field-bag embed** *(already done for 6 clusters; the pattern is proven)*
Group a cohesive field cluster into a named struct, anonymously **value-embed** it. Go field promotion keeps every `g.fieldX` call site unchanged. Blocker to check each time: does a `&Game{...}` / `&DrumView{...}` composite literal *key* any moved field? Composite literals can't set promoted fields → move those inits to statement form after construction. Never construct a literal of a struct containing a `sync.Mutex`/atomics (copylocks) — value-embed + statement-init avoids it.

**Rung 2 — Method relocation behind forwarding shims**
Move a method's *body* onto the sub-struct (`func (s *playbackSlate) seqScheduleTime()`), leaving a one-line shim on the God-object: `func (g *Game) seqScheduleTime() { g.playbackSlate.seqScheduleTime(...) }`. Call sites and **all existing tests are unchanged** — they still call `g.seqScheduleTime()`. This is the rung the prior campaign skipped. The sub-struct now needs access to what the method reads: pass a back-reference or (better) a narrow dependency at this stage as a plain struct field, not yet an interface.

**Rung 3 — Interface narrowing**
Replace the sub-struct's back-reference to the God-object with a **narrow capability interface** it depends on (`type SlateHost interface { ... }`, implemented by `*Game`). Production code now depends on the interface; the collaborator is independently constructible and testable with a fake host. Tests may *optionally* flip to `NewPlaybackSlate(fakeHost)` incrementally, per file — but the forwarding shims mean they don't *have* to.

The ladder guarantees: at no point does a single commit change both structure *and* hundreds of call sites. The shim layer is the shock absorber; it can be removed lazily once callers migrate to the collaborator directly (or kept permanently as the God-object's public façade — a legitimate end state).

---

## 5. The three phases

### PHASE 1 — Extract `PlaybackSlate` from Game *(high risk, highest value)*

**Thesis.** Game's B/C/D/E quartet — sequencer-scheduling, graph-beat-path build, drum-row refresh, parity — is a single cohesive *playback scheduling & row-slate materialization engine* buried inside Game. It communicates with the rest of Game almost entirely through channels (`hlCh`, `audioCh`) and the predictor/timeline services. Extract it as a collaborator `PlaybackSlate` (working name; alternatives: `SequencerCore`, `RowSlateEngine`).

**Fields to move** (from `game_struct.go`, verified line refs):
- Playhead counters: `nextBeatIdxs`, `seqNextIdxs`, `nextIdxSticky`, `elapsedBeats`, `startNextBuf`, `lastHLIdxByRow` (:117–175)
- Beat-path state: `beatInfos`, `drumBeatInfos`, `beatInfosByRow`, `isLoop*`/`loopStart*`/`loopLen*`/`originIdxs*`/`nextOriginIdx*` (:107–116)
- Sequencer spine: `seqCh`, `seqNextIdxs`, `seqQuit`, `seqMu` (:160–167), `seqPathSnap atomic.Pointer` (:170), `hlCh` (:178)
- Timeline/freeze: `timeline *timeline.Service` (:361), `frozenUpToByRow` (:364), `reconciledUpToByRow` (:379), `freezeBackfillIters` (:371)
- Path-change tracking: `pathSigByRow`, `rowsPathChanged`, `pathChangeBeatByRow`, `pathsDirty`, `rowRenderSig` (:347, :383–386)
- Row scratch: `rowStepsScratch`, `rowTypesScratch`, `rowSnapshotMode`, `lastStepsOffset`, `lastCellTypesOffset` (:351–358)
- Cross-thread invalidation: `audioGen atomic.Uint64` (:49)
- Trigger/mute logic state guarded by `triggerMu`: `nodeTriggerCountsByRow`, `lastEvalIdxByRowNode`, `nodeLogicTriggerCountsByRow`, `lastTriggeredByRow`, `lastFiredNodeByRow`, `muteUntilByRow`, `loopCountByRow` (:187–241)

**Methods to relocate** (the four families, ~1.9k LOC of the hottest code):
- `seqScheduleTime` (379 LOC, nest 7) + `scheduleSound`, `applySequencerHighlight`, mute/regular beat helpers
- `updateBeatInfos` (546 LOC) + per-row beat-calc helpers
- `refreshDrumRow` (422 LOC, nest 7) + `reconcileFrozen`, `predictAt`, `buildRowWindow`
- `parityScan`/`diffPredictorTimeline`/`parityCheck` — OR keep the already-embedded `parityTracker` as a *sub-collaborator of* PlaybackSlate (it already owns its own `parityMu`+atomics; nest it rather than flatten)

**The capability interface `PlaybackSlate` will depend on** (`SlateHost`, implemented by `*Game`), derived from what these methods read outside their own field cluster:
```go
type SlateHost interface {
    Predictor() *engine.Predictor          // engine (:35)
    Graph() *model.Graph                    // node/edge topology (:32)
    Rows() []*DrumRow                       // via g.drum — row indexing unit
    NodeCache() map[model.NodeID]model.Node // behind nodeCacheMu (:123)
    BPM() int                               // transport (:105)
    EnqueueHighlight(row, idx int, info model.BeatInfo)  // hlCh producer side
    PlaySound(req soundReq)                 // audioCh / scheduleSound sink
    // parity emit hook, mutateStructural choke-point, etc.
}
```
The exact surface is finalized during Rung 3; the point is it is **small and channel/service-shaped**, not "reach into 40 Game fields."

**Invariants this phase MUST preserve** (§6 details): the single seqMu region per frame, deferred-drain-after-unlock, `TryLock`-skip-if-held on the sequencer side, the single-critical-section invalidate→SetPaths→RebaseAt atomicity in `updateBeatInfos`, the `audioGen`/`seqPathSnap` atomic lock-free reads from the audio thread, and the `mutateStructural` choke-point.

**Migration order within Phase 1** (each a green checkpoint):
1. Rung 1: value-embed `playbackSlate` field cluster into Game (statement-init the mutex/atomic fields; check no composite literal keys them). ← struct-LOC win, zero behavior change.
2. Rung 2: relocate `refreshDrumRow` family first (most self-contained — it reads predictor+timeline, writes freeze state; fewest cross-thread hazards), behind a `g.refreshDrumRow()` shim. Verify.
3. Rung 2: relocate `updateBeatInfos` family. Verify (the single-seqMu-region test is the gate).
4. Rung 2: relocate `seqScheduleTime` family (runs on the sequencer goroutine — highest hazard; do last). Verify with `-race` on the parityGen concurrency tests.
5. Rung 3: introduce `SlateHost`, cut the back-reference, add a fake host, port a representative slice of tests to construct `PlaybackSlate` directly.

**Verification gate for Phase 1:** `-tags test` fast suite green; `xvfb` Mixer + real-Ebiten scheduling tests; `-race` on the 4 parityGen concurrency tests; `make test-parity` (cross-platform predictor/audio parity — the strongest behavioral guard on this exact code).

#### Implementation outcome (2026-07-12) — stopped at Rung 2; Rung 3 assessed and rejected

Rungs 1–2 were executed and verified: **54 methods** (row-refresh, beat-path, and scheduling families) relocated onto `*playbackSlate` via the verbatim `g := s.g` recipe with `*Game` forwarding shims; the 42-field cluster grouped into `playback_slate.go`. Every task passed the 3-target build, `go vet` (copylocks clean), the scoped fast suite, targeted `-race`, and native `TestCrossPlatformParity` (4/4). The scheduling logic is now a cohesive, named, greppable collaborator.

**Rung 3 (narrow `SlateHost` interface) was measured and deliberately not pursued.** The spec assumed a "narrow ~8-method `SlateHost`." Measurement of the relocated bodies proved this false: the relocated methods reference **149 distinct `g.<member>`**, of which — after excluding the 42 moved fields and the 54 sibling relocated methods — **75 are genuinely-external Game members** (`bpm`, `drum`, `engine`, `graph`, `grid`, `logger`, `activePulses`, `audioCh`, `currentStep`, `frame`, the whole parity subsystem, highlight/pulse ops, `seqMu`, `triggerMu`, `state`, `playFn`, render fields, …). A 75-method interface is not decoupling — it is Game's internals reflected through indirection — and building it would require rewriting all 54 method bodies (`g.X` → `s.X`/`host.X()`), destroying the byte-identical property that made Rungs 1–2 provably safe.

**Decision:** keep the `g *Game` back-reference permanently as the slate's collaboration edge (this was the spec's open-question #3 leaning). The independent-testability success metric is partially met — pure/near-pure slate helpers are unit-tested directly (`playback_slate_test.go`: `deterministicRoll`, `wrapBeatIndexRow`); the Game-coupled bodies remain driven through a minimal `Game`. **Architectural finding: the sequencer/beat-path/parity core is inherently broad-coupled to Game — it *is* the game loop — so "narrow-interface decoupling" is not an available win here.** The realized value is cohesion and navigability (a named `*playbackSlate` collaborator), not interface isolation.

---

### PHASE 2 — Finish the DrumView panel-component inversion *(medium risk, big LOC, proven pattern)*

**Key finding: the inversion is already half-built.** EQ and Chain are already real `Zone` components (`EQPanelZone` 80 methods, `ChainPanelZone` 45 methods) receiving their DrumView coupling through **capability-callback structs** (`EQCallbacks`, `ChainCallbacks`). Synth (`synth_panel_zone.go` 3971 LOC, 60 `dv`-methods) and Sampler (`sampler_panel_zone.go` 1744 LOC, 36 `dv`-methods) are the last two tabs whose renderers hang off `dv` — **but they are already routed through** `EQCallbacks.DrawSynthTab`/`DrawSamplerTab`/`…TabLayout`/`…TabHitAreas`/`…TabUpdate` closures wired in `drumview_ctor.go:481-508`, and `EQPanelZone.Draw` already dispatches `case TabSampler: z.callbacks.DrawSamplerTab(...)`.

**The rendering seam already exists as function pointers.** What remains is to give Synth/Sampler their own component structs and point those closures at `synthPanel.Draw` instead of `dv.drawSynthTab`.

**Overturning the prior "not advisable" verdict.** The earlier assessment reported "30+ DrumView internals" and rejected extraction. That count **conflated component-internal state** (`instEditor*` ×~40 fields, `sampler samplerState`) — which *moves into the component* — **with genuinely foreign operations.** The real cross-component surface, measured:

- **`SynthHost` ≈ 8 operations:** `Rows()`, `ActiveChannel()`, `MasterChannelSelected()`, `InvalidatePanel()` (the only high-frequency one, 7 sites), `ParamEditor()`, `CommitInstrumentParams(id)`, `SaveActiveRecipe()`, `ResetActiveRecipe()`, `PersistKnobStep/KnobStepPref`.
- **`SamplerHost` ≈ 13 operations:** the above shape plus `RowLabels()`, `CommitSamplerEdit()`, `RefreshInstruments()`, `MarkAllRowsDirty()`, `SetInstrument()`, `SelectAudioChannel()`, `RecordUndoStep()`, `ResetActiveSampler()`. None high-frequency, none a god-method.

That is an *order of magnitude* narrower than claimed. `samplerState` is **already a separate named type** (`sampler_state.go:45`) — half the extraction is done.

**Test coupling is NOT a blocker.** 117 sampler + 134 synth test sites reference internals, but:
- Field-access tests (75 sampler + majority of synth) **survive untouched** if the state stays reachable via `dv.sampler` / `dv.instEditor*` — re-expose the moved state as a field on DrumView (`dv.samplerPanel.st` aliased, or keep `dv.sampler` pointing at the panel's embedded state).
- Method-call tests (29 sampler + synth methods) **survive via one-line forwarding shims** (`func (dv *DrumView) drawSynthTab(...) { dv.synthPanel.Draw(...) }`) — exactly the shim style the ctor closures already use.

So production moves to the interface while **tests keep driving through DrumView.** A "tests target panel + fake host" rewrite is *optional*, incremental, per-file.

**Migration order within Phase 2:**
1. Extract `SamplerPanel` first (smaller, `samplerState` already extracted, 13-op host). Define `SamplerHost`, implement on `*Game`/`*DrumView`, move the 36 methods + state, retain `dv.*` shims, point the `EQCallbacks.*SamplerTab*` closures at it. Verify (135 sampler tests + `sampler_empty_state_draw_test.go`).
2. Extract `SynthPanel` (bigger, 8-op host). Same steps. Verify.
3. Optionally lift both to first-class `Zone`s later — **not required**; they render inside the EQ panel content rect as tabs, so the `EQCallbacks` routing is the correct integration point. Document this as the deliberate end state.

**Explicitly out of scope for Phase 2:** `recalcButtons` (912 LOC) is a **transport/global-chrome + mobile-bottom-sheet monolith, orthogonal to tab extraction** — verified: it lays out transport widgets only, touches Synth/Sampler solely for segment *enablement* (`synthViewDisabled()`/`samplerViewDisabled()`), and lays out zero panel buttons. Its refactor (below) is independent.

---

### PHASE 3 — Depth-flatten & declarative-ize the mega-functions *(low risk, mechanical)*

Four independent, low-risk cleanups. Each stands alone.

**3a. JS bridge → declarative table** (`js_exports_*.go`, 335 exports / 6162 LOC, all `//go:build js && !test`).
A `jsArgs`/`jsFn` facade already strips arg-parsing boilerplate. ~50–65% of exports are **thin delegators** (`g.method(args.Int(0), …); return nil`) — convert these to a declarative table row `{ "exportName", func(g,a){...} }` + one generic registrar loop. **Constraint (verified, documented in `js_exports_helpers.go:59-67`):** the drift guard `js_exports_catalogue_drift_test.go:46` scans source with regex `js\.Global\(\)\.Set\("([^"]+)"` — the table rows must keep each export name as a **literal string** the regex matches (`{"addNode", …}` satisfies it; a computed/derived key does not). Getter exports whose bodies *are* bespoke object-marshaling logic stay as-is (a table row would just hold the same closure — no reduction). This is WASM-only code, guarded by the catalogue drift test + `wasm_bridge_smoke.browser.test.js` — near-zero risk, pure maintenance-surface LOC win.

**3b. `Game.Update`** (1028 LOC, cd 23) → named phase methods. The body is ~20 **linear separable phases** (perf snapshot, benchmark lifecycle, frame-start snapshot, event drain, gesture polling, the seqMu critical section, deferred drains, camera, pulse/highlight, transport buttons, BPM re-anchor, refresh decision). Extract each to `updateXxx()` — respecting the `goto` labels (`eventsDone`, `pauseDrained`, `nextPulseForced`) and the load-bearing ordering of the seqMu Lock/Unlock relative to import/notify/action draining. Readability + testability only; no behavior change.

**3c. `recalcButtons`** (912 LOC, cyc 165) → extract `layoutTransportBar()` + `layoutMobileBottomSheet()` + `layoutGlobalChrome()`. Its 165 cyclomatic complexity is `mobile×desktop × collapsed×expanded × test-seam` branching — guard-clause + sub-method extraction (the `parityScan` pattern). Orthogonal to Phases 1–2.

**3d. Per-phase helpers for the remaining deep draw/build functions:** `updateBeatInfos` (if not already decomposed in Phase 1), `drawGridNodes` (519 LOC, nest 5), `drawGridEdges` (320 LOC, nest 6), `DrumView.buildFXPanel` (396 LOC), `rowsLayerMaybeRebuild`, `buildRowSprite` — extract inner loop bodies to named helpers. Follows the proven `parityScan` nest-7→5 guard-clause reduction.

---

## 6. Invariants the refactor must not break (guarded)

| Invariant | Guard | Where |
|---|---|---|
| One `seqMu` region per frame; deferred drain after Unlock | seqMu deadlock doc + tests | `game_update.go:417-425` |
| Sequencer side uses `TryLock`, skips if held | scheduling tests | `game_sequencer_schedule.go:18` |
| `updateBeatInfos` invalidate→SetPaths→RebaseAt is one atomic critical section | parity + stale-path tests | `game_graph_update_beat_infos.go:444-524` |
| `audioGen`/`seqPathSnap` read lock-free from audio thread | `-race` on parityGen tests | atomics |
| `mutateStructural` is the single structural-mutation choke-point | mutation-gate tests | `game_mutation_gate.go:59` |
| `Draw` / `DrumView.Draw` are dispatch-only (no draw primitives) | `TestRenderPipelineDiscipline` (AST) | `render_pipeline_discipline_test.go:32` |
| Single `gridTree.Draw` site; `gridDrawScreen`/`gridDrawCtx` stay Game-resident | `TestGridTreeDrawHasSingleDispatchSite` (AST) | `grid_draw_dispatch_discipline_test.go:27` |
| Zone visibility predicate gates BOTH Draw AND HitAreas | `TestZoneInvisibleEqualsNoInput` | `zone_input_isolation_discipline_test.go` |
| JS export names remain literal strings | `TestJSExportCatalogueDrift` (regex) | `js_exports_catalogue_drift_test.go:46` |
| Embedded structs with a mutex are never copied | `go vet` copylocks | — |

**Do NOT touch:** `Predictor.Ensure` (cyc 72 is essential, well-documented deep-module complexity — every branch guards a named correctness bug). The 6 existing field-bag embeds are the baseline to *extend*, not re-extract.

---

## 7. Testing strategy

- **Every rung is a green checkpoint** on the fast path: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui ./internal/timeline`.
- **Phase 1 additionally gates on:** `-race` (parityGen concurrency), `xvfb` real-Ebiten Mixer/scheduling, and `make test-parity` (the cross-platform predictor/audio golden — the definitive behavioral guard on the scheduling core).
- **Phase 2 additionally gates on:** the 135 sampler tests, synth-panel tests, and `audio_panel_render_pixels_test.go` (panel pixel content).
- **Phase 3 additionally gates on:** `wasm_bridge_smoke.browser.test.js` (JS bridge) and the AST discipline tests.
- **New tests to add:** once `SlateHost`/`SynthHost`/`SamplerHost` exist, add focused unit tests constructing each collaborator with a fake host — this is the *payoff* of the refactor (the scheduling core becomes testable without a full Game).

---

## 8. Success metrics

| Metric | Baseline | Target |
|---|---|---|
| `Game` method count | 342 | materially reduced (scheduling family → `PlaybackSlate`) |
| `DrumView` method count | 529 | −~96 (Synth 60 + Sampler 36 → panel components) |
| Largest single function (LOC) | `initJSGraphUI` 2158 | table-driven, ≪ |
| `Game.Update` LOC / phases | 1028 / inline | phase methods, thin dispatcher |
| Independently unit-testable collaborators | 0 (God-objects) | ≥3 (`PlaybackSlate`, `SynthPanel`, `SamplerPanel`) with fake hosts |
| Behavior | — | **identical** (all guards green, parity byte-identical) |

The *primary* success criterion is not LOC — it's **testability and comprehension**: can a developer understand and test the scheduling engine, the synth panel, and the sampler panel *without holding all of Game/DrumView in their head?* Today: no. After: yes.

---

## 9. Out of scope

- `core/engine`, `internal/audio`, `internal/timeline`, `internal/audio/fingerprint` — verified clean, no God-objects.
- `Predictor.Ensure` — essential deep-module complexity, do not refactor.
- Rewriting the ~250 existing panel/scheduling tests to target fakes — *optional, incremental, per-file* follow-on, not required by any phase.
- Lifting Synth/Sampler to first-class top-level `Zone`s — deliberately not done; they are EQ-panel tabs.
- Any behavior, layout, or audio change. This is a pure structural refactor.

---

## 10. Open questions for review

1. **Collaborator naming:** `PlaybackSlate` vs `SequencerCore` vs `RowSlateEngine`? (The name should convey "owns the row-indexed scheduling + freeze + slate materialization.")
2. **Parity placement:** keep `parityTracker` as a sub-collaborator *nested in* `PlaybackSlate`, or leave it embedded on Game and have `PlaybackSlate` emit parity events to it via the host interface? (Leaning: nest it — parity diffs the exact state the slate owns.)
3. **Shim lifetime:** remove the `*Game`/`*DrumView` forwarding shims once callers migrate, or keep them permanently as the God-object's public façade? (Leaning: keep — they document the public surface and cost nothing.)
