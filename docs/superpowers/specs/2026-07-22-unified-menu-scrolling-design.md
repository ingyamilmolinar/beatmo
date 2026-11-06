# Unified Smooth Menu Scrolling

**Date:** 2026-07-22
**Status:** Implemented (uncommitted per project rule). All list menus + FX + Settings on shared MenuScroll; screenshots verified; full `internal/ui` suite green except 2 proven-pre-existing failures (`TestSamplerMobileOverflowMenuHasSaveAsReset`, `TestFocusGraph_RespondsToEveryKnob`).
**Branch:** add-core-node-types

## Problem

The instrument submenu scrolls smoothly in response to user gestures. Every other
scrollable menu (template/overflow, settings/options, subdivision, logs, context,
EQ channel dropdown) feels bad by comparison, and the FX list — while smooth —
hand-rolls its own scroll glue instead of reusing shared code. The user wants all
component menus with scrollbars to behave like the instrument menu, with the
scroll logic living in **shared code, not spread around**.

## Root cause

A shared 3-layer scroll stack already exists:

- `scroll.go` — `VerticalScroller`: item-level state (First index, drag thumb).
- `touch_scroll.go` — `TouchScroller`: pixel-level velocity/momentum/inertia
  (`friction 0.92`, `UpdateMomentum()`). The only source of smooth inertial fling.
- `scroll_behavior.go` — `ScrollBehavior` composes both. Exposes two mutually
  exclusive wheel modes: continuous `HandleWheel(steps)` (magnitude-respecting,
  no cooldown = "smooth") vs `WheelStep(steps, cooldown)` (one item per notch,
  magnitude ignored, cooldown-locked = "clicky").
- `menu_scroll.go` — `MenuScroll` wraps `ScrollBehavior` + `DeferredTap`.
- `scrollbar_style.go` — `ScrollbarStyle`: shared scrollbar drawing.

The instrument menu feels smooth because it is the **one** caller that bypasses
`MenuScroll.HandleWheel` and drives the raw `ScrollBehavior` with the continuous
`HandleWheel` **and** runs `UpdateMomentum()` every frame.

Everything else feels bad for two reasons:

1. `MenuScroll.HandleWheel` (menu_scroll.go:48) **forces the clicky `WheelStep`**
   path — throws away wheel magnitude, hard-locks to one item per
   `controlGridScrollCooldownFrames` frames.
2. The `MenuScroll` menus (template/overflow, subdivision, logs) **never call
   `UpdateMomentum()`**, so a touch fling stops dead the instant the finger lifts.

The settings/options overlay is worse: `settingsPanelHandler.OnWheel` just returns
`InputConsumed` — it never scrolls at all, and rows past the panel bottom are
silently truncated (`Draw` breaks at `y+26 > o.rect.Max.Y`).

The FX list is smooth but bespoke: `fxScrollOffsetPx` (pixel offset) + a private
`fxScrollTS TouchScroller` + hand-computed thumb geometry (`drumview_fx_panel.go`),
duplicating what the shared engine already does.

Notably, `MenuScroll`'s **touch** state machine (`HandleInput`) already uses the
smooth momentum-capable `HandleTouchMove`/`HandleTouchEnd` and a **proportional**
thumb drag (`HandleDragTo`) — so the touch path is already correct; it just never
gets its per-frame momentum tick, and the wheel path is clicky.

## The rule this establishes

- **List / dropdown / popup menus → smooth.** Continuous wheel (respects
  magnitude, no cooldown), touch-drag with momentum/inertia, proportional thumb.
  The instrument-menu feel.
- **Control grids & the instrument row rack → stay clicky.** They call the raw
  engine's `WheelStep`/`StepDragTo` directly and are left untouched (precise
  one-item-per-notch stepping is desired for grids of controls).

## Design

### 1. Make `MenuScroll` itself smooth (single source)

`internal/ui/menu_scroll.go`:

- `HandleWheel(steps)` → call the continuous `sb.HandleWheel(steps)` instead of
  the clicky `sb.WheelStep(...)`.
- Add `MenuScroll.UpdateMomentum() bool` forwarding to `sb.HasMomentum()` /
  `sb.UpdateMomentum()`.
- Retire the now-unused `TickStep`/cooldown path for these menus (the continuous
  wheel path doesn't use the cooldown clock).

This one change flips **every** `MenuScroll`-based menu to a smooth wheel at once.

### 2. Drive momentum from the existing central tick (not per menu)

`internal/ui/drumview_update.go`: the existing `tickMenuScrollCooldowns()` already
iterates every menu's `MenuScroll`/`ScrollBehavior` once per frame. Rename/extend
it to `tickMenuScrolls()`: for each registered scroller, run `UpdateMomentum()` and
fire that menu's rebuild callback when the offset changed. One loop feeds every
menu's inertia — no per-menu momentum glue spread around.

Menus/scrollers in the tick: overflow (template), context, subdivision, instrument,
EQ channel dropdown, logs, settings (new), FX (migrated). Row rack keeps its own
`TickStep` (clicky) and is not folded into momentum.

### 3. Bring the stragglers onto the smooth path

- **Template/overflow, subdivision, logs** — already on `MenuScroll`; inherit
  smooth + momentum automatically from changes 1 & 2. Ensure each is registered in
  the central tick with a rebuild callback.
- **Context menu, EQ channel dropdown** — raw `ScrollBehavior`, already
  momentum-driven. Flip the single `WheelStep(...)` call to continuous
  `HandleWheel(...)`. Low risk; they already share the engine.
- **Settings / options overlay** (`settings_overlay.go`) — add a `MenuScroll` for
  the shortcuts list. `OnWheel` forwards to it; touch/drag routed through the
  shared `HandleInput`/`HandleScrollbarDrag`; `Draw` applies `OffsetPx()` and draws
  the scrollbar. No-ops when content fits; scrolls (and fixes the latent
  row-truncation) when the panel is height-clamped on small screens.

### 4. FX list — reuse the shared engine (pixel mode)

`ScrollBehavior`/`MenuScroll` is item-index based, but with `ItemHeight = 1` it
becomes a **pixel-offset** scroller: `VS.First` = pixel offset, `VS.Total` /
`VS.Visible` in px, `OffsetPx()` = the pixel offset, `applyPixelDelta` gives
sub-pixel momentum, and `BarRect`/`ThumbRect` are unit-agnostic (proportional).

Replace `fxScrollOffsetPx` + private `fxScrollTS` + hand-computed thumb with a
single shared `MenuScroll` in pixel mode:

- Configure each frame with px totals (content height / viewport height).
- Wheel → `HandleWheel(steps * fxWheelPx)` (preserve the ~20px/notch feel).
- Read the offset via `OffsetPx()` where `fxScrollOffsetPx` was read.
- Scrollbar draw → `MenuScroll.Draw` (shared `ScrollbarStyle`) instead of the
  hand-computed thumb.
- Momentum via the central tick (change 2).

Same wheel/momentum/thumb-drag/scrollbar-draw code as every menu.

### 5. Untouched

- Instrument **row rack** and synth **ControlGrid** — keep clicky
  `WheelStep`/`StepDragTo`. (User: "keep the instrument row clicky.")
- Instrument **menu** — stays smooth (it's the reference). Optionally simplified to
  call the new shared `MenuScroll.UpdateMomentum()` to prove reuse.

## Testing & verification

- **Update** the clicky-pinning tests to assert continuous magnitude + momentum:
  `menu_scroll_test.go`, `subdiv_menu_scroll_test.go`, `template_menu_scroll_test.go`.
- **Add functional tests** (drive real `Game.Update`, per project convention — no
  hand-built harness): wheel-magnitude scroll + post-release inertia on the template
  menu; settings scroll when height-clamped; FX pixel-scroll offset parity.
- **Screenshots (mandatory per CLAUDE.md UI rule):** capture `template_menu_open`,
  settings, subdivision, logs, and the FX panel scenes; open each PNG and confirm
  the scrollbar chrome and a scrolled-offset state render correctly. Desktop +
  `MOBILE=1`.

## Files touched (anticipated)

- `internal/ui/menu_scroll.go` — smooth wheel + `UpdateMomentum`.
- `internal/ui/drumview_update.go` — central momentum tick.
- `internal/ui/drumview_portal_open.go` — context/EQ wheel flip; FX wheel via MenuScroll.
- `internal/ui/eq_panel_zone.go` — EQ channel dropdown wheel flip.
- `internal/ui/settings_overlay.go` — add MenuScroll.
- `internal/ui/drumview_fx_panel.go`, `drumview.go`, `drumview_close_popups.go` —
  FX migration off `fxScrollOffsetPx`/`fxScrollTS`.
- Tests as above; scene catalog reuse for screenshots.

## Non-goals

- No change to row rack / ControlGrid clicky feel.
- No new scroll component — pixel mode reuses `MenuScroll` (`ItemHeight = 1`).
- No redesign of scrollbar visuals (already shared via `ScrollbarStyle`).
