# Armed Control — Reflow-Not-Hide + Grow/Shrink Animation

**Date:** 2026-07-23
**Status:** Approved design
**Related:** [[project_synth_inline_armed_controls]], [[project_synth_macro_drilldown_ui]]

## Goal

When a synth control is armed for inline barrel editing, it should grow to a **2× width × 2× height** footprint (easy to use) with a **brief eased animation**, and the surrounding controls should **reflow (never hide)** — sliding to the side or wrapping below. Locking reverses it with a **shrink animation**. This replaces the current instant grow that *hides* covered neighbours. It applies to the SHARED `growArmedCard`, so the regular stage editor and the macro drill‑down behave identically.

## Current behaviour (to replace)

`(*DrumView).growArmedCard(visible []int, grid *ControlGrid, innerR image.Rectangle, armedKIdx int)` (`synth_panel_zone.go`):
- Widens the armed cell rightward then leftward across sibling columns until it reaches `SynthArmedCardW`, clamped to `innerR`.
- Grows height to `SynthArmedCardH`.
- **Hides** same‑row siblings the widened span now covers (`SetRect(image.Rectangle{})`).
- Shifts cells in rows *below* the armed row down by the height delta; hides cells pushed past `innerR.Max.Y`.
- **Instant** — no animation.
- Called from the stage detail‑pane layout AND from `layoutSynthMacroLayer` (macro drilled branch). Drawn by `drawArmedControlCard` (barrel lanes) in both `drawSynthDetailPane` and `drawSynthMacroLayer`.

## New behaviour

### 1. Target footprint — 2×2

The armed control's target rect spans **2 columns × 2 rows** anchored at its resting cell, i.e. `spanCols = min(2, cols)`, `spanRows = 2`. To guarantee the two barrel lanes always fit, the target is widened/heightened to at least `max(2·cellW, SynthArmedCardW)` × `max(2·cellH, SynthArmedCardH)`, clamped to `innerR`. If the 2‑column span would overflow the row to the right, the anchor shifts left so the block fits within the columns. Vertical overflow past `innerR.Max.Y` is allowed and handled by scroll (below), not clamped away.

### 2. Reflow — push, never hide

Placement becomes reading‑order masonry over the grid's uniform cell lattice (same `cols`, `cellW`, `cellH`, gaps, origin the grid already computes):

- Build an occupied bitmap of grid cells. The armed control claims its `spanCols × spanRows` block (all its cells marked occupied); its rect is the union of those cells.
- Every **other** control, in its existing reading order, takes the **next free cell** scanning row‑major. None is hidden: a neighbour the block covers slides to the next free slot — to the side if the row still has a free cell, otherwise wrapping to a later row.
- The grid's scroll content height is extended to the **max occupied row** so a reflow taller than the pane scrolls instead of clipping. (`ControlGrid` already owns vertical scroll; we set its total‑rows/height to the reflowed maximum.)

**Invariant:** every control that had a non‑empty rect before arming has a non‑empty rect after arming (nothing vanishes), and no two control rects overlap.

### 3. Grow / shrink animation

A short eased transition driven by the existing per‑frame tick in `synthTabUpdate()` (the same loop that ticks the barrel kinetics).

Animation state on `*DrumView` (new fields):
- `armGrowIdx int` — control being grown/shrunk (`-1` = none).
- `armGrowInst string` — its instrument id.
- `armGrowT float64` — current progress, `0` = resting, `1` = fully grown.
- `armGrowTarget float64` — `1` while armed, `0` while locking.

Transitions:
- **Arm** (`armSynthControl`): set `armGrowIdx/armGrowInst`, `armGrowTarget = 1`. If a *different* control is armed while one is mid‑grow, snap the previous to resting and start the new one from `armGrowT = 0`.
- **Lock** (`lockSynthControl` / `toggleSynthControlArm` off): set `armGrowTarget = 0` but **retain** `armGrowIdx/armGrowInst` so the shrink can keep drawing after the control is no longer "armed"; clear them when `armGrowT` reaches `0`.

Advance (in `synthTabUpdate`, per frame): step `armGrowT` toward `armGrowTarget` by a fixed per‑frame delta `armGrowStep` (≈ `0.16`, so a full transition is ≈ 100–160 ms at 60 fps — deterministic for tests). Return `changed = true` while `|armGrowT − armGrowTarget| > eps`, which drives the redraw + relayout each frame. When `armGrowTarget == 0` and `armGrowT ≈ 0`, clear `armGrowIdx = -1`.

Layout interpolation: when `armGrowIdx >= 0`, `growArmedCard` computes **both** the resting uniform layout and the reflowed target layout for `armGrowIdx`, then sets each affected control's rect to `lerp(resting, target, ease(armGrowT))` where `ease` is smoothstep. Because layout sets the *interpolated* rects, **hit‑testing follows the visual** — what you click is what you see, mid‑animation included. During shrink `armGrowIdx` is still set and `armGrowT` decreases, so the same interpolation runs in reverse.

**Call‑site change (critical for shrink):** today both layout paths call the grower only `if synthControlArmed(...)`. On lock the control is no longer armed, so that guard would skip the reflow and the controls would snap back instantly — no shrink. The guard therefore becomes "is this control the grow‑animation subject?" via a new `armGrowControl(instID) (idx int, active bool)` (`active == armGrowIdx >= 0 && armGrowInst == instID`). Both the stage path (`synth_panel_zone.go` ~`:2205`) and the macro path (`layoutSynthMacroLayer`) switch to this guard, passing `armGrowIdx`. When fully settled at rest (`armGrowT == 0` and `armGrowTarget == 0`), `armGrowIdx` is cleared so the guard is false and the uniform layout stands.

Draw during grow vs. shrink:
- **Grow / armed** (`synthControlArmed` true, `instEditorArmedEditor != nil`): the draw loop routes the subject through `drawArmedControlCard`, which renders the barrels into the interpolated rect (`SetCardRect` each frame). At very low `armGrowT` the card is near resting size (barrels briefly cramped) — acceptable for a ~120 ms transition.
- **Shrink** (locked → `synthControlArmed` false, `instEditorArmedEditor == nil`): the draw loop's armed branch is false, so the subject falls through to the **resting ParamCard** draw — rendered at its interpolated (shrinking) rect. The barrels disappear on lock and the card collapses back to its cell while neighbours slide home. Keeping the barrels animating during shrink (retain the editor until `armGrowT==0`) is a **screenshot‑gated refinement**, applied only if the instant barrel removal reads as jarring.

### 4. Shared + consistency

All of the above lives in `growArmedCard` + a small `synth_armed_grow_anim.go` (state + `advanceArmGrow`). Both `drawSynthDetailPane` and `drawSynthMacroLayer` already route the armed control through `drawArmedControlCard`, so no per‑path draw change is needed. The 2–3 existing armed‑layout tests that assert the *instant* grown geometry are updated to settle the animation (tick until `armGrowT == 1`) before asserting; the "hidden sibling" expectations, if any, become "reflowed (moved, not empty)".

## Components

| Unit | Responsibility | Depends on |
|---|---|---|
| `synth_armed_grow_anim.go` (new) | `armGrow*` state fields + `advanceArmGrow()` (per‑frame step) + `armGrowControl(instID) (idx, active)` + `armGrowProgress()` (eased 0..1) helpers | `Profile()` (step constant), lerp/smoothstep |
| `growArmedCard` (rewrite) | masonry reflow to a 2×2 target; interpolate rects by `armGrowProgress`; extend grid scroll content | `ControlGrid` cell math, anim state |
| Layout call sites (edit) | stage path (`~:2205`) + `layoutSynthMacroLayer` guard on `armGrowControl(...)` instead of `synthControlArmed(...)`, so shrink also reflows | `armGrowControl` |
| `synthTabUpdate` (edit) | call `advanceArmGrow()`; fold its `changed` into the redraw signal | anim state |
| `armSynthControl` / `lockSynthControl` (edit) | set `armGrowTarget`/idx on arm/lock | anim state |
| `drawArmedControlCard` (unchanged) | draws barrels into whatever (interpolated) rect it's given | — |

## Testing (TDD)

**Reflow (`synth_armed_reflow_test.go`):**
- `TestArmedReflow_NothingHidden`: arm a control in a populated section/macro view; every control that had a non‑empty rect before arming still has one after (settle animation first).
- `TestArmedReflow_NeighbourMovesNotEmptied`: the cell the 2×2 block covers — its control's rect is non‑empty and *different* from its resting rect.
- `TestArmedReflow_NoOverlap`: no two control rects overlap after arming (settled).
- `TestArmedReflow_ScrollExtendsWhenTall`: when the reflow exceeds the pane height, the grid reports scrollable (content height > view).

**Animation (`synth_armed_grow_anim_test.go`):**
- `TestArmGrow_EasesInOnArm`: after arm, `armGrowTarget==1`; ticking `synthTabUpdate` advances `armGrowT` 0→1 and reports `changed` until settled.
- `TestArmGrow_EasesOutOnLock`: after lock, `armGrowTarget==0`, `armGrowIdx` retained; ticking drives `armGrowT`→0 then clears `armGrowIdx`.
- `TestArmGrow_MidAnimationRectBetween`: one tick after arm, the armed control's rect area is strictly between resting and fully‑grown.
- `TestArmGrow_SettledMatchesTarget`: fully settled, the armed rect equals the 2×2 target (≥ `SynthArmedCardW/H`).

**Migrated existing tests:** `TestArmedCardGrowsInPlace`, `TestArmedCardShiftsAdvancedDivider` settle the animation before asserting; `synth_armed_pixels_test.go` lane‑non‑empty checks settle first.

**Screenshots (mandatory, CLAUDE.md):** a mid‑grow frame (partial size + neighbours sliding) and the settled frame, desktop + mobile, for both the macro drilled view (`crop_synth_macro_armed`) and a stage‑editor armed scene (`crop_synth_control_armed`). Confirm no neighbour is hidden and the block is 2×2.

## Out of scope / follow‑ups
- Cross‑fade between the resting card and barrels during the transition (barrels simply render into the interpolated rect for now).
- Reordering animation of controls that wrap to a new row is a straight positional lerp (no path/arc easing).
- `ownerGrid()` returning nil in the macro view (swipe‑scroll on a control) is a separate, already‑logged follow‑up.
