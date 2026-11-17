# SamplerPanel Extraction (Phase 2a) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Extract the Sampler audio-panel tab out of the `DrumView` God-object into a cohesive `samplerPanel` collaborator with a narrow `SamplerHost` capability interface — behavior-preserving, guarded end-to-end by TDD.

**Architecture:** Two-rung safety ladder mirroring the proven Phase-1 PlaybackSlate extraction. **Rung A (verbatim, safe):** value-embed a `samplerPanel` struct into `DrumView`, move the ~46 sampler methods + `samplerState` onto it via the `dv := p.dv` verbatim prologue with `*DrumView` forwarding shims — zero behavior change, all 139 sampler tests stay green through the DrumView-facing API. **Rung B (the payoff, medium risk, its own checkpoint):** replace the `dv *DrumView` back-ref with a narrow `SamplerHost` interface (~15 ops), so the panel becomes independently testable with a fake host. Rung B is gated on a fresh coupling re-measurement (proceed only if the surface stays ≤~20 ops).

**Tech Stack:** Go 1.23 (`.tools/go/bin/go`); Ebiten stub (`-tags test`); the existing 139 sampler tests as the characterization net.

## Global Constraints

*Copied verbatim; every task's requirements include these.*

- **Bundled Go only:** `/home/ymolinar/Repos/beatmo/.tools/go/bin/go`, run from `src/go`.
- **NO COMMITS / no `git add`.** Leave work uncommitted (standing rule). "Checkpoint" steps = run the gate, confirm, PAUSE for review.
- **Implementers get a BUILD-ONLY contract** (the Phase-1 lesson): they relocate + `go build` (3 targets) only; they must NEVER run `go test`, NEVER `git stash/revert/reset/checkout/restore`, NEVER spawn background processes. **The controller runs every test gate.** Capture each task's diff to a file before dispatching review.
- **TDD is the spine.** The 139 existing sampler tests (`sampler_*_test.go`, `game_input_shortcuts_test.go`, `knob_interaction_functional_test.go`, `mobile_wheel_popup_keys_test.go`, …) are the upstream/downstream characterization net — they drive the sampler through `DrumView`'s public surface and MUST stay green at every rung (that is how we prove behavior is preserved). New behavior/structure additionally gets its own test written first.
- **Stub-path baseline:** 4 known user-owned tests fail under `-tags test` (real-Ebiten green). Gate = the suite with `-skip 'TestInstrumentCategoriesScrollMovesWindow|TestRowControls_NoCramping_AllProfiles|TestMobileRowNamesDoNotTruncate|TestFocusGraph_RespondsToEveryKnob'`; my diff must add ZERO new failures.
- **Preserve discipline invariants:** `DrumView.Draw` stays dispatch-only (AST guard `TestRenderPipelineDiscipline`); the `EQPanelZone`↔`EQCallbacks` tab routing stays the integration point (Sampler renders inside the EQ panel content rect via `EQCallbacks.DrawSamplerTab`/`SamplerTabHitAreas`/`SamplerTabUpdate`/`OnSamplerTabLayout`); zone input-isolation guards (`zone_input_isolation_discipline_test.go`) stay green.
- **`samplerState` is already a named struct** (`sampler_state.go`) — it becomes the panel's owned state.

## Component footprint (measured 2026-07-12)

Sampler-prefixed `*DrumView` methods: **36** in `sampler_panel_zone.go`, 4 in `sampler_wheel_popup.go`, 2 in `drumview_portal_open.go`, and 1 each in `synth_panel_zone.go`, `synth_panel_save_as_dialog.go`, `recipe_save_sink.go`, `drumview_release_commit.go`. Foreign coupling surface (candidate `SamplerHost`): ~15 genuinely-DrumView ops — `commitSamplerEdit`, `refreshInstruments`, `markAllRowsDirty`, `SetInstrument`, `selectAudioChannel`, `recordUndoStep`, `ResetActiveSampler`, `rowLabels`, `Rows`, `paramEditor`, `eqPanelZone`, `persistKnobStep`, `knobStepPref`, `selRow`, `synthTabActiveInstrument`.

**Shared-file methods stay put:** the 1-off sampler methods living in `synth_panel_zone.go`, `synth_panel_save_as_dialog.go`, `recipe_save_sink.go`, `drumview_release_commit.go` are shared infrastructure — they remain `*DrumView` methods and become part of the `SamplerHost` surface (the panel calls them through the host), NOT relocated. Only the sampler-dedicated files (`sampler_panel_zone.go`, `sampler_wheel_popup.go`, and the 2 sampler methods in `drumview_portal_open.go`) relocate onto `samplerPanel`.

---

## Task 0: Green baseline + coverage census

**Files:** none.

- [ ] **Step 1: Confirm the sampler net is green.** Run (from `src/go`): `../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run '[Ss]ampler' ./internal/ui`. Expected: PASS. Record the test count.
- [ ] **Step 2: Confirm the full scoped gate is green** (the Phase-1 exit state): the `-skip`'d ui+timeline suite. Expected: PASS. If red beyond the 4 known, STOP.
- [ ] **Step 3: Coverage census — identify any behavior with NO test** (TDD gap-fill target). Run `../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run '[Ss]ampler' -coverprofile=/tmp/sampler.cov ./internal/ui` and inspect coverage of `sampler_panel_zone.go`'s exported/host-facing methods. For any host-facing method (one in the `SamplerHost` candidate list) that is exercised only indirectly, note it — Rung B will need a direct test. No code changes.

## Task 1: Rung A — value-embed `samplerPanel`, relocate methods verbatim

**Files:** Create `internal/ui/sampler_panel.go` (the `samplerPanel` struct + back-ref); Modify `internal/ui/drumview.go` (embed), `internal/ui/drumview_ctor.go` (back-ref init), `internal/ui/sampler_panel_zone.go`, `internal/ui/sampler_wheel_popup.go`, `internal/ui/drumview_portal_open.go` (relocate the 2 sampler methods).

**Interfaces produced:** `type samplerPanel struct { dv *DrumView; st samplerState; ... }` value-embedded in `DrumView`; every relocated method retains a `*DrumView` shim so all 139 tests are unchanged.

- [ ] **Step 1: Create `samplerPanel`** holding a `dv *DrumView` back-ref and the `samplerState` (moved from DrumView's `sampler` field). Value-embed it in `DrumView` (anonymous) so `dv.sampler` and the panel methods stay promoted — the exact Phase-1 pattern (see `playback_slate.go`). Statement-init the back-ref in `drumview_ctor.go` (`dv.samplerPanel.dv = dv`); convert any `DrumView{...}` literal keying of `sampler` to statement form.
- [ ] **Step 2: Build** (3 targets). Expected: pass.
- [ ] **Step 3: Relocate the 36 `sampler_panel_zone.go` methods** onto `*samplerPanel` via the verbatim recipe: `func (p *samplerPanel) foo(...) { dv := p.dv; <byte-identical body> }` + `*DrumView` shim `func (dv *DrumView) foo(...) { [return] dv.samplerPanel.foo(...) }`. Build after the file.
- [ ] **Step 4: Relocate `sampler_wheel_popup.go` (4) + the 2 sampler methods in `drumview_portal_open.go`** the same way. Leave non-sampler methods in `drumview_portal_open.go` untouched. Build after each.
- [ ] **Step 5: Controller gate** (NOT the implementer): 3-target build + `go vet` (copylocks — `samplerState` has no locks, but confirm) + the sampler net (`-run '[Ss]ampler'`) + the full `-skip`'d scoped suite. All green; zero new failures. Fix any bare-`&DrumView{}` test that nil-panics on the back-ref by setting `dv.samplerPanel.dv = dv` after the literal (mirror the Phase-1 hazard fix).
- [ ] **Step 6: Task review** (diff-scoped) + checkpoint.

## Task 2: Rung B — narrow `SamplerHost` interface + fake-host tests *(CHECKPOINT before starting)*

**Precondition:** re-measure the foreign surface after Rung A (grep `p.dv.<X>` in `sampler_panel.go`/`sampler_panel_zone.go`/…). If it is ≤~20 genuinely-external ops, proceed. **If it balloons, STOP and escalate** — fall back to keeping the `dv` back-ref permanently (cohesion-only, the Phase-1 outcome).

**Files:** Modify `sampler_panel.go` (add `SamplerHost`, swap `dv`→`host`), the relocated method files (`p.dv.X` → `p.host.X()` for the ~15 foreign ops; `p.dv`-to-self stays `p`/`p.st`); Create `internal/ui/sampler_panel_host.go` (`*DrumView` implements `SamplerHost`) + `internal/ui/sampler_panel_test.go` (fake host + unit tests).

**Interface produced:**
```go
type SamplerHost interface {
    Rows() []*DrumRow
    RowLabels() []string
    SelectedRow() int                 // selRow
    ActiveChannel() string; InvalidatePanel()   // eqPanelZone.ActiveChannel / .Invalidate
    ParamEditor() *ParamValueEditor
    CommitSamplerEdit()
    RefreshInstruments()
    MarkAllRowsDirty()
    SetInstrument(row int, id string)
    SelectAudioChannel(id string)
    RecordUndoStep(k hooks.Kind)
    ResetActiveSampler() string
    PersistKnobStep(...); KnobStepPref(...)
    SynthTabActiveInstrument() string
}   // finalize exact set from the Rung-A re-measurement
```

- [ ] **Step 1: TDD — write the failing fake-host test first.** In `sampler_panel_test.go`, define a `fakeSamplerHost` implementing `SamplerHost` and a test exercising a host-coupled sampler operation end-to-end without a full DrumView (pick one that mutates via a host op, e.g. a sampler edit → assert `fakeSamplerHost.CommitSamplerEdit` was called and the panel state updated). Run: expect COMPILE failure (`SamplerHost`/`NewSamplerPanel` undefined).
- [ ] **Step 2: Add `SamplerHost` + `NewSamplerPanel(host)`; swap the back-ref.** Replace `dv *DrumView`/`dv := p.dv` with `host SamplerHost`/`h := p.host`; rewrite the ~15 foreign `dv.X` → `h.X()`, self refs → `p`/`p.st`. `*DrumView` shims stay (all 139 tests keep passing through them).
- [ ] **Step 3: `*DrumView` implements `SamplerHost`** in `sampler_panel_host.go` (thin accessors: `func (dv *DrumView) InvalidatePanel() { dv.eqPanelZone.Invalidate() }`, etc.). Wire `dv.samplerPanel = *NewSamplerPanel(dv)` in the ctor.
- [ ] **Step 4: Run the new test — expect PASS.**
- [ ] **Step 5: Controller full gate** — 3-target build + vet + sampler net + `-skip`'d scoped suite + the new fake-host tests. All green; zero new failures.
- [ ] **Step 6: Add 2–3 more fake-host tests** for the previously-untestable host-coupled paths flagged in Task 0 Step 3 (the concrete testability payoff). Gate green.
- [ ] **Step 7: Task review + final checkpoint.**

## Self-Review

- **Spec coverage:** matches design spec §Phase-2 (Sampler is the smaller first target; `SamplerHost` narrow-interface inversion; tests keep driving through DrumView via shims). ✔
- **TDD:** the 139-test net guards every rung (behavior preservation); Rung B adds fake-host tests written-first. ✔
- **Placeholder scan:** the `SamplerHost` interface is explicitly "finalize from the Rung-A re-measurement" — a discovery step, not a TODO (mirrors how Phase-1's host surface was measured, not guessed). Acceptable.
- **Risk gate:** Rung B is guarded by a re-measurement checkpoint and an explicit fallback (keep the back-ref) if the surface isn't narrow — the same evidence-first discipline that correctly killed Phase-1 Rung 3.
