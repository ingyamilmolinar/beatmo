# Shared Value Editor — unified inline numeric/text entry

**Date:** 2026-06-14
**Branch:** add-core-node-types
**Status:** Design approved, pending spec review

## Problem

Inline value entry in the UI is implemented four different ways, and two of them
actively defeat the user:

| Editor | Component | Pre-fills value? | "Enough space"? |
|---|---|---|---|
| EQ dB (×10) | raw `TextInput` | **No — clears on focus** | cramped column cell |
| BPM | raw `TextInput` | **No — clears on focus** | small fixed box |
| Synth knobs | shared `ParamValueEditor` | Yes | anchored to tiny readout |
| Sampler knobs | shared `ParamValueEditor` | Yes | anchored to tiny readout |

User complaints, mapped to code:

1. **"Easier to see the previous value and edit it."** EQ (`updateDBInputs`:
   `ti.SetText("")` on focus-gain) and BPM (clear-on-focus in the transport
   update) wipe the box the instant it gains focus, so the prior value is gone.
2. **"Text box with enough space to hold the entire value."** Every editor sizes
   itself to the cramped readout/column cell it anchors over, so long values clip.
3. **"Re-use the same component."** There are three separate commit/parse/clear
   code paths (`commitDBText`, `commitBPMText`, `ParamValueEditor.commit`) plus
   bespoke focus juggling in EQ.

## Goal

One shared popup value editor used by EQ, BPM, synth, and sampler entry. The
editor opens **pre-filled with the current value** (caret at the end — edit-first),
**auto-sizes to fit the whole value**, commits on Enter/blur, reverts on Escape,
and flashes red on invalid input — identically everywhere. Free-text fields
(rename / search / save-as) stay on the `TextInput` primitive but get the same
no-clear + size-to-fit consistency pass.

## Non-goals

- No dialog restructuring for rename/search/save-as — only no-clear + width.
- No new design tokens or color literals; reuse existing styles/tokens.

## Approach (selected: A — generalize the shared popup editor)

Evolve `ParamValueEditor` into a spec-driven `ValueEditor`. Migrate EQ dB and BPM
onto it (deleting their bespoke focus/commit/clear logic) while preserving their
native-mobile-input + soft-keyboard wiring. Synth/sampler knob entry **also gains**
native-mobile-input + soft-keyboard (previously popup-only). Add auto-width,
Escape-to-revert, and uniform error-flash.

Rejected alternatives:
- **B (shared behaviors, separate editors):** keeps 3+ copies of commit logic —
  fails "reuse the same component."
- **C (one new `ValueField` replacing `TextInput` + `ParamValueEditor`):** touches
  mobile native input, soft keyboard, and dialogs at once — highest regression
  risk for little gain over A.

## Design

### 1. `ValueEditor` (file: `param_value_editor.go`, evolved)

`ParamValueEditor` is generalized. The struct keeps a single wrapped `TextInput`
and adds a spec + revert/anchor state:

```go
type ValueSpec struct {
    Format func(float64) string         // render current value into the box
    Parse  func(string) (float64, bool) // parse+validate+clamp; ok=false ⇒ revert+flash
    MinW   int                          // comfortable floor width (px)
    Style  TextInputStyle               // default BPMBoxStyle; EQ passes EQDBBoxStyle
}

func (e *ValueEditor) Open(
    spec ValueSpec,
    anchor image.Rectangle,
    get func() float64,
    set func(float64),
    mobileInputID string, // native-input id: "eq-db-N", "bpm", "synth-param", "sampler-param"
)
```

The shared editor's wrapped `TextInput` wires `OnFocusGained → softKeyboardShow("numeric")`
and `OnFocusLost → softKeyboardHide()` once at construction, and sets
`MobileInputID = mobileInputID` per `Open`. Because the editor is a single
one-at-a-time instance, a stable per-surface id is enough; on mobile WASM the native
HTML `<input>` handles typing and `TextInput.Draw` skips its own render while active
(unchanged mechanism, now reused by synth/sampler too).

- The existing `audio.ParamDef`-based `Open` is kept as a thin adapter that builds
  a `ValueSpec` from `formatParamForEntry`/`parseParamEntry`, so synth/sampler call
  sites change minimally.
- `Active()`, `Update()`, `Draw()` keep their current contracts. Only one editor is
  open at a time (it already is a single shared instance).
- Naming: keep the type name `ParamValueEditor` (or alias) to minimize churn; the
  spec-driven `Open` is the new surface. Final name decided in the plan.

### 2. Behaviors

1. **Pre-fill, never clear.** `Open` calls `ti.SetText(spec.Format(get()))`; cursor
   lands at end (already `TextInput.SetText`'s behavior). The EQ `ti.SetText("")`
   clear and the BPM clear-on-focus are removed.
2. **Auto-width.** On open:
   `w = max(spec.MinW, TextWidth(text) + 2*pad + caretMargin)`. The box is
   re-centered horizontally on the anchor's center and clamped inside the owning
   zone / screen so it never clips the value or runs offscreen. Height stays the
   anchor's (touch-floor preserved on mobile).
3. **Escape / Enter / blur / invalid.** Enter and click-away commit (existing blur
   path). `ValueEditor.Update` polls `isKeyPressed(ebiten.KeyEscape)` while active
   → close without writing (revert). Invalid parse (`ok==false`) flashes
   `errorAnim` and reverts (existing mechanism, now uniform).

### 3. Migrations

- **EQ** (`eq_panel_zone.go`): delete `eqDBInputs [10]*TextInput`,
  `updateDBInputs`, `commitDBText`, `syncDBInputText`/`syncAllDBInputTexts`, and the
  `dbInputFocused`/`dbInputPrev`/`dbInputSyncing` bookkeeping. A click on band `i`'s
  dB readout opens the shared `ValueEditor` over that cell with
  `ValueSpec{Format: formatDB, Parse: dB→clamp[-12,+12]→round 0.1, Style: EQDBBoxStyle}`,
  `get`/`set` bound to `bandGainsDB[i]` + the existing `OnGainChange`/`OnApplyEQ`/
  `OnEQBandCommit` callbacks, and `mobileInputID = "eq-db-"+i`. "One focused at a
  time" becomes "one editor open at a time" for free. Per-band hit areas remain.
- **BPM** (`transport_zone.go`): `bpmBox` tap opens the shared editor;
  `commitBPMText` collapses into the spec's `Parse` (= `parseBPM`) plus the existing
  `SetBPM` / `OnNotifyError` wiring; `mobileInputID = "bpm"`.
- **Synth / Sampler** (`synth_panel_zone.go`, `sampler_panel_zone.go`): same
  component via the `ParamDef` adapter, now with auto-width + Escape **and** native
  mobile input + numeric soft keyboard. Pass `mobileInputID = "synth-param"` /
  `"sampler-param"`; register the native input region on open over the editor's
  (auto-sized) rect and tear it down on commit/close, mirroring the EQ/BPM lifecycle.

### 4. Text fields (consistency pass)

`rename`, instrument `search`, `jump`, and save-as boxes stay on `TextInput`. Apply:
- Confirm no clear-on-focus (rename/save-as already pre-fill — verify, fix if any).
- Route their box width through the same size-to-fit helper used by `ValueEditor`
  so content isn't cramped. No dialog layout restructuring.

## Testing (fast `-tags test` path; functional Update-loop style)

Update: `eq_db_input_test.go`, `bpm_editor_validation_test.go`,
`param_value_editor_test.go`, `text_input_escape_test.go`, `textinput_validation_test.go`.

New:
- EQ + BPM open **pre-filled** (box shows the prior value, not blank).
- Auto-width: a long value's full text fits inside the editor rect (no clip).
- Escape reverts on all four surfaces (no write).
- Invalid input flashes `errorAnim` and reverts (no write) on all four.
- `mobileInputID` is set on open for every surface: EQ (`eq-db-N`), BPM (`bpm`),
  synth (`synth-param`), sampler (`sampler-param`); soft keyboard shows on focus and
  hides on blur for each.
- Drive real `Game.Update()` input where the surface is reachable, asserting the
  user-visible outcome (per project testing discipline).

Discipline guards already in the tree must stay green: zone input isolation,
token discipline (no new literals), forbidden glyphs.

## Risks

- **EQ native-mobile path through a single shared editor.** The 10 per-band
  `eq-db-N` native inputs collapse onto one editor instance that sets
  `MobileInputID` per-open. Mitigation: test asserts the correct id is set on open;
  keep the per-band registration ids identical to today.
- **Visual regression in the tight EQ column.** Mitigation: EQ passes
  `EQDBBoxStyle`; auto-width clamps within the zone so it grows over neighboring
  chrome only as needed and never offscreen.
- **Synth/sampler native input rect must track auto-width.** The native HTML input
  region is registered over the editor's *final* (auto-sized, clamped) rect, not the
  knob readout. Mitigation: register after the width/recenter step; test asserts the
  registered region matches the editor rect.
