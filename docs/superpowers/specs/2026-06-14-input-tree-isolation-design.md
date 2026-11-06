# Input Tree Isolation — Design Spec (Umbrella)

**Date:** 2026-06-14
**Branch:** add-core-node-types
**Status:** Approved design; phases to be planned individually.

## Problem

Keyboard input in the Beatmo UI is handled **globally**, outside the component
trees. `Game.Update` calls `handleUndoRedoKeys`, `handleGlobalShortcuts`, and
`handleEscape` unconditionally every frame. `handleGlobalShortcuts` runs grid
actions (arrow-pan, zoom, Space, BPM±, tab-select, `?`) and then tries to *back
off* via an ad-hoc denylist:

```go
// game_input_shortcuts.go:123-127
if (g.drum.tree != nil && g.drum.tree.focusedZone != "") ||
    (g.drum.audioTree != nil && g.drum.audioTree.focusedZone != "") ||
    g.drum.valueEditorActive() {
    return
}
```

That denylist is the root cause of the reported bug: it must enumerate every
text-input source, and it misses several. When the **Save As** dialog or a
portal-hosted text input (rename / instrument-search / WAV-naming) is focused,
Left/Right arrows move the caret **and** pan the grid camera simultaneously.

Reproduced by three failing tests (Phase 0, already in tree):
`arrow_key_text_input_isolation_test.go` —
`TestArrowKeysYieldToSaveAsDialog`, `...Right`, `TestArrowKeysYieldToRenameTextInput`.

Pointer/wheel/touch isolation between the grid pane and the bottom panel already
works (RootTree per-tree `HitIndex`, `blocksAt`/`panOK` gating). The leak is
keyboard-only — but the deeper problem is that **input is handled in two places:
inside the trees (bottom-panel pointer) and globally in `Game.Update`
(keyboard + grid pointer via the legacy dispatcher).**

## Goal

A single input architecture where **all** input — pointer, wheel, touch, and
keyboard — flows through the component trees, arbitrated by z-axis priority, with
**no global input handling left in `Game.Update`**. Each component explicitly
receives input and claims it; nothing reads raw input outside a tree.

This is the user's mandate, verbatim intent:
> "Avoid all global user input handling. Make the code explicitly receive inputs
> and claim them according to the priority in the z-axis within their own tree
> structure (and nothing should exist outside the various component tree
> structures)."

## Current architecture (audit summary)

- **Bottom pane:** `dv.rootTree` (`RootTree`, `drumview_ctor.go:210-212`) owns
  `dv.tree` (`DrumViewTree`, z=0) and `dv.audioTree` (z=1), each with its own
  `HitIndex`, `OverlayPortal`, capture state, and `focusedZone`. Dispatched via
  `drum.Update` → `rootTree.Update` → per-tree `Tick`/`DispatchInput`. Pointer
  isolation, capture, blocking-portal, and `DismissOnForeignPress` already work.
- **Top pane:** `g.gridTree` (`GridTree`, `game_new.go:313`) is **draw-only** —
  a z-ordered Layer stack, no `HitIndex`, no input, no keyboard focus. Grid
  pointer/camera/gesture input runs through the **legacy path**: the flat
  `inputDispatcher` (splitter + sidebar + drumview as `InputHandler`s),
  `handleTapInGrid`, `handleEditor`, `cam.HandleMouse`, `updateLongPressPopup`.
- **Keyboard:** entirely global. `handleUndoRedoKeys`, `handleGlobalShortcuts`,
  `handleEscape` in `Game.Update`. The only focus model is per-tree
  `focusedZone` + `autoFocusPass` (re-evaluated every Tick), local to the bottom
  pane. Modal text inputs (rename, instrument-search, WAV-naming, Save As) read
  `inputChars()`/`isKeyPressed` directly and are **not** tree-focused zones.
- **Two roots, not one:** `g.gridTree` (Game) and `dv.rootTree` (DrumView) are
  independent. There is no single coordinator over the whole window.

## Target architecture

### 1. One root coordinator

Introduce a single top-level coordinator (promote/relocate so one tree owns the
whole window). It owns, in ascending z:

1. `GridTree` — top pane (nodes/edges/camera, sidebar, long-press popup,
   move-confirm, help button)
2. Splitter (divider)
3. `DrumViewTree` — rows, timeline, transport
4. `AudioTree` — audio-analysis panel

`Game.Update`'s input section collapses to:

```
frame := readInputFrame()          // the ONLY place raw input funcs are read
root.Dispatch(frame)               // pointer + wheel + touch + keyboard
root.DrainPayloads()               // existing post-dispatch work
```

No `isKeyPressed` / `cursorPosition` / `inputChars` / gesture reads outside tree
code.

### 2. Keyboard contracts

Keyboard is not spatial, so it needs explicit roles (two small interfaces):

```go
// A focused text/caret input. While focus is held it owns the keyboard
// exclusively (typed chars + caret/edit keys). Implemented by every text input.
type KeyboardReceiver interface {
    HandleKey(k ebiten.Key)
    HandleChars(chars []rune)
}

// A shortcut consumer, dispatched ONLY when no receiver is focused, in
// z-priority order, first-claim-wins.
type KeyboardShortcutHandler interface {
    HandleShortcut(k ebiten.Key, edge bool) bool // true = claimed
}
```

**Single focus owner.** The root coordinator holds exactly one
`focusOwner` (a path to the child tree + its focused zone/overlay) across the
whole window. Claiming focus anywhere clears it everywhere else. `OwnsKeyboard()`
on a tree is true when it (or a descendant) holds the focus owner, or hosts an
active value editor.

**Dispatch order per frame (keyboard):**
1. If a `focusOwner` exists → deliver `HandleKey`/`HandleChars` to it. Done
   (except Escape, see §3). No shortcut handler runs — this is the bug fix, by
   construction: arrows reach the caret, never the grid.
2. Else → walk `KeyboardShortcutHandler`s top-down by z; first claim wins.
   - `GridTree`: arrows (pan), `[`/`]`/`0` (zoom/reset).
   - transport (in `DrumViewTree`): Space (play/pause), `+`/`-` (BPM±).
   - audio panel / root: `1`-`7` (tab select), `?` (help).
   - root node: undo/redo; final Esc→Stop fallback.

### 3. Escape decomposition

The monolithic `handleEscape` ladder is replaced by per-component
`HandleEscape() bool`, dispatched top-down by priority by the coordinator:
portal overlay → focused receiver → sidebar → connect/move mode → link drag →
tree capture → touch gesture → pending click → **root fallback: Stop**. Each
component owns its own Esc; the root provides only the final fallback. Esc stays
a single authority (no double-fire) but is now distributed, not global.

### 4. Modal text-input migration

Each modal text input stops reading the keyboard globally and becomes a
`KeyboardReceiver` routed by its owning tree, so `OwnsKeyboard()` is accurate
while it is open:

- **Rename** overlay, **instrument-search** (`searchBox`/`jumpInput`),
  **WAV-naming** (`nameBox`) → drum-tree portal overlays; the tree routes keys to
  the top portal overlay when it implements `KeyboardReceiver`.
- **Save As** dialog (`dv.saveAsDialog`) → claimed by the audio tree while open.
- **Shared value editor** (transport BPM / EQ dB / synth knob) → keeps its own
  commit/caret `Update`, but its tree reports ownership while active (transport
  and EQ already auto-focus; synth `dv.paramEditor` gains a claim hook).

This migration is split across phases: **Phase 1** makes each surface *declare*
keyboard ownership (the `KeyboardClaimant` interface) so the grid yields — the
surfaces keep their current self-driven `TextInput.Update()` for key delivery.
**Phase 5** completes the move: `TextInput` and the modal updaters stop reading
raw input directly and the tree *delivers* keys to the focus owner, satisfying
"nothing outside the trees."

### 5. Parity seam (risk management)

Grid pointer/camera/gesture migration (the previously-deferred "Phase 3") is the
highest-risk piece (gesture-model rewrite; pan-from-node / double-fire hazards).
It ships **behind a test-overridable seam**: the new `GridTree` input path runs
and is proven equal to the legacy path surface-by-surface under tests, then the
seam is flipped and the legacy path deleted. No long-lived broken state.

### 6. Discipline lock

A guard test asserts that `Game.Update` and non-tree source files never call the
raw input functions (`isKeyPressed`, `isKeyJustPressed`, `cursorPosition`,
`isMouseButtonPressed`, `inputChars`, wheel/touch reads) — making "nothing
outside the trees" a permanent enforced invariant, like the existing
`zone_input_isolation_discipline_test.go` and grid-draw-dispatch guards.

## Non-changes

- **Undo/Redo** keeps its current behavior (works regardless of focused text
  input); it simply moves from a global handler into the root coordinator's
  shortcut node.
- **Soft-keyboard proxy** (WASM `SignalSoftKeyboardArrow*` / `...Escape`) keeps
  working — its forwarded keys flow into the focused `KeyboardReceiver`.
- **Pointer isolation already working** (RootTree per-tree HitIndex, capture,
  blocking portal, DismissOnForeignPress) is preserved and extended, not
  rewritten.

## Phase decomposition

Each phase is independently shippable with its own implementation plan, tests,
and review gate. Phase 1 alone fixes the reported bug.

| Phase | Scope | Outcome |
|---|---|---|
| **0 (done)** | RED tests for the arrow-key leak (`arrow_key_text_input_isolation_test.go`) | 3 failing tests in tree |
| **1** | Keyboard-**ownership contract**: a `KeyboardClaimant` interface that rename / instrument-search / WAV-naming / Save As / value editors implement; trees expose `OwnsKeyboard()`; one `DrumView.KeyboardClaimed()` predicate replaces the denylist gate so the grid yields whenever any surface claims the keyboard | **Phase-0 tests go green**; every bottom-panel text input declares its keyboard ownership; no denylist |
| **2** | Decompose `handleEscape` into per-component `HandleEscape()` dispatched by the trees; root keeps only the Stop fallback | Esc fully tree-routed; monolithic authority removed |
| **3 (done)** | Move app-global shortcut keys (undo/redo, Space, BPM±, tabs, `?`, pan/zoom) into per-component `KeyboardShortcutHandler` nodes dispatched by a Game-owned `keyboardShortcutRouter` (`keyboard_shortcuts.go`); `handleGlobalShortcuts` + `handleUndoRedoKeys` reduced to thin Update→router seams (the inline grab-bag deleted into nodes). **Resolved open-questions:** (D1) coordinator is keyboard-only & Game-owned — pointer unification deferred to Phase 4; (D2) two tiers — an **ungated** undo node (works while a text field is focused) + a **gated** tier dispatched only when `!KeyboardClaimed()`; (D3) router runs **all** gated nodes over a **disjoint+complete** key set (`TestShortcutKeysAreDisjointAndComplete`), preserving simultaneous-multi-key behavior — first-claim-wins is the moot-but-available contract hook; (D4) entry-point names retained so the ~40 existing behavior tests stay valid. | No inline keyboard grab-bag left; each surface owns its shortcut keys; behavior unchanged. Discipline guard: `TestGlobalShortcutsHasNoInlineKeyHandling`. |
| **4** | `GridTree` pointer/camera/gesture input behind the parity seam: grid `HitIndex` (canvas, sidebar, long-press, move-confirm, help), camera pan/zoom, gestures; splitter + sidebar folded into the coordinator | Grid input tree-routed, proven equal to legacy under tests |
| **5** | Flip the seam; delete `inputDispatcher` / `handleEditor` / `handleTapInGrid` / `cam.HandleMouse` direct calls / gesture globals; **migrate `TextInput` + the modal updaters off direct raw-input reads so the tree *delivers* keys to the focus owner** (completes §2 `KeyboardReceiver` key-delivery); add the §6 discipline guard | `Game.Update` has zero global input; `TextInput` no longer reads globals; invariant locked |

## Testing strategy

- **TDD throughout.** Phase 0 already established the RED tests.
- **Per-surface keyboard tests:** "arrows don't pan / zoom while X is open" for
  every text input (Save As, rename, instrument-search, WAV-naming, BPM, EQ dB,
  synth knob), plus "typed chars land", "Enter commits", "Esc cancels".
- **Functional tests** drive the real `Game.Update` loop via the `fakeInput`
  harness (`text_input_escape_functional_test.go` pattern) — assert user-visible
  outcomes, not isolated component state (per the project's
  functional-tests-over-isolation guidance).
- **Contract tests:** `OwnsKeyboard()` / `KeyboardClaimed()` true for each
  surface; first-claim-wins shortcut arbitration; single-focus-owner invariant.
- **Parity tests** (Phase 4): new GridTree input path vs legacy path produce
  identical node-create/select/drag/pan/zoom/long-press outcomes for the same
  input sequences.
- **Discipline test** (Phase 5): no raw input reads outside tree code.

## Open implementation questions (resolved in per-phase plans)

- Exact location of the unified root coordinator (promote `dv.rootTree` to Game
  level vs. a new `Game`-owned top tree wrapping `g.gridTree` + `dv.rootTree`).
  **Resolved (Phase 3, for keyboard only):** a lightweight Game-owned
  `keyboardShortcutRouter` handles keyboard shortcuts; the *pointer*-tree
  unification question is left to Phase 4 (and is the high-cost, deliberately-
  deferred piece — see `CLAUDE.md` GridTree note).
- Whether `KeyboardShortcutHandler` dispatch reuses the existing `commonKeys`
  edge/level machinery in `DrumViewTree.handleInput` or a new shared helper.
- Whether portal overlays route keyboard via the existing `focusedZone` map or a
  dedicated "top overlay is a receiver" branch.

These do not affect the architecture or the phase boundaries; they are decided
when each phase is planned.
