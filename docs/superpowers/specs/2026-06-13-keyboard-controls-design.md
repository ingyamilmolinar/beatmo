# Keyboard Controls — Design Spec

**Date:** 2026-06-13
**Branch:** add-core-node-types
**Status:** Approved (pending spec review)

## Goal

Add keyboard control for common Beatmo actions — **Space = play/pause** as the headline — plus a coherent set of game-style shortcuts, a discoverable help overlay, and a **universal Esc** that cancels any pending interaction. All additions are **purely additive**: existing mouse, wheel, and touch paths are untouched.

## Non-goals

- No new playback/audio code paths — keyboard play/pause/stop route through the *same* transport callbacks the on-screen buttons already use.
- No gesture-model rewrite, no node-drag-via-keyboard, no remappable key bindings (YAGNI for v1).
- No mobile/touch keyboard affordance beyond the desktop help chip (touch-only devices have no physical keyboard; key handlers simply never fire there).

## Key map (Scheme A)

| Key | Action | Trigger model |
|---|---|---|
| **Space** | Play / Pause toggle | edge (just-pressed) |
| **Esc** | Universal cancel → Stop (see § Universal Esc) | edge, precedence ladder |
| **← ↑ → ↓** | Pan the grid | continuous (held = keeps panning) |
| **[** / **]** | Zoom out / in (anchored at grid-pane center) | continuous with key-repeat |
| **0** | Reset zoom to default (scale 2.0, recenter) | edge |
| **−** / **+** | BPM −1 / +1 — **Shift** = ±5 | edge with key-repeat |
| **1 – 7** | Audio tab: Wave, Spectrum, Levels, EQ, Chain, Synth, Sampler | edge |
| **?** (Shift+/, also plain **/**) | Toggle keyboard-shortcuts overlay | edge |

`1–7` map to the **enum definition order** (`TabWave=0 … TabSampler=6` in `panel_tab.go`), not the display order.

## Architecture

### One new handler, mirroring `handleUndoRedoKeys`

A new `(*Game) handleGlobalShortcuts()` lives in a new file `game_input_shortcuts.go`. It is called from `game_update.go` immediately after `g.handleUndoRedoKeys()` (≈ line 263) — **before `seqMu` is acquired** (the lock is taken later, inside `g.drum.Update()` ≈ line 391/399). Like undo/redo, this means handlers run unconditionally (work over popups, during drags) but touch **only UI-layer state**: camera, transport pulse, audio-tab state, overlay portal. They must never mutate the graph/predictor (that requires `seqMu`).

### Action wiring — single source of truth per action

- **Play/Pause & Stop** → invoke the *same* path the transport buttons use (the `OnPlayToggle` / `OnStop` callbacks, i.e. the one-frame `playPressed` / `stopPressed` pulse consumed in `Update`). No parallel playback logic.
- **Pan** → mutate `g.cam.OffsetX/OffsetY` by a fixed per-frame pixel delta, then `g.cam.Snap()` — the exact pattern `Camera.HandleMouse` uses for drag-pan. Continuous (`isKeyPressed`) so a held arrow keeps panning. A modest per-frame delta (e.g. `panStep = 24px` tunable) feels smooth at 60fps.
- **Zoom** → call the existing `g.zoomAtScreen(centerX, centerY, ±steps)` anchored at the grid-pane center; bounds already clamp to `[0.1, 10]`. **`0`** resets `cam.Scale = 2.0` + zeroes offsets + `Snap()` (new tiny `resetCamera()` helper, since none exists today).
- **BPM** → `dv.SetBPM(dv.BPM() ± step)` — the same setter the BPM box and spinner use. (Verify `SetBPM` does not acquire `seqMu`; the BPM box already calls it from the UI layer, so this is expected safe. If it ever needs the lock, defer via a pending flag consumed after `seqMu.Unlock()`.)
- **Tabs** → `SetActiveTab(tab)` **plus** `EnsureAnalyzersForTab(tab, activeChannel, instrumentIDs…)` — the mandatory dispatcher chokepoint (CLAUDE.md: "Audio-panel tab switching MUST go through the dispatcher"). Never bypassed.

### Trigger models

- **Edge** (`isKeyJustPressed`): play/pause, stop-via-Esc, tab digits, help toggle, zoom-reset.
- **Edge + key-repeat**: BPM nudge and `[`/`]` zoom — hold to repeat at the standard repeat cadence (reuse the repeat pattern already in `textinput.go`, or a small per-key frame counter).
- **Continuous** (`isKeyPressed`): arrow-key pan.

## Text-input gate (critical)

Action keys must NOT fire while the user is typing (BPM box, instrument-name field, EQ dB inputs) — otherwise Space would refuse to type a space. `handleGlobalShortcuts` runs the Esc authority *first* (so Esc still reverts/blurs a focused field), then short-circuits the remaining action keys when a field owns the keyboard:

```go
if g.drum != nil && g.drum.tree != nil && g.drum.tree.FocusedZone() != "" {
    return // a text field owns the keyboard this frame
}
```

`focusedZone` is `""` unless a `TextInput` currently has focus — it is the existing single global predicate (set/cleared in the tree's Layout phase). A small `FocusedZone()` accessor is added if not already public.

> Esc is the one exception that must still work while a field is focused — but that case is handled *inside* the universal-Esc ladder (rung: "blur focused text input"), which runs ahead of the text-input gate. See below.

## Universal Esc

**Requirement (user):** Esc must *universally* close menus and cancel pending/in-progress user actions, falling through to Stop.

Today Esc behavior is **already implemented but scattered** across contexts (portal `CloseTop`, instrument menu, rename dialog, WAV-naming dialog, node sidebar, connect mode, move mode, BPM/EQ text revert). The problems: (a) no single precedence guarantee, (b) no final "nothing open → Stop" fallback, (c) risk that a future Esc binding both closes a menu *and* stops.

**Design: one Esc authority.** Introduce `(*Game) handleEscape() bool` as the *single* decision point for Esc, called at the top of `handleGlobalShortcuts` (before the text-input gate, edge-triggered on Esc just-pressed). It walks a precedence ladder, performs **exactly one** cancel, sets a frame-scoped `g.escConsumed` flag, and returns. The scattered Esc checks are **consolidated into / gated by** this authority so Esc never double-fires (existing per-context handlers either move into the ladder or early-return when `escConsumed` is set this frame).

**Precedence ladder** (first match wins, then return):

1. **Shortcuts help overlay open** → close it.
2. **Any portal overlay open** (`tree.Portal().IsOpen()`) → `CloseTop()`. Covers instrument menu, rename/WAV-naming dialogs, dropdowns, tooltips, modals.
3. **Node sidebar open** (`g.sidebar.IsOpen()`) → `g.sidebar.Close()`.
4. **Focused text input** (`tree.FocusedZone() != ""`) → revert + blur (delegate to the zone's existing Esc revert, e.g. BPM box restores prior value, then `SetFocus("")`).
5. **Connect mode** (`g.connectMode`) → `g.cancelConnectMode()`.
6. **Move mode** (`g.moveMode`) → `g.cancelMoveMode()`.
7. **Link drag active** (`g.linkDrag.active`) → `g.linkDrag = dragLink{}`.
8. **In-flight tree capture** (`tree.Capturing()`) → `tree.ClearCapture()`.
9. **Active touch gesture** (`globalTouchState` pinch/pan in flight) → `globalTouchState.Reset()`.
10. **Pending editor click state** (`pendingClick` / `camDragged` / `pendingStartRow`) → clear via a new `g.clearPendingInteraction()` helper.
11. **Fallback: nothing pending** → **Stop playback** (transport `OnStop` path).

Rungs 5–7 and 10 currently live in `handleEditor` (under the cursor-in-grid gate, inside `seqMu` scope). Because all their cancel seams only mutate plain UI fields (`cancelConnectMode`/`cancelMoveMode`/zeroing structs), they are safe to evaluate early, before `seqMu`. The plan migrates them into `handleEscape` and removes the duplicate Esc checks from `handleEditor`, leaving one authority.

This makes the behavior **uniform**: wherever you are, Esc peels off the single most-pending thing; press it again to peel the next; with nothing pending it stops the sequencer. Pressing Esc with a focused field still reverts/blurs the field (rung 4) before the text-input gate can swallow it.

## Help overlay (`?`)

A new `ShortcutsOverlay` implementing `PortalOverlay` (modeled on `TooltipOverlay`), shown via `tree.Portal().Open(PortalEntry{ID: "shortcuts", Scrim: true, Modal: false})`. It renders the key-map table using the existing `DrawTextStyled` / `TextRole` API and design tokens (no bespoke chrome — per the shared-widget-reuse and token-discipline rules). Dismissed by `?` again, by Esc (ladder rung 1), or by click-outside (`ShouldClose()`).

## Discoverability + platform

- A small **keyboard "?" chip** in the transport bar toggles the same overlay, giving mouse users an entry point. Shown **desktop-only** (`Profile().IsMobile() == false`) — shortcuts are meaningless on touch-only devices.
- Ebiten delivers keys uniformly on desktop and WASM-in-a-browser, so this works on beatmo.io with a physical keyboard for free. On touch-only mobile no key events fire — fully additive, no behavior change.

## Testing (functional, drive the real loop)

Per the repo's "functional over component-isolation" rule, all tests drive the **real `g.Update()`** loop with stubbed keys via the existing `stubKeys()` helper (`undo_keyboard_test.go`). New file `game_input_shortcuts_test.go`:

- **Transport**: Space toggles `g.Playing()` (and back); precedence-clean Esc stops.
- **Pan**: each arrow changes `g.cam.OffsetX/OffsetY` in the right direction; held arrow accumulates.
- **Zoom**: `[` / `]` change `g.cam.Scale` and stay within `[0.1, 10]`; `0` restores `Scale==2.0` and zero offsets.
- **BPM**: `+` / `−` change `dv.BPM()` by 1; Shift makes it ±5; clamps respected.
- **Tabs**: `1`–`7` set the expected `PanelTab` and the dispatcher enabled the right analyzer (assert via the active-tab + analyzer-enabled seam already used in `audio_panel_dispatcher_test.go`).
- **Help**: `?` opens the `shortcuts` portal; `?` again closes it.
- **Text-input gate**: focus the BPM box → Space does NOT toggle play and IS typed/handled by the box (proves the gate).
- **Universal Esc ladder** (one test per rung, top-down): with each pending state set up, Esc cancels exactly that state and does NOT stop playback; with nothing pending, Esc stops. Assert single-action (no double-fire) by checking lower rungs are untouched.

A discipline test asserts `handleGlobalShortcuts` is the **only** new site polling these action keys (mirror the spirit of the single-dispatch-site discipline tests), and that no new `runtime.GOOS == "js"` branch was introduced.

## Files touched (estimate)

- **New**: `game_input_shortcuts.go` (handlers), `shortcuts_overlay.go` (help overlay), `game_input_shortcuts_test.go` (tests).
- **Edited**: `game_update.go` (one call site), `game_input_editor.go` (remove migrated Esc checks), `camera.go` (add `resetCamera`/reset helper if not folded into Game), `drumview_tree.go` (expose `FocusedZone()` accessor if needed), `transport_zone.go` (the desktop "?" chip), possibly `panel_tab.go` (a `PanelTabFromIndex` convenience).

## Risks / watch-items

- **Esc consolidation regressions**: moving connect/move/sidebar Esc into one ladder is the riskiest part. Mitigate with one test per rung asserting both "cancels the right thing" and "leaves lower rungs alone", plus keeping the existing per-zone *revert text* logic intact (delegated, not duplicated).
- **`+`/`=` key**: on most layouts `+` is Shift+`=`. Bind BPM-up to both `ebiten.KeyEqual` (with/without shift semantics handled) and the numpad `+` to be forgiving; document in the overlay as `+`.
- **Repeat cadence**: reuse the existing text-input repeat constants so BPM/zoom auto-repeat feels consistent with the rest of the app.
