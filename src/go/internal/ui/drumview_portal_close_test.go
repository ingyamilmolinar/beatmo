//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

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
	// Popups live in the global overlay subtree, so a minimal DrumView must wire it.
	dv.overlayTree = NewDrumViewTree()
	dv.overlayTree.SetBounds(image.Rect(0, 0, 800, 600))
	return dv, restore
}

func openTestPortal(dv *DrumView, id string) {
	dv.portal().Open(PortalEntry{
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
	if !dv.portal().Has("subdiv-menu") {
		t.Fatal("portal 'subdiv-menu' should be open")
	}

	dv.closeSubdivMenuPortal()

	if dv.portal().Has("subdiv-menu") {
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
	if !dv.portal().Has("inst-menu") {
		t.Fatal("portal 'inst-menu' should be open")
	}

	dv.closeInstMenuPortal()

	if dv.portal().Has("inst-menu") {
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
	if !dv.portal().Has("color-wheel") {
		t.Fatal("portal 'color-wheel' should be open")
	}

	dv.closeColorWheelPortal()

	if dv.portal().Has("color-wheel") {
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
	if !dv.portal().Has("context-menu") {
		t.Fatal("portal 'context-menu' should be open")
	}

	dv.closeContextMenuPortal()

	if dv.portal().Has("context-menu") {
		t.Error("portal 'context-menu' should be closed after closeContextMenuPortal()")
	}
}

func TestCloseContextMenuPortal_NilTree(t *testing.T) {
	dv := &DrumView{}
	dv.closeContextMenuPortal()
}

// --- Portal tree input tests (color wheel / rename hold fix) ---

// The first click inside the freshly opened wheel picks.
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

	// Derived from the laid-out cells so it survives layout changes.
	cells := dv.colorWheelComp.SwatchCells()
	if len(cells) == 0 {
		t.Fatal("colorWheelComp has no swatch cells")
	}
	target := image.Point{}
	found := false
	for _, cr := range cells {
		cx := (cr.Min.X + cr.Max.X) / 2
		cy := (cr.Min.Y + cr.Max.Y) / 2
		col := dv.colorWheelComp.pickColorAt(cx, cy)
		if col != nil && dv.colorKey(col) != before {
			target = image.Pt(cx, cy)
			found = true
			break
		}
	}
	if !found {
		t.Fatal("no swatch cell differs from the current color")
	}
	res := dv.colorWheelComp.HandleInput(target.X, target.Y, true)
	if res == InputIgnored {
		t.Fatal("HandleInput returned InputIgnored — click was not processed")
	}

	after := dv.colorKey(dv.Rows[0].Color)
	if after == before {
		t.Fatalf("wheel click did not change color: still %s", after)
	}
}

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

// Open() must not leave hold set, or Capturing() stays true.
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
