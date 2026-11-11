# Wheel Popup Enter / Esc (Accept / Cancel) — Design

**Date:** 2026-07-03
**Status:** Approved (design), implementing TDD

## Summary

Make the shared `MobileWheelPopup` (used by the synth, sampler, and EQ tabs) a
*transactional* value editor:

- **Enter** → **Accept**: persist the edit (fire `OnCommit` once → one undo step), close.
- **Esc** → **Cancel**: revert the value to the snapshot taken at open, re-apply via
  `OnChange`, close, **without** committing.
- **Every other dismissal** (tap-outside, re-tap the opener icon, navigate away /
  `CloseAllPopups`, center-box → numeric editor) → **persist** (Accept). Only Esc discards.

Because all three wheels share `MobileWheelPopup` + `WheelBinding`, they get identical
behavior by construction. Tests assert this parity.

## Current behavior (what changes)

- The wheel applies **live per detent** (`OnChange`) and **commits undo per gesture/detent**
  (`OnCommit` on pointer release and per wheel-notch in `HandleWheel`).
- **Esc** closes the top portal (`handleEscape` → `CloseTop`), keeping the (already-committed)
  changes. There is **no Enter handling** for the popup.

## Design

### Transactional commit model (`mobile_wheel_popup.go`)

- New fields: `initialValue float64`, `dirty bool`.
- `Open` records `initialValue = binding.Knob.Value` (or 0 if no knob), `dirty = false`.
- Remove the `OnCommit` calls from `HandleInput` (release) and `HandleWheel` (per step).
  Keep `OnChange` (live preview). Set `dirty = true` wherever a **value** change fires
  `OnChange` (drag detent + wheel step). Resolution-strip changes are NOT value changes
  (they persist immediately via `OnResolution`) and are not part of the transaction.
- `Accept()`: `if !open { return }`; if `dirty && OnCommit != nil` → `OnCommit()`; `Close()`.
- `Cancel()`: `if !open { return }`; if `dirty && Knob != nil` → restore
  `Knob.Value = initialValue` and, if `OnChange != nil`, `OnChange()` (re-apply original);
  do NOT commit; `Close()`.
- Center-box branch (`HandleInput`, tap on `centerH`): `Accept()` then `OpenEditor()` — the
  wheel persists and closes, then the numeric editor opens. This removes the nested
  wheel+editor Esc ambiguity and matches "navigate-away = persist".

### Key routing

- **Esc** — `mobileWheelPopupPortalOverlay` implements the existing `portalEscapeHandler`:
  `HandleEscape() bool { o.popup.Cancel(); return false }`. Returning `false` lets
  `handleEscape` run `CloseTop` (removes the entry + fires `OnClose` → `popup.Close()`,
  idempotent). Net: value reverted, no undo step.
- **Enter** — new `portalEnterHandler interface { HandleEnter() bool }`. New
  `(g *Game).handleEnter()` (called in `handleGlobalShortcuts` right after `handleEscape`):
  on `isKeyJustPressed(KeyEnter)`, if the top portal overlay implements `portalEnterHandler`,
  delegate. `mobileWheelPopupPortalOverlay.HandleEnter() bool { o.popup.Accept(); return true }`.
  Accept sets `open=false`; `CleanupClosed` removes the entry next frame (same pattern as the
  anchor-close handler).

### Persist-on-dismiss wiring

- Anchor re-tap (`mobileWheelAnchorCloseHandler.OnPress`): `popup.Accept()` (was `Close()`).
- Programmatic close (`closeEQKnobWheelPopup`, `closeSynthKnobWheelPopup`,
  `closeSamplerKnobWheelPopup`): `popup.Accept()` (was `Close()`).
- **The real chokepoint is the portal `OnClose` callback.** Tap-outside (the tree's
  click-outside → `portal.CloseTop()`) and `CloseAllPopups`'s drain loop both dismiss via
  `CloseTop → OnClose`, NOT via the `close*KnobWheelPopup` helpers. So the three wheel
  `OnClose` callbacks (`drumview_portal_open.go`) call `popup.Accept()` (was `Close()`).
  This is safe for every path: Esc's `Cancel` sets `open=false` before `CloseTop` runs, so
  `OnClose→Accept` no-ops (revert preserved); Enter/anchor/center-box `Accept` first, so the
  later `OnClose→Accept` no-ops (no double-commit); tap-outside and drain commit exactly once.
  (Found in review — the earlier `close*` helper change alone missed these two dismissals.)

## Testing (TDD — failing first)

1. **Unit (synthetic binding)** — `mobile_wheel_popup_keys_test.go`:
   - Enter/Accept persists: a value change fires `OnChange` but NOT `OnCommit` mid-gesture
     (deferred); `Accept()` fires `OnCommit` exactly once and closes; value retained.
   - Esc/Cancel reverts: after a value change, `Cancel()` restores `Knob.Value` to the
     snapshot, re-applies via `OnChange`, does NOT commit, and closes.
   - Not-dirty Accept/Cancel are no-ops (no commit, no spurious OnChange).
2. **Routing** — the overlay implements `portalEscapeHandler` + `portalEnterHandler` and
   delegates to Cancel/Accept; Game-level `handleEnter`/`handleEscape` on an open EQ wheel
   persist/revert the source of truth (`bandGainsDB`).
3. **Parity** — table over the three real wheels (EQ via `setupEQTabForTest`; synth via
   `mobileSynthWheelSceneSetup`; sampler via `mobileSamplerTabSetup`): open, change the knob,
   Esc reverts `Knob.Value` to the snapshot; Enter keeps it and closes. Same assertions,
   same outcomes for all three — proving they accept input identically.

## Out of scope / YAGNI

- No change to the resolution-strip persistence (immediate, separate preference).
- No change to the numeric editor's own Enter/Esc (it already commits/cancels).
- NumpadEnter not wired (stub key set is Enter/Escape/Backspace); physical Enter only.
