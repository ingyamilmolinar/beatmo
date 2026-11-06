# Flat-Tonal Design System — app-wide button, menu, and control restyle

**Date:** 2026-07-25
**Status:** Design approved, implementation plan pending
**Scope:** `DESIGN.md` (normative), `internal/ui/` chrome renderers, generated design tokens

---

## Problem

The Synth tab's controls — macro concept cards, closed param cards, inline armed
barrels — read as calm, informative, and modern. The rest of the app does not.
Four screenshots (desktop main, desktop Synth, mobile main, mobile Synth,
captured 2026-07-25) make the gap concrete:

- **Everything is a physical object.** `Button.Draw` renders every discrete
  button as a matte keycap: socket side-wall, contact shadow, bevel, rim. At the
  24–28 px sizes actually used by transport keys, row chips, and tab pills, that
  chrome consumes most of the pixel budget. The 3D reads as clunk, not as
  hardware.
- **Nothing has hierarchy.** Because every control draws its own outlined box,
  seven audio-panel tab pills, four row-rack chips per row, six transport keys,
  and four Synth header buttons all compete at equal visual weight. There is no
  quiet state, so there is no loud state either.
- **Accent color is spent on chrome.** `primary #FFB30A` currently floods the
  selected tab pill and (in other hues) the active solo/FX chips. The saturated
  fill shouts, and the same gold is simultaneously the color of *data* in the
  synth gauges and graphs — so the accent means two different things.
- **Values are hidden.** Outside the Synth tab, a control's value is generally
  discoverable only by opening it. The synth cards already solve this with an
  at-rest gauge, a plain-English gloss, and an inline signature graph.

The Synth panel is not stylistically special; it is simply the one surface where
the newer rules were applied. This spec promotes those rules to the app-wide
default and rewrites `DESIGN.md` accordingly.

## Goals

1. Replace the matte-keycap depth doctrine with a flat-tonal one, app-wide.
2. Collapse ~16 `button-*` component recipes to five variants with one shared
   state model.
3. Introduce a single selection primitive used by every selectable surface.
4. Promote the Synth tab's value-hint affordances to shared primitives, and make
   "a control shows its value at rest" an enforceable rule.
5. Preserve the WASM idle budget: zero per-frame allocation and zero per-frame
   draw for a control that is at rest or steadily latched.

## Non-goals

- Changing the color palette. Every hue stays; only the *roles* of `primary` and
  the active-state fills change.
- Changing layout, spacing, or density tokens. This is a chrome restyle, not a
  re-layout.
- Restyling the data canvas (grid nodes, edges, timeline, analyzer traces).
- Restyling the drum pads / drum cells (`drawDrumCell`), knobs
  (`knurledCapSprite`), or slider thumbs. These are data controls with their own
  light model; they never used the keycap helpers.

---

## The doctrine

`DESIGN.md`'s "Elevation & Depth" section currently reads *"depth comes from edge
contrast — matte hardware on a vintage synth/mixing-console panel."* It is
replaced by:

> **Flat-tonal.** Depth comes from **tone**, feedback comes from **motion**, and
> the accent color belongs to **values, not chrome**.

Three rules follow.

### Rule 1 — A control's silhouette is a tone step, never a line

The existing four-level surface ladder already steps ~10–12 in luminance:

| Level | Hex |
|---|---|
| `background` | `#120A1C` |
| `surface-1` | `#1C1230` |
| `surface-2` | `#281A40` |
| `surface-3` | `#382354` |

That step is sufficient to read a control's edge unaided on every target
display. **No border at rest, anywhere.** Borders survive in exactly two roles:

- the hot-pink `focus-ring` (`#FF2D9E`), which remains the only outline in the
  system, and
- a 1 px hairline **separator** between menu groups and panel sections — a
  divider, never an enclosure.

Every other `border:` block in `components:` is removed.

### Rule 2 — Nothing is 3D

`drawKeycapShell`, `drawKeycapContactShadow`, `drawKeycapBevel`,
`drawKeycapActiveInset`, `drawKeycapPanelButton`, `drawKeycapEngageFlash`, and
`keycapCapRect` are deleted outright.

An audit of every call site (2026-07-25) found **no genuine physical-key user**.
The drum pads — the one surface where the metaphor would be literal — render
through `drawDrumCell` / `DrumCellStyle`, a plain rect path that never touched
the keycap helpers. All eight keycap call sites are chrome:

| Call site | Role |
|---|---|
| `uigrid.go:238` `Button.Draw` | every discrete button |
| `audio_sticky_bar.go:341,491` | channel pill, tab pills |
| `segmented_control.go:178` | mobile view switcher |
| `synth_panel_zone.go:3159` | synth chip strip |
| `param_card.go:71` | closed synth param card |
| `synth_macro_render.go:396` | macro concept card |
| `timeline_zone.go:592` | timeline popup action |
| `game_longpress_popup.go:213` | long-press popup action |

Note that `param_card.go` and `synth_macro_render.go` are on that list: the
reference aesthetic this spec is generalizing draws a keycap shell today. It
reads flat anyway because at card size a 2 px wall and a 1 px bevel are
invisible. Removing it makes those cards genuinely flat and costs nothing
visually — which is the empirical case that the keycap was never carrying its
weight anywhere.

Knob and slider-thumb volumetrics are a separate light model (`knurledCapSprite`
and the slider thumb cap), are not part of the keycap helpers, and are out of
scope for this spec.

### Rule 3 — Gold is data ink

`primary #FFB30A` stops being "the fill of the selected thing". It becomes the
color of *value*: gauges, signature curves, the selection indicator bar, meter
fills, and the armed-control highlight. **A control never floods itself with the
accent** — the one exception is `button-primary`, rationed to one per surface
(below).

This resolves the current double meaning where the same gold marks both "this
tab is selected" (chrome) and "this envelope's attack is 5 ms" (data).

---

## Button anatomy

One geometry, five variants. All share `rounded.sm`, `spacing.btn-md` height,
and `SpaceSM` inner padding; they differ only in fill role.

| Variant | Rest appearance | Used by |
|---|---|---|
| `button` | one surface level above its container | the default — transport keys, steppers, Save / Save As / Reset, row chips |
| `button-quiet` | no fill; label or icon only | icon-only chrome in a dense strip — kebab, close ×, FX card ▲▼/×, `+ Add Effect` |
| `button-primary` | accent fill, `background`-colored label | **at most one per surface** — the single next action |
| `button-danger` | `error` tint at `alpha.subtle`, red label | destructive actions; solid red is reserved for the second-click confirm |
| `segment` | member of a group sharing one recessed `surface-1` track | tab pills, mobile bottom nav, view switcher, HP/LP, mode chips |

`button-primary`'s one-per-surface cap is a design rule, not a mechanism — it is
stated in `DESIGN.md` prose and reviewed, not enforced by a test.

### The `segment` group

`segment` exists because a row of individually-boxed pills is the single loudest
element in the current UI. Today the audio-panel tab strip draws seven outlined
boxes with one flooded gold; the mobile bottom nav draws eight keycaps.

Instead, the **group** draws one recessed `surface-1` track, and the members are
text inside it. Selection is a single shared indicator that **slides** between
members.

```
TODAY                                    NEW
┌──┐┌────┐┌────────┐┌──────┐┌─────┐     ░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░
│EQ││Wave││Spectrum││Levels││Chain│      EQ  Wave  ▓Spectrum▓  Levels  Chain
└──┘└────┘└────────┘└──────┘└─────┘     ░░░░░░░░░░━━━━━━━━━━░░░░░░░░░░░░░░
 7 competing outlines,                   1 track, 1 sliding indicator
 active = solid flood
```

Surfaces that become `segment` groups: the audio-panel tab strip
(`audio_sticky_bar.go`), the mobile bottom nav, the mobile view switcher
(`SegmentedControl`), the EQ HPF/LPF pair, the Chain display-mode pills
(OVR/SPL/DIF), and the Spectrum slope pills.

---

## State model

Six states, one uniform recipe, applied identically to all five variants. No
per-variant special-casing.

| State | Visual | Motion |
|---|---|---|
| rest | tonal fill per variant | none |
| **hover** (desktop) | fill rises one surface level, **and** the accent indicator grows 0 → full width beneath the label | existing `button-hover-fade` spring drives both |
| **press** | fill −20 brightness; content nudges 1 px down | `button-press-decay`; no cap travel, no overshoot |
| **active** | accent tint fill at `alpha.subtle`, full-bright label, 2 px accent bar | inside a `segment` group the bar is shared and slides — new `segment-indicator-slide` token |
| disabled | fill drops to `surface-1`; label `on-surface-disabled` | none |
| focus | hot-pink `focus-ring` outline — unchanged | none |

Mobile has no hover, so the same lift is press-driven there. This is already the
documented rule for `button-hover-fade` and needs no new screen-class divergence.

### Token changes

**Retired** (8): `keycap-wall-depth`, `keycap-active-raise`,
`keycap-release-overshoot-frac`, `button-press-scale`,
`button-release-overshoot`, `button-engage-flash`, `button-toggle-pulse`,
`button-glow-rest`.

**Added** (1): `segment-indicator-slide` — an `exp-decay` animation driving the
shared indicator's travel between members, with a runtime accessor in
`animation_helpers.go`.

**Retained unchanged:** `button-hover-fade`, `button-press-decay`, the full
`colors:`, `spacing:`, `rounded:`, `alpha:`, and `densities:` blocks.

---

## Menus and list rows

The flat-menu doctrine already in `DESIGN.md` — rows sit directly on one panel
surface, no per-row pad or bevel, hairline separators for group breaks, unified
title and row-icon geometry — is **correct and stays**. Three changes.

### Menu-internal chrome goes flat

The `menu-search-field` loses its outline and becomes a recessed `surface-1`
track, matching a `segment` group's track. The menu close ×, the FX card's
▲▼/× buttons, and `+ Add Effect` become `button-quiet`.

### Every menu row gets a value slot

A right-aligned, dim `RoleCaption` string carrying the row's current value or
metadata. The slot is **reserved whether or not it is populated**, so rows never
reflow when a value appears. Examples: the subdiv menu shows its divisor, an FX
row shows mix %, an instrument row shows its family.

### One selection primitive, app-wide

The menu's identity-tinted left stripe and the `segment` group's sliding bottom
bar are the same idea, stated once in `DESIGN.md`:

> Selection is marked by a 2 px accent bar on the **leading edge** of the
> selected element: **left** in a vertical list, **bottom** in a horizontal
> group. Fill is a tint, never a flood. In a group with exactly one selection,
> the bar is a single shared element that animates between members.

In instrument-scoped menus the bar uses the owning instrument's identity color
rather than `primary`; that is existing behavior and is preserved — identity
color always outranks chrome accent.

---

## Value-hint system

Three affordances, all already implemented in `param_card.go` and
`synth_macro_render.go`, promoted from Synth-local to shared primitives.

| Affordance | Description | Applies to |
|---|---|---|
| **gauge** | 2 px hairline strip on the control's bottom inner edge; fill / bipolar / dots variants (`drawParamCardGauge`) | any continuous or enumerated value: volume, pan, BPM, subdiv, length, sends, EQ dB |
| **gloss** | plain-English caption beneath the label, via the existing `PlainEnglish()` | cards, menu rows, popups — wherever a second line fits |
| **signature graph** | inline mini-curve on a right-hand band | params whose value has a *shape*: envelopes, filters, oscillators |

Plus **hover readout** on desktop: hovering a valued control fades its numeric
value in beside the label, on the same `button-hover-fade` spring — so a value is
legible without opening anything.

The enforceable rule, stated in `DESIGN.md`:

> Every control that carries a value must show at least one hint at rest. A
> control whose value is discoverable only by opening it is a defect.

Phase 6 adds a discipline test over the registry of valued controls asserting
each declares at least one hint affordance.

---

## Architecture and boundaries

The restyle is deliberately concentrated so that most surfaces change without
being edited.

- **`ButtonStyle` / `Button.Draw`** remains the single chokepoint for all five
  variants. Phase 2 replaces its internals; every call site that already uses a
  `ButtonStyle` inherits the new look for free.
- **`segment` is a new group-level primitive**, not a button variant flag,
  because the track and the sliding indicator belong to the *group*, not to any
  member. It owns: track rect, member rects, selected index, and indicator
  animation state. Members remain ordinary `Button`s for hit-testing and input,
  so the input-tree contract is untouched.
- **Value-hint primitives** move from `param_card.go` into a shared
  `control_value_hint.go`, exposing `drawValueGauge`, `drawValueGloss`, and a
  `ValueHint` descriptor. `param_card.go` and `synth_macro_render.go` then call
  the shared primitives rather than owning them — no behavior change on the
  Synth tab, which is the reference implementation.
- **No input, layout, or tree changes.** `DrumViewTree` / `GridTree` dispatch,
  `HitIndex` z-ordering, zone visibility predicates, and the opaque-to-z contract
  are all untouched. This is a draw-path restyle.

## Performance contract

The WASM idle budget is preserved and is a hard requirement:

- All static appearance — tonal fill, tint fill, indicator bar at rest — bakes
  into the existing per-state sprite caches. `Button.Draw` blits a pre-rendered
  texture when nothing is changing.
- All motion — hover fade, press dim, indicator slide — is a blit-offset plus
  alpha overlay that writes nothing to the cache.
- A control at rest, or steadily latched, performs **zero per-frame allocation
  and zero per-frame draw**. The sliding indicator animates only during a
  transition and then settles.

Retiring the keycap shell, contact shadow, and bevel *reduces* per-state sprite
work, so this restyle should be net-neutral to net-positive on draw cost.

## Testing strategy

Guards that must be updated as part of the work, not after it:

| Guard | Change |
|---|---|
| `TestDesignMDDrift` | want-map updated for retired/added tokens |
| `TestDesignMDLintSnapshot` | orphan-token allowances change as `border:` blocks and keycap tokens are removed |
| `TestTokenDiscipline` | per-file literal ratchet moves as keycap draw code leaves `Button.Draw` |
| `button_keycap_test.go`, `button_depth_test.go` | rewritten to pin the flat contract instead of the keycap contract |
| `button_keycap_alloc_test.go` | retained, retargeted — the zero-idle-work assertion is unchanged and still applies |
| `scene_crop_test.go`, `audio_panel_render_pixels_test.go`, `synth_armed_pixels_test.go` | pixel expectations shift |
| `visual_regression`, `visual_device_parity`, `visual_mobile_parity` (browser) | baselines regenerate |

New coverage:

- `segment` group: layout, selected-index behavior, indicator animation settling
  to zero work, and input routing unchanged.
- Value-hint discipline test for the at-rest rule.

**Visual verification is mandatory and is its own phase.** Per `CLAUDE.md`, no UI
change is complete without an inspected screenshot. Phase 7 captures the full
scene catalog on desktop and `MOBILE=1`, and every `crop_*` subject, and iterates
until the rendered pixels are right. Green Go tests do not close this work.

## Rollout phases

| Phase | Content |
|---|---|
| 1 | `DESIGN.md` rewrite; retire 8 tokens; add `segment-indicator-slide`; `make gen-design-tokens`; update drift + lint guards. Doc and generated code land together so the tree stays green. |
| 2 | Flat `Button.Draw` + six state layers; keycap helpers deleted and all eight call sites migrated |
| 3 | `segment` group primitive → audio tab strip, mobile bottom nav, view switcher, HP/LP, mode and slope pills |
| 4 | Row-rack chips (M/S/FX/⋮), transport keys, steppers |
| 5 | Menu and panel chrome: search field, close ×, FX card buttons, `+ Add Effect`; menu row value slot |
| 6 | Value-hint primitives extracted to `control_value_hint.go` and applied; discipline test |
| 7 | Screenshot sweep — desktop + `MOBILE=1`, all `crop_*` scenes — and fix what looks wrong |

Phases 3–6 are independent of one another once phase 2 lands, and may be
reordered or parallelized. Phase 7 is a gate, not an option.

## Risks

- **Legibility of borderless controls on low-quality displays.** Mitigated by the
  ~10–12 luminance step between surface levels, which is larger than the
  1 px 8-alpha hairline it replaces. Phase 7 screenshots are the check.
- **Touch discoverability without hover.** Mobile keeps the tonal fill at rest —
  only the *hover* layer is desktop-only — so tap targets remain visible. The
  ghost-at-rest option was explicitly rejected for this reason.
- **Baseline churn.** Three browser visual suites and several Go pixel tests
  regenerate. This is expected and scheduled, not incidental breakage.
- **Scope creep into knobs and the data canvas.** Explicitly out of scope; knobs
  keep their volumetric model this pass.
