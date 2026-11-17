# Synth Inline Armed Controls — Design

**Date:** 2026-07-22
**Branch:** add-core-node-types
**Status:** Design approved; awaiting spec review → implementation plan
**Scope:** Synth tab control cards only (Sampler / EQ wheel popups untouched)

---

## 1. Problem

The synth tab's per-control editing model has grown three separate, partly
inconsistent interaction paths:

- **Desktop:** mouse wheel *over a control* nudges its value in place; wheel over
  whitespace/scrollbar scrolls the grid; **clicking a control opens the barrel
  `MobileWheelPopup`**; Shift+wheel = one resolution rung finer.
- **Mobile:** tap a control → the same barrel `MobileWheelPopup` (value barrel +
  resolution strip + numeric entry); a vertical swipe on a control scrolls the
  grid via `dragScrollArbiter`.
- **Row scrolling** is **row-by-row stepped** (one full row per wheel notch /
  `StepDragTo`), which is jumpy and hard to nudge slowly.

Problems this creates:

1. **Two editing surfaces to learn** (in-place wheel vs. popup) and a hidden
   rule (wheel changes value *only* over a control, scrolls *only* over
   whitespace) that isn't discoverable.
2. **The popup is heavy and skeuomorphic** — a modal barrel that visually
   detaches from the control it edits.
3. **Slow scrolling is hard** — stepped scroll jumps a whole row per notch, so
   there is no gentle "creep down one control" gesture.
4. **Resolution is buried** — you must open the popup and drag a separate strip.

## 2. Goals

Replace all of the above with **one unified, modern, in-place model** that is
immediate to use yet allows nuance in both value and resolution:

- **Click/tap a control to arm it** (pressed animation, card grows in place),
  edit with kinetic wheels, **click/tap again to lock** ("set").
- **Two inline inertial wheels** per armed control: **left = resolution**,
  **right = value** — smooth, endless, momentum + snap, no skeuomorphism.
- **Not armed:** wheel/swipe over a control **scrolls the row of controls**
  (smoothly, with light momentum), so slow scrolling is easy on both platforms.
- **One mental model across desktop and mobile.** The barrel popup is retired
  from the synth path.

### Non-goals

- Sampler and EQ tabs keep their existing `MobileWheelPopup` (possible later
  follow-up; explicitly out of scope here).
- No change to *which* params exist, their ranges, or the resolution ladder math
  (`stepLadder` / `KnobStepBadge` semantics are reused as-is).
- No change to the audio/commit pipeline (`commitInstrumentParams`,
  `requestSynthMirror`, `syncSliderFromKnob`, `propagateSynthSliderValue`).

## 3. Interaction Model

### 3.1 Arm / lock state machine

State is **per instrument**, at most **one control armed at a time**, stored as
`instEditorArmedControl map[string]int` on `DrumView` (sibling to the existing
`instEditorSelectedControl`). Arming a control also **selects** it (drives the
focus graph), reusing `setSynthSelectedControl`.

Transitions:

| From | Event | To |
|---|---|---|
| none | tap/click control *i* (stationary, no scroll) | armed(*i*), selected(*i*) |
| armed(*i*) | tap/click control *i* again | none (locked/"set") |
| armed(*i*) | tap/click control *j* | armed(*j*) (locks *i* first) |
| armed(*i*) | tap empty space / `Esc` / stage or instrument switch | none |
| armed(*i*) | scroll gesture that becomes a row scroll | none (scroll wins; see 3.4) |

"Lock" is not a separate commit — edits commit continuously while armed (see
3.3). Lock just snaps to the nearest step, disarms, and collapses the card.

### 3.2 Layout — expand in place

The armed card **animates taller** within the `ControlGrid` (a height tween;
neighbors reflow / relax), revealing two vertical lanes inside its own footprint:

```
 resting card            armed card (grown)
 ┌───────────┐           ┌─────────────────────┐
 │  CUTOFF   │           │  CUTOFF        ●armed│
 │  2.4 kHz  │    →      │ ┌────┐ ┌───────────┐ │
 └───────────┘           │ │ ×5 │ │   3.0 k   │ │
                         │ │ 10 │ │  ▸ 2.5 k ◂│ │  ← value lane (right)
                         │ │ ×2 │ │   2.0 k   │ │
                         │ └────┘ └───────────┘ │
                         │  res        value    │
                         └─────────────────────┘
```

- Left lane is narrow (resolution rungs); right lane is wider (value).
- The armed card is the tap/drag target for its lanes; a stationary tap anywhere
  on the header/whitespace of the armed card locks it.
- Because arming changes one row's height, the grid's laid-out rects are
  recomputed on arm/lock (Invalidate → next Layout), same as a scroll change.

### 3.3 The two wheels

Both lanes are an **infinite, inertial `BarrelWheel`** (new widget, §4.1) bound
directly to the existing value-model — **no duplicated value state**:

- **Value lane (right)** — bound to the control's `*Knob`. Spinning advances the
  value by the current resolution step (`StepMul` / `NudgeEndless` for
  continuous params; one item per detent for enum/discrete params, spinning
  through `def.Enum` labels). Precision is resolution-aware
  (`formatSynthParamValueStep`). On settle, snaps to the nearest step.
  - **Commit:** each landed detent fires `OnChange`
    (`syncSliderFromKnob` + `propagateSynthSliderValue`); a debounced
    `synthWheelNudge.bump` coalesces the spin into **one** undo step via
    `commitInstrumentParams` + `requestSynthMirror` (reusing today's wheel-nudge
    plumbing).
- **Resolution lane (left)** — bound to the control's `KnobStepBadge`. Spinning
  moves along the consecutive 1·2·5 rung ladder (`stepLadder`), clamped at the
  ladder ends. On change, persists via `persistKnobStep`. Changing resolution
  re-scales how fast the value lane moves and the value's display precision.

**Numeric entry (kept):** while armed, the value readout doubles as an inline
numeric field — desktop: start typing or double-click the readout; mobile: tap
the readout → numeric keypad. Reuses the existing `openSynthParamEditor` /
`paramEditor` path (which the popup's `OpenEditor` used).

### 3.4 Input mapping

**Desktop (armed):**
- **Mouse wheel** spins the lane the **cursor is hovering** — left half of the
  content area = resolution, right half = value.
- **Arrow keys:** `↑`/`↓` = value ± one detent; `←`/`→` = resolution ± one rung.
- **Click** (stationary) on the armed card again = lock.
- Fling is available via a quick wheel burst (impulse accumulates velocity, §4.2).

**Desktop (not armed):**
- **Mouse wheel over a control = scroll the row** (continuous + momentum). This
  is the deliberate change from today's in-place nudge. Wheel over whitespace
  behaves the same (already scrolls).
- **Click** (stationary) = arm.

**Mobile (armed):**
- **Swipe** on the left/right half spins that lane with inertia (fling → decay →
  snap).
- **Tap** on the armed card (not on a lane fling) = lock.

**Mobile (not armed):**
- **Swipe** over a control scrolls the row with momentum (existing
  `dragScrollArbiter` tap-vs-scroll gate decides tap=arm vs sweep=scroll).
- **Tap** = arm.

### 3.5 Row scrolling — smooth continuous + light momentum

The synth `ControlGrid` moves from **row-by-row stepped** to **pixel-continuous
scroll with light momentum**, natural direction preserved (content follows
finger). This gives a gentle "creep" for slow gestures and slow wheel scrolling.

- Reuse the momentum/continuous machinery already proven in the menu scrollers
  (`MenuScroll`, pixel mode + momentum) rather than inventing a new integrator —
  see `project_unified_menu_scrolling_2026_07_22` and `project_shared_menu_scrollbar`.
- The scrollbar thumb still follows the finger directly (non-natural), as today.
- Keep the existing peek band + edge fades (`control_grid_fades.go`,
  `ControlGrid.PeekRect`).

## 4. Architecture

### 4.1 `BarrelWheel` widget (new)

A self-contained, testable inertial wheel with **no owned value state** — it
reads/writes through a small binding interface so the `*Knob` (value) and
`KnobStepBadge` (resolution) remain the single sources of truth.

```
type barrelBinding interface {
    // current selection as a float position and its display label
    labelAt(offset int) string   // label `offset` detents from current (for the fade stack)
    stepBy(detents int) bool      // move current selection by N detents; returns true if it changed
    canStep(dir int) bool         // false at a hard clamp (ladder end / param min|max for discrete)
}
```

- **Value binding** wraps `*Knob` (+ `KnobStepBadge` for step size, `def` for
  labels/enum/precision).
- **Resolution binding** wraps `KnobStepBadge` over `stepLadder`.

`BarrelWheel` owns only **kinetic** state: `velocity`, `offsetPx` (sub-detent
scroll position), and a `settling` flag. It exposes:

- `Fling(v float64)` / `ImpulseNotch(dir int)` — seed velocity from a drag
  release or a wheel notch.
- `DragTo(dy int)` — direct positional drag while a pointer is down.
- `Tick(dt)` — integrate velocity, apply friction, cross detents (calling
  `stepBy`), and when velocity falls below threshold, **ease `offsetPx` to 0**
  (snap). Returns whether it still needs animating.
- `Draw(dst, rect, style)` — the modern flat renderer (§4.3).

### 4.2 Inertia model

- Drag: track pointer-y velocity (px/frame, lightly smoothed). On release,
  `Fling(velocity)`.
- Wheel: each notch calls `ImpulseNotch(dir)` — adds a fixed velocity impulse so
  repeated notches build speed and coast (feels inertial, not clicky), while a
  single slow notch still lands exactly one detent.
- Friction: velocity *= `barrelFriction` each tick; below `barrelMinVel`, enter
  settle and ease `offsetPx → 0` over a short tween (snap to nearest detent).
- Detent crossing: whenever `offsetPx` passes ±`detentPx`, call `stepBy(±1)` and
  wrap `offsetPx`. `canStep` false → kill velocity into the clamp (no overscroll
  past param/ladder ends; a subtle rubber-band is optional, default off).

Tunables live as density tokens (see §6), not magic numbers.

### 4.3 Modern renderer — "in touch with the control"

**Explicitly NOT the barrel/cylinder style** of `mobile_wheel_render.go` (no
cylinder gradient, ridges, specular, keycap, rails, thumb grip). New flat
renderer that inherits the ParamCard's own tokens:

- Each lane is a vertical stack of candidate labels from `labelAt(-n..+n)`.
- **Center row** = current value: bold (`RoleBody`), **accent-tinted**, marked by
  a thin accent hairline underline *or* a subtle accent-filled pill (single
  choice, picked during visual verification).
- **Neighbors** progressively dimmed via alpha; a soft `fillVerticalGradient`
  fade toward the top/bottom card edges (reuse the existing generic gradient
  helper — that is a primitive, not the barrel look).
- Uses the card's surface tokens (`surface-2`/`surface-3`), corner radius, and
  the same caption/body font roles the card already uses. The lanes read as the
  card's own content that happens to move.
- Left/right lanes separated by spacing or a 1px subtle token divider.
- Motion is sub-pixel (`offsetPx` offsets the whole stack) for smoothness.

### 4.4 Wiring into the existing adapter

`synthControlHitAdapter` (in `synth_panel_zone.go`) is the integration point.
Rework its handlers to branch on armed state:

- `OnPress` / `OnDrag` / `OnRelease`:
  - **not armed** → existing `dragScrollArbiter` (tap → arm on release; sweep →
    scroll the grid, now continuous+momentum).
  - **armed(this)** → route drag to the hovered lane's `BarrelWheel.DragTo`;
    release → `Fling`; a stationary tap → lock.
  - **armed(other)** → treat a tap as: lock other, arm this.
- `OnWheel`:
  - **not armed** → forward to the owner grid scroll (was: `nudgeKnobByWheel`).
    This is the desktop behavior change.
  - **armed(this)** → `ImpulseNotch` on the hovered lane.
- **Keyboard:** the panel gains an armed-control key handler (↑↓ value, ←→ res,
  `Esc` lock). Wired where the synth tab already handles focus/keys.

`openSynthControlWheelPopup` / `synthWheelPopup` usage is removed from the synth
path (the type stays for Sampler/EQ). `drumview_wheel_popup.go` synth shims are
deleted or repointed.

### 4.5 Per-frame animation & render cache

- The armed card must **tick every frame** while any lane has velocity or an
  active settle tween — the panel `Invalidate()`s until `Tick` reports settled.
- The armed card region **bypasses the synth render cache** (`TraceSprite` /
  synth tab draw cache — see `project_synth_tab_render_cache`) while animating,
  then re-caches on settle, so the kinetic lanes don't fight the cached sprite.
- Respect the existing synth-tab **alloc/drawcall budgets**
  (`synth_panel_fingerprint_test.go`, drag byte-budget); the barrel draw must be
  allocation-lean (precompute label strings per settle, not per frame where
  possible).

## 5. Isolation & input-tree correctness

- The armed lanes live **inside** the synth control card's existing hit area;
  gestures are captured on press (`InputCaptured`) so a fling doesn't leak to the
  grid/timeline (same catch-all/opaque-to-z contract as today — see
  `zone_input_isolation_discipline_test.go`).
- No modal portal is introduced, so none of the out-of-tree modal-leak concerns
  (`project_synth_wheel_popup_input_isolation`) apply — a win from dropping the
  popup. Verify the row-scroll and grid input still work with a control armed.

## 6. Tokens (density-tied)

New sizing knobs go in `DESIGN.md` `densities:` (all three densities) →
`make gen-design-tokens`, read via `Profile().DensityValues()`:

- `synthArmedCardH` — armed card height (drives the grow tween target).
- `synthWheelLaneW` (resolution lane) — value lane takes the remainder.
- `synthWheelRowH` — per-candidate row height (== detent px baseline).
- `synthWheelVisibleRows` — candidates drawn per lane (odd; center + N each side).

Inertia tunables (friction, min velocity, impulse magnitude, snap ease
duration) as `RuntimeProfile`-neutral constants unless a bench shows they must
diverge browser/desktop (add a one-line bench citation if so).

No new color tokens — reuse card surface/accent/`WithAlpha` buckets.

## 7. Testing & verification

**TDD, functional (drive real `Game.Update`), per house rules.**

Go (fast `-tags test`):
- `BarrelWheel` unit: fling decays and snaps to a detent; a single notch moves
  exactly one detent; clamps at ends (`canStep` false); enum lane cycles labels;
  resolution lane walks the 1·2·5 ladder and persists.
- Arm/lock state machine: tap arms + selects; tap again locks; arm-other locks
  prior; `Esc`/empty-tap/stage-switch locks; only one armed at a time.
- Input mapping: desktop wheel **not armed → scrolls** (regression against the
  old in-place nudge); armed → hovered lane spins; arrow keys map value/res.
- Row scroll: continuous + momentum, natural direction; thumb still direct;
  slow gesture creeps sub-row.
- Numeric entry still reachable while armed and commits.
- Perf/discipline: armed card bypasses cache while animating; alloc/drawcall/
  fingerprint budgets hold; zone input isolation intact.
- Resolution-aware precision unchanged for the non-armed caption.

Browser (JS charter — only the WASM↔bridge boundary, if any export shape
changes): update `wasm_bridge_smoke.browser.test.js` catalogue only if
`synthControlRects`/related exports change; **no new dedicated JS test** just for
this (logic lives in Go).

**Mandatory UI screenshot loop** (per CLAUDE.md):
- New/updated scenes in `scene_catalog.go`: `crop_synth_control_armed`
  (desktop) with `MobileSetup` for the mobile pass, driving a control into the
  armed state with both lanes visible mid-spin and at rest.
- Capture desktop + `MOBILE=1`, **open the PNGs**, confirm: card grew, both
  lanes render, center value bold/accented, fades correct, no skeuomorphic
  chrome, no overlap/clipping. Iterate until it looks right.
- Extend `audio_panel_render_pixels_test.go` if the armed lanes count as new
  audio-panel data pixels (color-count the lane rects).

## 8. Risks / open points (resolve during implementation)

- **Grow tween vs. grid reflow:** growing one row mid-list must not fight the
  peek band / windowed layout. Mitigation: treat the armed height as part of the
  row's laid-out height so `ControlGrid` windowing accounts for it; Invalidate on
  arm/lock.
- **Center hairline vs. pill:** decide by looking at both in the screenshot loop.
- **Fling-then-tap disambiguation on mobile:** a tap that lands on a still-moving
  lane should stop the fling, not immediately lock — pick a small settle
  deadzone; covered by a functional test.
- **Wheel sign / natural-scroll direction:** the historical flip-flop
  (`project_mobile_knob_wheel`) means direction must be pinned on a real device;
  anchor tests to "drag/scroll down → value increases; not-armed scroll down →
  content down" and verify on the user's device against an uncommitted build.

## 9. Out of scope (explicit)

- Sampler / EQ wheel popups (`sampler_wheel_popup.go`, `eq_wheel_popup.go`) —
  unchanged.
- Any change to the resolution ladder math, param definitions, or audio commit
  pipeline.
- Retiring `MobileWheelPopup` the type (still used by Sampler/EQ).
