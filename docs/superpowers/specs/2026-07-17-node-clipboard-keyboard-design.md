# Node Clipboard & Undo/Redo Keyboard Shortcuts — Design

**Date:** 2026-07-17
**Status:** Approved (pending spec review)
**Platforms:** Desktop (Ebiten/Go) + Browser (WASM)

## Summary

Add keyboard-driven copy / cut / paste for a single selected grid node, plus
global undo/redo key bindings:

| Chord | Action |
|---|---|
| `Ctrl/Cmd-C` | Copy the selected node (type + params) to an in-memory clipboard |
| `Ctrl/Cmd-X` | Cut the selected node (copy, then delete) |
| `Ctrl/Cmd-V` | Paste at the cursor-guide cell (empty → immediate; occupied → confirm modal) |
| `Ctrl/Cmd-Z` | Undo (**already wired**; keep, add regression test) |
| `Ctrl/Cmd-R` | Redo (**new** binding; keeps existing `Ctrl-Y` / `Shift-Ctrl-Z`) |

All mutations are undoable and logged through the existing `hooks.Bus` →
`eventlogger` pipeline. TDD throughout.

### Decisions (locked)

- **Scope:** single node only. No multi-node/group clipboard. (`g.sel`)
- **Clipboard:** internal in-memory Go struct on `Game`. No system clipboard, no
  cross-session persistence.
- **Paste connections:** params-only — the pasted node carries type + all params
  but **no edges** (user wires it manually). No auto-stitch.
- **Ctrl-R in browser:** ~~add a JS `keydown` `preventDefault` for `Ctrl/Cmd-R`~~
  **(Superseded during implementation — see addendum below.)** Empirically, Ebiten's
  own canvas keydown handler already `preventDefault`s every keydown when the canvas
  is focused (exactly when Ctrl-R can reach Go and redo), so a custom JS handler is
  redundant. The handler was **removed**; `F5` / the browser reload button remain
  available, and Ctrl-R redo works via the Go binding + Ebiten's existing behavior.

> **Addendum (implementation outcome):** The JS `index.html` Ctrl-R handler in the
> original design proved redundant — verified by removing it and re-running the
> browser test (still passes). Delivered feature ships **no `index.html` change**;
> `src/js/ctrl_r_redo.browser.test.js` is kept as an end-to-end guard (Ctrl-R redoes,
> no reload) and documents that Ebiten provides the reload-suppression. Also: paste
> uses a focused `pasteNodeFrom` rather than the spec's shared `insertNode` (the
> origin/start-entanglement in `tryAddNode` made extraction risky), and the
> undoable-kind wiring required two touchpoints the design omitted — an
> `undoActionCases()` round-trip case and a localized action label (`KeyActionPasteNode`,
> plus `KeyActionReplaceNode` for the overwrite path).

### Out of scope (YAGNI)

Multi-node/group clipboard, system-clipboard interop, cross-tab/cross-session
persistence, paste-preserving-edges, mobile (keyboard feature; mobile has no
physical keyboard — the cursor-guide paste target is mouse-driven).

---

## Architecture

### 1. Clipboard model

A new value field on `Game` (zero-value = empty clipboard):

```go
// nodeClipboard is an in-memory, single-node clipboard: a deep, independent
// snapshot of a node's type + params. No NodeID (paste mints a fresh one),
// no position (paste uses the cursor-guide cell), no edges.
type nodeClipboard struct {
    has    bool
    typ    model.NodeType
    params model.NodeParams // deep copy: EffectOverrides slice cloned; Logic
                            // func left nil (LogicKind/LogicN/LogicP carry the rule)
}

// on Game:
clipboard nodeClipboard
```

A small helper `snapshotNodeToClipboard(n *uiNode)` reads the authoritative
`model.Node` for `n.ID` from `g.graph`, deep-copies `NodeParams` (cloning the
`EffectOverrides` slice; not copying the non-serializable `Logic` func), and
stores `{has:true, typ, params}`. Deep-copy independence is a tested invariant —
mutating the original node afterward must not change the clipboard.

### 2. Keyboard wiring

**Clipboard shortcut** — a new **ungated** router node `"clipboard"` registered
alongside `"undo"` in `newKeyboardShortcutRouter` (`keyboard_shortcuts.go`),
declaring `ebiten.KeyC, ebiten.KeyX, ebiten.KeyV`. Ungated so it works during
selection/drag; it self-gates on the modifier. Delegates to
`g.clipboardShortcut()`, which mirrors the `undoShortcut` idiom:

```go
ctrl := isKeyPressed(ebiten.KeyControlLeft) || isKeyPressed(ebiten.KeyControlRight) ||
        isKeyPressed(ebiten.KeyMetaLeft)   || isKeyPressed(ebiten.KeyMetaRight)
if !ctrl { return }
switch {
case isKeyJustPressed(ebiten.KeyC): g.copySelectedNode()
case isKeyJustPressed(ebiten.KeyX): g.cutSelectedNode()
case isKeyJustPressed(ebiten.KeyV): g.pasteAtCursor()
}
```

- **Copy:** if `g.sel != nil` → `snapshotNodeToClipboard(g.sel)`; emit
  `EventNodeCopied`. No document mutation.
- **Cut:** if `g.sel != nil` → snapshot, then `deleteNode(g.sel)` (already
  undoable + logs node-deleted); emit `EventNodeCut` narrative line. Undo
  restores the node; the clipboard snapshot survives, so cut → paste-elsewhere
  behaves as a move.
- **Paste:** if `g.clipboard.has` → resolve target from
  `guideTargetAt(cursorX, cursorY)` (direct call, **not** the cached memo — input
  path) → `(i, j)`. Then the placement flow below.

**Redo (Ctrl-R)** — extend the existing ungated `"undo"` node's key list with
`ebiten.KeyR`, and add to `undoShortcut`:

```go
if isKeyJustPressed(ebiten.KeyR) && ctrl { g.performRedo(); return }
```

`Ctrl-Z` undo and `Ctrl-Y` / `Shift-Ctrl-Z` redo are unchanged.

**Browser Ctrl-R reload suppression** — add a document/window `keydown` listener
in `src/js/index.html` that, when the chord is `Ctrl/Cmd-R` (and no other
modifier semantics we care about), calls `event.preventDefault()` **without**
`stopPropagation()`. `preventDefault` suppresses only the browser's reload;
ebiten's own canvas keydown listener still receives the event, so
`IsKeyJustPressed(KeyR)` fires and the Go redo runs. F5 and the reload button are
untouched.

> Deploy note: `index.html` is already in the deploy manifest; no new asset ref
> is introduced, so `manifest_drift_test.go` is unaffected. (Confirm during
> implementation.)

### 3. Paste placement + overwrite confirmation

```
target (i,j) = guideTargetAt(cursor).I/.J        // commit cell
existing     = g.nodeAt(i, j)

existing == nil  → pasteNodeAt(i, j)                                  // immediate
existing != nil  → open pasteConfirmDialog(i, j, snapshot)
                     Overwrite → beginUndoGroup("replace node")
                                 deleteNode(existing); pasteNodeAt(i,j)
                                 endUndoGroup()                        // one step
                     Cancel/Esc/tap-outside → dismiss, no change
```

`pasteNodeAt(i, j)`:
- Wrapped in `beginUndoGroup("paste node")` / `defer endUndoGroup()`.
- Mints a node at `(i, j)` with `clipboard.typ` + a deep copy of
  `clipboard.params`, **auto-stitch suppressed** (no edges).
- Selects the new node (consistent with manual add).
- Emits `EventNodePasted`.

**Shared insert primitive (targeted refactor):** extract
`insertNode(i, j, typ, params, stitch bool) *uiNode` from the current
`tryAddNode` body. `tryAddNode` calls it with `stitch=true` + default params;
`pasteNodeAt` calls it with `stitch=false` + clipboard params. Keeps the
auto-select / `emitNodeAdded` / undo-group behavior in one place and avoids a
parallel node-creation path drifting from `tryAddNode`.

**`pasteConfirmDialog`** — modeled on `synthSaveAsDialog`
(`synth_panel_save_as_dialog.go`): a small modal centered over the grid pane with
"Replace node?" heading + Overwrite / Cancel buttons, a blocking `HitArea` at
popover z, dismissed by Overwrite / Cancel / Esc / tap-outside. It captures the
pending `(i, j)` **and** the clipboard snapshot at paste time, so a copy issued
while the dialog is open can't change what gets pasted.

### 4. Undo & logging

**Undo** is inherited, not re-implemented:
- Paste routes through the node-add helper → `emitNodeAdded` → `recordUndo`.
- The replace path collapses delete + add into one undo group.
- Cut's undoable effect is the existing node-delete step.

**Logging** — three new `hooks.Kind`s via the standard 5 touchpoints
(const + `KindAll` + `ActionRegistry` w/ lowercase tag `node` + `eventlogger`
formatter + bump `NumNonVerbose` by 3, currently 69 → 72):

| Kind | Scope | Undoable | Narrative |
|---|---|---|---|
| `EventNodeCopied` | `ScopeInteraction` | no | `[node] node copied id=… type=…` |
| `EventNodeCut` | `ScopeInteraction` | no (delete carries the mutation) | `[node] node cut id=… at (i,j)` |
| `EventNodePasted` | `ScopeDocument` | **yes** | `[node] node pasted at (i,j)` |

`documentScopeKinds` is derived from the registry, so
`TestUndoableSetMatchesRegistry` stays green automatically. Emit helpers live in
`internal/ui/event_helpers.go` beside `emitNodeAdded`.

---

## Testing (TDD)

Functional tests drive real `Game.Update` with the stubbed key vars
(`isKeyPressed` / `isKeyJustPressed`) — no isolated unit mocks. Fast path
(`-tags test`, ebitenstub) covers both desktop and browser for the Go logic
(WASM uses the identical ebiten keyboard path).

### Go tests (`internal/ui/`)

1. **Copy** — select node, Ctrl-C → clipboard populated; zero document change;
   `EventNodeCopied` emitted.
2. **Cut** — Ctrl-X → node removed from graph; clipboard populated; undo restores
   the node; cut + delete logged.
3. **Paste, empty cell** — copy, guide over empty cell, Ctrl-V → new node with
   same params, selected, undoable, `EventNodePasted`; **no edges** on the pasted
   node.
4. **Paste, occupied cell** — Ctrl-V over an existing node → confirm modal shown,
   no change yet; Overwrite replaces (one undo step); Cancel no-ops.
5. **Empty clipboard** — Ctrl-V with nothing copied → no-op.
6. **Params fidelity + independence** — copy a node with non-default
   volume/pitch/duration/logic/synth/effect-overrides → pasted node params
   deep-equal the original; mutating the original after copy does not change the
   clipboard/paste result.
7. **Undo/redo round-trips** — cut and paste both survive undo → redo (snapshot
   restore).
8. **Modifier parity** — both `Ctrl` and `Meta` trigger each of C/X/V and R.
9. **Redo key** — Ctrl-R calls `performRedo` (and Ctrl-Z undo regression test).
10. **Registry/keyboard guards** updated & green:
    `TestShortcutKeysAreDisjointAndComplete` (C/X/V/R now claimed),
    `TestEveryKindClassified`, `TestEveryActionHasLowercaseTag`,
    `TestHooks_NumNonVerboseUpdated` (→ 72), `eventlogger/coverage_test.go`.

### Browser test (Playwright) — Ctrl-R reload suppression

One end-to-end test (real key dispatch through the canvas) proving the JS
override works:
- Set a per-load sentinel on `window` at first load (e.g. `window.__loadedNonce`).
- Perform an edit → Ctrl-Z (undo) → Ctrl-R (redo).
- Assert: (a) `window.__loadedNonce` is unchanged → **the page did not reload**;
  (b) the edit is present again → **redo applied**.
- Optionally assert `event.defaultPrevented === true` via a probe listener.

Per the JS Test Charter, this is a legitimate JS-suite concern (browser-only
`preventDefault` + real-input dispatch). No new bridge export is added, so
`wasm_bridge_smoke` is untouched.

---

## Files touched (anticipated)

**Go — new/edited:**
- `internal/ui/game_struct.go` (or gesture-state file) — `clipboard nodeClipboard` field.
- `internal/ui/node_clipboard.go` *(new)* — `nodeClipboard`, `snapshotNodeToClipboard`,
  `copySelectedNode`, `cutSelectedNode`, `pasteAtCursor`, `pasteNodeAt`,
  `clipboardShortcut`.
- `internal/ui/paste_confirm_dialog.go` *(new)* — `pasteConfirmDialog` modal.
- `internal/ui/keyboard_shortcuts.go` — register `"clipboard"` node; add `KeyR`
  to `"undo"` node.
- `internal/ui/game_input_shortcuts.go` — `undoShortcut` handles `KeyR`.
- `internal/ui/game_graph_nodes.go` — extract `insertNode`; `tryAddNode` delegates.
- `internal/ui/event_helpers.go` — `emitNodeCopied` / `emitNodeCut` / `emitNodePasted`.
- `internal/hooks/events.go` — 3 consts, `KindAll` entries, `NumNonVerbose` 69→72.
- `internal/hooks/actions.go` — 3 `ActionRegistry` entries (tag `node`).
- `internal/eventlogger/format.go` — 3 formatters.

**JS:**
- `src/js/index.html` — Ctrl/Cmd-R `keydown` `preventDefault`.

**Tests:** new `internal/ui/*_test.go` (functional) + one
`src/js/*.browser.test.js` (Ctrl-R reload suppression) + updates to the guard
tests listed above.

---

## Risks & notes

- **Playing during paste/cut** — add/delete during playback can crackle (known
  behavior, see the circuit-edit audio-disruption note). This design does not
  change that path; scope is not to worsen it.
- **Ctrl-R muscle memory** — some users expect Ctrl-R = reload. Accepted
  trade-off per the decision to make redo global in-browser; F5 still reloads.
- **Never commit** — per project convention, the spec and all work are left
  **uncommitted** for the user to review/stage.
