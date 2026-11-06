# Mobile Knob → Vertical Scroll-Wheel Redesign

**Date:** 2026-06-16
**Branch:** add-core-node-types
**Status:** Design approved, pending spec review

## Summary

Replace rotary **knobs on mobile** with a **tap-to-open vertical scroll-wheel** popup
(a mouse-wheel / barrel-picker), while **desktop keeps the rotary knob unchanged**. The
swap is platform-driven through a single seam so the same parameter renders either control.

The user's framing:
- Knobs make mobile UX harder; users already scroll up/down naturally.
- "Toggle like the volume toggles" → tap a value to open a focused control (popup), like the volume slider.
- "Infinite scroll bars... no limits besides the theoretical maximum of the knob value set" →
  relative/endless value accumulation, clamped only at the param's true `Min`/`Max`.
- "Make it look like a vertically scrolling wheel (like a mouse wheel)" → barrel/cylinder visual.
- "Add-on of resolution which can also be dragged and slided" → a draggable resolution-rung strip.
- "All knobs. Leave them in desktop... ready for a simple component switch depending on the platform."

## Goals

- One consistent mobile gesture for **every** knob: tap → vertical scroll-wheel.
- Endless/relative value change with no travel limit (clamps only at `Scale.Min`/`Scale.Max`).
- Adjustable, draggable, **persisted** resolution.
- Desktop rotary knobs unchanged.
- A clean platform component-swap seam, sharing one value model so the two views can never desync.

## Non-Goals (YAGNI)

- No change to the underlying value math — `Knob`'s `Endless`/`StepMul` accumulation and the
  `KnobStepBadge` resolution rungs already exist and are reused.
- No conversion of controls that are **already sliders** (volume popups) — out of scope.
- No inertial flick/momentum in v1 (1:1 finger tracking only; can be added later if it feels stiff).
- No desktop visual or interaction change.

## Key Existing Infrastructure (reused, not rebuilt)

| Capability | Where | Reuse |
|---|---|---|
| Endless value accumulation, no travel limit | `knob.go` — `Endless`, `StepMul`, `updateEndless`, clamps at `Scale.Min/Max` | Drive it on the **Y axis** from the wheel drag. |
| Resolution rungs (fine→coarse), persisted per param | `KnobStepBadge`, `knobStepPref` | Back the draggable resolution strip. |
| Tap-to-open popup overlay, anchor + dismiss lifecycle | `slider_popup.go`, `drumview_volume_popup.go` | Sibling popup for the wheel. |
| Platform branch | `Profile().IsMobile()`, `DensityValues()` | The single swap decision. |
| Numeric entry w/ mobile native keyboard | `ParamValueEditor`, `mobileInputRegister` | Center-value tap → type exact value. |
| Param metadata (min/max/step/enum/unit/label) | `audio.ParamDef`, `scaleFromParamDef`, `synthParamFacet` | Drives wheel scale, enum labels, formatting. |

## Architecture — the platform component-swap seam

```
ParamControl  (one per param: ParamDef + GetValue/SetValue + shared *Knob value-model)
   │
   ├── desktop → renders rotary Knob inline, dragged in place        (unchanged)
   └── mobile  → renders a compact value Pill inline,
                  tap → opens MobileWheelPopup (overlay)             (new)
```

- A single `Profile().IsMobile()` decision at the knob-cell level picks **Pill vs Knob**.
- Both paths read/write the **same** param callbacks and the **same** underlying `Knob`
  value-model — one source of truth, so platform switching cannot desync a value.
- `synthKnobHitAdapter.OnPress` gains a mobile branch: instead of starting an in-place drag,
  it opens the wheel popup for that param. Desktop press/drag is unchanged.
- `MobileWheelPopup` is a sibling to `SliderPopup`, inheriting the proven tap-to-open /
  tap-outside-to-close lifecycle.
- **Scope:** every control rendered via the `Knob` type (synth/sampler param grid, plus any
  EQ/other knob using `Knob`). Already-slider controls (volume) are excluded.

## The MobileWheelPopup

### Visual — barrel / cylinder picker

```
┌─────────────────────┐
│   Brightness        │  ← title (param label)
│  ┌───────┐  ┌────┐   │
│  │  62%  │  │ ▓  │   │  ← ticks dim/compress toward edges (rotating-drum feel)
│  │  64%  │  │ ▓  │   │
│  │ ▏68%▕ │  │ ░  │   │  ← CENTER line = live value (bright, boxed)
│  │  72%  │  │ ░  │   │     resolution strip ─┘
│  │  76%  │  │    │   │
│  └───────┘  └────┘   │
└─────────────────────┘
```

- Tick values scroll **vertically** past a fixed bright **center line** (= live value).
- Ticks fade (alpha) and compress (spacing falloff) toward top/bottom to sell the drum curve —
  no real 3D; cheap to draw; fits the neon style.
- **Tick spacing = resolution:** each visible tick is one `StepMul` notch apart. Coarse rung →
  big jumps; fine rung → small steps. Barrel re-lays-out when resolution changes.
- **Bipolar params** (±dB, ±cents) show signed ticks with a marked **0** detent.
- Accent color follows the row instrument color (same as the volume popup).

### Interaction

- **Vertical drag = relative/endless accumulation.** Natural direction (content-follows-finger),
  matching the synth content-scroll convention already set on this branch. Lift and re-touch to
  keep going. Clamps only at `Scale.Min`/`Scale.Max`.
- Drives the existing `Knob.updateEndless` math on the **Y** axis instead of X — single value
  model, no new accumulation logic.
- **No flick momentum in v1** (1:1 tracking).

## Resolution strip

- Slim vertical strip right of the wheel showing `KnobStepBadge` rungs (fine→coarse).
- Drag/slide snaps between rungs; active rung highlights; wheel re-spaces ticks live.
- Chosen rung persists via existing `knobStepPref` (per param, across sessions).
- **Hidden for enum knobs** (resolution is meaningless).

## Enum knobs

- Barrel shows enum **labels** (Sine, Saw, Square…), one detent per item, snapping on release.
- No resolution strip.
- Same tap-to-open + vertical-drag gesture — just discrete.
- Center-value numeric entry omitted (nothing to type).

## Numeric entry (escape hatch)

- Tapping the **center value** opens `ParamValueEditor` with the mobile native keyboard
  (`mobileInputRegister`), exactly as the synth readout does today.

## Files

**New (mobile-only view; value model untouched):**
- `mobile_wheel_popup.go` — `MobileWheelPopup` overlay (barrel state, vertical endless-drag →
  `Knob.updateEndless` on Y, resolution strip, center-value entry hook). Sibling to `slider_popup.go`.
- `mobile_wheel_render.go` — barrel tick layout + alpha/spacing-falloff draw helpers.
- `drumview_wheel_popup.go` — `open…WheelPopup(param)` helper, mirroring `drumview_volume_popup.go`.

**Edited (the swap seam):**
- `synth_panel_zone.go` — mobile branch in `synthKnobHitAdapter.OnPress` (tap → open wheel);
  inline cell draws a value **Pill** on mobile instead of the rotary `Knob`.
- `param_value_editor.go` / param plumbing — reused as-is for center-value entry.
- `DESIGN.md` — new wheel sizing as density tokens (barrel width, tick spacing, resolution-strip
  width), per token discipline.

## Testing (TDD, fast `-tags test` path)

Per the project's "functional over component-isolation" rule, drive the real `Game.Update()`
input loop on a mobile profile:

- Tap a knob pill → popup opens.
- Vertical drag → param value accumulates and **clamps at `Min`/`Max`**.
- Resolution drag → tick spacing / `StepMul` rung changes **and persists** (`knobStepPref`).
- Enum wheel → snaps through labels.
- Center tap → numeric editor opens (mobile native keyboard registered).
- **Platform-swap test:** same param renders rotary `Knob` on desktop, Pill→wheel on mobile, and
  both write identical values.
- **Alloc discipline:** barrel render within the per-tab alloc budget (cache tick sprites like the
  knurled cap cache).
- Visual scene + a mobile screenshot scene for the open wheel.

## Risks / Notes

- Single WASM thread: keep barrel draw cheap (sprite-cached ticks, alpha falloff) so it doesn't
  starve the sequencer — mirror the knurled-cap cache pattern.
- Keep the value model as the single source of truth; the wheel is purely a view + Y-axis gesture
  over the existing `Knob`.
- This branch carries large uncommitted WIP; verify with `-run` filters against pre-existing
  failures rather than assuming a clean suite.
