# Node Clipboard & Undo/Redo Keyboard Shortcuts Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Ctrl/Cmd-C/X/V copy-cut-paste for a single selected grid node, plus a global Ctrl-R redo binding (Ctrl-Z undo already exists), all undoable and logged.

**Architecture:** An in-memory `nodeClipboard` on `Game` holds a deep, independent snapshot (type + params, no edges) of the selected node. A new ungated keyboard-router node reads C/X/V; the existing ungated undo node gains R. Copy snapshots; cut snapshots then deletes; paste places a fresh node at the cursor-guide cell — immediately if empty, or behind a portal-based "Replace node?" confirmation if occupied. Undo comes from the existing node-add/delete emit helpers; logging adds three `hooks.Kind`s. Browser reload on Ctrl-R is suppressed by a JS `keydown` `preventDefault`.

**Tech Stack:** Go 1.26 (Ebiten), WASM, JS (Playwright browser test).

## Global Constraints

- **Bundled Go only:** `/home/ymolinar/Repos/beatmo/.tools/go/bin/go` for every Go command. Never system `go`.
- **Fast Go test invocation:** `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./<pkg>/ -run <Name> -v`.
- **Browser test invocation:** `cd /home/ymolinar/Repos/beatmo && GO=$(pwd)/.tools/go/bin/go node src/js/<file>.browser.test.js`.
- **DO NOT stage or commit** (project convention — [[feedback_never_stage_or_commit]]). Every task ends by leaving changes uncommitted and running the verification command. No `git add` / `git commit`.
- **Modifier idiom (verbatim):** `ctrl := isKeyPressed(ebiten.KeyControlLeft) || isKeyPressed(ebiten.KeyControlRight) || isKeyPressed(ebiten.KeyMetaLeft) || isKeyPressed(ebiten.KeyMetaRight)` — accepts Ctrl and Cmd everywhere, no OS detection.
- **Read keys only through the swappable vars** `isKeyPressed` / `isKeyJustPressed` (input.go:57-58) and cursor through `cursorPosition()` (input.go:45). Tests stub these.
- **Input-path guide reads use `guideTargetAt(x,y)` directly**, never the cached `guideTargetCached` (grid_guide.go:84-86 comment).
- **Tags are lowercase.** All three new event tags are `node`.
- **No new JS bridge export** — do not touch `wasm_bridge_smoke.browser.test.js`.

---

## File Structure

**Go — create:**
- `src/go/internal/ui/node_clipboard.go` — `nodeClipboard`, `cloneNodeParams`, `snapshotNodeToClipboard`, `copySelectedNode`, `cutSelectedNode`, `pasteAtCursor`, `pasteNodeFrom`, `clipboardShortcut`, paste-confirm state + `beginPasteConfirm`/`ConfirmPasteOverwrite`/`CancelPasteOverwrite`/`openPasteConfirmPortal`/`closePasteConfirmPortal`/`drawPasteConfirm`.
- `src/go/internal/ui/node_clipboard_test.go` — functional tests (package `ui`).

**Go — modify:**
- `src/go/internal/hooks/events.go` — 3 consts, `KindAll` entries, `NumNonVerbose` 69→72.
- `src/go/internal/hooks/actions.go` — 3 `ActionRegistry` entries.
- `src/go/internal/eventlogger/format.go` — 3 formatters.
- `src/go/internal/ui/event_helpers.go` — `emitNodeCopied`/`emitNodeCut`/`emitNodePasted`.
- `src/go/internal/ui/keyboard_shortcuts.go` — register `"clipboard"` node; add `KeyR` to `"undo"` node.
- `src/go/internal/ui/game_input_shortcuts.go` — `undoShortcut` handles `KeyR`; `handleEscape` gains a paste-confirm cancel rung.
- `src/go/internal/ebitestub/ebiten/ebiten.go` — add `KeyC`, `KeyX`, `KeyV`, `KeyR`.

**JS — modify:**
- `src/js/index.html` — top-level Ctrl/Cmd-R `keydown` `preventDefault`.

**JS — create:**
- `src/js/ctrl_r_redo.browser.test.js` — Ctrl-R redo + no-reload.

---

## Task 1: hooks kinds + registry + formatters

Adds `EventNodeCopied`, `EventNodeCut`, `EventNodePasted` and satisfies the registry/formatter/count guards. No behavior yet — validated by the existing guard tests going red then green.

**Files:**
- Modify: `src/go/internal/hooks/events.go` (const block ~36-43, `KindAll` ~197-199, `NumNonVerbose` :245)
- Modify: `src/go/internal/hooks/actions.go` (~:70-77)
- Modify: `src/go/internal/eventlogger/format.go` (~:73-93)

**Interfaces:**
- Produces: `hooks.EventNodeCopied`, `hooks.EventNodeCut`, `hooks.EventNodePasted` (`hooks.Kind`), each classified in `ActionRegistry` and formatted in `eventlogger`. Payload type for all three: existing `hooks.NodeEdit{ID,I,J,Type}`.

- [ ] **Step 1: Add the const declarations.** In `events.go`, after line 39 (`EventNodeTypeChanged`), insert:

```go
	EventNodeCopied        Kind = "node.copied"
	EventNodeCut           Kind = "node.cut"
	EventNodePasted        Kind = "node.pasted"
```

- [ ] **Step 2: Add to `KindAll` (non-verbose region).** In `events.go`, change the node line (~197) from:

```go
	EventNodeAdded, EventNodeDeleted, EventNodeMoved, EventNodeTypeChanged,
```

to:

```go
	EventNodeAdded, EventNodeDeleted, EventNodeMoved, EventNodeTypeChanged,
	EventNodeCopied, EventNodeCut, EventNodePasted,
```

- [ ] **Step 3: Run the classification guard — expect FAIL.**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/hooks/ -run 'TestEveryKindClassified|TestHooks_NumNonVerboseUpdated' -v`
Expected: FAIL — new kinds have no `ActionRegistry` entry and `NumNonVerbose` (69) no longer matches (now 72 non-verbose kinds).

- [ ] **Step 4: Add `ActionRegistry` entries.** In `actions.go`, after line 73 (`{EventNodeTypeChanged, ...}`), insert (copy = interaction/not-undoable; cut = interaction narrative, its delete carries the doc mutation; paste = document/undoable):

```go
	{EventNodeCopied, "copy node", "node", ScopeInteraction, ""},
	{EventNodeCut, "cut node", "node", ScopeInteraction, ""},
	{EventNodePasted, "paste node", "node", ScopeDocument, ""},
```

- [ ] **Step 5: Bump `NumNonVerbose`.** In `events.go`, change `const NumNonVerbose = 69` to `const NumNonVerbose = 72` and append to the tally comment block above it:

```go
// + node.copied + node.cut + node.pasted (clipboard copy/cut/paste) = 72.
```

- [ ] **Step 6: Add eventlogger formatters.** In `format.go`, after the `EventNodeTypeChanged` formatter (~line 93), insert:

```go
	hooks.EventNodeCopied: func(p any) string {
		n, _ := p.(hooks.NodeEdit)
		return fmt.Sprintf("node copied id=%d at (%d,%d)", n.ID, n.I, n.J)
	},
	hooks.EventNodeCut: func(p any) string {
		n, _ := p.(hooks.NodeEdit)
		return fmt.Sprintf("node cut id=%d at (%d,%d)", n.ID, n.I, n.J)
	},
	hooks.EventNodePasted: func(p any) string {
		n, _ := p.(hooks.NodeEdit)
		return fmt.Sprintf("node pasted id=%d at (%d,%d)", n.ID, n.I, n.J)
	},
```

- [ ] **Step 7: Run all guards — expect PASS.**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/hooks/ ./internal/eventlogger/ -v -run 'TestEveryKindClassified|TestHooks_NumNonVerboseUpdated|TestEveryActionHasLowercaseTag|Coverage'`
Expected: PASS (all kinds classified, lowercase tags, count = 72, every kind has a formatter).

- [ ] **Step 8: Full hooks + eventlogger packages green; leave uncommitted.**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/hooks/ ./internal/eventlogger/`
Expected: `ok`. Do NOT commit.

---

## Task 2: Clipboard model + copy

The `nodeClipboard`, the deep-copy helper, `snapshotNodeToClipboard`, `copySelectedNode`, and `emitNodeCopied`. Driven by direct method call (no keyboard yet).

**Files:**
- Create: `src/go/internal/ui/node_clipboard.go`
- Create: `src/go/internal/ui/node_clipboard_test.go`
- Modify: `src/go/internal/ui/event_helpers.go`

**Interfaces:**
- Consumes: `hooks.EventNodeCopied` (Task 1); `model.Graph.GetNodeByID(id) (model.Node, bool)`; `nodeTypeName(model.NodeType) string` (event_helpers.go:375); `g.sel *uiNode`; `g.clipboard nodeClipboard`.
- Produces: `type nodeClipboard struct { has bool; typ model.NodeType; params model.NodeParams }`; `func cloneNodeParams(model.NodeParams) model.NodeParams`; `func (g *Game) snapshotNodeToClipboard(*uiNode) bool`; `func (g *Game) copySelectedNode()`; `func emitNodeCopied(id model.NodeID, i, j int, t model.NodeType)`.

- [ ] **Step 1: Write the failing test.** Create `src/go/internal/ui/node_clipboard_test.go`:

```go
package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

func newClipboardTestGame(t *testing.T) *Game {
	t.Helper()
	assertDefaultParityState(t)
	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	return g
}

func TestCopySelectedNodePopulatesClipboardAndEmits(t *testing.T) {
	g := newClipboardTestGame(t)
	n := g.tryAddNode(20, 20, 0) // regular; auto-selects → g.sel = n
	// Give the node non-default params to prove they are captured.
	node, _ := g.graph.GetNodeByID(n.ID)
	node.Params.Volume = 0.42
	node.Params.Pitch = 7
	node.Params.LogicKind = "probability"
	node.Params.LogicP = 0.5
	node.Params.EffectOverrides = []model.EffectOverride{{Type: "distortion", Param: "drive", Value: 0.8}}
	g.graph.Nodes[n.ID] = node

	before := len(g.nodes)
	e := awaitEmit(t, hooksEventNodeCopied(), func() { g.copySelectedNode() })

	if !g.clipboard.has {
		t.Fatal("clipboard not populated after copy")
	}
	if g.clipboard.params.Volume != 0.42 || g.clipboard.params.Pitch != 7 {
		t.Fatalf("clipboard params not captured: %+v", g.clipboard.params)
	}
	if len(g.nodes) != before {
		t.Fatalf("copy changed the document: nodes %d -> %d", before, len(g.nodes))
	}
	if e.Kind != hooksEventNodeCopied() {
		t.Fatalf("wrong event kind %v", e.Kind)
	}

	// Independence: mutating the source node must not change the clipboard.
	node2, _ := g.graph.GetNodeByID(n.ID)
	node2.Params.Volume = 0.99
	node2.Params.EffectOverrides[0].Value = 0.1
	g.graph.Nodes[n.ID] = node2
	if g.clipboard.params.Volume != 0.42 || g.clipboard.params.EffectOverrides[0].Value != 0.8 {
		t.Fatal("clipboard aliased the source node params (deep copy failed)")
	}
}
```

> Note: `hooksEventNodeCopied()` is a tiny local shim so this test file doesn't import `hooks` directly for a single symbol; define it at the bottom of the test file:
> ```go
> func hooksEventNodeCopied() hooks.Kind { return hooks.EventNodeCopied }
> ```
> and add `"github.com/ingyamilmolinar/beatmo/internal/hooks"` to the test imports. (Equivalently, import `hooks` and use `hooks.EventNodeCopied` directly — either is fine.)

- [ ] **Step 2: Run — expect FAIL (compile error: undefined `nodeClipboard`/`copySelectedNode`).**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestCopySelectedNodePopulatesClipboardAndEmits -v`
Expected: FAIL — `undefined: g.clipboard` / `g.copySelectedNode`.

- [ ] **Step 3: Add the `emitNodeCopied` helper.** In `event_helpers.go`, after `emitNodeTypeChanged` (~line 60), insert:

```go
// emitNodeCopied publishes EventNodeCopied. Interaction-scoped (not undoable):
// copy does not mutate the document, so it records no undo step.
func emitNodeCopied(id model.NodeID, i, j int, t model.NodeType) {
	hooks.PublishWithSource(hooks.EventNodeCopied, hooks.NodeEdit{
		ID: int(id), I: i, J: j, Type: nodeTypeName(t),
	}, hooks.CaptureSource(1))
}
```

- [ ] **Step 4: Create `node_clipboard.go` with the model + copy.**

```go
package ui

import (
	"github.com/ingyamilmolinar/beatmo/core/model"
)

// nodeClipboard is an in-memory, single-node clipboard: a deep, independent
// snapshot of a node's type + params. It holds no NodeID (paste mints a fresh
// one), no grid position (paste uses the cursor-guide cell), and no edges
// (params-only paste). The zero value is an empty clipboard.
type nodeClipboard struct {
	has    bool
	typ    model.NodeType
	params model.NodeParams
}

// cloneNodeParams returns a deep copy of p with the non-serializable Logic func
// dropped (LogicKind/LogicN/LogicP carry the rule) and the EffectOverrides
// slice cloned, so later edits to the source node cannot mutate the clipboard.
func cloneNodeParams(p model.NodeParams) model.NodeParams {
	c := p
	c.Logic = nil
	if len(p.EffectOverrides) > 0 {
		c.EffectOverrides = make([]model.EffectOverride, len(p.EffectOverrides))
		copy(c.EffectOverrides, p.EffectOverrides)
	} else {
		c.EffectOverrides = nil
	}
	return c
}

// snapshotNodeToClipboard captures n's authoritative model type + params into
// g.clipboard. Returns false (no capture) if n is nil or has no model node.
func (g *Game) snapshotNodeToClipboard(n *uiNode) bool {
	if n == nil {
		return false
	}
	node, ok := g.graph.GetNodeByID(n.ID)
	if !ok {
		return false
	}
	g.clipboard = nodeClipboard{has: true, typ: node.Type, params: cloneNodeParams(node.Params)}
	return true
}

// copySelectedNode snapshots the currently selected node into the clipboard and
// emits EventNodeCopied. No-op when nothing is selected.
func (g *Game) copySelectedNode() {
	if g.sel == nil {
		return
	}
	if !g.snapshotNodeToClipboard(g.sel) {
		return
	}
	emitNodeCopied(g.sel.ID, g.sel.I, g.sel.J, g.clipboard.typ)
}
```

- [ ] **Step 5: Add the `clipboard` field to `Game`.** In `game_struct.go`, add a field to the `Game` struct (place it near the selection fields, e.g. after `selNeighbors`):

```go
	clipboard nodeClipboard // single-node copy/cut buffer (node_clipboard.go)
```

- [ ] **Step 6: Run — expect PASS.**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestCopySelectedNodePopulatesClipboardAndEmits -v`
Expected: PASS. Do NOT commit.

---

## Task 3: Keyboard wiring — clipboard node + Ctrl-R redo

Adds the ebitestub keys, registers the ungated `"clipboard"` router node, wires `Ctrl-R` redo into the existing `"undo"` node, and updates the disjointness guard.

**Files:**
- Modify: `src/go/internal/ebitestub/ebiten/ebiten.go` (const block ends :248)
- Modify: `src/go/internal/ui/keyboard_shortcuts.go` (:130)
- Modify: `src/go/internal/ui/game_input_shortcuts.go` (`undoShortcut` :113-136)
- Modify: `src/go/internal/ui/node_clipboard.go`
- Modify: `src/go/internal/ui/node_clipboard_test.go`

**Interfaces:**
- Consumes: `g.copySelectedNode` (Task 2); `g.handleUndoRedoKeys()` (drives `dispatchUngated`); `g.performRedo()` (undo_seam.go:76).
- Produces: `func (g *Game) clipboardShortcut() bool`; ebiten stub keys `KeyC`/`KeyX`/`KeyV`/`KeyR`; `"clipboard"` ungated router node owning `{KeyC, KeyX, KeyV}`; `KeyR` added to `"undo"` node.

- [ ] **Step 1: Add the ebitestub keys.** In `ebitestub/ebiten/ebiten.go`, before the closing `)` of the `const` block (after `KeyNumpadSubtract`, line 248):

```go
	KeyC
	KeyX
	KeyV
	KeyR
```

- [ ] **Step 2: Write the failing keyboard tests.** Append to `node_clipboard_test.go`:

```go
// withStubbedKeys swaps the isKeyPressed/isKeyJustPressed vars for the duration
// of fn: keys in pressed report IsKeyPressed==true; keys in just report
// IsKeyJustPressed==true (and IsKeyPressed==true too, matching real hardware).
func withStubbedKeys(pressed, just []ebiten.Key, fn func()) {
	oldP, oldJ := isKeyPressed, isKeyJustPressed
	defer func() { isKeyPressed, isKeyJustPressed = oldP, oldJ }()
	set := func(keys []ebiten.Key) map[ebiten.Key]bool {
		m := map[ebiten.Key]bool{}
		for _, k := range keys {
			m[k] = true
		}
		return m
	}
	pm, jm := set(pressed), set(just)
	isKeyPressed = func(k ebiten.Key) bool { return pm[k] || jm[k] }
	isKeyJustPressed = func(k ebiten.Key) bool { return jm[k] }
	fn()
}

func TestCtrlCCopiesViaKeyboard(t *testing.T) {
	g := newClipboardTestGame(t)
	g.tryAddNode(21, 21, 0) // auto-selects
	withStubbedKeys([]ebiten.Key{ebiten.KeyControlLeft}, []ebiten.Key{ebiten.KeyC}, func() {
		g.handleUndoRedoKeys()
	})
	if !g.clipboard.has {
		t.Fatal("Ctrl-C did not populate the clipboard")
	}
}

func TestCmdCCopiesViaKeyboard(t *testing.T) {
	g := newClipboardTestGame(t)
	g.tryAddNode(22, 22, 0)
	withStubbedKeys([]ebiten.Key{ebiten.KeyMetaLeft}, []ebiten.Key{ebiten.KeyC}, func() {
		g.handleUndoRedoKeys()
	})
	if !g.clipboard.has {
		t.Fatal("Cmd-C did not populate the clipboard")
	}
}

func TestCtrlRRedoesViaKeyboard(t *testing.T) {
	g := newClipboardTestGame(t)
	n := g.tryAddNode(23, 23, 0)
	base := len(g.nodes)
	g.deleteNode(n) // undoable
	if len(g.nodes) != base-1 {
		t.Fatalf("delete failed: %d", len(g.nodes))
	}
	g.performUndo() // node restored
	if len(g.nodes) != base {
		t.Fatalf("undo failed: %d", len(g.nodes))
	}
	withStubbedKeys([]ebiten.Key{ebiten.KeyControlLeft}, []ebiten.Key{ebiten.KeyR}, func() {
		g.handleUndoRedoKeys()
	})
	if len(g.nodes) != base-1 {
		t.Fatalf("Ctrl-R did not redo the delete: nodes=%d want=%d", len(g.nodes), base-1)
	}
}
```

Add `"github.com/hajimehoshi/ebiten/v2"` to the test imports (the ebitestub is aliased to this path under `-tags test`).

- [ ] **Step 3: Run — expect FAIL (undefined `clipboardShortcut` not yet wired; Ctrl-R does nothing).**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestCtrlCCopiesViaKeyboard|TestCmdCCopiesViaKeyboard|TestCtrlRRedoesViaKeyboard' -v`
Expected: FAIL — Ctrl-C leaves clipboard empty; Ctrl-R does not redo.

- [ ] **Step 4: Add `clipboardShortcut`.** In `node_clipboard.go`, add:

```go
// clipboardShortcut handles Ctrl/Cmd-C/X/V. Ungated router node: it self-gates
// on the modifier so plain C/X/V never trigger it. Returns whether it acted.
func (g *Game) clipboardShortcut() bool {
	ctrl := isKeyPressed(ebiten.KeyControlLeft) || isKeyPressed(ebiten.KeyControlRight) ||
		isKeyPressed(ebiten.KeyMetaLeft) || isKeyPressed(ebiten.KeyMetaRight)
	if !ctrl {
		return false
	}
	switch {
	case isKeyJustPressed(ebiten.KeyC):
		g.copySelectedNode()
		return true
	case isKeyJustPressed(ebiten.KeyX):
		g.cutSelectedNode()
		return true
	case isKeyJustPressed(ebiten.KeyV):
		g.pasteAtCursor()
		return true
	}
	return false
}
```

Add the ebiten import to `node_clipboard.go`:

```go
import (
	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/core/model"
)
```

> `cutSelectedNode` and `pasteAtCursor` are added in Tasks 4-6. To keep this task compiling on its own, add temporary no-op stubs now and fill them in later tasks:
> ```go
> func (g *Game) cutSelectedNode()  {} // implemented in Task 4
> func (g *Game) pasteAtCursor()    {} // implemented in Task 5
> ```
> Tasks 4 and 5 REPLACE these stubs with the real bodies (delete the stub line when adding the real one).

- [ ] **Step 5: Register the `"clipboard"` router node.** In `keyboard_shortcuts.go` `newKeyboardShortcutRouter`, after the `"undo"` line (:130) add:

```go
	r.add("clipboard", ZShortcutUndo, false /*ungated*/, []ebiten.Key{ebiten.KeyC, ebiten.KeyX, ebiten.KeyV}, shortcutFunc(g.clipboardShortcut))
```

- [ ] **Step 6: Add `KeyR` to the `"undo"` node's key list.** In `keyboard_shortcuts.go` :130, change:

```go
	r.add("undo", ZShortcutUndo, false /*ungated*/, []ebiten.Key{ebiten.KeyZ, ebiten.KeyY}, shortcutFunc(g.undoShortcut))
```

to:

```go
	r.add("undo", ZShortcutUndo, false /*ungated*/, []ebiten.Key{ebiten.KeyZ, ebiten.KeyY, ebiten.KeyR}, shortcutFunc(g.undoShortcut))
```

- [ ] **Step 7: Handle `KeyR` in `undoShortcut`.** In `game_input_shortcuts.go`, inside `undoShortcut`, after the `KeyY` block (:134) and before the final `return false`, add (gate on `!shift` so `Ctrl-Shift-R` stays a browser hard-reload):

```go
	if isKeyJustPressed(ebiten.KeyR) && !shift {
		g.performRedo()
		return true
	}
```

Also update the doc comment above `undoShortcut` (:110) to mention Ctrl/Cmd-R:

```go
// undoShortcut handles Ctrl/Cmd+Z (undo), Shift+Ctrl/Cmd+Z, Ctrl/Cmd+Y and
// Ctrl/Cmd+R (redo). Ungated node: works even while a text field is focused.
```

- [ ] **Step 8: Run the keyboard tests — expect PASS.**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestCtrlCCopiesViaKeyboard|TestCmdCCopiesViaKeyboard|TestCtrlRRedoesViaKeyboard' -v`
Expected: PASS.

- [ ] **Step 9: Run the shortcut disjointness guard — expect PASS.**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestShortcutKeysAreDisjointAndComplete -v`
Expected: PASS — C/X/V (clipboard) and R (undo) are newly claimed and disjoint from all other nodes. If this test enumerates an expected key set, update it to include `KeyC/KeyX/KeyV` under `"clipboard"` and `KeyR` under `"undo"`. Do NOT commit.

---

## Task 4: Cut

`cutSelectedNode` = snapshot + delete (delete is already undoable and logs `node deleted`); plus the `EventNodeCut` narrative line.

**Files:**
- Modify: `src/go/internal/ui/node_clipboard.go` (replace the Task-3 `cutSelectedNode` stub)
- Modify: `src/go/internal/ui/event_helpers.go`
- Modify: `src/go/internal/ui/node_clipboard_test.go`

**Interfaces:**
- Consumes: `g.snapshotNodeToClipboard` (Task 2); `g.deleteNode(*uiNode)` (game_graph_nodes.go:222, undoable); `hooks.EventNodeCut` (Task 1).
- Produces: real `func (g *Game) cutSelectedNode()`; `func emitNodeCut(id model.NodeID, i, j int, t model.NodeType)`.

- [ ] **Step 1: Write the failing test.** Append to `node_clipboard_test.go`:

```go
func TestCutRemovesNodeAndFillsClipboardAndUndoRestores(t *testing.T) {
	g := newClipboardTestGame(t)
	n := g.tryAddNode(24, 24, 0)
	base := len(g.nodes)
	id := n.ID

	g.cutSelectedNode()

	if g.nodeByID(id) != nil {
		t.Fatal("cut did not remove the node from the graph")
	}
	if len(g.nodes) != base-1 {
		t.Fatalf("node count after cut = %d; want %d", len(g.nodes), base-1)
	}
	if !g.clipboard.has {
		t.Fatal("cut did not populate the clipboard")
	}
	g.performUndo()
	if len(g.nodes) != base {
		t.Fatalf("undo after cut did not restore the node: %d", len(g.nodes))
	}
}
```

- [ ] **Step 2: Run — expect FAIL (cut is a no-op stub).**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestCutRemovesNodeAndFillsClipboardAndUndoRestores -v`
Expected: FAIL — node still present, clipboard empty.

- [ ] **Step 3: Add `emitNodeCut`.** In `event_helpers.go`, after `emitNodeCopied`:

```go
// emitNodeCut publishes EventNodeCut (narrative only). The undoable mutation is
// the subsequent node delete, which records its own undo step, so cut records
// none here.
func emitNodeCut(id model.NodeID, i, j int, t model.NodeType) {
	hooks.PublishWithSource(hooks.EventNodeCut, hooks.NodeEdit{
		ID: int(id), I: i, J: j, Type: nodeTypeName(t),
	}, hooks.CaptureSource(1))
}
```

- [ ] **Step 4: Replace the `cutSelectedNode` stub in `node_clipboard.go`:**

```go
// cutSelectedNode snapshots the selected node, emits the cut narrative, then
// deletes it (deleteNode is undoable and logs the delete). The clipboard
// snapshot survives the delete, so cut → paste-elsewhere behaves as a move.
func (g *Game) cutSelectedNode() {
	if g.sel == nil {
		return
	}
	n := g.sel
	if !g.snapshotNodeToClipboard(n) {
		return
	}
	emitNodeCut(n.ID, n.I, n.J, g.clipboard.typ)
	g.deleteNode(n)
}
```

(Delete the `func (g *Game) cutSelectedNode() {}` stub line added in Task 3.)

- [ ] **Step 5: Run — expect PASS.**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestCutRemovesNodeAndFillsClipboardAndUndoRestores -v`
Expected: PASS. Do NOT commit.

---

## Task 5: Paste into an empty cell

`pasteNodeFrom` (focused node creation, no stitch, params applied, selects, one undo step, emits `EventNodePasted`) and `pasteAtCursor` (resolve guide cell; empty → paste immediately).

**Files:**
- Modify: `src/go/internal/ui/node_clipboard.go` (replace the Task-3 `pasteAtCursor` stub; add `pasteNodeFrom`)
- Modify: `src/go/internal/ui/event_helpers.go`
- Modify: `src/go/internal/ui/node_clipboard_test.go`

**Interfaces:**
- Consumes: `g.clipboard`; `cloneNodeParams`; `g.graph.AddNode(i,j,typ) model.NodeID`; `g.graph.GetNodeByID`; `g.graph.Nodes[id]=`; `g.cacheNode(id)`; `g.grid.Unit() float64`; `g.notifyPredictorNode(id)`; `g.computeSelNeighbors()`; `g.updateBeatInfos()`; `beginUndoGroup`/`endUndoGroup`; `cursorPosition()`; `g.guideTargetAt(x,y)` (`.I`,`.J`); `g.nodeAt(i,j)`; `hooks.EventNodePasted` (Task 1).
- Produces: `func (g *Game) pasteNodeFrom(i, j int, cb nodeClipboard) *uiNode`; real `func (g *Game) pasteAtCursor()`; `func emitNodePasted(id model.NodeID, i, j int, t model.NodeType)`.

- [ ] **Step 1: Write the failing tests.** Append to `node_clipboard_test.go`:

```go
// pasteAtCursor reads cursorPosition() then guideTargetAt(). Stub the cursor to
// point at a grid cell by driving the guide resolver directly via the camera is
// complex; instead we place the cursor and assert on the resolved cell by
// choosing an empty target and reading the pasted node.
func withStubbedCursor(x, y int, fn func()) {
	old := cursorPosition
	defer func() { cursorPosition = old }()
	cursorPosition = func() (int, int) { return x, y }
	fn()
}

func TestPasteIntoEmptyCellCreatesIndependentCopyNoEdges(t *testing.T) {
	g := newClipboardTestGame(t)
	src := g.tryAddNode(30, 30, 0)
	node, _ := g.graph.GetNodeByID(src.ID)
	node.Params.Volume = 0.33
	node.Params.Pitch = 5
	g.graph.Nodes[src.ID] = node
	g.copySelectedNode()

	// Paste directly at a known-empty cell (bypass cursor->guide resolution by
	// calling pasteNodeFrom, which pasteAtCursor delegates to).
	before := len(g.nodes)
	pasted := g.pasteNodeFrom(40, 40, g.clipboard)
	if pasted == nil {
		t.Fatal("pasteNodeFrom returned nil")
	}
	if len(g.nodes) != before+1 {
		t.Fatalf("paste did not add a node: %d -> %d", before, len(g.nodes))
	}
	pn, _ := g.graph.GetNodeByID(pasted.ID)
	if pn.Params.Volume != 0.33 || pn.Params.Pitch != 5 {
		t.Fatalf("pasted params not copied: %+v", pn.Params)
	}
	if pasted.ID == src.ID {
		t.Fatal("pasted node reused the source NodeID")
	}
	if g.sel != pasted {
		t.Fatal("paste did not select the new node")
	}
	// No edges on the pasted node.
	for e := range g.graph.Edges {
		if e[0] == pasted.ID || e[1] == pasted.ID {
			t.Fatalf("pasted node has an edge %v; want none", e)
		}
	}
}

func TestPasteWithEmptyClipboardIsNoOp(t *testing.T) {
	g := newClipboardTestGame(t)
	before := len(g.nodes)
	g.pasteAtCursor() // clipboard empty
	if len(g.nodes) != before {
		t.Fatalf("empty-clipboard paste changed node count: %d -> %d", before, len(g.nodes))
	}
}

func TestPasteAtCursorUndoRestores(t *testing.T) {
	g := newClipboardTestGame(t)
	g.tryAddNode(31, 31, 0)
	g.copySelectedNode()
	base := len(g.nodes)
	g.pasteNodeFrom(41, 41, g.clipboard)
	if len(g.nodes) != base+1 {
		t.Fatalf("paste failed: %d", len(g.nodes))
	}
	g.performUndo()
	if len(g.nodes) != base {
		t.Fatalf("undo after paste did not remove the node: %d", len(g.nodes))
	}
}
```

- [ ] **Step 2: Run — expect FAIL (undefined `pasteNodeFrom`; `pasteAtCursor` is a stub).**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestPasteIntoEmptyCellCreatesIndependentCopyNoEdges|TestPasteWithEmptyClipboardIsNoOp|TestPasteAtCursorUndoRestores' -v`
Expected: FAIL — `undefined: g.pasteNodeFrom`.

- [ ] **Step 3: Add `emitNodePasted`.** In `event_helpers.go`, after `emitNodeCut`:

```go
// emitNodePasted publishes EventNodePasted and records an undo step (paste is a
// document mutation).
func emitNodePasted(id model.NodeID, i, j int, t model.NodeType) {
	hooks.PublishWithSource(hooks.EventNodePasted, hooks.NodeEdit{
		ID: int(id), I: i, J: j, Type: nodeTypeName(t),
	}, hooks.CaptureSource(1))
	recordUndo(hooks.EventNodePasted)
}
```

- [ ] **Step 4: Add `pasteNodeFrom` and the real `pasteAtCursor` to `node_clipboard.go`.** Replace the `pasteAtCursor` stub with:

```go
// pasteNodeFrom mints a fresh node at (i,j) from cb: cb.typ + a deep copy of
// cb.params, with NO edges (no auto-stitch) — this is deliberately NOT
// tryAddNode, whose new-node tail is entangled with origin/start-node logic
// that paste must not run. Selects the node, collapses to one undo step, emits
// EventNodePasted. Precondition: (i,j) is empty (caller clears any occupant).
func (g *Game) pasteNodeFrom(i, j int, cb nodeClipboard) *uiNode {
	if !cb.has {
		return nil
	}
	beginUndoGroup("paste node")
	defer endUndoGroup()

	id := g.graph.AddNode(i, j, cb.typ)
	if node, ok := g.graph.GetNodeByID(id); ok {
		node.Params = cloneNodeParams(cb.params)
		g.graph.Nodes[id] = node
	}
	g.cacheNode(id)

	unit := g.grid.Unit()
	n := &uiNode{ID: id, I: i, J: j, X: float64(i) * unit, Y: float64(j) * unit}
	g.nodes = append(g.nodes, n)
	g.nodesByID[n.ID] = n
	if !g.importing {
		g.notifyPredictorNode(id)
	}

	// Select the pasted node (consistent with manual add).
	if g.sel != nil {
		g.sel.Selected = false
	}
	g.sel = n
	n.Selected = true
	g.computeSelNeighbors()
	g.coordBadgeNode = n
	g.coordBadgeFrame = g.frame

	if !g.importing {
		g.updateBeatInfos()
	}
	emitNodePasted(n.ID, i, j, cb.typ)
	return n
}

// pasteAtCursor resolves the cursor-guide cell and pastes the clipboard node
// there. Empty cell → immediate paste. Occupied cell → open the overwrite
// confirmation (Task 6). No-op with an empty clipboard.
func (g *Game) pasteAtCursor() {
	if !g.clipboard.has {
		return
	}
	x, y := cursorPosition()
	gt := g.guideTargetAt(x, y)
	i, j := gt.I, gt.J
	if existing := g.nodeAt(i, j); existing != nil {
		g.beginPasteConfirm(i, j)
		return
	}
	g.pasteNodeFrom(i, j, g.clipboard)
}
```

(Delete the `func (g *Game) pasteAtCursor() {}` stub from Task 3.)

> `beginPasteConfirm` is added in Task 6. Add a temporary stub now so this task compiles:
> ```go
> func (g *Game) beginPasteConfirm(i, j int) {} // implemented in Task 6
> ```
> Task 6 replaces it.

- [ ] **Step 5: Run — expect PASS.**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestPasteIntoEmptyCellCreatesIndependentCopyNoEdges|TestPasteWithEmptyClipboardIsNoOp|TestPasteAtCursorUndoRestores' -v`
Expected: PASS. Do NOT commit.

---

## Task 6: Paste over an occupied cell — confirmation

Pending-paste state on `Game`, the portal-based "Replace node?" confirmation, and the public `ConfirmPasteOverwrite` / `CancelPasteOverwrite` methods (Go tests drive these directly, mirroring `ConfirmSaveAsDialog`). Esc cancels.

**Files:**
- Modify: `src/go/internal/ui/node_clipboard.go` (replace `beginPasteConfirm` stub; add confirm/cancel/portal/draw)
- Modify: `src/go/internal/ui/game_struct.go` (pending state field)
- Modify: `src/go/internal/ui/game_input_shortcuts.go` (`handleEscape` rung)
- Modify: `src/go/internal/ui/node_clipboard_test.go`

**Interfaces:**
- Consumes: `g.pasteNodeFrom`; `g.deleteNode`; `g.nodeAt`; `beginUndoGroup`/`endUndoGroup`; `g.drum.portal()` (`*OverlayPortal`, dv_overlay_portal.go:14); `PortalEntry` (portal.go:18); `dvOverlayPortal` (dv_overlay_portal.go:24); `g.split.GridRect(winW,winH)` (splitter.go); `gridTopOffset()`; drawing helpers `drawRoundedRect`, `DrawTextStyled`, tokens `TokenSurface2()`/`TokenBorderMedium()`/`TokenTextPrimary()`, `RadiusMD`, `SpaceMD`.
- Produces: `pastePendingState` + `g.pastePending`; `func (g *Game) beginPasteConfirm(i, j int)`; `func (g *Game) ConfirmPasteOverwrite()`; `func (g *Game) CancelPasteOverwrite()`; `func (g *Game) pasteConfirmActive() bool`.

- [ ] **Step 1: Write the failing tests.** Append to `node_clipboard_test.go`:

```go
func TestPasteOverOccupiedOpensConfirmNoImmediateChange(t *testing.T) {
	g := newClipboardTestGame(t)
	g.tryAddNode(50, 50, 0) // source, selected
	g.copySelectedNode()
	occupant := g.tryAddNode(60, 60, 0) // target cell occupied
	base := len(g.nodes)

	g.beginPasteConfirm(60, 60)

	if !g.pasteConfirmActive() {
		t.Fatal("confirm not active after paste over occupied cell")
	}
	if len(g.nodes) != base {
		t.Fatalf("occupied paste mutated the graph before confirm: %d -> %d", base, len(g.nodes))
	}
	if g.nodeByID(occupant.ID) == nil {
		t.Fatal("occupant deleted before confirmation")
	}
}

func TestConfirmPasteOverwriteReplacesInOneUndoStep(t *testing.T) {
	g := newClipboardTestGame(t)
	src := g.tryAddNode(51, 51, 0)
	node, _ := g.graph.GetNodeByID(src.ID)
	node.Params.Pitch = 9
	g.graph.Nodes[src.ID] = node
	g.copySelectedNode()
	occupant := g.tryAddNode(61, 61, 0)
	occID := occupant.ID
	base := len(g.nodes)

	g.beginPasteConfirm(61, 61)
	g.ConfirmPasteOverwrite()

	if g.pasteConfirmActive() {
		t.Fatal("confirm still active after overwrite")
	}
	if g.nodeByID(occID) != nil {
		t.Fatal("occupant not replaced")
	}
	if len(g.nodes) != base {
		t.Fatalf("replace changed node count: %d -> %d", base, len(g.nodes))
	}
	pn := g.nodeAt(61, 61)
	if pn == nil {
		t.Fatal("no node at target after overwrite")
	}
	m, _ := g.graph.GetNodeByID(pn.ID)
	if m.Params.Pitch != 9 {
		t.Fatalf("overwritten node has wrong params: %+v", m.Params)
	}
	// One undo step: a single undo reverses the whole replace (occupant back).
	g.performUndo()
	if g.nodeByID(occID) == nil {
		t.Fatal("single undo did not restore the occupant (replace was not one step)")
	}
}

func TestCancelPasteOverwriteLeavesGraphUnchanged(t *testing.T) {
	g := newClipboardTestGame(t)
	g.tryAddNode(52, 52, 0)
	g.copySelectedNode()
	occupant := g.tryAddNode(62, 62, 0)
	base := len(g.nodes)

	g.beginPasteConfirm(62, 62)
	g.CancelPasteOverwrite()

	if g.pasteConfirmActive() {
		t.Fatal("confirm still active after cancel")
	}
	if g.nodeByID(occupant.ID) == nil || len(g.nodes) != base {
		t.Fatal("cancel mutated the graph")
	}
}

func TestEscCancelsPasteConfirm(t *testing.T) {
	g := newClipboardTestGame(t)
	g.tryAddNode(53, 53, 0)
	g.copySelectedNode()
	g.tryAddNode(63, 63, 0)
	g.beginPasteConfirm(63, 63)

	withStubbedKeys(nil, []ebiten.Key{ebiten.KeyEscape}, func() {
		g.handleEscape()
	})
	if g.pasteConfirmActive() {
		t.Fatal("Esc did not cancel the paste confirmation")
	}
}
```

- [ ] **Step 2: Run — expect FAIL (undefined `pasteConfirmActive`/`ConfirmPasteOverwrite`/etc.).**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestPasteOverOccupiedOpensConfirmNoImmediateChange|TestConfirmPasteOverwriteReplacesInOneUndoStep|TestCancelPasteOverwriteLeavesGraphUnchanged|TestEscCancelsPasteConfirm' -v`
Expected: FAIL — undefined symbols.

- [ ] **Step 3: Add the pending-state field to `Game`.** In `game_struct.go`, near the `clipboard` field added in Task 2:

```go
	pastePending pastePendingState // in-flight paste-overwrite confirmation (node_clipboard.go)
```

- [ ] **Step 4: Replace the `beginPasteConfirm` stub in `node_clipboard.go`** and add the confirm/cancel/portal/draw code. Add `"image"` and `"github.com/hajimehoshi/ebiten/v2"` to the imports if not present (ebiten already added in Task 3):

```go
const pasteConfirmPortalID = "paste-confirm"

// pastePendingState holds an in-flight paste-overwrite confirmation. snap is the
// clipboard captured at paste time, so a copy issued while the dialog is open
// can't change what gets pasted.
type pastePendingState struct {
	active bool
	i, j   int
	snap   nodeClipboard
}

func (g *Game) pasteConfirmActive() bool { return g.pastePending.active }

// beginPasteConfirm arms the overwrite confirmation for cell (i,j) and opens the
// modal portal. The graph is NOT mutated until ConfirmPasteOverwrite.
func (g *Game) beginPasteConfirm(i, j int) {
	g.pastePending = pastePendingState{active: true, i: i, j: j, snap: g.clipboard}
	g.openPasteConfirmPortal()
}

// ConfirmPasteOverwrite deletes the occupant at the pending cell and pastes the
// captured snapshot, collapsed into one "replace node" undo step. Public so the
// portal button, Enter, and tests drive it uniformly.
func (g *Game) ConfirmPasteOverwrite() {
	if !g.pastePending.active {
		return
	}
	i, j, snap := g.pastePending.i, g.pastePending.j, g.pastePending.snap
	g.pastePending = pastePendingState{}
	g.closePasteConfirmPortal()

	beginUndoGroup("replace node")
	defer endUndoGroup()
	if existing := g.nodeAt(i, j); existing != nil {
		g.deleteNode(existing)
	}
	g.pasteNodeFrom(i, j, snap)
}

// CancelPasteOverwrite dismisses the confirmation without mutating the graph.
func (g *Game) CancelPasteOverwrite() {
	if !g.pastePending.active {
		return
	}
	g.pastePending = pastePendingState{}
	g.closePasteConfirmPortal()
}

// pasteConfirmRect centers the confirmation dialog over the grid content pane.
func (g *Game) pasteConfirmRect() image.Rectangle {
	grid := g.split.GridRect(g.winW, g.winH)
	top := gridTopOffset()
	cx := (grid.Min.X + grid.Max.X) / 2
	cy := top + (grid.Max.Y-top)/2
	w, h := 320, 140
	return image.Rect(cx-w/2, cy-h/2, cx+w/2, cy+h/2)
}

func (g *Game) pasteConfirmOverwriteBtnRect() image.Rectangle {
	r := g.pasteConfirmRect()
	btnW, btnH := 120, 32
	return image.Rect(r.Max.X-SpaceMD-btnW, r.Max.Y-SpaceMD-btnH, r.Max.X-SpaceMD, r.Max.Y-SpaceMD)
}

func (g *Game) pasteConfirmCancelBtnRect() image.Rectangle {
	r := g.pasteConfirmRect()
	btnW, btnH := 120, 32
	return image.Rect(r.Min.X+SpaceMD, r.Max.Y-SpaceMD-btnH, r.Min.X+SpaceMD+btnW, r.Max.Y-SpaceMD)
}

// openPasteConfirmPortal shows the modal confirmation. The pending state is the
// source of truth; if no portal is available (headless tests) the confirmation
// logic still works via Confirm/Cancel.
func (g *Game) openPasteConfirmPortal() {
	if g.drum == nil || g.drum.portal() == nil {
		return
	}
	g.drum.portal().Open(PortalEntry{
		ID:    pasteConfirmPortalID,
		Modal: true,
		Scrim: true,
		Overlay: &dvOverlayPortal{
			id:       pasteConfirmPortalID,
			isOpenFn: func() bool { return g.pastePending.active },
			rectFn:   func() image.Rectangle { return g.pasteConfirmRect() },
			drawFn:   func(dst *ebiten.Image) { g.drawPasteConfirm(dst) },
			inputFn: func(x, y int, pressed bool) InputResult {
				if !pressed {
					return InputConsumed
				}
				pt := image.Pt(x, y)
				if pt.In(g.pasteConfirmOverwriteBtnRect()) {
					g.ConfirmPasteOverwrite()
					return InputConsumed
				}
				if pt.In(g.pasteConfirmCancelBtnRect()) {
					g.CancelPasteOverwrite()
					return InputConsumed
				}
				return InputConsumed // swallow stray taps inside the modal
			},
			escapeFn: func() bool { g.CancelPasteOverwrite(); return false },
		},
		OnClose: func() { g.pastePending = pastePendingState{} },
	})
}

func (g *Game) closePasteConfirmPortal() {
	if g.drum == nil || g.drum.portal() == nil {
		return
	}
	g.drum.portal().Close(pasteConfirmPortalID)
}

// drawPasteConfirm renders the "Replace node?" modal: panel + prompt + two
// buttons. Button chrome mirrors drawSaveAsDialog's rounded-rect style.
func (g *Game) drawPasteConfirm(dst *ebiten.Image) {
	if !g.pastePending.active {
		return
	}
	r := g.pasteConfirmRect()
	drawRoundedRect(dst, r, TokenSurface2(), RadiusMD, true)
	drawRoundedRect(dst, r, TokenBorderMedium(), RadiusMD, false)
	DrawTextStyled(dst, "Replace node?", r.Min.X+SpaceMD, r.Min.Y+SpaceMD, RolePanelTitle, TokenTextPrimary())

	ov := g.pasteConfirmOverwriteBtnRect()
	drawRoundedRect(dst, ov, TokenSurface3(), RadiusSM, true)
	DrawTextStyled(dst, "Overwrite", ov.Min.X+SpaceSM, ov.Min.Y+SpaceXS, RoleButton, TokenTextPrimary())

	cn := g.pasteConfirmCancelBtnRect()
	drawRoundedRect(dst, cn, TokenSurface3(), RadiusSM, true)
	DrawTextStyled(dst, "Cancel", cn.Min.X+SpaceSM, cn.Min.Y+SpaceXS, RoleButton, TokenTextPrimary())
}
```

> If any token/role name above (`TokenSurface3`, `RadiusSM`, `SpaceSM`, `SpaceXS`, `RolePanelTitle`, `RoleButton`) does not resolve, grep `theme_tokens.go` / `design_tokens.gen.go` for the nearest existing equivalent and substitute — these are cosmetic and not asserted by tests. Keep the two button rects exactly as defined (they ARE the hit targets).

- [ ] **Step 5: Add the Esc rung.** In `game_input_shortcuts.go` `handleEscape`, after the portal-close block (the `g.drum.portal().IsOpen()` block, ~:353) or alongside the `CancelActiveTextDialog` rung (~:370), add a rung so Esc cancels a paste confirmation even if the portal path didn't consume it:

```go
	// Paste-overwrite confirmation cancel (no text input; its own rung).
	if g.pasteConfirmActive() {
		g.CancelPasteOverwrite()
		return true
	}
```

> Placement note: the portal's own `escapeFn` (in `openPasteConfirmPortal`) already calls `CancelPasteOverwrite` when the portal is open and Esc reaches rung 1, so this rung is the belt-and-suspenders path for headless/no-portal runs (and is what `TestEscCancelsPasteConfirm` exercises). Put it BEFORE the `CancelActiveTextDialog` rung.

- [ ] **Step 6: Run — expect PASS.**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestPasteOverOccupiedOpensConfirmNoImmediateChange|TestConfirmPasteOverwriteReplacesInOneUndoStep|TestCancelPasteOverwriteLeavesGraphUnchanged|TestEscCancelsPasteConfirm' -v`
Expected: PASS.

- [ ] **Step 7: Run the whole `internal/ui` package — expect PASS.**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/`
Expected: `ok` — the new feature plus all existing tests (including `TestShortcutKeysAreDisjointAndComplete`, zone-isolation, undo-set guards) stay green. Do NOT commit.

---

## Task 7: JS Ctrl-R reload suppression + browser verification

The `keydown` `preventDefault` in `index.html`, and a Playwright test proving Ctrl-R redoes and does NOT reload.

**Files:**
- Modify: `src/js/index.html` (last `<script>` region, ~:785-791)
- Create: `src/js/ctrl_r_redo.browser.test.js`

**Interfaces:**
- Consumes: Go redo on Ctrl-R (Task 3); JS exports `addNode`, `totalNodes`, `ensureDefaultPath`, `forceDraw`; helper `setupFullWasm` (real_input_test_helpers.js:346).
- Produces: browser test `ctrl_r_redo.browser.test.js`.

- [ ] **Step 1: Add the `keydown` handler to `index.html`.** Inside the last top-level `<script>` (the one with the `touchstart`/`touchmove` block, ~:785), add:

```js
    // Ctrl/Cmd-R is redo in-app (Go handles the redo). Suppress the browser's
    // reload so the keydown reaches ebiten. NOT stopPropagation — ebiten's
    // canvas keydown listener must still fire. Do NOT hijack while a real DOM
    // text field (mobile soft-keyboard proxy / native input overlay) is focused,
    // and leave Ctrl+Shift+R (hard reload) and Ctrl+Alt+R alone.
    document.addEventListener('keydown', function (e) {
      const isR = (e.key === 'r' || e.key === 'R');
      const mod = e.ctrlKey || e.metaKey;
      if (!isR || !mod || e.shiftKey || e.altKey) return;
      const ae = document.activeElement;
      const domEditing = ae && /^(INPUT|TEXTAREA)$/.test(ae.tagName);
      const mobileEditing =
        (typeof window._mobileInputAnyActive === 'function') && window._mobileInputAnyActive();
      if (domEditing || mobileEditing) return;
      e.preventDefault();
    }, { capture: false });
```

- [ ] **Step 2: Write the browser test.** Create `src/js/ctrl_r_redo.browser.test.js`:

```js
// COVERED-BY-GO: Go covers the redo wiring (internal/ui/node_clipboard_test.go
// TestCtrlRRedoesViaKeyboard). This JS test owns ONLY the browser-only concern:
// the index.html keydown preventDefault actually suppresses the page reload so
// Ctrl-R drives Go redo instead of reloading the tab.
import { setupFullWasm } from "./real_input_test_helpers.js";
import assert from "node:assert";

let page, cleanup;

async function run() {
  ({ page, cleanup } = await setupFullWasm());

  // Boot settle + a per-load sentinel: a reload wipes the JS heap, so if this
  // survives Ctrl-R the page did NOT reload.
  await page.evaluate(() => {
    ensureDefaultPath?.();
    forceDraw?.();
    window.__loadedSentinel = "alive";
  });
  await page.waitForTimeout(300);

  // Focus the canvas so ebiten receives keydowns; ensure the kb-proxy isn't focused.
  await page.evaluate(() => {
    if (typeof _kbProxyBlur === "function") _kbProxyBlur();
    document.querySelector("canvas")?.focus();
  });
  await page.mouse.click(200, 200);
  await page.waitForTimeout(100);

  const base = await page.evaluate(() => totalNodes());
  await page.evaluate(() => { addNode?.(70, 70, "regular"); forceDraw?.(); });
  await page.waitForTimeout(200);
  const afterAdd = await page.evaluate(() => totalNodes());
  assert.strictEqual(afterAdd, base + 1, "addNode should add one node");

  // Undo via Ctrl-Z.
  await page.keyboard.down("Control");
  await page.waitForTimeout(60);
  await page.keyboard.press("KeyZ");
  await page.keyboard.up("Control");
  await page.waitForTimeout(200);
  const afterUndo = await page.evaluate(() => totalNodes());
  assert.strictEqual(afterUndo, base, "Ctrl-Z should undo the add");

  // Redo via Ctrl-R (the new binding). The keydown handler must preventDefault
  // the reload; if it fails, the page reloads and both assertions below break.
  await page.keyboard.down("Control");
  await page.waitForTimeout(60);
  await page.keyboard.press("KeyR");
  await page.keyboard.up("Control");
  await page.waitForTimeout(300);

  const sentinel = await page.evaluate(() => window.__loadedSentinel);
  assert.strictEqual(sentinel, "alive", "page reloaded on Ctrl-R (preventDefault failed)");

  const afterRedo = await page.evaluate(() => totalNodes());
  assert.strictEqual(afterRedo, base + 1, "Ctrl-R should redo the add");

  console.log("PASS ctrl_r_redo: redo applied, no reload");
}

run()
  .then(async () => { await cleanup?.(); process.exit(0); })
  .catch(async (err) => { console.error(err); await cleanup?.(); process.exit(1); });
```

- [ ] **Step 3: Run the browser test — expect PASS.**

Run: `cd /home/ymolinar/Repos/beatmo && GO=$(pwd)/.tools/go/bin/go node src/js/ctrl_r_redo.browser.test.js`
Expected: `PASS ctrl_r_redo: redo applied, no reload` and exit 0. (First run builds `main.wasm`; allow a few minutes. If flaky on focus, confirm the `page.mouse.click` lands on empty canvas, not a node.)

- [ ] **Step 4: Confirm no deploy-manifest drift.** `index.html` is already an `ENTRY_FILES` asset and no new file ref was added, so the guard should stay green:

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/deploy/ -run TestManifestDrift -v`
Expected: PASS. Do NOT commit.

---

## Final verification (run after all tasks)

- [ ] **Full fast Go suite for touched packages:**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ ./internal/hooks/ ./internal/eventlogger/ ./internal/deploy/`
Expected: all `ok`.

- [ ] **gofmt clean** (the overhaul had gofmt-seed failures before — [[project_overhaul_test_fixes_2026_07_09]]):

Run: `cd src/go && ../../.tools/go/bin/gofmt -l internal/ui/node_clipboard.go internal/ui/event_helpers.go internal/ui/keyboard_shortcuts.go internal/ui/game_input_shortcuts.go internal/hooks/events.go internal/hooks/actions.go internal/eventlogger/format.go internal/ebitestub/ebiten/ebiten.go`
Expected: no output (all formatted).

- [ ] **Browser test green** (see Task 7 Step 3).

- [ ] **Leave everything uncommitted** for user review. Do NOT `git add` / `git commit`.

---

## Self-Review Notes (traceability)

- Spec §1 Clipboard model → Task 2 (`nodeClipboard`, `cloneNodeParams` deep-copy + independence test).
- Spec §2 Keyboard (C/X/V ungated, Ctrl||Meta, Ctrl-R redo, WASM same path) → Task 3 (+ ebitestub keys, disjointness guard).
- Spec §3 Paste placement + overwrite confirm → Tasks 5 (empty) + 6 (occupied, portal, one undo step). **Deviation from spec:** confirmation uses the portal system (lower-risk, free modality/scrim/dismissal/Esc) instead of a hand-rolled `synthSaveAsDialog`-style modal; and paste uses a focused `pasteNodeFrom` instead of extracting a shared `insertNode` from the origin/start-entangled `tryAddNode`.
- Spec §4 Undo & logging → Task 1 (3 kinds/formatters/count) + undo inherited via `emitNodePasted`/`deleteNode`/nested groups; cut = two lines (intent+effect) by design.
- Spec §5 Tests → Tasks 2-6 Go functional tests + Task 7 browser test. Modifier parity (Ctrl+Meta) = Task 3. Registry/keyboard guards = Tasks 1, 3.
- Spec Ctrl-R browser reload → Task 7 (JS preventDefault gated on non-text-field focus + verification test).
