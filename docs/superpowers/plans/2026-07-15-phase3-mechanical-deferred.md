# Phase 3 (Mechanical Wins) — Deferred Plan

**Status:** DEFERRED 2026-07-15. The three God-object extractions (Phase 1 PlaybackSlate, Phase 2a samplerPanel, Phase 2b synthPanel) are complete and green. Phase 3 is lower-value readability/declarative polish; it was deferred because two of its four targets are dirty with active WIP and the clean high-value target (`recalcButtons`) is delicate. Pick this up when the tree is calmer.

## Preconditions before starting
- `game_update.go` and `js_exports_graph_ui.go` must be **clean** (committed/settled). At defer time they were dirty with active WIP; extracting from them would collide.
- Re-confirm the baseline: build clean under Go 1.26.5; skip-list is just `TestFocusGraph_RespondsToEveryKnob` (re-measure — the user was actively fixing it); ui suite ~600–850s so gate with `-timeout 900s`.
- Use the proven **build-only implementer contract** (relocate/extract + `go build` only; controller runs all test gates; no `git stash/revert`, no background test runs).

## Targets (independent; do any subset)

### 3a. `js_exports_eq_widgets.go` → declarative table *(safest, smallest)*
Clean file, 38 exports. Convert the thin-delegator exports to a `{name, handler}` table + generic registrar loop. **Constraint:** keep each export NAME as a literal string — the drift guard `js_exports_catalogue_drift_test.go` regexes `js.Global().Set("<name>"`. Getter exports whose bodies build bespoke JS objects stay as-is. WASM-only (`//go:build js`); verify with the browser bridge smoke test. Modest LOC win, near-zero risk.

### 3b. `js_exports_graph_ui.go` → declarative table *(BLOCKED — dirty)*
Same as 3a but the biggest file (150 exports, ~2255 LOC). Wait until clean. Highest LOC win of Phase 3, but pure maintenance-surface (WASM-only, guarded).

### 3c. `Game.Update` phase-extraction *(BLOCKED — dirty)*
`game_update.go` is ~1137 LOC of ~20 linear phases. Extract each into a named `updateXxx()` method — pure readability, no behavior change. Respect the load-bearing ordering: the single `seqMu` critical section, deferred `pendingImportData`/`pendingActions`/`pendingNotifs` draining AFTER `seqMu.Unlock()`, and the `goto` labels (`eventsDone`, `pauseDrained`, `nextPulseForced`). Wait until clean.

### 3d. `recalcButtons` phase-extraction *(clean but DELICATE — do with care)*
`drumview_layout.go:25`, **833 LOC**. Extract `layoutTransportBar()` / `layoutMobileBottomSheet()` / `layoutGlobalChrome()` sub-methods. **Risk:** threads many shared locals (`p := Profile()`, rects, `eqH`) and has test seams (`eqPanelHeightForTest`) + mobile/desktop branches; layout tests (`row_controls_layout_test.go`, `row_typography_test.go`, `TestRowControls_NoCramping_AllProfiles`, etc.) are sensitive. TDD spine: those layout tests must stay green through every extraction. Extract only self-contained blocks; where a block shares locals, either pass them as params or compute them inside the helper — do NOT promote layout locals to fields casually. Stop and reassess if a block can't be cleanly separated.

### 3e. Deep draw-func helpers *(optional, low priority)*
`drawGridNodes`/`drawGridEdges`/`buildFXPanel` — extract inner loop bodies into named helpers (the proven `parityScan` guard-clause pattern). Pure readability.

## Note on the pattern
All three completed extractions kept a permanent back-ref (`g *Game` / `dv *DrumView`) rather than a narrow host interface, because the measured coupling was too broad in every case (75 for scheduling, 22 for sampler; synth similar). Phase 3 is NOT extraction — it's in-place readability/declarative refactoring, so the interface question doesn't arise. The realized architectural win from Phases 1–2 is **cohesion + navigability** (named `*playbackSlate`/`*samplerPanel`/`*synthPanel` collaborators), not interface isolation.
