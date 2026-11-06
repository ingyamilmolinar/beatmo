# Wheel direction + default resolution — design

Date: 2026-07-27
Branch: `add-core-node-types`

## Problem

Three related complaints about the value wheels that edit synth and sampler
params:

1. **Numbers grow downward.** The synth tab's armed inline barrels draw the
   larger value (and the coarser resolution rung) *below* the center selector.
   Larger numbers should read *above* center, smaller below.
2. **The wheel scrolls against the finger.** Dragging the synth barrel down
   translates its label stack *up*. A wheel must move with the pointer: drag
   down → the stack slides down → the rows that were above (higher numbers)
   descend into the center selector → the value rises.
3. **Default resolutions are arbitrary.** A control's starting rung is picked
   by "whichever ladder rung gives ~400 detents across the range", which yields
   values like `±0.005 s` or `±50 Hz` — not numbers a human would choose to
   step by.

## Current behavior (as of this branch)

| Surface | Widget | Value order | Resolution order | Follows finger |
|---|---|---|---|---|
| Synth tab (desktop + mobile) | armed inline `BarrelWheel` ×2 (`drawBarrelLane`) | larger **below** ✗ | coarser **below** ✗ | inverted ✗ |
| Sampler tab (all platforms) | `MobileWheelPopup` | larger **above** ✓ | coarsest **below** ✗ | quantized (one row per detent) ✓ |
| EQ dB | `MobileWheelPopup` (shared) | larger **above** ✓ | coarsest **below** ✗ | quantized ✓ |

`drawBarrelLane` places label `i` at `y = centerY + i*rowH - offset` and the
accent hairline at `centerY - offset`. `synthValueBinding.labelAt(+1)` is the
*larger* value; `synthResolutionBinding.labelAt(+1)` is the *coarser* rung.
Hence both lanes grow downward, and because `offsetPx` is subtracted the stack
translates opposite the drag.

Every synth input path already means "down = +1":

- `BarrelWheel.DragTo`: `dy = y - lastDragY`, `advance(dy)`, detent crossing at
  `offsetPx >= detentPx` → `stepBy(+1)`.
- `ArmedControlEditor.Wheel`: positive `steps` (a downward scroll on the target
  device) → `ImpulseNotch(+1)`.
- Keyboard: `ArrowUp → StepValue(+1)`, `ArrowRight → StepResolution(+1)`.

So the fix is confined to the renderer.

## Design

### 1. Flip the armed barrel lane render

In `drawBarrelLane` (`synth_barrel_render.go`):

```go
y := centerY - i*rowH + offset - StyledTextHeight(role)/2
hairlineY := centerY - barrelHairlineH/2 + offset
```

Consequences:

- `labelAt(+1)` (larger value / coarser rung) renders one row **above** center.
- A positive `offsetPx` (accumulated from a downward drag) shifts every label
  **down** by the same pixel count — the stack tracks the pointer 1:1.
- `BarrelWheel.StepImmediate`'s residual `offsetPx = -d*detentPx` stays correct:
  after a `+1` step the new center label starts one row above center and eases
  down. (The two sign flips cancel; verified arithmetically and pinned by a test.)

No change to `BarrelWheel`, the bindings, the editor, the keyboard rungs, or the
wheel routing.

### 2. Flip the popup's resolution ladder

`MobileWheelPopup`'s resolution strip is a static ladder of tappable rung pills,
currently stacked finest-at-top. Reverse the visual order so the **coarsest
(largest) step is at the top**, matching the value barrel above it.

`rungRects()` is the single source of truth for pill geometry, shared by
`drawResStrip`, the tap hit-test, and hover. Reversing the mapping there — pill
slot `s` shows ladder index `len(ladder)-1-s` — keeps draw, tap, and hover in
lockstep with one edit.

Drag and scroll are unchanged: `applyResDelta` already coarsens on drag-down and
`HandleWheel`'s over-strip branch already coarsens on scroll-down, which now
reads as "bigger numbers come down to the center", consistent with the value
barrel.

The popup's **value** barrel needs no change — `drawBarrel` already walks larger
values above center, `applyValueDelta` already raises the value on drag-down,
and `barrelRowAt`/`barrelRowValue` already treat `+n` as above-center-and-larger.

This surface is shared with the EQ dB wheel, which gets the same flip — the same
direction everywhere rather than a per-tab exception.

### 3. Default resolution rung = nearest round decade

`defaultStepIndex` (`knob_step.go`) currently returns the rung minimizing
`|span/step - 400|`. Replace with:

> Among the ladder's rungs whose mantissa is 1 (a pure `1×10^k`), pick the one
> closest in ratio to `span/200`. If the ladder contains no decade rung, fall
> back to the rung closest in ratio to `span/200`.

Ratio distance (`max(a/b, b/a)`) rather than absolute distance, because step
sizes are logarithmically spaced.

Worked examples:

| Param | span | old default | new default | detents |
|---|---|---|---|---|
| Attack | 0–2 s | ±0.005 s | ±0.01 s | 200 |
| Cutoff | 20–20000 Hz | ±50 Hz | ±100 Hz | ~200 |
| Mix | 0–1 | x0.002 | x0.01 | 100 |
| Gain | −60–+12 dB | ±0.2 dB | ±1 dB | 72 |
| Detune | −100–+100 cents | ±0.5 c | ±1 cent | 200 |

Persisted per-param rung choices (`persistKnobStep` → userprefs) still win on
load, so this only affects controls the user has never adjusted.

## Testing

Behavioral tests, written first:

1. `drawBarrelLane` — with a stub binding whose labels are distinguishable,
   assert the `+1` label's baseline is **above** the center label's, and that a
   positive `OffsetPx` moves every baseline **down**. (Existing
   `TestDrawBarrelLane_RendersCenterAndNeighbors` covers presence only, not order.)
2. Armed end-to-end — a downward drag across one detent on the value lane
   raises the param; on the resolution lane coarsens the rung. (Already covered
   by the armed drag tests; re-assert direction explicitly.)
3. `rungRects()` order — slot 0 is the coarsest rung; tapping slot 0 selects the
   coarsest; hover index matches.
4. `defaultStepIndex` — table over the five params above.

Regression surfaces: the existing `crop_synth_control_armed*` scenes, plus a new
`sampler_wheel_open` scene that opens the sampler value wheel so the popup's
barrel + ladder are captured. Desktop and mobile passes, inspected as PNGs.

## Out of scope

- Adding sub-pixel kinetic offset to `MobileWheelPopup` (it steps one row per
  detent, which already moves the content with the finger).
- Desktop rotary knobs on other tabs.
- The horizontal two-finger value gesture on desktop dials.
