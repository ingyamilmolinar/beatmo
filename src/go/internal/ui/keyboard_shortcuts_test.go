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
		ebiten.KeySpace,                                                                   // play/pause
		ebiten.KeyArrowLeft, ebiten.KeyArrowRight, ebiten.KeyArrowUp, ebiten.KeyArrowDown, // pan
		ebiten.KeyBracketLeft, ebiten.KeyBracketRight, ebiten.Key0,                      // zoom/reset
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
