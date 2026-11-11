# Mechanical Keycap Buttons, Knurled Knobs & Analogue Motion

**Date:** 2026-06-15
**Branch:** add-core-node-types
**Status:** Design approved (visual direction validated via brainstorming visual companion)

## Goal

Redesign the texture, hover, and press/release feel of **every** interactive control
in Beatmo so it reads as **slightly voluminous, mechanical, and analogue** — like a
mechanical-keyboard keycap and a knurled synth encoder on a warm slate desk. Replace
the current distracting cyan bloom-on-hover with a restrained, physical affordance.
The redesign must cover all buttons (transport, synth, sampler, EQ, spectrum, levels,
chain, sidebar, dialogs, row controls), control knobs, and slider thumbs — and must be
**smooth and performant on every platform, with WASM (single audio thread) as the
binding constraint.**

This builds on, and stays consistent with, the recent **neutral button styling pass**
(`2026-06-15-neutral-button-styling-design.md`): chrome hover stays de-cyaned/neutral;
cyan is reserved for genuine *active/ON* states.

## Validated visual direction

All four feel decisions were chosen by the user from live interactive mockups:

1. **Keys → Sculpted Keycap.** An extruded cap: a lit top face (top-edge specular
   highlight), a darker **side wall** below it, and a soft **contact shadow**. Real
   volume — the cap visibly stands above the panel.
2. **Knobs → Knurled Grip.** A fluted/ridged rim (mechanical encoder), a raised center
   dome with a top specular highlight, a side wall + contact shadow, keeping the
   existing 270° cyan value arc and a fat cyan pointer.
3. **Active / ON → Lit & Raised.** An active/latched control **pops up ~1px** and its
   face **lights solid cyan** with a *steady* (non-pulsing) glow rim, plus a brief
   one-shot "engage" brightness flash on activation. **This replaces today's pulsing
   `button-toggle-pulse` bloom entirely** — no continuous breathing animation.
4. **Motion → Cushioned Analogue.** Press **eases down** into a soft bottom-out
   (~90 ms); release **springs back** with a single damped settle (~340 ms). The knob
   pointer settles to a nudged value with the same spring. Nothing is instant.
5. **Hover (desktop only).** Drop the multi-layer cyan bloom. Replace with a restrained
   **top-face warm + hairline rim light** on the single hovered control.

### Starting token values (tunable)

| Concept | Value | Notes |
|---|---|---|
| Keycap side-wall depth (rest) | 4 px | extrusion below the cap |
| Keycap travel depth (pressed) | 4 px | cap descends until wall ≈ 0 (bottom-out) |
| Active raise | 1 px | "lit & raised" upward offset |
| Top-edge specular | `WithAlpha(on-surface, AlphaSubtle)` 1px | reuse existing top-edge highlight |
| Contact shadow alpha | ~64 (black) | reuse `drawButtonDropShadow` band |
| Press ease-down | ~90 ms ease-in | new `button-press-down` anim |
| Release spring | ~340 ms damped, slight overshoot | retune `button-press-decay` |
| Engage flash | ~300 ms brightness decay | new `button-engage-flash` anim |
| Knob grip ridges | ~16–20 around rim | static, cached |

These are starting points; final values land during tuning against the mockups.

## The binding constraint: WASM-safe motion

Beatmo's audio runs on a single thread under WASM. Two existing fixes set the bar:
the **chain-tab trace cache** (offscreen-cache a costly per-frame draw) and the
**zoom texture-realloc fix** (never reallocate a texture per frame — reuse Clear+redraw).
This redesign follows the same discipline. The core architectural rule:

> **Static volume is baked into a per-state sprite cache. All motion (press travel,
> active raise, engage flash, hover rim, knob value/pointer) is expressed as cheap
> blit-time destination offsets and alpha-blended overlays over the cached sprite —
> never by re-rasterizing the sprite or allocating per frame.**

Concretely:

- **Keycap body** (cap fill, border, top specular, side wall) is baked into the
  existing button sprite cache (`drawRoundedButton`, keyed by size + rest/on state).
  The cache key must **not** include the in-flight spring value (else every animation
  frame rasterizes a new sprite and thrashes the cache).
- **Press travel + active raise** are applied as a **destination-Y offset** when the
  cached cap sprite is blitted. The side wall is drawn as a cheap static base band
  *behind* the cap; as the cap blits downward the visible wall shrinks, producing the
  bottom-out "travel" with zero re-rasterization. This **replaces the `scaleRectAbout
  Center` shrink** model with a vertical-offset model.
- **Engage flash** is a one-shot alpha-fading cyan fill blitted over the on-sprite,
  driven by a per-button frame counter via exp-decay — a few `drawRect`s for ~0.3 s,
  then nothing.
- **Hover rim** stays in the existing **overlay** (`hover_glow_overlay.go`), which
  already draws *outside* all sprite caches, once per frame, for the single hovered
  control. We only change what it paints (restrained rim instead of bloom). It already
  no-ops on mobile.
- **Knob:** introduce a **static-cap sprite cache** (new) keyed by (diameter, enabled,
  density) for the knurled dome + ridges + wall + contact shadow — these change only on
  resize. Per frame we blit that cached cap and draw only the **dynamic** value arc +
  pointer on top (as today). This *lowers* per-frame knob cost versus drawing all the
  ridges every frame, which matters on the synth tab (many small knobs).

**Allocation budget:** no new per-frame heap allocations. Spring state lives as `float64`
fields on `Button`/`Knob`, ticked in place (the existing `pressAnim` pattern). Reuse the
package scratch buffers (`drawArcVS`/`drawArcIS`). The `eq_panel_draw_alloc_discipline_test.go`
budget and a new alloc-rate guard (modeled on `zoom_alloc_rate_test.go`) enforce this.

## Architecture & code-path map

One `Button` type feeds every button instance, so the work concentrates in shared code:

| Area | File(s) | Change |
|---|---|---|
| Button struct + Draw | `uigrid.go` (Button, `Draw` ~187, `AdvancePressAnim` ~396, press/release in `HandleInputResult` ~328, `SetToggled` ~410) | Replace scale-press with travel-offset; add `engageAnim` field + tick; drive active raise; remove toggle-pulse glow call |
| Cushion helpers | `button_cushion.go` | Rewrite: `drawButtonHoverGlow` → restrained rim; add keycap side-wall/base + travel-offset helpers + engage-flash; retire/repurpose `scaleRectAboutCenter`, `buttonGlowAlphaAnimated` pulse path |
| Hover overlay | `hover_glow_overlay.go` (`drawHoverGlow`, `glowTarget`) | Keep architecture; repaint as rim. Still desktop-only, single control/frame |
| Static chrome | `drawing.go` (`drawButton` ~101, `drawRoundedButton` ~131) | Bake top specular + side-wall into cached sprite; cap fill for on-state |
| Spec render | `render.go` (`Render` ~42) + `design_components.gen.go` | On-state = lit cyan fill; hover/press deltas align to new model |
| Knobs | `knob.go` (`Draw` ~436, `drawArc` ~573, geom consts) | Knurled cached cap + dome + wall + contact shadow; keep arc/pointer; pointer spring on nudge |
| Slider thumbs | `slider_neon.go` (`drawSliderThumb`) | Small sculpted-cap volume (modest); rails unchanged |
| Tokens | `DESIGN.md` (geometry + animations + Elevation prose) → `make gen-design-tokens` → `design_tokens.gen.go` | New geometry/anim tokens; update Elevation & Depth prose |

### Scope boundary — keycap vs flat

- **Get the keycap/knob/cap treatment:** every discrete `*Button` (transport, audio-tab
  controls, sidebar buttons, dialog buttons, row mute/solo/FX), every `Knob`, slider
  thumbs.
- **Stay flat (NOT keycaps):** menu/list rows — context menu items, instrument-list
  rows, sidebar menu *items*, template list (`menu_chrome.go` `drawMenuItemBackground`).
  These keep their existing hover-tint row treatment; a keycap-per-row would look wrong.
  Panels, cards, and the grid stay flat (unchanged — "non-interactive surfaces stay flat").

### Platform behavior

| Cue | Desktop | Mobile |
|---|---|---|
| Static keycap/knob volume (wall, specular, shadow) | ✅ | ✅ |
| Press travel + active raise + engage flash | ✅ | ✅ (touch presses) |
| Hover rim | ✅ | ❌ (no cursor; overlay no-ops) |

## Animation model (token-driven)

All springs reuse the closed `animations:` token kinds in DESIGN.md (`exp-decay`,
`sin-pulse`, `fade`) and are ticked via the existing per-frame counters (`uiAnimFrame`,
`AdvancePressAnim`). No new animation *kinds* — only new entries + accessors in
`animation_helpers.go`.

- **Press down:** on press edge, set travel target to full depth; ease in over ~90 ms.
- **Release spring:** `exp-decay` of the travel offset back to rest with a small
  overshoot (retune `button-press-decay`; the existing release-overshoot scalar maps to
  the new vertical model instead of scale).
- **Engage flash:** new `button-engage-flash` (`fade`/`exp-decay`) — one-shot alpha on
  activation, then silent. No continuous motion when settled.
- **Active steady state:** raised + lit, **no animation once settled** (kills the old
  `button-toggle-pulse` per-frame redraw — a net WASM win).
- **Knob pointer nudge:** `exp-decay` settle of the pointer angle toward the new value.

**Settle-to-silence is mandatory:** once a control is at rest or steadily-on, it must
produce **zero per-frame work** (no redraw, no cache invalidation), so an idle or
playing sequencer isn't paying for chrome. Only in-flight transitions animate.

## Testing strategy

- **Retarget existing:** `button_anim_test.go`, `button_depth_test.go`,
  `button_click_test.go`, `hover_glow_overlay_test.go` to the travel/rim/engage model
  (the hover test hardcodes the ring color — update it).
- **Token guards (same change):** `design_md_drift_test.go` want-map + `token_discipline_test.go`
  budget for any new literals/tokens; DESIGN.md lint snapshot.
- **Visual:** add/extend a crop scene + `audio_panel_render_pixels_test.go`-style check
  proving the on-state lit cap and keycap wall render pixels (not just bounds).
- **Performance (the important ones):**
  - Go alloc-rate guard (modeled on `zoom_alloc_rate_test.go`): pressing/toggling a
    button and animating a knob over N frames does **not** grow `imagesAllocatedTotal`
    and allocates zero per-frame heap.
  - Settle-to-silence test: after a press/engage animation completes, the button/knob
    reports no dirty/redraw and the sprite cache is not re-keyed.
  - Browser/WASM: extend a `webaudio_perf`-style check that toggling buttons + dragging
    knobs **under playback** does not spike `drawMax` or audio `qlat` (the metric that
    caught the chain-tab starvation).

## Risks & gotchas

- **Cache-key contamination (top risk):** if the spring value leaks into the sprite or
  knob-cap cache key, every frame rasterizes → WASM audio starvation. The offset/overlay
  split is precisely to prevent this. Guarded by the alloc-rate + settle tests.
- **Sprite-cache `Draw` isn't per-frame:** buttons render from state-hashed caches, so
  `Button.Draw` doesn't run every frame — motion that must animate per frame lives in
  the overlay or the blit-offset path, not inside the cached sprite (this is why the
  hover overlay exists at all).
- **Concurrent WIP churn:** this branch carries large uncommitted WIP, including the
  neutral-button pass and foreign `zz_*.go` orphans that can break the build (`mv` aside
  + `-vet=off`, never `rm`). Verify own work via `-run` filters; the full `internal/ui`
  suite has pre-existing unrelated failures.
- **Don't reintroduce cyan on hover:** stay consistent with the neutral pass — cyan is
  for ON-states only; hover is neutral rim.
- **Phantom `./...` build** on this branch — verify with explicit package path
  `./internal/ui/`, trust `go vet -tags test ./internal/ui/` over gopls.

## Out of scope

- Menu/list row restyling (stays flat hover-tint).
- Knob interaction logic (detents/endless/numeric entry) — untouched; visual only.
- Slider rails, EQ curve handles, timeline scrub visuals.
- Any new color tokens beyond what the keycap/lit-cap needs (reuse existing palette).
