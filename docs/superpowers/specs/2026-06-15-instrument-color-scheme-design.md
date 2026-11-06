# Per-instrument color scheme for row controls & menus

Date: 2026-06-15
Branch: add-core-node-types

## Problem

The instrument-row mute / solo / FX controls, the FX-menu sliders, the per-row
kebab (ellipsis), and the per-row menus all use a fixed light-blue
(`genColorPrimary` cyan) or fixed magenta/cyan toggle styles. These **drift**
from the instrument's own color: a row whose instrument is orange still shows a
cyan FX badge, a magenta mute button, and cyan FX sliders. The user wants every
control and menu that belongs to an instrument to read as **shades of that
instrument's color**, so the color is informational and consistent.

## Decisions (user-approved)

- **Shade scheme:** all states derive from the row's instrument color
  (`DrumRow.Color`).
  - **mute ON** = instrument color **darkened** (deeper shade).
  - **solo ON** = instrument color **brightened** (lighter shade).
  - **FX ON** = instrument color at full hue (+ recolored badge).
  - **any OFF** = dim tint of the instrument color (low-alpha wash) + thin hued
    border, so each row reads as "owned" by its hue even when idle.
- The M / S / FX letters still carry the semantic meaning; lightness separates
  the three active states.
- **Menus + swatch:** the per-row context (kebab) menu and the instrument menu
  opened from a row adopt the instrument color for their accent (header
  underline + active-item stripe) and show the instrument color swatch/dot,
  consistently across desktop and mobile.

## Design

### 1. Shade helper — `row_instrument_shades.go` (new, pure)

Turns one base `color.Color` into the family of per-role visuals using the
existing `adjustColor` (lighten/darken) and `WithAlphaFromColor` helpers — no
raw color literals (keeps `token_discipline_test.go` clean).

```
type rowToggleRole int // roleMute, roleSolo, roleFX, roleKebab

func rowToggleStyle(base color.Color, role rowToggleRole, on bool) ButtonStyle
func rowToggleFillRGBA(base color.Color, role rowToggleRole, on bool) color.RGBA
```

Shade deltas are named package constants with rationale comments:
- `rowShadeMuteDarken` (mute active darken amount)
- `rowShadeSoloLighten` (solo active lighten amount)
- `rowShadeBorderLift` (active border lifted above fill for edge definition)
Inactive fill/border reuse existing `Alpha*` buckets (dim wash + subtle border).

Invariants the helper guarantees (and tests assert) for a mid-tone base:
mute fill darker than base; solo fill brighter than base; fx fill == base hue;
off fill is low-alpha (dim); mute ≠ solo ≠ off fills all distinct.

### 2. Row rack wiring — `row_rack_zone.go`

Both draw paths (`drawRowControlsToCache`, `drawRowControlsDirect`) already
reassign each button's `.Style` every frame. Replace the static
`MuteActiveStyle` / `SoloActiveStyle` / `FXActiveStyle` / `InstButtonStyle`
on/off styles with `rowToggleStyle(rows[i].Color, role, on)`. The kebab
(`menuBtn`) style is likewise set per-frame to the dim instrument tint. The FX
badge (`drawFXBadge`) takes the row color (brightened) instead of
`colAccentBright`.

### 3. FX-menu sliders — `slider.go` + `drumview_fx_panel.go`

Add `Slider.FillCol color.RGBA` (zero = keep `genColorPrimary` default — nothing
else changes). When the FX panel builds `fxPanelSliders`, set `FillCol` to the
owning row's color (`dv.Rows[dv.fxPanelRow].Color`).

### 4. Menus + swatch — `menu_chrome.go` + `drumview_context_menu.go`

Add accent-parameterized variants `drawMenuHeaderBandAccent(dst, r, accent)`
and `drawMenuItemBackgroundAccent(dst, r, state, accent)`; the existing
functions delegate with `colAccent` (no change to global menus). The per-row
context menu and the row-opened instrument menu pass the row's instrument color
so their header underline + active stripe + the color dot all match the
instrument. Ensure the color dot/swatch shows on desktop too (currently
mobile-only header).

### 5. Tests

- `row_instrument_shades_test.go` (new): helper invariants above.
- Rewrite `row_active_state_test.go`: drive the real draw path; assert two
  differently-colored rows get **different** mute fills, mute darker / solo
  brighter than their base, off = dim.
- Extend `drumview_fx_button_state_test.go`: FX badge/style follow row color.
- FX slider: `FillCol` == row color after panel build.
- Kebab + context-menu accent == row color.
- Update drift/role tests that asserted the fixed mute-red / solo-cyan fills.

### 6. DESIGN.md

Update the "three reds, three roles" semantic invariant: the row **mute button
is no longer the fixed `mute` magenta** — it is a darkened shade of the
instrument color. Document the per-instrument shading of row toggles + menus.
Keep the `mute` color token defined (still used elsewhere / drift test), but
remove the "mute = row mute button fill only" claim. Repoint the affected guard
tests.

## Out of scope

Global menus not tied to a single instrument (subdiv menu, transport overflow)
keep `colAccent`. Grid node/edge colors already derive from the instrument
color sequence and are untouched.
