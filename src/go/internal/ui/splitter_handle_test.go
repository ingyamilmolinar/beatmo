//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// ─── Main Splitter Handle Tests ─────────────────────────────────────────────

func TestSplitter_HandleOnlyInitiatesDrag_Horizontal(t *testing.T) {
	s := NewSplitter(600)
	s.Y = 300
	s.winW = 800
	s.totalH = 600

	// Click on the pill handle center (winW/2, Y) → should start drag
	result := s.HandleInput(400, 300, true)
	if result != InputCaptured {
		t.Errorf("click on handle center: expected InputCaptured, got %v", result)
	}
	if !s.dragging {
		t.Error("click on handle center should start drag")
	}

	// Release
	s.HandleInput(400, 300, false)

	// Click on the divider line but far from the handle (x=50, y=300) → no drag
	result = s.HandleInput(50, 300, true)
	if s.dragging {
		t.Error("click far from handle should NOT start drag")
	}
}

func TestSplitter_HandleOnlyInitiatesDrag_Vertical(t *testing.T) {
	s := NewSplitter(600)
	s.horizontal = false
	s.X = 400
	s.winW = 800
	s.totalH = 600

	// Click on the pill handle center (X, totalH/2) → should start drag
	result := s.HandleInput(400, 300, true)
	if result != InputCaptured {
		t.Errorf("click on handle center: expected InputCaptured, got %v", result)
	}
	if !s.dragging {
		t.Error("click on handle center should start drag")
	}

	// Release
	s.HandleInput(400, 300, false)

	// Click on the divider line but far from the handle (x=400, y=50) → no drag
	result = s.HandleInput(400, 50, true)
	if s.dragging {
		t.Error("click far from handle should NOT start drag")
	}
}

func TestSplitter_HandleRect_Horizontal(t *testing.T) {
	s := NewSplitter(600)
	s.Y = 300
	s.winW = 800
	s.totalH = 600

	r := s.HandleRect()

	// Handle should be centered at (400, 300)
	expectedCX := 400
	expectedCY := 300
	cx := (r.Min.X + r.Max.X) / 2
	cy := (r.Min.Y + r.Max.Y) / 2
	if cx != expectedCX || cy != expectedCY {
		t.Errorf("handle center: got (%d,%d), expected (%d,%d)", cx, cy, expectedCX, expectedCY)
	}
	if r.Dx() != SplitterHandleLen() {
		t.Errorf("handle width: got %d, expected %d", r.Dx(), SplitterHandleLen())
	}
	if r.Dy() != SplitterHandleThick() {
		t.Errorf("handle height: got %d, expected %d", r.Dy(), SplitterHandleThick())
	}
}

func TestSplitter_HandleRect_Vertical(t *testing.T) {
	s := NewSplitter(600)
	s.horizontal = false
	s.X = 400
	s.winW = 800
	s.totalH = 600

	r := s.HandleRect()

	// Handle should be centered at (400, 300)
	expectedCX := 400
	expectedCY := 300
	cx := (r.Min.X + r.Max.X) / 2
	cy := (r.Min.Y + r.Max.Y) / 2
	if cx != expectedCX || cy != expectedCY {
		t.Errorf("handle center: got (%d,%d), expected (%d,%d)", cx, cy, expectedCX, expectedCY)
	}
	if r.Dx() != SplitterHandleThick() {
		t.Errorf("handle width: got %d, expected %d", r.Dx(), SplitterHandleThick())
	}
	if r.Dy() != SplitterHandleLen() {
		t.Errorf("handle height: got %d, expected %d", r.Dy(), SplitterHandleLen())
	}
}

func TestSplitter_DragContinuesOutsideHandle(t *testing.T) {
	s := NewSplitter(600)
	s.Y = 300
	s.winW = 800
	s.totalH = 600

	// Start drag on handle
	s.HandleInput(400, 300, true)
	if !s.dragging {
		t.Fatal("expected drag to start on handle center")
	}

	// Move far away from handle — drag should continue (capture semantics)
	result := s.HandleInput(50, 250, true)
	if result != InputCaptured {
		t.Errorf("during drag, expected InputCaptured, got %v", result)
	}
	if !s.dragging {
		t.Error("drag should continue even when cursor moves far from handle")
	}

	// Release
	s.HandleInput(50, 250, false)
	if s.dragging {
		t.Error("drag should end on release")
	}
}

func TestSplitter_ClickOnLineNotHandle_Ignored(t *testing.T) {
	s := NewSplitter(600)
	s.Y = 300
	s.winW = 800
	s.totalH = 600

	// Click exactly on the divider line (y=300) but at x=100, far from handle (center x=400)
	// This is within the old grab zone but NOT within the handle
	result := s.HandleInput(100, 300, true)
	if s.dragging {
		t.Error("click on line but outside handle should NOT start drag")
	}
	// Should still be ignored (not captured)
	if result == InputCaptured {
		t.Error("click on line outside handle should not return InputCaptured")
	}
}

// ─── Mobile Column Divider Suppression Tests ────────────────────────────────

func TestMobileColumnDividerNotDetectable(t *testing.T) {
	forceSmallScreenForTest = true
	defer func() { forceSmallScreenForTest = false }()

	dv := newTestDrumViewWithBounds(image.Rect(0, 300, 800, 600))
	h := dv.layoutHandler

	// Get the column divider X position
	colX := dv.widgets.colPos[1] + dv.widgets.offset.X
	rackY := dv.widgets.rowPos[1] + dv.widgets.offset.Y + 10

	axis, idx := h.detectColumnDivider(colX, rackY)
	if axis != "" || idx != -1 {
		t.Errorf("on mobile, detectColumnDivider should return (\"\", -1), got (%q, %d)", axis, idx)
	}
}

func TestMobileNoLayoutGuideLines(t *testing.T) {
	forceSmallScreenForTest = true
	defer func() { forceSmallScreenForTest = false }()

	dv := newTestDrumViewWithBounds(image.Rect(0, 300, 800, 600))

	h := dv.layoutHandler

	// Column dividers should not be detectable
	for i := 1; i < len(dv.widgets.colPos)-1; i++ {
		colX := dv.widgets.colPos[i] + dv.widgets.offset.X
		for j := 0; j < len(dv.widgets.rowPos)-1; j++ {
			midY := (dv.widgets.rowPos[j] + dv.widgets.rowPos[j+1]) / 2
			midY += dv.widgets.offset.Y
			axis, idx := h.detectColumnDivider(colX, midY)
			if idx >= 0 {
				t.Errorf("mobile: column divider detected at col=%d row=%d (axis=%q idx=%d)", i-1, j, axis, idx)
			}
		}
	}

	// Row dividers should not be detectable
	for i := 1; i < len(dv.widgets.rowPos)-1; i++ {
		rowY := dv.widgets.rowPos[i] + dv.widgets.offset.Y
		for j := 0; j < len(dv.widgets.colPos)-1; j++ {
			midX := (dv.widgets.colPos[j] + dv.widgets.colPos[j+1]) / 2
			midX += dv.widgets.offset.X
			axis, idx := h.detectRowDivider(midX, rowY)
			if idx >= 0 {
				t.Errorf("mobile: row divider detected at row=%d col=%d (axis=%q idx=%d)", i-1, j, axis, idx)
			}
		}
	}
}

// ─── Layout Divider Handle Tests ────────────────────────────────────────────

func TestLayoutDivider_HandleOnlyInitiatesDrag(t *testing.T) {
	dv := newTestDrumViewWithBounds(image.Rect(0, 300, 800, 600))
	h := dv.layoutHandler

	// Get column divider position in rack area (valid)
	colX := dv.widgets.colPos[1] + dv.widgets.offset.X
	rackY := dv.widgets.rowPos[1] + dv.widgets.offset.Y + 10

	// Verify divider is detectable
	axis, idx := h.detectColumnDivider(colX, rackY)
	if idx < 0 {
		t.Fatal("column divider not detected — test setup issue")
	}
	_ = axis

	// Click on divider line but NOT on the pill handle center
	// Handle is centered vertically in the DrumView bounds
	handleR := h.columnHandleRect(idx)
	// Click well outside handle Y range
	offHandleY := handleR.Min.Y - 50
	if offHandleY < dv.Bounds.Min.Y {
		offHandleY = dv.Bounds.Min.Y + 5
	}
	// Must still be in a valid segment for detectColumnDivider
	_, segIdx := h.detectColumnDivider(colX, offHandleY)
	if segIdx >= 0 {
		// Divider is detected but click is outside handle → should NOT start drag
		result := h.HandleInput(colX, offHandleY, true)
		if h.Capturing() {
			t.Error("click on column divider line outside handle should NOT start drag")
		}
		if result == InputConsumed || result == InputCaptured {
			t.Errorf("click outside handle should not consume/capture, got %v", result)
		}
	}

	// Now click on the handle center → should start drag
	handleCX := (handleR.Min.X + handleR.Max.X) / 2
	handleCY := (handleR.Min.Y + handleR.Max.Y) / 2
	// Ensure this point detects the divider
	_, segIdx2 := h.detectColumnDivider(handleCX, handleCY)
	if segIdx2 >= 0 {
		result := h.HandleInput(handleCX, handleCY, true)
		if !h.Capturing() {
			t.Error("click on column divider handle should start drag")
		}
		if result != InputConsumed {
			t.Errorf("click on handle should return InputConsumed, got %v", result)
		}
		// Release
		h.HandleInput(handleCX, handleCY, false)
	}
}

func TestLayoutDivider_HoverSetsState(t *testing.T) {
	dv := newTestDrumViewWithBounds(image.Rect(0, 300, 800, 600))
	h := dv.layoutHandler

	// Get column divider pill handle center — hover only activates near the pill
	hr := h.columnHandleRect(0)
	pillCX := (hr.Min.X + hr.Max.X) / 2
	pillCY := (hr.Min.Y + hr.Max.Y) / 2

	// Hover (no press) at pill center
	h.HandleInput(pillCX, pillCY, false)

	if dv.layoutHoverAxis != "col" {
		t.Errorf("hover axis: got %q, expected \"col\"", dv.layoutHoverAxis)
	}
	if dv.layoutHoverIdx != 0 {
		t.Errorf("hover idx: got %d, expected 0", dv.layoutHoverIdx)
	}

	// Move away from divider
	h.HandleInput(dv.Bounds.Min.X+10, pillCY, false)

	if dv.layoutHoverIdx != -1 {
		t.Errorf("after move away, hover idx should be -1, got %d", dv.layoutHoverIdx)
	}
}

func TestLayoutDivider_RowHandleOnlyInitiatesDrag(t *testing.T) {
	dv := newTestDrumViewWithBounds(image.Rect(0, 300, 800, 600))
	h := dv.layoutHandler

	// Row 0 divider is fully suppressed (Timeline spans rows 0-1).
	// Use the EQ boundary pill (row divider 1) instead.
	rowY := dv.widgets.rowPos[2] + dv.widgets.offset.Y

	// Verify detection via the EQ pill-only path
	axis, idx := h.detectRowDivider(dv.Bounds.Min.X+dv.Bounds.Dx()/2, rowY)
	if idx < 0 {
		t.Fatal("EQ boundary row divider not detected — test setup issue")
	}
	_ = axis

	handleR := h.rowHandleRect(idx)
	if handleR.Empty() {
		t.Fatal("EQ boundary row handle rect is empty")
	}
	handleCX := (handleR.Min.X + handleR.Max.X) / 2
	handleCY := (handleR.Min.Y + handleR.Max.Y) / 2

	// Verify handle rect dimensions are correct.
	if handleR.Dx() != SplitterHandleLen() {
		t.Errorf("row handle width: got %d, expected %d", handleR.Dx(), SplitterHandleLen())
	}
	if handleR.Dy() != SplitterHandleThick() {
		t.Errorf("row handle height: got %d, expected %d", handleR.Dy(), SplitterHandleThick())
	}

	// If handle is not inside a row control, test drag
	if h.pointInsideRowControl(handleCX, handleCY) {
		t.Log("handle center overlaps row control — skipping drag test")
		return
	}

	result := h.HandleInput(handleCX, handleCY, true)
	if !h.Capturing() {
		t.Error("click on EQ boundary row handle should start drag")
	}
	if result != InputConsumed {
		t.Errorf("expected InputConsumed, got %v", result)
	}
	// Release
	h.HandleInput(handleCX, handleCY, false)
}

// ─── SplitterHandleRect Tests ───────────────────────────────────────────────

func TestSplitterHandleRect_Horizontal(t *testing.T) {
	r := SplitterHandleRect(100, 200, true)
	if r.Dx() != SplitterHandleLen() {
		t.Errorf("horizontal pill width: got %d, expected %d", r.Dx(), SplitterHandleLen())
	}
	if r.Dy() != SplitterHandleThick() {
		t.Errorf("horizontal pill height: got %d, expected %d", r.Dy(), SplitterHandleThick())
	}
	cx := (r.Min.X + r.Max.X) / 2
	cy := (r.Min.Y + r.Max.Y) / 2
	if cx != 100 || cy != 200 {
		t.Errorf("horizontal pill center: got (%d,%d), expected (100,200)", cx, cy)
	}
}

func TestSplitterHandleRect_Vertical(t *testing.T) {
	r := SplitterHandleRect(100, 200, false)
	if r.Dx() != SplitterHandleThick() {
		t.Errorf("vertical pill width: got %d, expected %d", r.Dx(), SplitterHandleThick())
	}
	if r.Dy() != SplitterHandleLen() {
		t.Errorf("vertical pill height: got %d, expected %d", r.Dy(), SplitterHandleLen())
	}
	cx := (r.Min.X + r.Max.X) / 2
	cy := (r.Min.Y + r.Max.Y) / 2
	if cx != 100 || cy != 200 {
		t.Errorf("vertical pill center: got (%d,%d), expected (100,200)", cx, cy)
	}
}

// ─── Dimension Accessor Tests ───────────────────────────────────────────────

func TestSplitterHandleDimensions_DesktopVsMobile(t *testing.T) {
	// Desktop defaults (forceSmallScreenForTest = false)
	forceSmallScreenForTest = false
	if SplitterHandleLen() != desktopSplitterHandleLen {
		t.Errorf("desktop pill length: got %d, expected %d", SplitterHandleLen(), desktopSplitterHandleLen)
	}
	if SplitterHandleThick() != desktopSplitterHandleThick {
		t.Errorf("desktop pill thickness: got %d, expected %d", SplitterHandleThick(), desktopSplitterHandleThick)
	}

	// Mobile
	forceSmallScreenForTest = true
	defer func() { forceSmallScreenForTest = false }()
	if SplitterHandleLen() != touchSplitterHandleLen {
		t.Errorf("mobile pill length: got %d, expected %d", SplitterHandleLen(), touchSplitterHandleLen)
	}
	if SplitterHandleThick() != touchSplitterHandleThick {
		t.Errorf("mobile pill thickness: got %d, expected %d", SplitterHandleThick(), touchSplitterHandleThick)
	}
}

// ─── Unified Splitter Handle Render Tests ───────────────────────────────────
// The handle renders identically on every platform: a square (sharp-cornered)
// handle plus a soft glow halo, with NO grip lines. Desktop's former grip-line
// variant was retired so the same component renders one way everywhere.

type capturedHandleRect struct {
	r   image.Rectangle
	col color.Color
}

// captureSplitterHandleDraws records every filled rect (with its color) drawn
// by a single non-hover DrawSplitterHandle call (horizontal pill at 100,100).
func captureSplitterHandleDraws() []capturedHandleRect {
	dst := ebiten.NewImage(200, 200)
	var got []capturedHandleRect
	origDrawRect := drawRect
	drawRect = func(d *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		if filled {
			got = append(got, capturedHandleRect{r, c})
		}
		origDrawRect(d, r, color.RGBA{}, filled)
	}
	defer func() { drawRect = origDrawRect }()
	DrawSplitterHandle(dst, 100, 100, true, false)
	return got
}

// assertUnifiedSplitterHandle asserts: no grip lines, a square handle rect
// exactly equal to the pill bounds, and a glow halo strictly larger than it.
// Returns the handle's fill color for color-unification checks.
func assertUnifiedSplitterHandle(t *testing.T) color.Color {
	t.Helper()
	draws := captureSplitterHandleDraws()
	pillR := SplitterHandleRect(100, 100, true)

	var handleCol color.Color
	handleDrawn, glowDrawn := false, false
	for _, d := range draws {
		r := d.r
		// No 1px grip ticks inside the pill.
		if r.Dx() == 1 && r.Min.X >= pillR.Min.X && r.Max.X <= pillR.Max.X &&
			r.Min.Y >= pillR.Min.Y && r.Max.Y <= pillR.Max.Y {
			t.Errorf("expected no grip lines, found 1px rect %v inside the pill", r)
		}
		if r == pillR {
			handleDrawn = true
			handleCol = d.col
		}
		if r.Min.X < pillR.Min.X && r.Min.Y < pillR.Min.Y &&
			r.Max.X > pillR.Max.X && r.Max.Y > pillR.Max.Y {
			glowDrawn = true
		}
	}
	if !handleDrawn {
		t.Errorf("expected a square handle rect equal to pill bounds %v", pillR)
	}
	if !glowDrawn {
		t.Error("expected a glow halo rect strictly larger than the pill")
	}
	return handleCol
}

func TestDrawSplitterHandle_Unified_Desktop(t *testing.T) {
	forceSmallScreenForTest = false
	assertUnifiedSplitterHandle(t)
}

func TestDrawSplitterHandle_Unified_Mobile(t *testing.T) {
	forceSmallScreenForTest = true
	defer func() { forceSmallScreenForTest = false }()
	assertUnifiedSplitterHandle(t)
}

// TestSplitterHandle_ColorUnified verifies the resting (non-hover) handle fill
// color is identical on desktop and mobile, and is the cyan splitter-handle
// token (blue channel dominant) — desktop is no longer a gray pill.
func TestSplitterHandle_ColorUnified(t *testing.T) {
	forceSmallScreenForTest = false
	deskCol := assertUnifiedSplitterHandle(t)
	forceSmallScreenForTest = true
	defer func() { forceSmallScreenForTest = false }()
	mobileCol := assertUnifiedSplitterHandle(t)

	dr, dg, db, da := deskCol.RGBA()
	mr, mg, mb, ma := mobileCol.RGBA()
	if dr != mr || dg != mg || db != mb || da != ma {
		t.Errorf("desktop handle color %v != mobile handle color %v — not unified", deskCol, mobileCol)
	}
	// Cyan: blue channel dominant over red (a gray pill would have R≈B).
	if db <= dr {
		t.Errorf("expected cyan handle (B>R), got R=%d B=%d", dr>>8, db>>8)
	}
}

// ─── Cursor Shape Tests ─────────────────────────────────────────────────────

func TestCursorShape_SplitterPillHover(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	forceSmallScreenForTest = false

	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	// Track cursor shape calls
	var lastShape ebiten.CursorShapeType
	origSet := setCursorShape
	setCursorShape = func(s ebiten.CursorShapeType) { lastShape = s }
	defer func() { setCursorShape = origSet }()

	// Position cursor on the main splitter pill center
	hr := g.split.HandleRect()
	hx := (hr.Min.X + hr.Max.X) / 2
	hy := (hr.Min.Y + hr.Max.Y) / 2

	restore := SetInputForTest(
		func() (int, int) { return hx, hy },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	g.updateCursorShape()

	// Horizontal splitter → NS resize cursor
	if lastShape != ebiten.CursorShapeNSResize {
		t.Errorf("expected CursorShapeNSResize on horizontal splitter hover, got %d", lastShape)
	}
}

func TestCursorShape_Default(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	forceSmallScreenForTest = false

	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	var lastShape ebiten.CursorShapeType = ebiten.CursorShapeNSResize // start non-default
	origSet := setCursorShape
	setCursorShape = func(s ebiten.CursorShapeType) { lastShape = s }
	defer func() { setCursorShape = origSet }()

	// Position cursor far from any splitter
	restore := SetInputForTest(
		func() (int, int) { return 10, 10 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	g.updateCursorShape()

	if lastShape != ebiten.CursorShapeDefault {
		t.Errorf("expected CursorShapeDefault when not hovering any pill, got %d", lastShape)
	}
}
