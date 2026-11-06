# Neutral, Unified Button Styling — Design

**Date:** 2026-06-15
**Branch:** add-core-node-types
**Status:** Approved (uncommitted, per repo convention — no `git add`/commit)

## Problem

The button styling across the app is inconsistent:

- The **timeline cell-size +/−** buttons (right of the timeline; `lenIncBtn`/`lenDecBtn`,
  mobile `rowZoomInc/DecBtn`) use the navy `button-stepper` recipe on desktop and
  `button-secondary` on mobile — so they don't read as part of the transport cluster.
- The **popup +/−** buttons in the node sidebar (Volume / Pitch / Duration / Logic-N /
  Logic-P / Groove) are **not real buttons** — `drawIncDecBtn()`/`drawValuePill()` hand-draw
  tiny, sharp-cornered rects with raw `−`/`+` text glyphs, bypassing the cushion / glow /
  icon system entirely.
- The **close "X"** buttons use the shared `Button` + `PopupButtonStyle` but at
  `CloseButtonSize` = 16px desktop (tiny), and the color-wheel one is hand-positioned
  (3px inset) instead of via `closeButtonRect()`.
- Cyan accent leaks across chrome (hover glow, toggled/active states, track-lock).

## Goal

Every chrome / stepper / popup button shares the **neutral surface** look. Color is
reserved and meaningful:

- **Play** — green (kept)
- **Stop** — red (kept)
- **Record** — red dot (kept)
- **Close "X"** — red glyph only (body neutral)
- Nothing else is colored. No cyan on chrome — hover, toggle, and track-lock all go neutral.

## Decisions (locked with user)

1. **+/− family → neutral** (not accent, not navy). Match `button-secondary`.
2. **Sidebar +/− → full real `*Button`s** with cushion/glow + `IconMinus`/`IconPlus`.
3. **Close X → enlarge + unify all** (incl. color-wheel); **body neutral, X glyph red**.
4. **Hover + toggle → neutral brighten**: hover = brighter surface + border (no colored
   glow); ON = `surface-3` fill + brighter border.
5. **Scope** = transport chrome + steppers + popups. **Leave** per-instrument row
   mute/solo/FX (instrument-colored) and audio-data visuals (EQ/scope/levels) untouched.
6. **Play green / Stop red kept**; track-follow → neutral; record → red (kept).

## Design

### A. +/− steppers → neutral

Repoint the stepper recipe (`button-stepper`, backing **BPM ±** and **timeline cell-size ±**)
to the neutral `button-secondary` look (surface-2 body, muted icon, thin border, `btn-md`).
Retire the navy `stepper-fill`/`stepper-border` tokens and the orphaned `colIncDec*` colors
(once the sidebar is converted in B).

`refreshLenButtonsStyle()`'s mobile/desktop split collapses — both platforms get the same
neutral stepper; mobile `rowZoomInc/DecBtn` repoint to it too. Geometry/placement unchanged
(they already render through `Button.Draw`).

### B. Sidebar popup +/− → real Buttons

Replace `drawIncDecBtn()`/`drawValuePill()` in `game_node_sidebar.go` with real `*Button`s
using `IconMinus`/`IconPlus` + the neutral style, sized to a real target (≥ `btn-md`), wired
to the existing inc/dec handlers (including hold-to-repeat). The value readout becomes a
shared styled pill (kept editable via `ParamValueEditor` if it currently is). This removes
the tiny / sharp-cornered / raw-glyph look; they gain the same cushion as transport.

### C. Close "X" → bigger, unified, red glyph

- Bump `CloseButtonSize` (desktop 16→22, tablet 14→20; mobile 28 stays) in DESIGN.md
  `profileOverrides` **and** `densities`.
- Body stays neutral `button-secondary`; **X icon tinted red**, reusing the existing
  `error`/stop red (no 4th red introduced — the glyph is an icon/text-level signal).
- Route the color-wheel close (3px hack) and the node-sidebar close through the shared
  `closeButtonRect()` helper.

### D. Hover + toggle → neutral (shared cushion)

- Recolor the shared hover glow (`button_cushion.go` / `hover_glow_overlay.go`) from cyan
  (`primary`/`primary-bright`) to a **neutral white/grey** lift. Affects every button's hover
  halo; instrument **fills are untouched**, so row mute/solo/FX keep their color.
- Chrome **toggles** (track-lock ON, play-active) show ON via **surface-3 fill + brighter
  border** instead of a cyan glow. `button-transport-follow-on` loses its cyan border → neutral.

### Untouched

Per-instrument row mute/solo/FX (instrument-colored). EQ/scope/levels data viz.
Play-green / Stop-red / Record-red.

## Test / guard impact

Updated in lockstep with each DESIGN.md change:

- `design_md_drift_test` (color/token want-maps — retired stepper tokens, etc.)
- `TestComponentSpecsDrift` (button-stepper redefinition, follow-on spec)
- `design_md_lint` snapshot (`testdata/design_md_lint.expected.json`)
- profile/density token tests (`CloseButtonSize` bump)
- `token_discipline` (any inline-literal deltas; neutral glow color source)

New / extended tests:

- Sidebar +/− are real `*Button`s (not `drawRect` glyphs).
- Close "X" glyph color is the red token; body is neutral.
- Hover glow / toggled chrome color is neutral (no cyan).
- Refresh affected pixel/scene tests (sidebar, close buttons).

## Process notes

- Regenerate via `make gen-design-tokens` after each DESIGN.md edit.
- **Leave everything uncommitted** in the working tree.
- Verify with `-run` filters — the branch carries pre-existing, unrelated test failures.

## Out of scope

- No recoloring of Play/Stop/Record.
- No layout/positional moves beyond the close-button sizing.
- No change to instrument-derived row-control colors.
