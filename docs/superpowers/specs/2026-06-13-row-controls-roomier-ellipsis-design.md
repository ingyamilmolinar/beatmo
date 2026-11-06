# Roomier, touch-friendly drum-row controls (ellipsis menu)

**Date:** 2026-06-13
**Branch:** add-core-node-types
**Status:** Design — awaiting review

## Problem

The per-row control strip (instrument label · volume · mute · solo · FX · ⋯ menu) is
laid out with a proportional weighted grid (`rowControlWeights`). Two concrete defects:

1. **Desktop ellipsis is cramped.** Weights are `[6, 2.5, 2, 2, 2.5, 1]` (total 16); the
   ⋯ overflow button gets weight **1 ≈ 6% of row width** — the narrowest cell, well below
   the 28px `RowControlBtnSize` it should occupy. On a ~280px row the ⋯ cell is ~17px wide
   before its 3px inset (~11px drawable). It is the hardest control to hit and visually
   pinched.
2. **Mobile has no ellipsis at all.** On mobile the ⋯ button is hidden
   (`menuBtn.SetRect(image.Rectangle{})`). The row menu (rename / color / change-origin /
   delete) is reachable only by **tapping the instrument label** — a hidden, undiscoverable
   gesture.

The weighted grid also gives no guarantee that any button meets a comfortable size; cells
just shrink with the row width.

## Goals

- The ⋯ ellipsis is a comfortable, clearly-hittable button on **both** desktop and mobile.
- All row controls (mute / solo / FX / ⋯) render at a fixed, non-cramped size; the label
  flexes to fill the remaining space.
- Mobile gains a **visible, discoverable** ellipsis button (touch target ≥ 44px).
- No regressions to the existing label-tap menu, slider drag, or hit-area dispatch.

## Non-goals

- No change to *what* the row menu contains or how the bottom-sheet / overflow menu renders.
- No change to the desktop label-tap behavior (`OnInstMenuOpen`) — left as-is.
- No new menu affordance beyond making the existing `menuBtn` visible on mobile.

## Design

### Layout: fixed-width controls, flexing label

Replace the proportional weighted grid in `(*RowRackZone).positionRowWidgets`
(`row_rack_zone.go:895`) with a **right-anchored, fixed-width** layout, shared by both
profiles:

```
[ Label ……… flexes to fill ……… ] [ Vol ] [ M ] [ S ] [ FX ] [ ⋯ ]
```

- **Control buttons** (Mute, Solo, FX, ⋯) each get a fixed square width = `rowControlBtnW()`:
  - Desktop: `RowControlBtnSize()` = **28px** (vs ~17px today for ⋯ → "much more room").
  - Mobile: `RowControlBtnSize()` = **36px** visible (token bumped 32 → 36, see below). The
    44px touch minimum is met by the existing `HitArea.Touch = true` skirt expansion, so the
    visible button stays balanced and the label keeps more room.
- **Volume** gets a fixed, wider cell (drag room) derived from the button width
  (`volW = rowControlBtnW() * 2`, a mini-bar / slider).
- **Label** flexes to fill everything to the left of the volume cell, separated by
  `ControlGap`, each control inset by `SpaceXS` as today.
- **Narrow-row clamp:** if `rowRect` is too narrow to fit all controls + a minimum readable
  label (`labelMinW`), the control widths shrink proportionally so the label never collapses
  to zero. Protects very narrow phones.

The layout is computed by walking fixed widths inward from the right edge; only the label
cell is variable. `menuBtn` is the rightmost control on **both** profiles.

### Mobile ellipsis

Stop zeroing `menuBtn` on mobile in `positionRowWidgets`; assign it the rightmost fixed
cell. **No other wiring is needed** because everything keys off the rect:

- `rebuildHitAreas` (`row_rack_zone.go:1053`) already includes `e.menuBtn` with `Touch: true`
  + `ClipRect: z.rect`, skipping it only when `Rect().Empty()`.
- The draw paths (`row_rack_zone.go:1389`, `:1499`) already guard on `!Rect().Empty()`.
- `menuBtn.OnClick` (`row_rack_zone.go:737`) already calls `OnContextMenuOpen(idx)` — the
  identical callback mobile's label-tap uses.

Label-tap on mobile remains as a harmless secondary entry point (no behavior change).

### Token change

`rowControlBtnSize` mobile/spacious value **32 → 36** in `DESIGN.md` (profileOverrides.mobile
and densities.spacious), then `make gen-design-tokens`. Safe because `RowControlBtnSize()`
has **zero production call sites today** — adopting it here is its first real use.

### Cleanup

`rowControlWeights()` (`drumview_layout.go:981`) becomes unused → delete it and its two
weight arrays.

## Testing

- **Update** the tests that currently assert the mobile kebab is empty:
  `row_label_kebab_test.go`, `mobile_ui_overhaul_test.go` → assert it is **present,
  width/height ≥ touch-comfortable, right-anchored**.
- **New layout assertions** (mobile + desktop profiles via `SetRuntimeProfileForTest` /
  density override):
  - Every control button (mute/solo/fx/menu) width ≥ `RowControlBtnSize()` (no cramping).
  - Control buttons do not overlap each other or the label.
  - ⋯ menu button is the rightmost control and is non-empty on both profiles.
  - Label width ≥ `labelMinW` (narrow-row clamp holds) and ≥ 0 always.
  - Desktop ⋯ width strictly larger than the old weighted value (regression witness).
- **Existing** slider-drag / context-menu / hit-area tests must stay green.
- **Visual check:** `make screenshots-all` desktop + mobile row-rack crops.

## Files touched

- `src/go/internal/ui/row_rack_zone.go` — `positionRowWidgets` rewrite; mobile `menuBtn` rect.
- `src/go/internal/ui/drumview_layout.go` — delete `rowControlWeights`.
- `DESIGN.md` + regenerated `design_profile.gen.go` / `design_density.gen.go` — token bump.
- Test files above.
