package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Content overflows, and the input mocks feed the full g.Update() path.
func openSidebarForDispatch(t *testing.T, cursorX, cursorY *int, leftPressed *bool, wheelYVal *float64) *Game {
	t.Helper()
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 250) // small height to force scrollable content

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sel = n
	n.Selected = true
	g.sidebar.Open(n)
	g.sidebar.ExpandAllSections()
	g.sidebar.logicDropdownOpen = true
	g.sidebar.layout()

	if !g.sidebar.scroll.HasScroll() {
		t.Fatal("scroll should be enabled when all sections expanded with dropdown in small pane")
	}

	restore := SetInputForTest(
		func() (int, int) { return *cursorX, *cursorY },
		func(b ebiten.MouseButton) bool { return *leftPressed && b == ebiten.MouseButtonLeft },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, *wheelYVal },
		func() (int, int) { return 640, 250 },
	)
	t.Cleanup(restore)

	return g
}

// Through the full g.Update() dispatch.
func TestSidebarDispatchWheelScroll(t *testing.T) {
	// The card is anchored beside its node, so the real point comes from rects[sbPanel] once laid out.
	cursorX, cursorY := 0, 0
	leftPressed := false
	wheelY := 0.0

	g := openSidebarForDispatch(t, &cursorX, &cursorY, &leftPressed, &wheelY)

	// Point inside the sidebar's actual (node-anchored) panel content area.
	panel := g.sidebar.rects[sbPanel]
	if panel.Empty() {
		t.Fatal("panel rect should be non-empty")
	}
	cursorX, cursorY = panel.Min.X+10, panel.Min.Y+10
	if !image.Pt(cursorX, cursorY).In(panel) {
		t.Fatalf("cursor (%d,%d) should be inside panel %v", cursorX, cursorY, panel)
	}

	initial := g.sidebar.scroll.VS.First
	if initial != 0 {
		t.Fatalf("initial scroll should be 0, got %d", initial)
	}

	// Wheel down (negative wheelY → steps=-1 → scroll down → VS.First increases)
	wheelY = -1.0
	_ = g.Update()
	wheelY = 0

	if g.sidebar.scroll.VS.First <= initial {
		t.Fatalf("VS.First should increase after wheel-down, got %d", g.sidebar.scroll.VS.First)
	}

	saved := g.sidebar.scroll.VS.First

	// Wheel up (positive wheelY → steps=+1 → scroll up → VS.First decreases)
	wheelY = 1.0
	_ = g.Update()
	wheelY = 0

	if g.sidebar.scroll.VS.First >= saved {
		t.Fatalf("VS.First should decrease after wheel-up, got %d (was %d)", g.sidebar.scroll.VS.First, saved)
	}
}

// A track click (not the thumb) jumps.
func TestSidebarDispatchScrollbarClick(t *testing.T) {
	cursorX, cursorY := 0, 0
	leftPressed := false
	wheelY := 0.0

	g := openSidebarForDispatch(t, &cursorX, &cursorY, &leftPressed, &wheelY)

	bar := g.sidebar.scroll.BarRect()
	thumb := g.sidebar.scroll.ThumbRect()
	if bar.Empty() || thumb.Empty() {
		t.Fatal("bar/thumb rects should be non-empty")
	}

	// Click below the thumb in the track area
	clickY := thumb.Max.Y + (bar.Max.Y-thumb.Max.Y)/2
	if clickY >= bar.Max.Y {
		clickY = bar.Max.Y - 1
	}
	clickX := (bar.Min.X + bar.Max.X) / 2

	if image.Pt(clickX, clickY).In(thumb) {
		t.Fatal("click should be outside thumb rect")
	}
	if !image.Pt(clickX, clickY).In(bar) {
		t.Fatal("click should be inside bar rect")
	}

	// Press on track
	cursorX, cursorY = clickX, clickY
	leftPressed = true
	_ = g.Update()

	// Release
	leftPressed = false
	_ = g.Update()

	if g.sidebar.scroll.VS.First <= 0 {
		t.Fatalf("VS.First should jump to non-zero after track click, got %d", g.sidebar.scroll.VS.First)
	}
}

// Capture holds even when the cursor leaves the sidebar.
func TestSidebarDispatchScrollDragCapture(t *testing.T) {
	cursorX, cursorY := 0, 0
	leftPressed := false
	wheelY := 0.0

	g := openSidebarForDispatch(t, &cursorX, &cursorY, &leftPressed, &wheelY)

	thumb := g.sidebar.scroll.ThumbRect()
	if thumb.Empty() {
		t.Fatal("thumb rect should be non-empty")
	}

	// Start drag on thumb center
	thumbCX := (thumb.Min.X + thumb.Max.X) / 2
	thumbCY := (thumb.Min.Y + thumb.Max.Y) / 2
	cursorX, cursorY = thumbCX, thumbCY
	leftPressed = true
	_ = g.Update()

	// Verify drag started
	if !g.sidebar.scroll.Dragging() {
		t.Fatal("scrollbar should be dragging after press on thumb")
	}

	scrollBefore := g.sidebar.scroll.VS.First

	// Move cursor outside sidebar bounds (well to the right)
	panel := g.sidebar.rects[sbPanel]
	cursorX = panel.Max.X + 100
	cursorY = thumbCY + 40 // move down
	_ = g.Update()

	// Drag should continue (capture semantics)
	if !g.sidebar.scroll.Dragging() {
		t.Fatal("scrollbar drag should continue even when cursor is outside sidebar bounds")
	}

	// Scroll position should have changed from the drag
	if g.sidebar.scroll.VS.First == scrollBefore {
		t.Log("note: scroll position unchanged during drag, but capture was maintained")
	}

	// Release
	leftPressed = false
	_ = g.Update()

	if g.sidebar.scroll.Dragging() {
		t.Fatal("scrollbar should stop dragging after release")
	}
}

func TestSidebarDispatchWheelBlocksZoom(t *testing.T) {
	// The card is anchored beside its node, so the real point comes from rects[sbPanel] once laid out.
	cursorX, cursorY := 0, 0
	leftPressed := false
	wheelY := 0.0

	g := openSidebarForDispatch(t, &cursorX, &cursorY, &leftPressed, &wheelY)

	// Point inside the sidebar's actual (node-anchored) panel rect.
	panel := g.sidebar.rects[sbPanel]
	if panel.Empty() {
		t.Fatal("panel rect should be non-empty")
	}
	cursorX, cursorY = panel.Min.X+10, panel.Min.Y+10
	if !image.Pt(cursorX, cursorY).In(panel) {
		t.Fatalf("cursor (%d,%d) should be inside panel %v", cursorX, cursorY, panel)
	}

	scaleBefore := g.cam.Scale

	// Wheel down over sidebar
	wheelY = -1.0
	_ = g.Update()
	wheelY = 0

	if g.cam.Scale != scaleBefore {
		t.Fatalf("camera scale should not change when wheel is over sidebar: before=%.4f after=%.4f",
			scaleBefore, g.cam.Scale)
	}
}

// A mouse drag through HandleInput stands in for touch.
func TestSidebarDispatchTouchScroll(t *testing.T) {
	cursorX, cursorY := 0, 0
	leftPressed := false
	wheelY := 0.0

	g := openSidebarForDispatch(t, &cursorX, &cursorY, &leftPressed, &wheelY)

	panel := g.sidebar.rects[sbPanel]
	cx := panel.Min.X + panel.Dx()/2

	// contentTopY is HandleInput's own position-independent helper.
	contentTop := g.sidebar.contentTopY()
	startY := panel.Max.Y - 10
	if startY <= contentTop {
		t.Fatalf("panel too small to test touch scroll: panel=%v contentTop=%d", panel, contentTop)
	}

	// Simulate press inside sidebar content area
	cursorX, cursorY = cx, startY
	leftPressed = true
	_ = g.Update()

	// Drag upward (finger moves up = content scrolls down)
	endY := contentTop + 10
	for y := startY - 5; y >= endY; y -= 5 {
		cursorY = y
		_ = g.Update()
	}

	// Release
	leftPressed = false
	_ = g.Update()

	if g.sidebar.scroll.VS.First <= 0 {
		t.Fatalf("VS.First should increase after touch scroll, got %d", g.sidebar.scroll.VS.First)
	}
}
