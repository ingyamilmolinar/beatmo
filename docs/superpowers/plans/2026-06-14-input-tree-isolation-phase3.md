# Input Tree Isolation — Phase 3: Component-Owned Keyboard Shortcuts — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the two monolithic global keyboard handlers (`handleGlobalShortcuts`'s inline key blocks + `handleUndoRedoKeys`) with a small z-ordered router that dispatches to per-component shortcut nodes — so each surface (grid pane, transport, audio panel, undo) *owns* its keys, and there is no grab-bag of inline `isKeyPressed` checks left in the Game-level handler.

**Architecture:** A Game-owned `keyboardShortcutRouter` holds shortcut nodes, each declaring the keys it consumes and a `HandleShortcut() bool`. Two tiers: an **ungated** tier (undo/redo — must work even while a text field is focused, preserving current behavior) dispatched unconditionally, and a **gated** tier (Space, pan, zoom, reset, BPM±, tabs, `?`) dispatched only when `!KeyboardClaimed()`. The current key set is **disjoint across nodes**, so the router runs every node each frame (preserving today's simultaneous-multi-key behavior) and a contract test enforces the disjointness + completeness that makes the spec's "first-claim-wins by z" a moot-but-available guarantee. The `handleGlobalShortcuts` / `handleUndoRedoKeys` function names are retained as thin Update→router seams (the behavior-test and discipline-test anchors); their inline key logic is deleted into nodes.

**Tech Stack:** Go 1.23, Ebiten v2, project test harness (`-tags test -modfile=go.test.mod`).

**Repo policy (IMPORTANT):** Do **NOT** `git add`, stage, or commit in this repo — leave all changes in the working tree; the user owns commits. Every task below ends with a verification run, **not** a commit. (Follows the Phase 1 plan + project memory.)

**Bundled Go:** always use `../../.tools/go/bin/go` from `src/go`.

**Fast test command (run from `src/go`):**
```bash
../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run '<Pattern>' ./internal/ui/
```
Append `2>&1 | grep -E '^(=== RUN|--- (FAIL|PASS|SKIP)|PASS|FAIL|ok|.*\.go:[0-9]+:)' | grep -v DEBUG` to isolate results.

---

## Design decisions (resolved spec open-questions)

The umbrella spec (`docs/superpowers/specs/2026-06-14-input-tree-isolation-design.md`) leaves three open questions. Phase 3 resolves them as follows — these are deliberate, evidence-grounded choices:

- **D1 — Coordinator location:** a lightweight **Game-owned** `keyboardShortcutRouter` (keyboard only). Phase 3 does **NOT** unify the pointer trees (`g.gridTree` is draw-only; `dv.rootTree` owns bottom-pane pointer). Pointer/camera/gesture migration is Phase 4. Building a full window-spanning root coordinator now would drag Phase 4's risk forward for no Phase-3 benefit.
- **D2 — Two tiers, not one focus model:** `handleUndoRedoKeys` runs **ungated** today (Ctrl/Cmd+Z works over popups, during drags, while a text field is focused — see `game_update.go:265-269`). The full single-`focusOwner` model that would gate everything is Phase 5. Phase 3 preserves the current split exactly: an **ungated** node tier (undo) dispatched unconditionally, and a **gated** tier dispatched only when `!dv.KeyboardClaimed()`.
- **D3 — Run-all over literal first-claim-wins:** today every gated block runs each frame (Space + an arrow pan can both fire in one frame). To preserve that exactly, the router runs **all** gated nodes in z-order rather than stopping at the first claimer. The current nodes own **disjoint** key sets, so "run all" ≡ "first-claim-wins". `TestShortcutKeysAreDisjointAndComplete` pins this; if a future key conflict is introduced, that test fails and precedence is added then (YAGNI). The `HandleShortcut() bool` return is retained as the contract hook.
- **D4 — Keep the entry-point names:** `handleGlobalShortcuts` and `handleUndoRedoKeys` remain as ≤5-line seams (the spec's "delete" is satisfied in spirit — the monolithic inline key handling is gone; keys live in component nodes). This avoids rewriting ~40 existing test call sites (`game_input_shortcuts_test.go`, `undo_keyboard_test.go`, `arrow_key_text_input_isolation_test.go`, `text_input_escape_test.go`) which would be pure churn and pure regression risk. The existing behavior suite is the primary safety net for this refactor.

---

## File Structure

| File | Responsibility | Action |
|---|---|---|
| `src/go/internal/ui/keyboard_shortcuts.go` | `KeyboardShortcutHandler` interface, `shortcutFunc` adapter, `keyboardShortcutRouter` (registration + `dispatchUngated`/`dispatchGated`), `ZShortcut*` z-consts, `newKeyboardShortcutRouter(g)` wiring the four nodes | **Create** |
| `src/go/internal/ui/game_input_shortcuts.go` | Add the four node methods (`undoShortcut`, `gridShortcut`, `transportShortcut`, `audioPanelShortcut`); shrink `handleGlobalShortcuts` to the Esc→gate→`dispatchGated` seam; move `handleUndoRedoKeys` body into `undoShortcut` and make it a `dispatchUngated` seam | Modify |
| `src/go/internal/ui/game_input_editor.go` | `handleUndoRedoKeys` becomes a one-line `return g.keyboardRouter.dispatchUngated()` seam | Modify |
| `src/go/internal/ui/game_struct.go` | Add `keyboardRouter *keyboardShortcutRouter` field | Modify (+1 field) |
| `src/go/internal/ui/game_new.go` | Construct `g.keyboardRouter = newKeyboardShortcutRouter(g)` | Modify (+1 line) |
| `src/go/internal/ui/keyboard_shortcuts_test.go` | Router registration, disjoint+complete key-set contract, per-node claim tests | **Create** |
| `src/go/internal/ui/keyboard_focus_discipline_test.go` | Extend: `handleGlobalShortcuts` has no inline shortcut-key constants and dispatches via the router | Modify (append) |

---

## Task 1: Router + node methods (additive; no behavior change yet)

Create the contract, the router, and the four node methods (extracting — by **copy**, not move — the logic from the existing handlers). Wire `g.keyboardRouter`. Do **not** yet change the old handlers' control flow. This task is purely additive: the old inline path still runs, so every existing test stays green; the new router is exercised only by the new test.

**Files:**
- Create: `src/go/internal/ui/keyboard_shortcuts.go`
- Modify: `src/go/internal/ui/game_struct.go` (add field), `src/go/internal/ui/game_new.go` (construct), `src/go/internal/ui/game_input_shortcuts.go` (add the four node methods)
- Test: `src/go/internal/ui/keyboard_shortcuts_test.go` (create)

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/ui/keyboard_shortcuts_test.go`:

```go
package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// The router must register exactly the four expected nodes with the expected
// gating, in descending z for the gated tier.
func TestShortcutRouter_NodesRegistered(t *testing.T) {
	g := newTestGameForUndo(t)
	if g.keyboardRouter == nil {
		t.Fatal("Game must construct a keyboardShortcutRouter")
	}
	names := g.keyboardRouter.nodeNames()
	want := map[string]bool{"undo": true, "grid": true, "transport": true, "audiopanel": true}
	if len(names) != len(want) {
		t.Fatalf("router nodes = %v, want the 4 keys of %v", names, want)
	}
	for _, n := range names {
		if !want[n] {
			t.Fatalf("unexpected node %q (have %v)", n, names)
		}
	}
	// Undo is the only UNGATED node (works while a text field is focused).
	if !g.keyboardRouter.isUngated("undo") {
		t.Fatal("undo node must be ungated")
	}
	for _, n := range []string{"grid", "transport", "audiopanel"} {
		if g.keyboardRouter.isUngated(n) {
			t.Fatalf("%q must be gated", n)
		}
	}
}

// The keys owned by the nodes must be DISJOINT (so run-all == first-claim-wins)
// and COMPLETE (the refactor drops no shortcut key). This is the contract that
// lets the router run every node each frame without a precedence rule.
func TestShortcutKeysAreDisjointAndComplete(t *testing.T) {
	g := newTestGameForUndo(t)
	seen := map[ebiten.Key]string{}
	for _, n := range g.keyboardRouter.nodeNames() {
		for _, k := range g.keyboardRouter.keysOf(n) {
			if other, dup := seen[k]; dup {
				t.Fatalf("key %v claimed by both %q and %q — nodes must own disjoint keys", k, other, n)
			}
			seen[k] = n
		}
	}
	// COMPLETE: every shortcut key handled before Phase 3 is still owned.
	want := []ebiten.Key{
		ebiten.KeyZ, ebiten.KeyY, // undo/redo
		ebiten.KeySpace,                                                       // play/pause
		ebiten.KeyArrowLeft, ebiten.KeyArrowRight, ebiten.KeyArrowUp, ebiten.KeyArrowDown, // pan
		ebiten.KeyBracketLeft, ebiten.KeyBracketRight, ebiten.Key0,            // zoom/reset
		ebiten.KeyEqual, ebiten.KeyNumpadAdd, ebiten.KeyMinus, ebiten.KeyNumpadSubtract, // BPM±
		ebiten.Key1, ebiten.Key2, ebiten.Key3, ebiten.Key4, ebiten.Key5, ebiten.Key6, ebiten.Key7, // tabs
		ebiten.KeySlash, // help
	}
	for _, k := range want {
		if _, ok := seen[k]; !ok {
			t.Fatalf("shortcut key %v is no longer owned by any node (dropped in refactor)", k)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run from `src/go`:
```bash
../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestShortcutRouter_NodesRegistered|TestShortcutKeysAreDisjointAndComplete' ./internal/ui/ 2>&1 | grep -v DEBUG | tail -20
```
Expected: **build failure** — `g.keyboardRouter undefined`.

- [ ] **Step 3: Create `keyboard_shortcuts.go`**

Create `src/go/internal/ui/keyboard_shortcuts.go`:

```go
package ui

import (
	"sort"

	"github.com/hajimehoshi/ebiten/v2"
)

// keyboard_shortcuts.go — component-owned keyboard shortcut routing.
//
// Keyboard shortcuts are not spatial, so they cannot be arbitrated by the
// HitIndex. Instead, each surface (grid pane, transport, audio panel, undo)
// owns a shortcut NODE that declares the keys it consumes and a HandleShortcut
// that acts on them. The router dispatches nodes in descending z.
//
// Two tiers:
//   - UNGATED (undo/redo): dispatched unconditionally every frame, so Ctrl/Cmd+Z
//     keeps working over popups, during drags, and while a text field is focused
//     (its prior behavior — see game_update.go). dispatchUngated is called from
//     handleUndoRedoKeys.
//   - GATED (Space, pan, zoom, reset, BPM±, tabs, "?"): dispatched only when no
//     surface claims the keyboard (DrumView.KeyboardClaimed()). dispatchGated is
//     called from handleGlobalShortcuts after the Esc + KeyboardClaimed gate.
//
// The current nodes own DISJOINT key sets (TestShortcutKeysAreDisjointAnd
// Complete), so the router runs EVERY node each frame — preserving today's
// behavior where, e.g., Space + an arrow pan both fire in one frame. The
// HandleShortcut() bool return is the first-claim-wins contract hook for any
// future overlapping key; it is not used for flow control while keys stay
// disjoint.

// KeyboardShortcutHandler is a keyboard-shortcut consumer. HandleShortcut reads
// the keys this node owns for the current frame and returns whether it acted.
type KeyboardShortcutHandler interface {
	HandleShortcut() bool
}

// shortcutFunc adapts a plain func to KeyboardShortcutHandler.
type shortcutFunc func() bool

func (f shortcutFunc) HandleShortcut() bool { return f() }

// Z-priority for shortcut nodes (descending dispatch). Disjoint keys make the
// exact order behaviorally moot today; the ordering mirrors the pointer-pane
// intuition (panel over grid) so a future overlapping key resolves sensibly.
const (
	ZShortcutGrid       = 10
	ZShortcutTransport  = 20
	ZShortcutAudioPanel = 30
	ZShortcutUndo       = 100 // ungated tier; z only orders within the tier
)

type shortcutChild struct {
	h      KeyboardShortcutHandler
	z      int
	name   string
	keys   []ebiten.Key
	gated  bool
}

// keyboardShortcutRouter holds the shortcut nodes and dispatches them in two
// tiers. Owned by Game.
type keyboardShortcutRouter struct {
	children []shortcutChild
}

// add registers a node, keeping children sorted by descending z (stable).
func (r *keyboardShortcutRouter) add(name string, z int, gated bool, keys []ebiten.Key, h KeyboardShortcutHandler) {
	r.children = append(r.children, shortcutChild{h: h, z: z, name: name, keys: keys, gated: gated})
	sort.SliceStable(r.children, func(i, j int) bool { return r.children[i].z > r.children[j].z })
}

// dispatchUngated runs the ungated tier (undo/redo) and returns whether any
// ungated node claimed this frame. Called unconditionally.
func (r *keyboardShortcutRouter) dispatchUngated() bool {
	claimed := false
	for _, c := range r.children {
		if c.gated {
			continue
		}
		if c.h.HandleShortcut() {
			claimed = true
		}
	}
	return claimed
}

// dispatchGated runs the gated tier. Called only when no surface owns the
// keyboard. Runs every gated node (keys are disjoint — see file header).
func (r *keyboardShortcutRouter) dispatchGated() {
	for _, c := range r.children {
		if !c.gated {
			continue
		}
		c.h.HandleShortcut()
	}
}

// --- test/contract introspection helpers ---

func (r *keyboardShortcutRouter) nodeNames() []string {
	out := make([]string, 0, len(r.children))
	for _, c := range r.children {
		out = append(out, c.name)
	}
	return out
}

func (r *keyboardShortcutRouter) keysOf(name string) []ebiten.Key {
	for _, c := range r.children {
		if c.name == name {
			return c.keys
		}
	}
	return nil
}

func (r *keyboardShortcutRouter) isUngated(name string) bool {
	for _, c := range r.children {
		if c.name == name {
			return !c.gated
		}
	}
	return false
}

// newKeyboardShortcutRouter builds the router with the four standing nodes,
// each delegating to a Game method that owns its keys.
func newKeyboardShortcutRouter(g *Game) *keyboardShortcutRouter {
	r := &keyboardShortcutRouter{}
	r.add("undo", ZShortcutUndo, false /*ungated*/, []ebiten.Key{ebiten.KeyZ, ebiten.KeyY}, shortcutFunc(g.undoShortcut))
	r.add("grid", ZShortcutGrid, true /*gated*/, []ebiten.Key{
		ebiten.KeyArrowLeft, ebiten.KeyArrowRight, ebiten.KeyArrowUp, ebiten.KeyArrowDown,
		ebiten.KeyBracketLeft, ebiten.KeyBracketRight, ebiten.Key0,
	}, shortcutFunc(g.gridShortcut))
	r.add("transport", ZShortcutTransport, true, []ebiten.Key{
		ebiten.KeySpace, ebiten.KeyEqual, ebiten.KeyNumpadAdd, ebiten.KeyMinus, ebiten.KeyNumpadSubtract,
	}, shortcutFunc(g.transportShortcut))
	r.add("audiopanel", ZShortcutAudioPanel, true, []ebiten.Key{
		ebiten.Key1, ebiten.Key2, ebiten.Key3, ebiten.Key4, ebiten.Key5, ebiten.Key6, ebiten.Key7,
		ebiten.KeySlash,
	}, shortcutFunc(g.audioPanelShortcut))
	return r
}
```

- [ ] **Step 4: Add the field and construct the router**

In `game_struct.go`, add a field immediately after `undoManager` (line 27):

```go
	keyboardRouter            *keyboardShortcutRouter // component-owned keyboard shortcut dispatch (keyboard_shortcuts.go)
```

In `game_new.go`, immediately after `g.gridTree = NewGridTree()` (line 313), add:

```go
	g.keyboardRouter = newKeyboardShortcutRouter(g)
```

- [ ] **Step 5: Add the four node methods (copy logic; old handlers still run)**

In `game_input_shortcuts.go`, add these four methods (place them just above `handleGlobalShortcuts`). They are **copies** of the existing inline logic, each returning whether it acted. The old inline blocks remain in place for now (Task 2 deletes them).

```go
// undoShortcut handles Ctrl/Cmd+Z (undo), Shift+Ctrl/Cmd+Z and Ctrl/Cmd+Y
// (redo). Ungated node: works even while a text field is focused. Mirrors the
// pre-Phase-3 handleUndoRedoKeys body.
func (g *Game) undoShortcut() bool {
	if g.undoManager == nil {
		return false
	}
	ctrl := isKeyPressed(ebiten.KeyControlLeft) || isKeyPressed(ebiten.KeyControlRight) ||
		isKeyPressed(ebiten.KeyMetaLeft) || isKeyPressed(ebiten.KeyMetaRight)
	if !ctrl {
		return false
	}
	shift := isKeyPressed(ebiten.KeyShiftLeft) || isKeyPressed(ebiten.KeyShiftRight)
	if isKeyJustPressed(ebiten.KeyZ) {
		if shift {
			g.undoManager.Redo()
		} else {
			g.undoManager.Undo()
		}
		return true
	}
	if isKeyJustPressed(ebiten.KeyY) {
		g.undoManager.Redo()
		return true
	}
	return false
}

// gridShortcut handles the grid pane's keys: arrow-pan (level/continuous),
// [ ] zoom (level/continuous), 0 reset (edge). Returns whether any grid key
// acted this frame.
func (g *Game) gridShortcut() bool {
	acted := false
	panned := false
	if isKeyPressed(ebiten.KeyArrowLeft) {
		g.cam.OffsetX += panStep
		panned = true
	}
	if isKeyPressed(ebiten.KeyArrowRight) {
		g.cam.OffsetX -= panStep
		panned = true
	}
	if isKeyPressed(ebiten.KeyArrowUp) {
		g.cam.OffsetY += panStep
		panned = true
	}
	if isKeyPressed(ebiten.KeyArrowDown) {
		g.cam.OffsetY -= panStep
		panned = true
	}
	if panned {
		g.cam.Snap()
		acted = true
	}
	if isKeyPressed(ebiten.KeyBracketRight) || isKeyPressed(ebiten.KeyBracketLeft) {
		cx := float64(g.split.GridW(g.winW)) / 2
		cy := float64(gridTopOffset()) + float64(g.split.GridH(g.winH))/2
		steps := zoomKeyStep
		if isKeyPressed(ebiten.KeyBracketLeft) {
			steps = -zoomKeyStep
		}
		g.zoomAtScreen(cx, cy, steps)
		acted = true
	}
	if isKeyJustPressed(ebiten.Key0) {
		g.cam.Reset()
		acted = true
	}
	return acted
}

// transportShortcut handles Space (play/pause) and BPM nudge (+/- by 1, incl.
// numpad). Returns whether any transport key acted.
func (g *Game) transportShortcut() bool {
	if g.drum == nil {
		return false
	}
	acted := false
	if isKeyJustPressed(ebiten.KeySpace) {
		g.drum.TriggerPlayPause()
		acted = true
	}
	if isKeyJustPressed(ebiten.KeyEqual) || isKeyJustPressed(ebiten.KeyNumpadAdd) {
		g.drum.SetBPM(g.drum.BPM() + 1)
		acted = true
	}
	if isKeyJustPressed(ebiten.KeyMinus) || isKeyJustPressed(ebiten.KeyNumpadSubtract) {
		g.drum.SetBPM(g.drum.BPM() - 1)
		acted = true
	}
	return acted
}

// audioPanelShortcut handles number keys 1-7 (select audio-panel tab in display
// order) and "?" (toggle the shortcuts help overlay). Returns whether a key
// acted.
func (g *Game) audioPanelShortcut() bool {
	acted := false
	if g.drum != nil && g.drum.eqPanelZone != nil {
		tabs := AllPanelTabs()
		for k := ebiten.Key1; k <= ebiten.Key7; k++ {
			if isKeyJustPressed(k) {
				if idx := int(k - ebiten.Key1); idx < len(tabs) {
					g.drum.eqPanelZone.SetActiveTab(tabs[idx])
				}
				acted = true
				break
			}
		}
	}
	if isKeyJustPressed(ebiten.KeySlash) {
		g.toggleShortcutsOverlay()
		acted = true
	}
	return acted
}
```

- [ ] **Step 6: Run the new test + the full existing keyboard suite**

Run from `src/go`:
```bash
../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestShortcutRouter_NodesRegistered|TestShortcutKeysAreDisjointAndComplete' ./internal/ui/ 2>&1 | grep -E '^(--- (FAIL|PASS)|ok|FAIL)' | grep -v DEBUG
../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestSpaceTogglesPlayPulse|TestArrowKeysPan|TestBracketKeysZoom|TestZeroResetsZoom|TestEqualKeyRaisesBPM|TestMinusKeyLowersBPM|TestNumberKeysSwitchTabs|TestSlashTogglesHelpOverlay|TestUndoKeyboardShortcut|TestRedoKeyboardShortcut|TestArrowKeysYieldTo' ./internal/ui/ 2>&1 | grep -E '^(--- FAIL|ok|FAIL)' | grep -v DEBUG
```
Expected: new tests PASS; no `--- FAIL` (old inline path unchanged, so behavior tests still green).

- [ ] **Step 7: Verify (no commit — repo policy)**

---

## Task 2: Rewire the seams; delete the inline grab-bag

Make `handleGlobalShortcuts` and `handleUndoRedoKeys` delegate to the router, and delete the now-duplicated inline key logic. The existing behavior suite is the regression net — it must stay fully green (this is a behavior-preserving refactor, so the safety check is "existing tests still pass", not a new RED test).

**Files:**
- Modify: `src/go/internal/ui/game_input_shortcuts.go` (`handleGlobalShortcuts`)
- Modify: `src/go/internal/ui/game_input_editor.go` (`handleUndoRedoKeys`)

- [ ] **Step 1: Shrink `handleGlobalShortcuts` to the seam**

In `game_input_shortcuts.go`, replace the entire body of `handleGlobalShortcuts` (the block from `if isKeyJustPressed(ebiten.KeySpace)` at line 127 through the closing `}` of the `?` block at line 199) so the function reads exactly:

```go
func (g *Game) handleGlobalShortcuts() {
	if g.drum == nil {
		return
	}

	// Esc is the universal cancel; it owns its own focus handling.
	if g.handleEscape() {
		return
	}

	// A surface that accepts typed/caret keys owns the keyboard this frame, so
	// the grid yields entirely (arrows reach the caret, not the camera). This is
	// the single keyboard-ownership predicate — every text-input surface declares
	// itself via KeyboardClaimant (keyboard_focus.go). Do NOT reintroduce an
	// inline denylist here; add new surfaces to KeyboardClaimed instead.
	if g.drum.KeyboardClaimed() {
		return
	}

	// Gated shortcut keys are owned by per-component nodes (keyboard_shortcuts.go):
	// grid (pan/zoom/reset), transport (Space, BPM±), audio panel (tabs, "?").
	// Do NOT add inline isKeyPressed checks here — add keys to a node instead.
	g.keyboardRouter.dispatchGated()
}
```

Keep `panStep`, `zoomKeyStep`, the soft-keyboard helpers, `gridHelpButtonRect`, `toggleShortcutsOverlay`, `handleEscape`, `valueEditorActive`, `cancelActiveValueEditor`, and the four node methods from Task 1 — only the inline gated-key blocks inside `handleGlobalShortcuts` are removed.

- [ ] **Step 2: Make `handleUndoRedoKeys` a seam**

In `game_input_editor.go`, replace the entire `handleUndoRedoKeys` body (lines 16-39) with:

```go
// handleUndoRedoKeys dispatches the ungated shortcut tier (undo/redo). Kept as
// the Game.Update entry point and bool-returning test anchor; the actual key
// logic lives in the undo node (g.undoShortcut, keyboard_shortcuts.go). Returns
// whether an ungated node claimed the frame.
func (g *Game) handleUndoRedoKeys() bool {
	if g.keyboardRouter == nil {
		return false
	}
	return g.keyboardRouter.dispatchUngated()
}
```

- [ ] **Step 3: Build + run the full behavior suite (regression net)**

Run from `src/go`:
```bash
../../.tools/go/bin/go vet -tags test ./internal/ui/ 2>&1 | grep -v DEBUG | tail -20
../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestSpaceTogglesPlayPulse|TestActionKeysIgnoredWhileTextInputFocused|TestArrowKeysPan|TestBracketKeysZoom|TestZoomStaysInBounds|TestZeroResetsZoom|TestEqualKeyRaisesBPM|TestMinusKeyLowersBPM|TestShiftPlusStillRaisesBPMByOne|TestNumberKeysSwitchTabs|TestSlashTogglesHelpOverlay|TestSlashIgnoredWhileTextInputFocused|TestNumpadAdjustsBPM|TestBPMNudgeClampsAtMinimum|TestEsc|TestUndoKeyboardShortcut|TestRedoKeyboardShortcut|TestArrowKeysYieldTo|TestFunctional_ArrowKeys_SaveAsDialog' ./internal/ui/ 2>&1 | grep -E '^(--- FAIL|ok|FAIL|panic)' | grep -v DEBUG
```
Expected: `vet` clean; no `--- FAIL` — every pinned shortcut behavior is preserved by the node path. (`TestEsc*` confirm Esc still runs first; `TestActionKeysIgnoredWhileTextInputFocused` + `TestSlashIgnoredWhileTextInputFocused` confirm the gate still suppresses gated keys; `TestUndo/RedoKeyboardShortcut` confirm the ungated seam's bool return.)

- [ ] **Step 4: Verify (no commit)**

---

## Task 3: Discipline lock + per-node claim contract

Pin that the inline grab-bag cannot creep back, and that each node actually owns its keys end-to-end.

**Files:**
- Modify: `src/go/internal/ui/keyboard_focus_discipline_test.go` (append)
- Test: `src/go/internal/ui/keyboard_shortcuts_test.go` (append)

- [ ] **Step 1: Append the discipline guard**

Append to `keyboard_focus_discipline_test.go` (it already has `extractFuncBody`):

```go
// After Phase 3, handleGlobalShortcuts must contain NO inline shortcut-key
// handling — every gated key is owned by a component node dispatched via the
// router. This guards against the monolithic grab-bag creeping back.
func TestGlobalShortcutsHasNoInlineKeyHandling(t *testing.T) {
	src, err := os.ReadFile("game_input_shortcuts.go")
	if err != nil {
		t.Fatalf("read game_input_shortcuts.go: %v", err)
	}
	body := extractFuncBody(t, string(src), "handleGlobalShortcuts")

	if !strings.Contains(body, "dispatchGated()") {
		t.Fatal("handleGlobalShortcuts must dispatch gated shortcuts via g.keyboardRouter.dispatchGated()")
	}
	// No inline shortcut-key constants — they belong in the nodes now. (KeyEscape
	// is NOT checked: Esc routing via handleEscape is Phase 2's concern and the
	// call site, not a constant, lives here.)
	for _, forbidden := range []string{
		"KeyArrowLeft", "KeyArrowRight", "KeyArrowUp", "KeyArrowDown",
		"KeySpace", "KeyBracketLeft", "KeyBracketRight", "Key0",
		"KeyEqual", "KeyMinus", "KeyNumpadAdd", "KeyNumpadSubtract",
		"Key1", "KeySlash",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("handleGlobalShortcuts must not handle %s inline — move it to a keyboard_shortcuts.go node", forbidden)
		}
	}
}
```

- [ ] **Step 2: Append per-node claim tests**

Append to `keyboard_shortcuts_test.go`:

```go
// Each node claims (returns true for) its own keys and ignores others.
func TestShortcutNodes_ClaimOwnKeys(t *testing.T) {
	// transport: Space claims.
	g := newTestGameForUndo(t)
	restore := stubKeys(nil, map[ebiten.Key]bool{ebiten.KeySpace: true})
	if !g.transportShortcut() {
		t.Fatal("transport node must claim Space")
	}
	restore()

	// grid: an arrow claims; no key => no claim.
	g = newTestGameForUndo(t)
	restore = stubKeys(map[ebiten.Key]bool{ebiten.KeyArrowLeft: true}, nil)
	if !g.gridShortcut() {
		t.Fatal("grid node must claim ArrowLeft")
	}
	restore()
	if g.gridShortcut() {
		t.Fatal("grid node must NOT claim when no grid key is pressed")
	}

	// audiopanel: "/" claims.
	g = newTestGameForUndo(t)
	restore = stubKeys(nil, map[ebiten.Key]bool{ebiten.KeySlash: true})
	if !g.audioPanelShortcut() {
		t.Fatal("audiopanel node must claim Slash")
	}
	restore()

	// undo (ungated): Ctrl+Z claims; bare Z does not.
	g = newTestGameForUndo(t)
	restore = stubKeys(map[ebiten.Key]bool{ebiten.KeyControlLeft: true}, map[ebiten.Key]bool{ebiten.KeyZ: true})
	if !g.undoShortcut() {
		t.Fatal("undo node must claim Ctrl+Z")
	}
	restore()
	restore = stubKeys(nil, map[ebiten.Key]bool{ebiten.KeyZ: true})
	if g.undoShortcut() {
		t.Fatal("undo node must NOT claim bare Z (no Ctrl)")
	}
	restore()
}

// The ungated tier (undo) fires even when a surface owns the keyboard, while the
// gated tier does not — the Phase-3 tier split, end to end through the router.
func TestRouterTierGating(t *testing.T) {
	g := newTestGameForUndo(t)
	g.drum.tree.SetFocus("transport") // KeyboardClaimed() == true

	// Gated: Space must be suppressed by handleGlobalShortcuts' gate.
	restore := stubKeys(nil, map[ebiten.Key]bool{ebiten.KeySpace: true})
	g.handleGlobalShortcuts()
	restore()
	if g.drum.PlayPressed() {
		t.Fatal("gated Space must be suppressed while a surface owns the keyboard")
	}

	// Ungated: Ctrl+Z still claims through the ungated dispatch.
	restore = stubKeys(map[ebiten.Key]bool{ebiten.KeyControlLeft: true}, map[ebiten.Key]bool{ebiten.KeyZ: true})
	defer restore()
	if !g.handleUndoRedoKeys() {
		t.Fatal("ungated undo must fire even while a surface owns the keyboard")
	}
}
```

- [ ] **Step 3: Run the discipline + contract tests**

Run from `src/go`:
```bash
../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestGlobalShortcutsHasNoInlineKeyHandling|TestGridKeyboardGateUsesSinglePredicate|TestShortcutNodes_ClaimOwnKeys|TestRouterTierGating' ./internal/ui/ 2>&1 | grep -E '^(--- (FAIL|PASS)|ok|FAIL)' | grep -v DEBUG
```
Expected: all PASS. To confirm the discipline guard bites: temporarily paste `isKeyJustPressed(ebiten.KeySpace)` back into `handleGlobalShortcuts`, re-run (expect FAIL on `TestGlobalShortcutsHasNoInlineKeyHandling`), then revert.

- [ ] **Step 4: Verify (no commit)**

---

## Task 4: Full-suite regression + spec/memory update

- [ ] **Step 1: Run the whole `internal/ui` package**

Run from `src/go`:
```bash
../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ 2>&1 | grep -E '^(--- FAIL|ok|FAIL|panic)' | grep -v DEBUG | tail -40
```
Expected: package `ok`. Isolate any FAIL with `-run '<Name>'` and confirm whether it is pre-existing on this branch (project memory notes several flaky/pre-existing non-keyboard failures from concurrent WIP — e.g. `audio_panel_tree_isolation_test.go` "no synth knob rect", parity-ordering panics). Only keyboard/shortcut/router failures are yours.

- [ ] **Step 2: `vet` build guard (explicit path — branch has a phantom `./...` package)**

Run from `src/go`:
```bash
../../.tools/go/bin/go vet -tags test ./internal/ui/ 2>&1 | grep -v DEBUG | tail -20
```
Expected: clean.

- [ ] **Step 3: Mark Phase 3 done in the spec**

In `docs/superpowers/specs/2026-06-14-input-tree-isolation-design.md`, update the Phase-3 row of the phase-decomposition table to note completion and the resolved open-questions (D1-D4 above): Game-owned keyboard-only router; two-tier (ungated undo + gated rest); run-all over disjoint keys; entry-point names retained. Add a one-line note under "Open implementation questions" that the coordinator-location question is resolved (keyboard-only Game router; pointer unification deferred to Phase 4).

- [ ] **Step 4: Report**

Summarise: router + four component nodes; `handleGlobalShortcuts`/`handleUndoRedoKeys` reduced to seams; inline grab-bag deleted; tier split preserves undo-while-typing; disjoint+complete key contract + discipline guard green; full package status (pre-existing failures explicitly identified). Leave everything in the working tree.

---

## Self-Review (completed by plan author)

- **Spec coverage:** Phase-3 row ("move app-global shortcut keys into handler nodes dispatched by the root; delete `handleGlobalShortcuts`+`handleUndoRedoKeys`; GridTree owns its shortcut keys") → Tasks 1-2 (router + nodes + seam) and Task 3 (discipline lock). Spec §2 `KeyboardShortcutHandler` interface + z-priority + first-claim-wins → `keyboard_shortcuts.go` (interface + descending-z dispatch; first-claim-wins documented as moot-but-available under disjoint keys, D3). Spec "Non-changes: undo/redo keeps current behavior" → ungated tier (D2) + `TestRouterTierGating`. Deferred items (Esc decomposition = Phase 2; pointer/gesture = Phase 4; single-focus-owner key *delivery* + raw-input discipline guard = Phase 5) correctly excluded.
- **Deliberate spec deviations (documented):** (a) `handleGlobalShortcuts`/`handleUndoRedoKeys` retained as thin seams rather than literally deleted — spirit satisfied, ~40 test sites preserved (D4). (b) Run-all dispatch rather than literal stop-at-first-claim — preserves simultaneous-multi-key behavior; equivalent under the enforced disjoint key set (D3). (c) Coordinator is keyboard-only and Game-owned, not a unified window root — keyboard-only Phase 3 scope (D1).
- **Placeholder scan:** none — every step has exact code, paths, commands, expected output.
- **Type consistency:** `keyboardShortcutRouter`, `shortcutChild`, `shortcutFunc`, `KeyboardShortcutHandler`, `newKeyboardShortcutRouter`, `dispatchGated`/`dispatchUngated`, `nodeNames`/`keysOf`/`isUngated`, node methods `undoShortcut`/`gridShortcut`/`transportShortcut`/`audioPanelShortcut`, `ZShortcut*` — names identical across all tasks. `handleGlobalShortcuts`, `handleUndoRedoKeys`, `handleEscape`, `KeyboardClaimed`, `extractFuncBody`, `stubKeys`, `newTestGameForUndo`, `AllPanelTabs`, `SetActiveTab`, `zoomAtScreen`, `TriggerPlayPause`, `SetBPM`/`BPM` confirmed against current source.
- **Repo policy:** every task ends in verification, never a commit.
```
