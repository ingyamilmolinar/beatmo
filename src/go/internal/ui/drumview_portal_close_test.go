//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// setupPortalCloseTest creates a DrumView with a tree and sets up input stubs.
// Returns the DrumView and a cleanup function.
func setupPortalCloseTest(t *testing.T) (*DrumView, func()) {
	t.Helper()
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	dv := &DrumView{}
	dv.tree = NewDrumViewTree()
	dv.tree.SetBounds(image.Rect(0, 0, 800, 600))
	return dv, restore
}

// openTestPortal opens a minimal portal entry with the given ID on the tree.
func openTestPortal(dv *DrumView, id string) {
	dv.tree.Portal().Open(PortalEntry{
		ID:      id,
		Overlay: &testPortalOverlay{},
		Modal:   false,
	})
}

// --- closeSubdivMenuPortal ---

func TestCloseSubdivMenuPortal(t *testing.T) {
	dv, restore := setupPortalCloseTest(t)
	defer restore()

	openTestPortal(dv, "subdiv-menu")
	if !dv.tree.Portal().Has("subdiv-menu") {
		t.Fatal("portal 'subdiv-menu' should be open")
	}

	dv.closeSubdivMenuPortal()

	if dv.tree.Portal().Has("subdiv-menu") {
		t.Error("portal 'subdiv-menu' should be closed after closeSubdivMenuPortal()")
	}
}

func TestCloseSubdivMenuPortal_NilTree(t *testing.T) {
	dv := &DrumView{}
	// Should not panic with nil tree.
	dv.closeSubdivMenuPortal()
}

// --- closeInstMenuPortal ---

func TestCloseInstMenuPortal(t *testing.T) {
	dv, restore := setupPortalCloseTest(t)
	defer restore()

	openTestPortal(dv, "inst-menu")
	if !dv.tree.Portal().Has("inst-menu") {
		t.Fatal("portal 'inst-menu' should be open")
	}

	dv.closeInstMenuPortal()

	if dv.tree.Portal().Has("inst-menu") {
		t.Error("portal 'inst-menu' should be closed after closeInstMenuPortal()")
	}
}

func TestCloseInstMenuPortal_NilTree(t *testing.T) {
	dv := &DrumView{}
	dv.closeInstMenuPortal()
}

// --- closeColorWheelPortal ---

func TestCloseColorWheelPortal(t *testing.T) {
	dv, restore := setupPortalCloseTest(t)
	defer restore()

	openTestPortal(dv, "color-wheel")
	if !dv.tree.Portal().Has("color-wheel") {
		t.Fatal("portal 'color-wheel' should be open")
	}

	dv.closeColorWheelPortal()

	if dv.tree.Portal().Has("color-wheel") {
		t.Error("portal 'color-wheel' should be closed after closeColorWheelPortal()")
	}
}

func TestCloseColorWheelPortal_NilTree(t *testing.T) {
	dv := &DrumView{}
	dv.closeColorWheelPortal()
}

// --- closeContextMenuPortal ---

func TestCloseContextMenuPortal(t *testing.T) {
	dv, restore := setupPortalCloseTest(t)
	defer restore()

	openTestPortal(dv, "context-menu")
	if !dv.tree.Portal().Has("context-menu") {
		t.Fatal("portal 'context-menu' should be open")
	}

	dv.closeContextMenuPortal()

	if dv.tree.Portal().Has("context-menu") {
		t.Error("portal 'context-menu' should be closed after closeContextMenuPortal()")
	}
}

func TestCloseContextMenuPortal_NilTree(t *testing.T) {
	dv := &DrumView{}
	dv.closeContextMenuPortal()
}

// --- Portal tree input tests (color wheel / rename hold fix) ---

// TestColorWheelPickViaPortalTree verifies that after opening the color wheel
// via the button callback, a click inside the wheel picks a color on the first
// attempt. Before the fix, colorHold + state.hold blocked all input.
func TestColorWheelPickViaPortalTree(t *testing.T) {
	assertDefaultParityState(t)
	dv := NewDrumView(image.Rect(0, 0, 500, 300), nil, testLogger)
	dv.calcLayout()

	before := dv.colorKey(dv.Rows[0].Color)

	// Open the color wheel via the button callback (same path as production).
	dv.rowColorBtns()[0].OnClick()
	if !dv.IsColorMenuOpen() {
		t.Fatal("color menu should be open")
	}

	// Pick color near the right edge of the wheel via direct HandleInput.
	r := dv.colorWheelComp.InputBounds()
	if r.Empty() {
		t.Fatal("colorWheelComp InputBounds is empty")
	}
	x := (r.Min.X+r.Max.X)/2 + r.Dx()/3
	y := (r.Min.Y + r.Max.Y) / 2
	res := dv.colorWheelComp.HandleInput(x, y, true)
	if res == InputIgnored {
		t.Fatal("HandleInput returned InputIgnored — click was not processed")
	}

	after := dv.colorKey(dv.Rows[0].Color)
	if after == before {
		t.Fatalf("wheel click did not change color: still %s", after)
	}
}

// TestColorWheelPortalClickNotBlocked verifies that opening the color wheel
// does NOT set anyDragActive(), allowing the portal tree to dispatch input.
func TestColorWheelPortalClickNotBlocked(t *testing.T) {
	assertDefaultParityState(t)
	dv := NewDrumView(image.Rect(0, 0, 500, 300), nil, testLogger)
	dv.calcLayout()

	dv.rowColorBtns()[0].OnClick()
	if !dv.IsColorMenuOpen() {
		t.Fatal("color menu should be open")
	}

	if dv.anyDragActive() {
		t.Fatal("anyDragActive() should be false after opening color wheel — hold flags no longer block")
	}
}

// TestRenamePortalClickNotBlocked verifies that opening the rename overlay
// does NOT set anyDragActive(), allowing the portal tree to dispatch input.
func TestRenamePortalClickNotBlocked(t *testing.T) {
	assertDefaultParityState(t)
	dv := NewDrumView(image.Rect(0, 0, 500, 300), nil, testLogger)
	dv.calcLayout()

	// Open rename via the row rack callback.
	if len(dv.rowEditBtns()) == 0 {
		t.Skip("no rowEditBtns")
	}
	dv.rowEditBtns()[0].OnClick()

	if dv.anyDragActive() {
		t.Fatal("anyDragActive() should be false after opening rename — hold flags no longer block")
	}
}

// TestColorWheelPortalHoldCleared verifies that after opening the color wheel
// via the production callback path, the component's hold state is cleared
// (Capturing() returns false). Before the fix, Open() set hold=true and
// nothing cleared it.
func TestColorWheelPortalHoldCleared(t *testing.T) {
	assertDefaultParityState(t)
	dv := NewDrumView(image.Rect(0, 0, 500, 300), nil, testLogger)
	dv.calcLayout()

	dv.rowColorBtns()[0].OnClick()
	if !dv.IsColorMenuOpen() {
		t.Fatal("color menu should be open")
	}

	if dv.colorWheelComp.Capturing() {
		t.Fatal("colorWheelComp.Capturing() should be false after portal open — hold was cleared")
	}
}
