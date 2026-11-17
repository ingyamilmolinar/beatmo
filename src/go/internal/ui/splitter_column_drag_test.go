//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestColumnDividerDragResizesWidgets verifies that pressing on the column
// divider pill between the instrument panel (Col 0) and the timeline grid
// (Col 1) and dragging it actually changes the column widths.
func TestColumnDividerDragResizesWidgets(t *testing.T) {
	assertDefaultParityState(t)

	logger := game_log.New(testLogOutput(), game_log.LevelError)
	graph := model.NewGraph(logger)
	bounds := image.Rect(0, 0, 800, 400)
	dv := NewDrumView(bounds, graph, logger)

	// Run one Update frame to initialize the tree, hit areas, etc.
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 400 },
	)
	dv.Update()
	restore()

	if dv.widgets == nil {
		t.Fatal("widget board not initialized after first Update")
	}
	if dv.layoutResizeZone == nil {
		t.Fatal("layoutResizeZone not stored on DrumView")
	}

	// Find the column divider pill rect between Col 0 and Col 1.
	pillRect := dv.layoutHandler.columnHandleRect(0)
	if pillRect.Empty() {
		t.Fatal("column handle rect is empty — no pill between Col 0 and Col 1")
	}

	initialColW := dv.widgets.ColWidth(0)
	if initialColW <= 0 {
		t.Fatalf("initial ColWidth(0)=%d, expected >0", initialColW)
	}

	// Simulate press at pill center.
	cx := (pillRect.Min.X + pillRect.Max.X) / 2
	cy := (pillRect.Min.Y + pillRect.Max.Y) / 2
	restore = SetInputForTest(
		func() (int, int) { return cx, cy },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 400 },
	)
	dv.Update()
	restore()

	if !dv.layoutHandler.dragging {
		t.Fatal("expected layoutHandler to be dragging after press on pill")
	}

	// Simulate drag 50px to the right (still pressed).
	dragX := cx + 50
	restore = SetInputForTest(
		func() (int, int) { return dragX, cy },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 400 },
	)
	dv.Update()
	restore()

	// Release.
	restore = SetInputForTest(
		func() (int, int) { return dragX, cy },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 400 },
	)
	dv.Update()
	restore()

	newColW := dv.widgets.ColWidth(0)
	if newColW == initialColW {
		t.Errorf("ColWidth(0) unchanged after drag: %d == %d", newColW, initialColW)
	}
	delta := newColW - initialColW
	// Allow some tolerance since the widget board may clamp.
	if delta < 20 {
		t.Errorf("ColWidth(0) increased by only %d, expected ~50px increase", delta)
	}
}

// TestColumnDividerNotVisibleOnMobile verifies that on mobile the layout
// resize zone returns no hit areas (EnableLayoutResize=false) and dragging
// at the column divider position has no effect.
func TestColumnDividerNotVisibleOnMobile(t *testing.T) {
	assertDefaultParityState(t)

	forceSmallScreenForTest = true
	defer func() { forceSmallScreenForTest = false }()
	UpdateProfile()
	defer UpdateProfile()

	p := Profile()
	if p.EnableLayoutResize {
		t.Fatal("expected EnableLayoutResize=false on mobile profile")
	}

	logger := game_log.New(testLogOutput(), game_log.LevelError)
	graph := model.NewGraph(logger)
	bounds := image.Rect(0, 0, 400, 700)
	dv := NewDrumView(bounds, graph, logger)

	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 400, 700 },
	)
	dv.Update()
	restore()

	if dv.layoutResizeZone == nil {
		t.Fatal("layoutResizeZone not stored on DrumView")
	}

	// On mobile, HitAreas should return nil.
	areas := dv.layoutResizeZone.HitAreas()
	if len(areas) != 0 {
		t.Errorf("expected 0 hit areas on mobile, got %d", len(areas))
	}

	if dv.widgets == nil {
		t.Skip("widget board not initialized on mobile")
	}

	initialColW := dv.widgets.ColWidth(0)

	// Attempt to drag at the approximate column divider position.
	// Even if we press in the right spot, it should not resize.
	approxX := dv.widgets.ColWidth(0) + dv.Bounds.Min.X
	approxY := dv.Bounds.Min.Y + dv.Bounds.Dy()/2
	restore = SetInputForTest(
		func() (int, int) { return approxX, approxY },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 400, 700 },
	)
	dv.Update()
	restore()

	if dv.layoutHandler.dragging {
		t.Error("layout handler should not be dragging on mobile")
	}

	dragX := approxX + 50
	restore = SetInputForTest(
		func() (int, int) { return dragX, approxY },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 400, 700 },
	)
	dv.Update()
	restore()

	restore = SetInputForTest(
		func() (int, int) { return dragX, approxY },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 400, 700 },
	)
	dv.Update()
	restore()

	newColW := dv.widgets.ColWidth(0)
	if newColW != initialColW {
		t.Errorf("ColWidth(0) changed on mobile: %d → %d", initialColW, newColW)
	}
}
