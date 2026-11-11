# BPM Editor Unification — one numeric-entry component

**Date:** 2026-06-14
**Branch:** add-core-node-types
**Status:** Design approved, pending spec review
**Builds on:** `2026-06-14-shared-value-editor-design.md`

## Problem

Inline numeric entry now has **two** components:

1. **`ParamValueEditor`** (shared, spec-driven popup) — EQ dB, synth knobs, sampler knobs. Owns parse/commit/clamp, error-flash, revert, caret keys, Enter/Esc, auto-width, soft-keyboard, and the mobile native-input lifecycle.
2. **BPM** — a bespoke `bpmBox *TextInput` in `transport_zone.go` plus a parallel re-implementation of the same concerns: `commitBPMText` (≈ spec `Parse` + commit), `bpmErrorAnim` (≈ `errorAnim`), `bpmPrev` (≈ revert), `forceBlurBPM`, and its own focus/clear handling. Its mobile native input is registered per-frame in `drumview_layout.go`.

Two components means every fix lands twice and drifts. Concretely: arrow-key caret navigation and the WASM soft-keyboard arrow bridge were fixed for the shared editor (so EQ/synth/sampler work) but BPM, being separate, still misbehaves on WASM. Maintaining the parallel BPM path is the root cause.

## Goal

BPM joins the **single shared `ParamValueEditor`**. The transport BPM number becomes a **readout** that, on tap, opens the shared editor anchored over it — identical to EQ/synth/sampler. One component, one presentation mode (tap-to-popup). The bespoke BPM editing path is deleted. Because BPM stops being a second component, the keyboard-yield guard, arrow handling, and WASM soft-keyboard arrow bridge that already work for EQ apply to BPM with no second copy.

## Non-goals

- The `±` BPM stepper (`bpmDelta`/`bpmIncBtn`/`bpmDecBtn`) is a separate concern (numeric increment), unchanged.
- No change to `SetBPM`/`BPM()`/BPM clamping range (`[1, maxBPM=1000]`, `parseBPM`).
- No new design tokens; reuse `BPMBoxStyle`.

## Approach (selected: tap-to-popup readout)

The BPM number renders live from the current BPM (like the EQ dB cell renders
`formatDB(bandGainsDB[i])`). A tap opens `dv.paramEditor` (the same instance
synth/sampler use — transport and synth/sampler are both in the main tree, so
`valueEditorActive()` already covers it) over the readout via a `bpmSpec`.

Rejected: keep BPM inline as a persistent box with an "inline editor mode" — more
component surface (two presentation modes) for no behavior gain; tap-to-popup is
the simpler single mode the user chose.

## Design

### 1. `bpmSpec` (transport_zone.go)

```go
func (z *TransportZone) bpmSpec() ValueSpec {
	return ValueSpec{
		Format: func(v float64) string { return strconv.Itoa(int(v)) },
		Parse: func(s string) (float64, bool) {
			if v, ok := parseBPM(strings.TrimSpace(s)); ok {
				return float64(v), true
			}
			// Match the legacy "Invalid BPM" toast on rejection.
			if z.callbacks.OnNotifyError != nil {
				z.callbacks.OnNotifyError("Invalid BPM")
			}
			return 0, false
		},
		MinW:   60,
		Style:  BPMBoxStyle,
		MaxLen: 4,
	}
}
```

### 2. Readout + open path

- `bpmReadoutRect` (laid out where `bpmBox.Rect` was set today — same transport
  geometry, so the `±` group outline and layout are unchanged).
- A live readout draw: `drawEQCellText`-equivalent rendering of `strconv.Itoa(bpm)`
  in the readout rect (or reuse the existing transport text style).
- `openBPMEditor()` opens `dv.paramEditor` (reached via a transport callback, e.g.
  `z.callbacks.OnOpenBPMEditor`, wired by DrumView to a method that calls
  `dv.paramEditor.OpenValue(...)`), with:
  - `Spec: bpmSpec()`, `Anchor: bpmReadoutRect`, `Clamp: transport bounds`,
  - `MobileInputID: "bpm"`,
  - `Get: func() float64 { return float64(z.bpm) }`,
  - `Set: func(v float64) { z.SetBPM(int(v)) }`.
- A `bpmOpenAdapter` HitHandler (tap → `openBPMEditor`) registered over
  `bpmReadoutRect` at the BPM box's current z, replacing the `textInputHitAdapter`
  registration.

Because BPM uses `dv.paramEditor`, the editor's existing Update/Draw pumping (the
synth/sampler path) renders + drives it; `valueEditorActive()` already returns true
while it's open, so global shortcuts yield and arrows reach the caret on desktop
and (via the soft-keyboard bridge) WASM.

### 3. Deletions

- `transport_zone.go`: `bpmBox *TextInput`, `commitBPMText`, `forceBlurBPM`,
  `bpmErrorAnim` field, `bpmPrev`, and the bespoke focus-gain/clear/blur/Enter
  handling in the transport update method (the editor owns all of it now).
- `drumview_layout.go`: the per-frame `mobileInputRegister("bpm", …)` block (the
  editor registers native input on open) and the `mobileInputPollResult("bpm")`
  poll in `transport_zone.go` (superseded by the editor-owned poll).
- `BPMErrorAnim()` becomes a thin read of the shared editor's flash (or is dropped
  if only tests use it — verify callers).

### 4. Kept

- `±` stepper (`bpmDelta`, `bpmIncBtn`, `bpmDecBtn`), `computeBPMGroupRect`, the BPM
  group outline, `SetBPM`, `BPM()`, `parseBPM`, `maxBPM`.

## Testing

Mirror the EQ 4b migration. Atomic source-delete + test-rewrite in one pass
(`internal/ui` tests compile together). Old→new mapping:

- `bpmBox().Value()` → `strconv.Itoa(g.drum.BPM())` (readout derives live).
- `bpmBox().Focused()` → `<transport-editor>.Active()`.
- `bpmBox().SetText(x); commitBPMText()` →
  `openBPMEditor(); paramEditor.ti.SetText(x); paramEditor.commit()`.
- `bpmBox().Rect` → `bpmReadoutRect()`.
- `bpmPrev`/`bpmErrorAnim` assertions → editor revert (no write) + `errorAnim`.

New **functional** tests (real `Game.Update` loop via the `fakeInput` harness in
`text_input_escape_functional_test.go`), since isolated tests passed while the real
app was broken:

- `TestFunctional_ArrowKeys_BPMEditor` — open via real click, Left/Right move the
  caret, grid does not pan.
- `TestFunctional_SoftKeyboardArrow_BPMEditor` — fire only the forwarded
  `SignalSoftKeyboardArrowLeft` (no canvas key); caret moves (the WASM path).
- Enter commits / Esc reverts through the real loop (positive + negative controls).

Discipline guards must stay green: zone input isolation, token discipline, the
keyboard-yield (`TestArrowKeysPan`, `TestActionKeysIgnoredWhileTextInputFocused`).

## Blast radius / risk

`bpmBox`/`commitBPMText`/`bpmPrev`/`bpmErrorAnim`/`forceBlurBPM` are referenced
**~340 times across ~34 files** (top: `transport_zone.go` 76, `transport_zone_test.go`
55, `transport.go` 25, plus `drumview_*`, `bpm_*_test`, `focus_ring_blur_*`,
`input_isolation`). Same shape/risk as the approved EQ 4b migration; the non-test
production refs are concentrated in `transport_zone.go`/`transport.go`/`drumview_layout.go`/
`drumview_toolbar.go`/`drumview_transport.go` (~120) and are the careful part.

**Risk:** BPM is wired into transport layout, the `±` group outline, JS exports, and
the toolbar; the readout rect must keep the exact geometry `bpmBox.Rect` had so the
group outline and stepper don't shift. Mitigation: `bpmReadoutRect` is assigned at
the identical layout sites; a layout test pins the group-rect geometry unchanged.
