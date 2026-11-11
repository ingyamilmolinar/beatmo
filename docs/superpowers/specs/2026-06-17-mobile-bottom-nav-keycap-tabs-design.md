# Mobile bottom-nav keycap tabs — design

**Date:** 2026-06-17
**Branch:** add-core-node-types
**Status:** approved, implementing

## Problem

The mobile bottom audio tab selector (`dv.viewSwitchSegmented`, a `SegmentedControl`
in `internal/ui/segmented_control.go`) renders *flat*: a `surface-2` rounded
container, a `border-medium` outline, and the active segment gets a
semi-transparent accent fill (`TokenAccent` @ `AlphaMedium`). It does not match
the polished matte-keycap style of the desktop audio-tab pills (`drawPillTabAt`
in `audio_sticky_bar.go`) and the active tab does not "pop".

The control has 8 segments: Pads / EQ / Wave / Spec / Lvl / Chn / Syn / Smpl
(labels are i18n-resolved via `dv.bottomNavLabels()`). It is mobile-only, drawn by
`layer_view_switch.go` at `ZViewSwitch` (150).

## Goal

Restyle the segmented control so each segment is a matte keycap — visually
identical to the desktop audio-tab pills — with the active segment a solid
sunset-gold (`#FFB30A`) recessed keycap so it pops. DESIGN.md compliant (single
sunset-gold chrome accent; matte retro-analogue restyle 2026-06-17).

Layout choice (user-approved): **joined keycaps in a tray** — keep the
`surface-2` container as a backdrop tray; each segment is a keycap inset 1px so
the tray shows through as a hairline divider between caps.

## Non-goals

- No geometry change: `SegmentRect`, equal-width partition, `HitTest`,
  `SetSegmentDisabled` semantics, and i18n relabeling are unchanged.
- No animation: segments are not `Button`s, so the keycap is static (no
  press-spring / engage flash) — only the static composition from `drawPillTabAt`
  is reused.

## Approach

Rewrite `(*SegmentedControl).Draw`:

1. Draw the tray backdrop: `drawRoundedRect(surface-2 fill)` + `border-medium`
   outline at `RadiusMD` (unchanged).
2. For each segment, render a matte keycap mirroring `drawPillTabAt`'s static
   parts, into the cell `insetRect(segR, 1)` (1px → hairline seam divider):
   - radius `RadiusMD/2` (4px) — same as desktop tabs.
   - shell/socket (`drawRoundedButton` with the wall shade) when
     `genGeomKeycapWallDepth != 0`.
   - contact shadow (`drawKeycapContactShadow`).
   - cap face (`drawRoundedButton` cap fill + border).
   - active → `drawKeycapActiveInset` (recessed inverted bevel); inactive →
     `drawKeycapBevel`; disabled → no bevel (matches pill disabled treatment).
   - label centered on the cap rect.
3. Palette: reuse the pre-boxed `pill*` package vars from `audio_sticky_bar.go`
   (`pillCapFillActive` = `genColorSunsetGold300` = `TokenAccent`, etc.). Active
   text = `genColorBackground` (dark on amber); inactive = `colTextSecondary`;
   disabled = `colTextDisabled`. **Zero new color literals** so
   `TestTokenDiscipline` stays green.

## Tests

- Rewrite `TestSegmentedControl_ActiveSegmentUsesAccent`: use `drawCallRecorder`,
  assert the active segment's cap is drawn via `drawRoundedButton` with the accent
  (`#FFB30A`) fill, located inside the active segment rect; assert inactive +
  disabled segment caps are not amber.
- Keep `TestSegmentedControl_DrawsThreeEqualWidthCells`,
  `_TapDispatchesIndex`, `_OutsideTapIgnored`, `_SetActiveDoesNotFireCallback`
  (geometry/behavior unchanged).
- Verify `view_mode_segmented_test.go` (`_DrawnAfterBottomBar` draw-order) and
  `bottom_nav_relabel_test.go` still pass.

## Risks

- Narrow segments (~48px across 8 on a 390px phone). `drawKeycapBevel` self-guards
  on `capR.Dx() < 2*rad+4` (12px) so it degrades gracefully; geometry untouched.
- Per-frame allocation: Draw runs once/frame on mobile; reusing pre-boxed `pill*`
  vars keeps it allocation-free (no per-call color boxing).
