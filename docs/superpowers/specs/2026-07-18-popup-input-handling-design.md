# Popup input handling: opening-gesture guard, shared tap debounce, uniform outside-click

- **Date:** 2026-07-18
- **Branch:** add-core-node-types (uncommitted lineage of `2026-07-16-param-card-popup-controls`)
- **Status:** IMPLEMENTED + verified (uncommitted). Parts A/B/C all TDD RED→GREEN; discipline guards green; `internal/ui` builds native + wasm. Pre-existing branch failures (instrument-menu `-run` ordering leak, node-logic-RED-at-HEAD, `audio/songrender` wasm build) confirmed unrelated.
- **Package:** `src/go/internal/ui`

## Problem

Three related defects in mobile popup / button input handling:

1. **Open-then-close (primary):** On mobile, tapping a synth-tab ParamCard opens its
   wheel popup and the *same tap* immediately closes it — unless the finger drags
   into the popup body before releasing. The popup is placed below the card, so it
   never overlaps the tap point.

2. **No shared tap debounce:** The mobile tap-fire path (`fireTapAt → OnClick()`
   direct) has no press guard at all. Touch-bounce or a fast double-tap fires the
   action twice. Desktop single-fires via the `Button.held` press-edge counter, but
   the mobile direct-fire paths bypass it. There is no single primitive both paths
   share.

3. **Outside-click leaks to surface below (non-blocking overlays):** Blocking
   scrim/modal popups already close on an outside tap and consume the press. But
   non-blocking interactive overlays (docked FX panel, EQ channel dropdown) are
   gated in the out-of-tree grid/camera handlers only by `modalOverlayActive()` =
   `PortalHasBlocking()`, so an outside tap can close the overlay *and* leak to the
   grid/camera beneath.

## Root cause of #1 (confirmed by code trace)

The ParamCard opens the popup on the **physical touch-down** (`synthKnobHitAdapter.OnPress`
→ `openSynthKnobWheelPopup`, `synth_panel_zone.go:3569-3583`, returns `InputConsumed`).
The card lives in the **audio subtree** (`dv.audioTree`, RootTree z=1); the popup opens
on `dv.portal()` = **overlay subtree** (`dv.overlayTree`, RootTree z=2). Two subtrees,
two independent `wasPressed` fields and hit indices.

On finger-lift, `GestureTap` fires `injectTouchTap(liftX, liftY)` (`game_update.go:274-289`)
— a 2-frame synthetic press/release that **replays the same physical gesture**. The
injection-edge clear at **`drumview_tree.go:557-560`**:

```go
injecting := isTouchTapInjecting()
if injecting && !t.wasInjecting && pressed && t.capturedHandler == nil {
    t.wasPressed = false   // re-arms a fresh press so the replayed tap reaches its control
}
t.wasInjecting = injecting
```

fires **unconditionally**, including on the overlay subtree whose portal this very
gesture just opened. So the injected replay is treated as a brand-new press over the
now-open popup. Because the replay coords sit on the card (outside the popup body), it
lands on either:

- the popup's **anchor tap-to-close** hit area registered over the opener
  (`mobile_wheel_popup_portal.go:32-38, 93-99` → `popup.Accept()`), or
- the generic **click-outside** branch (`drumview_tree.go:636-650` when `len(hits)==0`,
  or `:673-678` when the hit is `z < ZOverlayMin`) → `portal.CloseTop()`.

Either path closes the popup on the same tap that opened it. Dragging into the popup
moves the lift onto the popup body's own `z≥300` hit area, so neither dismissal branch
fires — exactly the "unless I drag inside" behavior.

**There is no guard recording that the gesture opened the popup**, so the replay cannot
be distinguished from a genuine second tap. That missing guard is the fix.

Sequence (clean tap):
- **D0 physical down:** overlay dispatched first (portal not open yet) → sets
  `overlay.wasPressed=true`, no hits, returns. Then audio dispatched → card `OnPress`
  opens the portal (Modal+Scrim).
- **D1..Dn held:** overlay is the exclusive blocking owner; `pressed && wasPressed` → no
  new-press edge; popup stable.
- **Injection frame 0 (lift):** injection-edge clear resets `overlay.wasPressed=false`
  → new-press edge fires → anchor-close / click-outside → **popup closes**.

## Design

Three cohesive changes, all in shared input infra so every popup/button benefits.

### Part A — Opening-gesture guard (fixes #1)

Add a narrowly-scoped guard on `DrumViewTree` so the injected replay of the gesture that
opened a portal is **not** re-armed as a fresh press.

New fields on `DrumViewTree`:
- `prevPortalOpen bool` — `t.portal.IsOpen()` observed last frame.
- `portalOpenedThisPress bool` — the portal opened during the current, still-unreleased
  press.

In `handleInput`, near the injection-edge clear:

```go
portalOpen := t.portal.IsOpen()
if !portalOpen {
    t.portalOpenedThisPress = false
} else if pressed && t.wasPressed && !t.prevPortalOpen {
    // Portal opened during this press (observed on the frame after the opener ran).
    t.portalOpenedThisPress = true
}

injecting := isTouchTapInjecting()
if injecting && !t.wasInjecting && pressed && t.capturedHandler == nil && !t.portalOpenedThisPress {
    t.wasPressed = false
}
t.wasInjecting = injecting
t.prevPortalOpen = portalOpen
```

Clear `portalOpenedThisPress` on real release (the `!pressed && t.wasPressed` branch,
`drumview_tree.go:573`).

**Why it works / scope:**
- Keys on **press-continuity**, not a time window — robust no matter how long the finger
  is held before lifting.
- Engages **only** when a portal opened during the press, so the existing replay behavior
  (`TestKebabTapOverlapScrollerActive` — replayed tap must reach its control when no portal
  opened) is preserved.
- A genuine **second** tap-to-close is unaffected: on its physical down the popup is already
  open, so the overlay dispatches the anchor-close/​click-outside on the real press edge (not
  via the guarded injection path). Toggle-to-close still works.
- **Shared across all popups:** every popup opens via `dv.portal()` and dismisses through
  this one `handleInput`, so any press-opened popup is protected from its own opening
  gesture's replay. The "drag into the popup" workaround becomes unnecessary.
- Desktop is unaffected (`isTouchTapInjecting()` is false there).

### Part B — Shared mobile tap debounce (fixes #2)

One frame-count-based, location-keyed gate that every mobile tap-fire funnels through.

Primitive (new small type, e.g. `tapDebounce` in a new `tap_debounce.go`):
- State: `lastX, lastY, lastTick int`, `has bool`; a monotonic `tick` advanced once per
  `DrumView.Update`.
- `Accept(x, y int) bool` — returns `false` (reject) when a prior accepted tap exists
  within `tapDebounceSlopPx` (~12px) **and** `tapDebounceFrames` (~9 frames ≈ 150ms at
  60fps); otherwise records `(x, y, tick)` and returns `true`.

Rationale for location+time keying: it is genuinely shareable (each fire site passes only
`(x, y)`), it catches true bounce/double-tap (same spot, rapid) without blocking legitimate
fast taps on *different* controls (12px ≪ 44px touch target), and frame-count timing is the
codebase idiom (`multiTouchCooldownFrames`, `controlGridScrollCooldownFrames`) and is
deterministic for tests. Auto-repeat buttons are unaffected — they fire via `held`/`repeatTick`,
not fresh taps.

Wiring (the "exact same infra"):
- Hold the gate on `DrumView` (`dv.tapGate`); advance its tick once per `Update`.
- Gate the two real chokepoints:
  1. `Button.applyPress` at the `held==1` press edge (covers desktop + `injectTouchTap`-driven
     buttons).
  2. The direct-fire popup paths: the `fireTapAt` implementations
     (`InstrumentMenuComponent`, `NodeSidebar`, `scrollableDropdownHandler`) and the
     `MobileWheelPopup` tap-on-row / center-accept fire sites.
- Interaction with Part A: Part A suppresses the injected *replay* before it reaches any fire
  site, so the gate never sees it — the two mechanisms are orthogonal (replayed-same-gesture vs.
  two-distinct-gestures) and do not conflict.

Note: `Button.applyPress` already provides desktop single-fire via `held`; the gate adds the
*time* dimension (two distinct fast taps) and unifies mobile direct-fire paths under the same
primitive.

### Part C — Uniform outside-click consumption (fixes #3)

Make one predicate the single source of truth for "an interactive overlay owns input right
now," and gate the out-of-tree handlers on it.

- Add `Passive bool` to `PortalEntry` (`portal.go`); set it `true` for tooltip opens
  (`chain-tt`, `levels-agg-tt`, `tooltip_overlay.go`).
- Add `OverlayPortal.OwnsInput()` = any open entry that is **not** `Passive`; expose
  `DrumView.PortalOwnsInput()` (overlay tree).
- Change `modalOverlayActive()` (`game_input_drag.go:88`) to return
  `g.drum.PortalOwnsInput()` instead of `PortalHasBlocking()`. This makes the out-of-tree
  handlers (`handleTapInGrid`, two-finger pan, pinch, long-press, camera `panOK`) stand down
  for **any** interactive overlay — including the non-blocking FX panel and EQ dropdown —
  while still ignoring passive tooltips.

The tree-level click-outside (`drumview_tree.go` branches A/B) already consumes for any open
portal regardless of Modal/Scrim, so no change is needed there. Combined effect: an outside
tap on any interactive overlay closes it (top entry) and is fully consumed at both the tree
and out-of-tree layers — never reaching the surface below.

**Accepted behavior change:** the docked FX panel is no longer "interact-around." An outside
tap closes it and is consumed, so editing the grid while it is open now takes an explicit
dismiss first. This is the intended uniformity (per design discussion).

## Round 2 — the real root cause (injectTouchTap double-processing)

Part A (SetInputForTest-tested) did not fix the live bug: it never exercised
`game_update.go`'s real `GestureTap → injectTouchTap → reset wasPressed` path. The
`injectTouchTap` replay is meant ONLY to deliver a tap the mobile row-scroll dead zone
blocked from live dispatch. Outside that dead zone (which excludes `MobileEQMode`, i.e.
every non-Pads mobile view — where all popups live) the single-touch override already
published the hold-press, so the touch-DOWN dispatched live AND the injected replay
re-fired the same gesture:

- Tap a synth card → DOWN opens its popup; replay re-fires on it.
- Tap OUTSIDE an open popup → DOWN closes it (overlay click-outside); replay lands on the
  now-exposed element beneath and OPENS it ("I can still open other elements").

**Fix** (`game_update.go`): gate the injection on
`(dv.tapInScrollDeadZone(gesture.X, gesture.Y) || !suppressClicksUntilRelease)`. New helper
`tapInScrollDeadZone = IsMobile() && !MobileEQMode() && pt(x,y, rowsRect())`. The dead zone
ALSO sets `suppressClicksUntilRelease` (it defers the click to distinguish tap from scroll)
without consuming the down, so a plain `!suppress` gate would kill legitimate dead-zone
injection (kebab, row buttons) — hence the `tapInScrollDeadZone ||` clause. Net: dead-zone
taps always inject; elsewhere inject only when the down wasn't consumed, so a
popup-opening/dismissing tap never gets a redundant replay. Part A's tree guard is kept as
harmless defense-in-depth. Tests: `popup_dismiss_leak_test.go` (real touch pipeline;
`TestTapOutsidePopupDoesNotOpenElementBeneath` RED→GREEN + `TestSynthCardTapViaRealPipelineStaysOpen`);
the dead-zone debounce test forces mobile via `forceSmallScreenForTest` (note:
`SetForceMobileProfile` is a no-op stub under `-tags test`).

## Non-goals

- No change to how popups are *positioned* (below-anchor placement stays).
- No change to the anchor tap-to-close *feature* (a deliberate second tap on the opener still
  closes and persists).
- No change to desktop mouse behavior beyond adding the shared debounce time-dimension to
  `applyPress` (desktop already single-fires).
- `PortalHasBlocking()` and RootTree tier-2 exclusive routing are left as-is; Part C only
  broadens the out-of-tree gate.
- Tooltips remain passive (never own input, never consume clicks).

## Testing (TDD, functional — drive real `Update()`)

Per repo convention, drive the real `DrumView.Update()` / `Game.Update()` loop rather than
poking internals ([[feedback_functional_tests_over_isolation]]).

- **Part A:** reproduce RED first. Open the mobile synth wheel popup by a clean simulated tap
  on a ParamCard (reuse `mobileSynthWheelSceneSetup` from `synth_wheel_popup_isolation_test.go`),
  advance through the touch-down → held → lift+`injectTouchTap` sequence, assert the popup is
  **still open** after the tap settles. Add: a second, separate tap on the opener **closes** it
  (toggle preserved); a tap-and-drag-into-body keeps it open. Guard the no-regression case
  (`TestKebabTapOverlapScrollerActive` stays green).
- **Part B:** two `injectTouchTap`s at the same coords within the window fire the underlying
  action **once**; two taps >window apart fire twice; two taps at different coords both fire;
  an auto-repeat button still repeats.
- **Part C:** with the FX panel (non-blocking) open, an outside tap over the grid closes the
  panel and does **not** create/select a node or move the camera; same for the EQ channel
  dropdown. A visible tooltip does **not** block a grid tap.

## Verification commands

```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod \
  -run 'SynthWheel|TapDebounce|PopupOutsideClick|Kebab|FXPanel' ./internal/ui/
# Full ui suite is ~600s; run in -run splits under `timeout 900`.
GOOS=js GOARCH=wasm ../../.tools/go/bin/go build ./...   # from src/go: confirm wasm still builds
```

## Risks / gotchas (from prior sessions in this repo)

- **Git is READ-ONLY for any subagent.** Never `git stash`/`git add`/`git commit` — a prior
  subagent `git stash push -u` deleted 2,614 tracked files. Leave everything uncommitted for
  the user to review ([[feedback_never_stage_or_commit]]).
- User WIP files are **not** gofmt-1.26-clean — never run package-wide `gofmt -w`; format only
  files this change touches.
- Full `internal/ui` suite ~600s and trips the 900s timeout only under `./...` CPU contention;
  run targeted `-run` splits.
- This branch has pre-existing unrelated RED tests (density/typography WIP, node-logic volume);
  prove causation before attributing any failure to this change ([[feedback_no_preexisting_failure_baselines]]).
