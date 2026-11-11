# Matte retro-3D analogue button/control restyle

**Date:** 2026-06-17
**Branch:** add-core-node-types
**Status:** approved, in implementation

## Problem

Buttons and controls read as "too glossy and shiny" — distracting and tacky.
The current mechanical-keycap system layers a sheen band, a crisp specular
highlight line, a cyan hover-glow ring, a `|sin|` toggle-pulse glow, a cyan
engage flash, plus specular dome highlights on the slider thumb and knob cap.
The user wants a **minimalistic, retro, 3D *analogue*** feel instead: depth
from edge contrast, not from shine or glow.

## Decisions (from brainstorming)

1. **Aesthetic = matte hardware bevel** — flat matte fill framed by a faint
   light top edge + a soft dark bottom edge. No gloss, no glow. (70s/80s synth
   / mixing-console feel.)
2. **Active/latched state = inset + accent fill** — an engaged control looks
   pressed-IN (recessed/inverted bevel) and takes a solid accent-tinted fill,
   like a latching switch. No glow.
3. **Active accent = warm amber/lamp** — not cyan. Sourced from the existing
   on-palette Vice City **sunset-gold** ramp (e.g. `#FFB30A`) so
   `TestPaletteMembership` stays green. Dark text on the amber fill.
4. **Hover (desktop only) = subtle ~8% fill lift** — a flat lighten, no ring,
   no specular. Mobile stays flat.
5. **Geometry = keep slim extrusion** — reduce `keycap-wall-depth` 4 → 2 so
   buttons keep a little physical pop but read flat-matte. Press still sinks.

## Three states (all components)

| State | Look |
|---|---|
| Rest | Flat matte fill; faint light top edge + soft dark bottom lip = chamfer; slim wall. |
| Press | Content sinks (press travel) + top inner-shadow (`drawButtonInnerShadow`). |
| Active/latched | Inverted bevel (shadow top, light bottom) + warm amber fill + dark text. No pulse, no flash. |

Hover (desktop): flat ~8% fill lighten.

## Removed (gloss/shine sources)

- `keycapSheenCol` 45%-upper-face band (`button_cushion.go`).
- Crisp specular line alpha 120 → faint ~40 matte light edge.
- `drawButtonHoverGlow` rim + warm-highlight glow → flat fill lift.
- `drawKeycapEngageFlash` cyan bloom → no-op.
- `button-toggle-pulse` `|sin|` glow (no longer rendered).
- `slider_neon.go` thumb specular highlight + rail outer-glow stroke + cyan drag edge.
- `knob.go` specular dome highlight (`knurledCapSprite`). Knurl ridges kept (analogue texture).

## Per-component

- **Buttons** (`Button.Draw`, uigrid.go): states per table; hover lift desktop-only.
- **Pill tabs** (`drawPillTabAt`, audio_sticky_bar.go): active = latched-down amber.
- **FX toggle pill** (`drawFXTogglePillCol`): active amber, no glow.
- **Slider** (slider_neon.go): matte rail + matte bevel thumb.
- **Knob** (knob.go): matte dome, keep knurl.

## DESIGN.md + tokens

- `keycap-wall-depth` 4 → 2; regen via `make gen-design-tokens`.
- Rewrite "Elevation & Depth" / keycap prose to the matte + latched-amber model.
- Document amber-active-fill as a new semantic role alongside the
  cyan+hot-pink accents (it does not become a free hue — it is an existing
  sunset-gold palette member used in a new role).
- Now-unused animation tokens (`button-toggle-pulse`, `button-glow-rest`,
  `button-engage-flash`, `button-hover-fade`) are retained in YAML to avoid
  gen/test breakage but are no longer rendered; prose notes them as legacy.

## Tests to update

button_anim_test.go, button_depth_test.go, hover_glow_overlay_test.go,
components_drift_test.go, eq_panel_draw_alloc_discipline_test.go,
audio_panel_render_pixels_test.go, design_md_drift / token-discipline as
needed, plus any scene/screenshot baselines. Verify via fast `-tags test`
ui + audio suite.

## Process constraints

- **No commits / no staging** (standing rule) — all changes stay in the
  working tree, including this spec.
- No `git checkout/restore/reset/stash`; revert manually with the editor.
