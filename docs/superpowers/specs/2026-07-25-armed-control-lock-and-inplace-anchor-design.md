# Armed Control — Lock-on-Any-Click, No Text Entry, In-Place Anchor

**Date:** 2026-07-25
**Status:** Approved (brainstorm), implementation in progress
**Branch:** add-core-node-types (uncommitted, per project rule)
**Predecessors:** `2026-07-22-synth-inline-armed-controls-design.md`, `2026-07-23-armed-control-reflow-animation-design.md`

## Problem

Three UX issues on the Synth-tab inline **armed control** (a control that, when tapped, grows to a 2×2 block with two inertial barrel lanes):

1. **Locking is inconsistent.** A stationary click on the value-readout band opens a numeric text editor instead of locking; only clicks elsewhere on the card lock. Users expect *any* click on the expanded control to lock it in.
2. **Manual text entry is unwanted for now.** The numeric editor path (`openArmedNumericEditor`) should be disabled (retained in code, un-triggered, easy to re-enable).
3. **Arming the last control in a row relocates it.** `reflowArmed` places the 2×2 block at the *earliest free* cell in reading order, so arming a control at the right edge bumps the block down to the next row. The clicked control jumps away from the pointer.

## Goals

- Any stationary click inside the armed card **locks** the control (value readout included).
- No numeric text editor opens from the armed control (disabled, not deleted).
- The armed control **stays at its clicked position**; displaced controls (starting with the immediate left neighbour) reflow into free cells below. Order is non-strict and restored on lock.
- Close the 5 stale wheel→armed migration test gaps; triage (not bundle) the unrelated sampler-menu failure.

## Design

### Part 1 — Lock on any click (`synth_panel_zone.go`, `synthControlHitAdapter.OnRelease`)

Current stationary-release branch:
```go
if !h.armedMoved {
    if h.armedValueReadoutHit(x, y) { h.dv.openArmedNumericEditor(...) }
    else { h.dv.toggleSynthControlArm(h.instID, h.idx) }
}
```
becomes:
```go
if !h.armedMoved {
    h.dv.toggleSynthControlArm(h.instID, h.idx) // any stationary click locks
}
```
A dragged spin (`armedMoved == true`) still stays armed. `armedValueReadoutHit` is removed (only caller gone).

### Part 2 — Disable text entry

`openArmedNumericEditor` stays defined but has no caller. The `paramEditor`-active swallows in `OnPress`/`OnWheel` remain (they guard the shared inline editor used by non-armed param cards). Re-enabling later = restore the one branch in Part 1.

### Part 3 — In-place anchor (`synth_armed_reflow.go`, `reflowArmed`)

Replace the armed block's `earliestFree(true)` placement with an **anchored** placement, in a new no-phantom branch (phantom/divider inputs keep the existing algorithm as fallback, since `growArmedCard` already routes divider sections to `growArmedCardLegacy`):

```
armedRow = armedPos / cols
armedCol = clamp(armedPos % cols, 0, cols - spanCols)
reserve block cells [armedRow .. +spanRows-1] × [armedCol .. +spanCols-1]
for each control in reading order (skip armed): place at earliest free non-reserved cell
armed control's rect = union(cellAt(armedRow, armedCol), cellAt(armedRow+spanRows-1, armedCol+spanCols-1))
```

Behaviour:
- **Interior / left-edge control:** block grows right+down from the clicked cell (clicked = top-left).
- **Right-edge control (last in row):** `armedCol` clamps to `cols-spanCols`, so the clicked cell becomes the block's **top-right** corner — unchanged position — and the left neighbour is pushed to a later (lower) cell.
- Nothing is hidden; no cells overlap; `rowMax` still feeds `SetContentRows(rowMax+1)` for scroll extension.

**Vertical edge:** the block anchors at the clicked *row* and extends one row down; `SetContentRows` + scroll reveal it. The reported defect is horizontal; row-clamping is added only if a screenshot shows bottom-row clipping.

`growArmedCard` is unchanged (it already passes `visible, armedPos, cols, spanCols, spanRows, cellAt`); the resting→target lerp now interpolates toward the in-place anchor, so the clicked control barely moves while neighbours slide.

## Testing

**`reflowArmed` (pure geometry, `synth_armed_reflow_test.go`):**
- Clicked cell is always covered by the returned block rect (interior, left-edge, right-edge).
- Right-edge arm: block right column == clicked cell's column; the pre-arm left neighbour lands in a strictly later cell (pushed below), not hidden.
- Nothing hidden / no overlap preserved; existing 2×2 and clamp-span cases still pass.

**Functional (real `Game`, armed layout tests):**
- Arm the last control in a row → armed rect's `Min.Y` equals its resting row's Y (does not drop to the row below); a previously-visible neighbour remains non-empty.

**Lock / no-numeric (`synth_armed_numeric_test.go` rewritten, OnRelease tests):**
- Stationary click on the value-readout band → control **locks** (armed state cleared), no numeric editor active.
- Dragging a lane → value changes and control **stays armed**.

**Migration gaps closed:**
- `synth_panel_drag_e2e_test.go` (2) → arm-first, then drag/wheel the value lane mutates the param.
- `param_card_input_test.go` `WheelNudgeFlushes*` (2) → arm-first, wheel, then tab/view switch flushes exactly one undo step.
- `basic_scenes.txt` → remove retired `crop_mobile_synth_wheel`.

**Out of scope / triage:** `TestSamplerMobileOverflowMenuHasSaveAsReset` (sampler overflow menu shows 4 vs expected 2) — unrelated to armed controls; diagnosed and reported separately.

**Visual verification (mandatory, per CLAUDE.md):** capture `crop_synth_control_armed` (and a last-in-row variant) desktop + mobile; confirm the armed block sits where the control was, neighbours reflowed not hidden, no numeric editor.
