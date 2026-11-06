# Value Wheel + TextField Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace every typed numeric entry with one shared wheel component (go-to inline in the nav card; BPM / node values as wheel popups; logs HH:MM inline), and route every text entry through one `TextField` that owns its desktop/mobile keyboard wiring.

**Architecture:** `MobileWheelPopup` keeps its name and becomes a frame that EMBEDS (by value) a new `ValueWheel` view, so every field/method existing tests touch still resolves. All Knob/Badge/ParamDef access moves behind `WheelModel`; `knobModel` reproduces today's semantics byte-for-byte, `NumberModel` serves the new surfaces. `TextField` wraps `TextInput` and self-registers native candidates.

**Tech Stack:** Go 1.26, Ebiten 2.9, `-tags test` ebitenstub harness, Playwright browser tests, screenshot harness.

**Spec:** `docs/superpowers/specs/2026-10-03-value-wheel-and-text-field-design.md`

## Global Constraints

- Never stage or commit (user memory rule) — "commit" steps below are replaced by "run the phase gate".
- Bundled Go only: `../../.tools/go/bin/go` from `src/go`. Fast suite: `go test -tags test -modfile=go.test.mod ./internal/ui/`.
- Labels shown to the user are **1-based beats**; internals stay 0-based steps.
- Wheel direction/feel unchanged: drag DOWN / scroll DOWN raises the value; ladder coarsest at top; `wheelStepEventsPerNotch = 3`; arrows Up/Down value, Right = coarser.
- Live-apply vs gesture-commit: live writes per detent; ONE emit + ONE undo per gesture.
- Only sanctioned files may read arrow keys (`TestArrowKeysReadOnlyAtSanctionedSites`) — update the allowlist when the reader moves.
- Every new popup/menu: `TestEveryPortalMenuUsesMenuCard` exemption or MenuCard; a row in `TestEveryMenuClosesTheSameWay`.
- UI work is done only with a screenshot opened and inspected (CLAUDE.md, BEFORE/AFTER pairs).
- Screenshots with `XDG_CONFIG_HOME=<scratch>/cfg`.
- No `runtime.GOOS` checks in `internal/ui`; no inline colour literals (token discipline ratchet).

## Review Focus

1. **Go-to while playing**: the playhead keeps moving while the card is open → wheel value must stay where the user put it; Go seeks to the wheel value, not the playhead. (Task 3.3 test `TestGotoWheelDoesNotChasePlayhead`.)
2. **Timeline length changes while the card is open** (beats +/-) → wheel max must follow; a pending value beyond the new end clamps. (Task 3.3 `TestGotoWheelClampsWhenTimelineShrinks`.)
3. **Tap-away from a node-value wheel by tapping ANOTHER node** → commit exactly once for the old node, never write the new node. (Task 4.2 `TestNodeWheelCommitsToOriginalNodeOnRetarget`.)
4. **BPM wheel + Esc after many detents** → BPM, audio clock, and undo stack return exactly to pre-open state; zero undo steps recorded. (Task 4.1 `TestBPMWheelEscRestoresAndRecordsNothing`.)
5. **Mobile rename after TextField migration** → tapping "Rename" in the row context menu still places the native input over the row label (trigger ≠ input rect). (Task 2.2 `TestRenameTriggerKeepsLabelInputRect`.)

---

## Phase 0 — Baseline

### Task 0.1: Record baseline + BEFORE screenshots

- [ ] Fast UI suite → `scratch/baseline_ui.txt`; note any red tests and prove cause before proceeding.
- [ ] Build screenshot binary; capture BEFORE (desktop 1280×720 and mobile `-mobile -scene-pass mobile -win-w 390 -win-h 844`) for: `nav_menu_open`, `nav_menu_position_sync`, `locale_es_nav_menu`, `sampler_wheel_open`, `sampler_wheel_keyboard_step`, EQ wheel scene(s), `theme_editor`, settings scene, logs datetime picker scene, node sidebar scene, save-as scenes, inst menu search, template search, FX panel.

## Phase 1 — WheelModel + ValueWheel (pure refactor)

### Task 1.1: Characterization tape (written against TODAY's code)

**Files:** Create `src/go/internal/ui/wheel_characterization_test.go`, golden `testdata/wheel_characterization.golden.json`.

Drive `samplerWheelPopup` (idx 0 Start, idx 2 Transpose) and `eqWheelPopup` (band 3) through their real open paths (`openSamplerKnobWheelPopup`, `openEQKnobWheelPopup`) and the real hit handler / key claimant:

```go
type wheelTapeStep struct{ Op string; X, Y, N int }
// ops: press, drag, release, wheel, key-up, key-down, key-left, key-right, enter, esc, rung-tap
type wheelTapeFrame struct {
    Op string; Value float64; StepMul float64; Rung int; Dirty bool; Open bool; Commits int
}
```
Tape per popup: drag 3 notches down, release; click row +2; wheel ×7 down; wheel ×3 over strip; rung tap index 1; key-up ×2; key-right; shift+wheel ×3; esc (record restore); reopen; key-down; enter.
Golden written with `WHEEL_TAPE_UPDATE=1`; the test asserts frame-by-frame equality.

- [ ] Write test, generate golden on the CURRENT tree, run twice → identical.

### Task 1.2: `WheelModel` + `knobModel`

**Files:** Create `wheel_model.go`, `wheel_model_knob.go`, `wheel_model_test.go`. Modify `mobile_wheel_popup.go`, `mobile_wheel_render.go`.

**Produces:**
```go
type WheelModel interface {
    Snapshot() float64
    Restore(v float64)
    CenterLabel() string
    RowLabel(offset int) (string, bool) // distinct-label walk, offset>0 = larger
    SelectRow(offset int) bool
    Step(n int) bool                    // n detents; false when value unchanged
    StepFine(n int) bool                // shift: one rung finer, not persisted
    HasLadder() bool
    LadderLen() int
    LadderIndex() int
    LadderLabel(i int) string           // i indexes fine→coarse (Badge order)
    SetLadderIndex(i int) bool
    AdjustLadder(d int) bool            // Badge semantics: +1 = finer
}
func newKnobModel(k *Knob, b *KnobStepBadge, def audio.ParamDef, discrete bool) WheelModel
```
`WheelBinding` gains `Model WheelModel`; `(*MobileWheelPopup).model()` returns `Model` or a cached `newKnobModel(...)` from the legacy fields. Every `w.binding.Knob/Badge/Def` use in popup + render is rewritten to the model; StepMul sync moves into `knobModel` ladder methods. Dirty semantics are preserved exactly (drag/wheel set dirty when n≠0; keys only on change).

- [ ] Unit tests for `knobModel` vs the old helpers (enum + continuous) — `RowLabel` matches `walkRows`' sequence.
- [ ] Characterization tape + all `mobile_wheel_*`, `sampler_wheel_*`, `eq_wheel_*` tests green, unchanged.

### Task 1.3: Split `ValueWheel` out of the popup

**Files:** Create `value_wheel.go` (state, layout(body), input, keys), move barrel/strip draw into `value_wheel_render.go`. `MobileWheelPopup` embeds `ValueWheel` by value and keeps rect/anchor/title/open/initialValue/Accept/Cancel/Draw-frame.

**Produces:**
```go
type ValueWheel struct { /* valRect, resRect, centerH, notchers, drag, hover, dirty */ }
func (v *ValueWheel) Bind(m WheelModel, hooks ValueWheelHooks)
type ValueWheelHooks struct{ OnChange, OnResolution, OnCenterTap func(); Accent func() color.Color }
func (v *ValueWheel) Layout(body image.Rectangle)
func (v *ValueWheel) HandleInput(x, y int, pressed bool) bool
func (v *ValueWheel) HandleWheel(x, y, steps int) bool
func (v *ValueWheel) ClaimedKeys() []ebiten.Key
func (v *ValueWheel) HandleClaimedKeys() bool
func (v *ValueWheel) HoverAt(x, y int)
func (v *ValueWheel) DrawBody(dst *ebiten.Image)
func (v *ValueWheel) Dragging() bool
```
- [ ] Move code, update `TestArrowKeysReadOnlyAtSanctionedSites` allowlist if the key reader file changed.
- [ ] Gate: tape + full fast UI suite green; `sampler_wheel_open` screenshot pixel-identical to BEFORE (compare with a pixel diff, not by eye).

### Task 1.4: `NumberModel`

**Files:** Create `wheel_model_number.go`, `wheel_model_number_test.go`.
```go
type NumberModel struct {
    Min, Max, Value float64
    Ladder []float64 // fine→coarse
    Rung int
    Format func(v, step float64) string
    LadderFormat func(step float64) string
    OnRung func(i int) // optional persistence
}
```
Step snaps to the rung grid relative to Min, clamps to [Min,Max]; RowLabel(offset) = Format(snap(Value)+offset·step) or false out of range; SelectRow sets that value; StepFine uses Ladder[Rung-1].
- [ ] Tests: clamp both ends, snap from off-grid value, rung change, row labels out of range, StepFine at finest rung no-ops to normal step.

## Phase 2 — TextField

### Task 2.1: `TextField` + registry

**Files:** Create `textfield.go`, `textfield_test.go`. Modify `native_gesture.go`.
```go
type TextField struct {
    *TextInput
    ID, Owner string
    Live bool                          // mirror native value every frame (search)
    InputRect func() image.Rectangle   // optional: native input placed elsewhere
    Visible func() bool
    OnLive func(string)
    OnCommit func(string)
}
func NewTextField(id, owner string, style TextInputStyle, maxLen int) *TextField
func (dv *DrumView) registerTextField(f *TextField)
func (f *TextField) Update() bool // TextInput.Update + native polling
```
`softKeyboardCandidates` / `nativeInputCandidates` build from the registry (visible fields with non-empty rect). Focus hooks call `softKeyboardShow("text")` / `softKeyboardHide()`.
- [ ] Tests: unique IDs; each visible field → exactly one candidate per platform (`SetForceMobileProfile`); hidden field → none.

### Task 2.2: Migrate the eight fields

Rename (context-menu trigger + label `InputRect`, poll-commit), WAV name (poll-commit), inst search / template search / logs search (Live via `mobileInputGetValue`), FX search (Live, NEW mobile), Synth Save As + Sampler Save As (poll-commit, NEW mobile). `menu_search_field.go` builds a `TextField`.
- [ ] `TestRenameTriggerKeepsLabelInputRect`; `TestSaveAsAndFXSearchHaveNativeCandidates`; `TestNoNewTextInputOutsideTextField` (AST scan, allow `textfield.go` + tests).
- [ ] Existing rename / naming / search / mobile_input tests green. Browser: `mobile_native_input`, `mobile_text_input` green.

## Phase 3 — Go-to inline wheel

### Task 3.1: Go-to models + formatting
**Files:** Create `nav_goto_model.go`, `nav_goto_model_test.go`.
- `newGotoBeatModel(div, maxSteps, startSteps)`: Ladder `{1, div, 4·div, 16·div}` (drop the `1` when div==1), default rung = 1 beat, Format → `formatGotoBeat(steps, div)` = `"Beat N"` / `"Beat N · a/b"` (1-based, reduced fraction).
- `newGotoTimeModel(maxSec, startSec)`: Ladder `{1,5,15,60}`, Format `formatClock`.
- `gotoConvert`: beat→time `steps/div·60/bpm` (floor seconds); time→beat `timeToSteps`.
- [ ] Tests: labels at 0, mid-beat, last step; div 1/4/8; conversion round trips.

### Task 3.2: Nav card layout + draw
**Files:** Modify `navigation_panel.go`, `drumview_overlay_nav_comp.go`.
Rows: readout(title band) · jog · unit · divider+"GO TO" caption with Beat|Time segmented control · wheel (5 rows × tickGap) · footer (secondary readout left, **Go** primary button right). Remove gotoBeat/gotoTime rows. Card width `menuWEditor()` desktop; bottom sheet on mobile with height cap.
- [ ] Scenes `nav_goto_beat`, `nav_goto_time`, `crop_nav_goto` → capture desktop + mobile, inspect.

### Task 3.3: Behaviour
Preview-only: wheel changes set `dv.timelineZone` scrub ghost to the pending step; Go/Enter → `RequestSeekSteps(steps, seekOriginCounter)`, re-seat wheel at new playhead, clear ghost; Esc → cancel wheel + clear ghost (existing Esc closes card); outside tap → cancel; key claims via `ValueWheel`.
- [ ] Real-input tests (`nav_goto_wheel_test.go`): scroll doesn't seek; ghost shown at pending; Go seeks once; Enter seeks; Esc no seek; outside tap no seek; mode switch converts; `TestGotoWheelDoesNotChasePlayhead`; `TestGotoWheelClampsWhenTimelineShrinks`; arrows don't pan camera while open; Space still toggles play; closed claims nothing; add to `TestEveryMenuClosesTheSameWay`.
- [ ] Fix scrub-ghost chip + nav trailing labels to 1-based; scene `nav_goto_preview_ghost`.

## Phase 4 — BPM, node values, logs

### Task 4.1: Generic value-wheel popup + BPM
`dv.valueWheelPopup` (MobileWheelPopup with NumberModel) + portal id `value-wheel-popup`. `TransportZone.SetBPMLive(b)` (no emit) + commit → `emitBPMChange` once. bpmOpenAdapter opens the wheel. ± unchanged.
- [ ] Tests: detent changes engine BPM live; close → one undo + one emit; `TestBPMWheelEscRestoresAndRecordsNothing`; ± repeat still works; scene `bpm_wheel_open`.

### Task 4.2: Node values
`nodeParamLimits` (vol −40..+12 dB/muted, dur 0.25..4, N 1..64, P 0..1, groove 0..1) used by sidebar ±, JS bridge, wheels. `setNodeParamsLive` / `commitNodeParams`. Tapping the value text in the sidebar opens the popup.
- [ ] Tests per value: live predictor update, one emit/undo per gesture, ± clamps, `TestNodeWheelCommitsToOriginalNodeOnRetarget`; scene `node_volume_wheel_open`.

### Task 4.3: Logs HH:MM wheels
Replace hour/minute ± in `logs_datetime_picker.go` with two inline `ValueWheel`s (NumberModel, no ladder).
- [ ] Test: same `sel` timestamps as the ± path for a scripted sequence; scene `logs_time_wheels`.

## Phase 5 — Remove typed numeric entry

- [ ] Delete `ParamValueEditor`, `ValueSpec`, `ValueOpen`, `positionValueSpec`, `parseClock`, `openGotoBeatEditor/TimeEditor`, `openSamplerParamEditor`, `openEQDBEditor` + `eqDBSpec`, `openSynthParamEditor`, `openArmedNumericEditor`, legacy `transport.go` `Transport`, `activeParamEditorNativeRects`, numeric native ids, the wheel `OpenEditor` hook (centre tap becomes inert on continuous, Accept stays on discrete).
- [ ] Map each deleted test to its replacement in a table in this plan's appendix; port behaviour assertions that still apply.
- [ ] `wasm_bridge_smoke` catalogue unaffected (no JS exports removed) — verify.

## Phase 6 — DESIGN.md + tokens + ratchet

- [ ] DESIGN.md: `profileOverrides` `menuWAction(Min/Max)`, `menuWList`, `menuWEditor`, `menuWWideMax` (desktop values 160/240, 300, 360, 960; mobile same values, unused because sheets are full width); remove `popupPanelW`; new "Menu sizing & layout" subsection. `make gen-design-tokens`.
- [ ] `settingsOverlayW` → `menuWEditor()`; nav + value-wheel popup use it.
- [ ] `TestMenusUseWidthClass` ratchet list.
- [ ] Drift/discipline/lint tests green.

## Final gate

- [ ] Fast suite `./...`, `make test-real`, browser suites (`mobile_native_input`, `mobile_text_input`, `touch_integration`, `e2e_timeline_seek`, `wasm_bridge_smoke`).
- [ ] AFTER screenshots for every BEFORE scene + new scenes, desktop + mobile, inspected and compared.
