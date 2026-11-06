# EQ Handle Hover Highlight — Design

**Date:** 2026-07-16
**Status:** Approved (scope: band + HPF/LPF handles)

## Goal

Add a hover highlight to the EQ tab's draggable handles (the 10 round band
"nodes" plus the two square HPF/LPF filter markers). On desktop, when the mouse
hovers over a handle, it blooms with the **same highlight styling and color the
triggered grid nodes use** — so hover affordance reads as one consistent visual
language across the app.

## What exists today (`internal/ui/eq_panel_zone.go`)

- 10 band handles drawn at `eqBandHandlePos(i)` (radius `eqHandleRadius=10`) with
  a static amber accent glow behind each; HPF/LPF handles drawn as squares.
- Per-handle state is **drag/select only**: `curveDragBand` (int, -1 idle),
  `curveDragFilter` (string "hpf"/"lpf"/""), `eqSelectedBand` (mobile stepper).
  **There is no hover state.**
- Hit-testing happens **only on press**, in `curveHandleHitAdapter.OnPress`:
  filter handles first (priority), then band handles, via a circular distance
  test using `Profile().EQHandleRadius` (desktop 16 / mobile 26).

## Highlight infra being reused (the triggered-node language)

- `drawNodeBloom(dst, cx, cy, rPx, nodeColor, intensity)` (`components.go:98`) —
  the concentric neon halo helper triggered grid nodes bloom with.
- Color token `colHighlight` = `#FDF2FF` (`theme.go`) — the exact cool-white the
  triggered nodes use. `drawNodeBloom` preserves the RGB and varies only alpha.

## Plan

1. **Hover state** — add to `EQPanelZone`, mirroring the drag fields:
   - `curveHoverBand int` (init `-1`)
   - `curveHoverFilter string` (init `""`)
   Desktop-only feature; on mobile both stay idle so touch is unchanged.

2. **Shared hit-test helpers** — extract the two press-time hit tests into pure
   lookups so hover and press share identical geometry (no drift between where a
   handle is highlighted and where it grabs):
   - `bandHandleAtPoint(x, y int) int` → band index or `-1` (desktop precise
     circle; returns `-1` on mobile).
   - `filterHandleAtPoint(x, y int) string` → `"hpf"` / `"lpf"` / `""`.
   `OnPress` is refactored to call these — no behavior change, verified by the
   existing press/drag flow staying green.

3. **Per-frame detection** — `updateEQHandleHover()` called from `Update()`
   (right after `updateAudioPanelCursor()`):
   - Only on the EQ tab, desktop; otherwise reset both fields to idle.
   - Read `cursorPosition()`; apply the same **filter-first priority** as
     `OnPress`: if a filter handle is hovered, set `curveHoverFilter` and clear
     `curveHoverBand`; else set `curveHoverBand` from `bandHandleAtPoint`.

4. **Render** — in the handle draw loops, when a handle is hovered draw
   `drawNodeBloom(dst, cx, cy, eqHandleRadius, colHighlight, intensity)` behind
   the handle — same helper, same color as triggered nodes.

## Tests (TDD — functional, drive real `Update`/`Draw`)

Use `SetInputForTest` for the cursor and a rounded-rect interceptor that records
color (to distinguish the `#FDF2FF` hover bloom from the existing amber glow).

- `TestEQHandleHoverStateTracksCursor`
  - cursor over band *i* → `curveHoverBand == i`, `curveHoverFilter == ""`
  - cursor over the HPF handle → `curveHoverFilter == "hpf"`, `curveHoverBand == -1`
  - cursor off every handle → both idle
  - mobile profile → both stay idle regardless of cursor
- `TestEQHandleHoverDrawsHighlight`
  - with a band handle hovered, `Draw` emits a rounded-rect in `#FDF2FF`
    centered on that handle
  - with a filter handle hovered, same in `#FDF2FF` at that handle
  - with nothing hovered, no `#FDF2FF` bloom is drawn

## Out of scope

- No change to mobile/touch behavior.
- No change to drag, selection, or gain-editing logic.
- No new design tokens (reuses `colHighlight`, `drawNodeBloom`,
  `eqHandleRadius`).
