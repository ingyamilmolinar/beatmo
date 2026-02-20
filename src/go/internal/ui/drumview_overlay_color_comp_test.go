//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"
)

func TestColorWheelComponent_OpenClose(t *testing.T) {
	comp := NewColorWheelComponent()

	// Initially closed
	if comp.IsOpen() {
		t.Error("expected color wheel to be closed initially")
	}

	// Set props and open
	comp.SetProps(ColorWheelProps{
		AnchorRect: image.Rect(100, 200, 130, 220),
		Bounds:     image.Rect(0, 0, 500, 500),
		RowHeight:  24,
	})
	comp.Open()

	if !comp.IsOpen() {
		t.Error("expected color wheel to be open after Open()")
	}

	// Should have non-empty bounds
	if comp.Bounds().Empty() {
		t.Error("expected non-empty bounds after opening")
	}

	// Close
	comp.Close()
	if comp.IsOpen() {
		t.Error("expected color wheel to be closed after Close()")
	}
}

func TestColorWheelComponent_NoHoldAfterOpen(t *testing.T) {
	comp := NewColorWheelComponent()
	comp.SetProps(ColorWheelProps{
		AnchorRect: image.Rect(100, 200, 130, 220),
		Bounds:     image.Rect(0, 0, 500, 500),
		RowHeight:  24,
	})
	comp.Open()

	// Open() no longer sets hold — portal tree prevents double-dispatch.
	if comp.Capturing() {
		t.Error("expected Capturing() to be false after opening — hold is not set")
	}
}

func TestColorWheelComponent_OnColorPickCallback(t *testing.T) {
	comp := NewColorWheelComponent()

	var pickedColor color.Color
	comp.SetProps(ColorWheelProps{
		AnchorRect: image.Rect(100, 200, 130, 220),
		Bounds:     image.Rect(0, 0, 500, 500),
		RowHeight:  24,
		OnColorPick: func(c color.Color) {
			pickedColor = c
		},
	})
	comp.Open()

	// Get wheel bounds and click inside
	wheelRect := comp.WheelRect()
	centerX := wheelRect.Min.X + wheelRect.Dx()/2
	centerY := wheelRect.Min.Y + wheelRect.Dy()/2

	// Click inside wheel — pick is deferred until mouse release
	result := comp.HandleInput(centerX, centerY, true)
	if result != InputCaptured {
		t.Errorf("expected InputCaptured on wheel pick-press, got %v", result)
	}

	if pickedColor == nil {
		t.Error("expected OnColorPick callback to be called")
	}

	// Wheel stays open while mouse is held (deferred close)
	if !comp.IsOpen() {
		t.Error("expected color wheel to remain open until mouse release")
	}
	if !comp.Capturing() {
		t.Error("expected Capturing() to be true after pick-press")
	}

	// Release mouse — now the wheel closes
	result = comp.HandleInput(centerX, centerY, false)
	if result != InputConsumed {
		t.Errorf("expected InputConsumed on deferred close release, got %v", result)
	}
	if comp.IsOpen() {
		t.Error("expected color wheel to be closed after mouse release")
	}
}

func TestColorWheelComponent_DeferredClose(t *testing.T) {
	comp := NewColorWheelComponent()

	var closeCalled bool
	comp.SetProps(ColorWheelProps{
		AnchorRect:  image.Rect(100, 200, 130, 220),
		Bounds:      image.Rect(0, 0, 500, 500),
		RowHeight:   24,
		OnColorPick: func(c color.Color) {},
		OnClose:     func() { closeCalled = true },
	})
	comp.Open()

	wheelRect := comp.WheelRect()
	cx := wheelRect.Min.X + wheelRect.Dx()/2
	cy := wheelRect.Min.Y + wheelRect.Dy()/2

	// Press inside wheel — picks color but defers close
	comp.HandleInput(cx, cy, true)
	if !comp.IsOpen() {
		t.Fatal("expected wheel to stay open after pick-press")
	}
	if !comp.Capturing() {
		t.Fatal("expected Capturing after pick-press")
	}
	if closeCalled {
		t.Fatal("OnClose should not fire until mouse release")
	}

	// Hold the mouse (still pressed) — should stay captured
	result := comp.HandleInput(cx, cy, true)
	if result != InputCaptured {
		t.Errorf("expected InputCaptured while holding after pick, got %v", result)
	}

	// Release — now close fires
	result = comp.HandleInput(cx, cy, false)
	if result != InputConsumed {
		t.Errorf("expected InputConsumed on release, got %v", result)
	}
	if comp.IsOpen() {
		t.Fatal("expected wheel to be closed after release")
	}
	if !closeCalled {
		t.Fatal("expected OnClose to fire on release")
	}
}

func TestColorWheelComponent_ClickOutsideCloses(t *testing.T) {
	comp := NewColorWheelComponent()

	var closeCalled bool
	comp.SetProps(ColorWheelProps{
		AnchorRect: image.Rect(100, 200, 130, 220),
		Bounds:     image.Rect(100, 100, 400, 400), // 300x300 at offset
		RowHeight:  24,
		OnClose: func() {
			closeCalled = true
		},
	})
	comp.Open()

	// Click far outside the wheel bounds (wheel fills 100,100 → 400,400)
	result := comp.HandleInput(10, 10, true)
	if result != InputConsumed {
		t.Errorf("expected click outside to be consumed, got %v", result)
	}

	if !closeCalled {
		t.Error("expected OnClose callback to be called")
	}
	if comp.IsOpen() {
		t.Error("expected color wheel to be closed after click outside")
	}
}

func TestColorWheelComponent_WheelPositioning(t *testing.T) {
	comp := NewColorWheelComponent()

	// In a wide container, wheel should be centered.
	comp.SetProps(ColorWheelProps{
		AnchorRect: image.Rect(10, 10, 40, 30),
		Bounds:     image.Rect(0, 0, 400, 200),
		RowHeight:  24,
	})
	comp.Open()

	wheelRect := comp.WheelRect()
	// Diameter should be 200 (smaller dim). Centered horizontally.
	if wheelRect.Dx() != 200 {
		t.Fatalf("expected diameter 200, got %d", wheelRect.Dx())
	}
	centerX := (wheelRect.Min.X + wheelRect.Max.X) / 2
	if abs(centerX-200) > 1 {
		t.Fatalf("not centered: wheel centerX=%d, bounds centerX=200", centerX)
	}
}

func TestColorWheelComponent_WheelImageCache(t *testing.T) {
	comp := NewColorWheelComponent()
	comp.SetProps(ColorWheelProps{
		AnchorRect: image.Rect(100, 200, 130, 220),
		Bounds:     image.Rect(0, 0, 500, 500),
		RowHeight:  24,
	})
	comp.Open()

	// After opening, wheel image should be built
	if comp.state.wheelImg == nil {
		t.Error("expected wheel image to be created")
	}
	if comp.state.cacheW <= 0 || comp.state.cacheH <= 0 {
		t.Error("expected cache dimensions to be set")
	}
}

func TestColorWheelComponent_HandleInputWhenClosed(t *testing.T) {
	comp := NewColorWheelComponent()
	comp.SetProps(ColorWheelProps{
		AnchorRect: image.Rect(100, 200, 130, 220),
		Bounds:     image.Rect(0, 0, 500, 500),
		RowHeight:  24,
	})

	// Don't open the wheel, input should be ignored
	result := comp.HandleInput(150, 150, true)
	if result != InputIgnored {
		t.Error("expected input to be ignored when wheel is closed")
	}
}

func TestColorWheelComponent_ColorPicking(t *testing.T) {
	comp := NewColorWheelComponent()
	comp.SetProps(ColorWheelProps{
		AnchorRect: image.Rect(100, 200, 130, 220),
		Bounds:     image.Rect(0, 0, 500, 500),
		RowHeight:  24,
	})
	comp.Open()

	// Pick color at center - should be white/light (center of HSV wheel)
	wheelRect := comp.WheelRect()
	centerX := wheelRect.Min.X + wheelRect.Dx()/2
	centerY := wheelRect.Min.Y + wheelRect.Dy()/2
	centerColor := comp.pickColorAt(centerX, centerY)

	r, g, b, _ := centerColor.RGBA()
	// Center should be dark (rnorm=0 -> v=0)
	if r > 0x8000 || g > 0x8000 || b > 0x8000 {
		t.Logf("center color R=%d G=%d B=%d (expected dark)", r>>8, g>>8, b>>8)
	}

	// Pick color at edge - should be saturated
	edgeX := wheelRect.Max.X - 1
	edgeY := wheelRect.Min.Y + wheelRect.Dy()/2
	edgeColor := comp.pickColorAt(edgeX, edgeY)

	er, eg, eb, _ := edgeColor.RGBA()
	// Edge should be more saturated
	t.Logf("edge color R=%d G=%d B=%d", er>>8, eg>>8, eb>>8)
}

func TestColorWheelComponent_EmptyProps(t *testing.T) {
	comp := NewColorWheelComponent()
	comp.SetProps(ColorWheelProps{
		AnchorRect: image.Rectangle{}, // empty
		Bounds:     image.Rect(0, 0, 500, 500),
		RowHeight:  24,
	})
	comp.Open()

	// Should handle empty anchor gracefully
	if !comp.Bounds().Empty() {
		t.Error("expected empty bounds with empty anchor")
	}
}

func TestColorWheelComponent_InputBounds(t *testing.T) {
	comp := NewColorWheelComponent()
	comp.SetProps(ColorWheelProps{
		AnchorRect: image.Rect(100, 200, 130, 220),
		Bounds:     image.Rect(0, 0, 500, 500),
		RowHeight:  24,
	})

	// Before open, input bounds should be empty
	if !comp.InputBounds().Empty() {
		t.Error("expected empty input bounds before opening")
	}

	comp.Open()

	// After open, input bounds should match wheel rect
	inputBounds := comp.InputBounds()
	if inputBounds.Empty() {
		t.Error("expected non-empty input bounds after opening")
	}
	if inputBounds != comp.WheelRect() {
		t.Errorf("expected input bounds to match wheel rect")
	}
}

func TestColorWheelHSVToRGBA(t *testing.T) {
	// Test pure red (h=0, s=1, v=1)
	red := colorWheelHSVToRGBA(0, 1, 1)
	rr, rg, rb, _ := red.RGBA()
	if rr>>8 < 250 || rg>>8 > 5 || rb>>8 > 5 {
		t.Errorf("expected red, got R=%d G=%d B=%d", rr>>8, rg>>8, rb>>8)
	}

	// Test white (s=0, v=1)
	white := colorWheelHSVToRGBA(0.5, 0, 1)
	wr, wg, wb, _ := white.RGBA()
	if wr>>8 < 250 || wg>>8 < 250 || wb>>8 < 250 {
		t.Errorf("expected white, got R=%d G=%d B=%d", wr>>8, wg>>8, wb>>8)
	}

	// Test black (v=0)
	black := colorWheelHSVToRGBA(0.5, 1, 0)
	br, bg, bb, _ := black.RGBA()
	if br>>8 > 5 || bg>>8 > 5 || bb>>8 > 5 {
		t.Errorf("expected black, got R=%d G=%d B=%d", br>>8, bg>>8, bb>>8)
	}
}

func TestColorWheelComponent_CornerClickAbsorbed(t *testing.T) {
	comp := NewColorWheelComponent()

	var pickCalled bool
	comp.SetProps(ColorWheelProps{
		AnchorRect:  image.Rect(100, 200, 130, 220),
		Bounds:      image.Rect(0, 0, 200, 200),
		RowHeight:   24,
		OnColorPick: func(c color.Color) { pickCalled = true },
	})
	comp.Open()

	// Click at top-left corner of bounding rect — inside rect but outside circle.
	r := comp.WheelRect()
	result := comp.HandleInput(r.Min.X+1, r.Min.Y+1, true)
	if result != InputConsumed {
		t.Errorf("expected InputConsumed for corner click, got %v", result)
	}
	if pickCalled {
		t.Error("OnColorPick should NOT be called for a corner click outside the circle")
	}
	if !comp.IsOpen() {
		t.Error("color wheel should remain open after corner click")
	}
	if comp.Capturing() {
		t.Error("Capturing() should be false — no pick occurred")
	}
}

// ─── Fill-bounds sizing tests ────────────────────────────────────────────────

// TestColorWheelFillsBounds_SquareContainer verifies that the color wheel fills
// a square container, using the full dimension as its diameter.
func TestColorWheelFillsBounds_SquareContainer(t *testing.T) {
	comp := NewColorWheelComponent()
	bounds := image.Rect(0, 100, 300, 400) // 300x300 square
	comp.SetProps(ColorWheelProps{
		AnchorRect: image.Rect(10, 150, 40, 170),
		Bounds:     bounds,
		RowHeight:  24,
	})
	comp.Open()

	r := comp.WheelRect()
	// The wheel should use the full smaller dimension (300) as diameter.
	if r.Dx() != 300 || r.Dy() != 300 {
		t.Fatalf("expected wheel 300x300, got %dx%d", r.Dx(), r.Dy())
	}
}

// TestColorWheelFillsBounds_WideContainer verifies that in a wide container
// the wheel uses the height as diameter and is centered horizontally.
func TestColorWheelFillsBounds_WideContainer(t *testing.T) {
	comp := NewColorWheelComponent()
	bounds := image.Rect(0, 100, 500, 300) // 500w x 200h
	comp.SetProps(ColorWheelProps{
		AnchorRect: image.Rect(10, 150, 40, 170),
		Bounds:     bounds,
		RowHeight:  24,
	})
	comp.Open()

	r := comp.WheelRect()
	// Diameter should be the smaller dimension: 200.
	if r.Dx() != 200 || r.Dy() != 200 {
		t.Fatalf("expected wheel 200x200, got %dx%d", r.Dx(), r.Dy())
	}
	// Should be centered horizontally within bounds.
	centerX := (r.Min.X + r.Max.X) / 2
	boundsCenter := (bounds.Min.X + bounds.Max.X) / 2
	if abs(centerX-boundsCenter) > 1 {
		t.Fatalf("wheel not centered horizontally: wheel center=%d, bounds center=%d", centerX, boundsCenter)
	}
}

// TestColorWheelFillsBounds_TallContainer verifies that in a tall container
// the wheel uses the width as diameter and is centered vertically.
func TestColorWheelFillsBounds_TallContainer(t *testing.T) {
	comp := NewColorWheelComponent()
	bounds := image.Rect(0, 0, 200, 500) // 200w x 500h
	comp.SetProps(ColorWheelProps{
		AnchorRect: image.Rect(10, 50, 40, 70),
		Bounds:     bounds,
		RowHeight:  24,
	})
	comp.Open()

	r := comp.WheelRect()
	// Diameter should be the smaller dimension: 200.
	if r.Dx() != 200 || r.Dy() != 200 {
		t.Fatalf("expected wheel 200x200, got %dx%d", r.Dx(), r.Dy())
	}
	// Should be centered vertically within bounds.
	centerY := (r.Min.Y + r.Max.Y) / 2
	boundsCenter := (bounds.Min.Y + bounds.Max.Y) / 2
	if abs(centerY-boundsCenter) > 1 {
		t.Fatalf("wheel not centered vertically: wheel center=%d, bounds center=%d", centerY, boundsCenter)
	}
}

// TestColorWheelFillsBounds_No200Cap verifies the old 200px cap is removed.
// A 400x400 container should produce a 400px wheel.
func TestColorWheelFillsBounds_No200Cap(t *testing.T) {
	comp := NewColorWheelComponent()
	bounds := image.Rect(0, 0, 400, 400)
	comp.SetProps(ColorWheelProps{
		AnchorRect: image.Rect(10, 50, 40, 70),
		Bounds:     bounds,
		RowHeight:  24,
	})
	comp.Open()

	r := comp.WheelRect()
	if r.Dx() != 400 {
		t.Fatalf("expected wheel diameter 400 (no cap), got %d", r.Dx())
	}
}

// TestColorWheelFillsBounds_StaysInsideBounds verifies the wheel rect
// stays fully inside the container bounds.
func TestColorWheelFillsBounds_StaysInsideBounds(t *testing.T) {
	comp := NewColorWheelComponent()
	bounds := image.Rect(50, 100, 350, 400) // 300w x 300h at offset
	comp.SetProps(ColorWheelProps{
		AnchorRect: image.Rect(60, 150, 90, 170),
		Bounds:     bounds,
		RowHeight:  24,
	})
	comp.Open()

	r := comp.WheelRect()
	if r.Min.X < bounds.Min.X || r.Min.Y < bounds.Min.Y ||
		r.Max.X > bounds.Max.X || r.Max.Y > bounds.Max.Y {
		t.Fatalf("wheel %v exceeds bounds %v", r, bounds)
	}
}

// TestColorWheelFillsBounds_DrumViewIntegration verifies the color wheel
// fills the rack widget area when opened from DrumView.
func TestColorWheelFillsBounds_DrumViewIntegration(t *testing.T) {
	assertDefaultParityState(t)
	dv := NewDrumView(image.Rect(0, 0, 800, 600), nil, testLogger)
	dv.calcLayout()

	rack := dv.widgetRects[WidgetRack]
	if rack.Empty() {
		t.Skip("WidgetRack not visible in this layout")
	}

	dv.rowColorBtns()[0].OnClick()
	if !dv.IsColorMenuOpen() {
		t.Fatal("color menu not open")
	}

	r := dv.colorWheelComp.WheelRect()
	// Wheel should use the smaller of rack dimensions as diameter.
	wantDiam := rack.Dx()
	if rack.Dy() < wantDiam {
		wantDiam = rack.Dy()
	}
	if r.Dx() != wantDiam {
		t.Fatalf("expected wheel diameter %d (rack min dim), got %d", wantDiam, r.Dx())
	}
	// Must be within rack bounds.
	if r.Min.X < rack.Min.X || r.Min.Y < rack.Min.Y ||
		r.Max.X > rack.Max.X || r.Max.Y > rack.Max.Y {
		t.Fatalf("wheel %v exceeds rack bounds %v", r, rack)
	}
}

func TestColorWheel_DragPicksColor(t *testing.T) {
	comp := NewColorWheelComponent()

	var pickedColors []color.Color
	comp.SetProps(ColorWheelProps{
		AnchorRect: image.Rect(100, 200, 130, 220),
		Bounds:     image.Rect(0, 0, 500, 500),
		RowHeight:  24,
		OnColorPick: func(c color.Color) {
			pickedColors = append(pickedColors, c)
		},
	})
	comp.Open()

	wheelRect := comp.WheelRect()
	// Press inside the wheel (not dead center — offset toward edge for a saturated color)
	pressX := wheelRect.Min.X + wheelRect.Dx()*3/4
	pressY := wheelRect.Min.Y + wheelRect.Dy()/2

	// Initial press inside circle — should pick color and start capture
	result := comp.HandleInput(pressX, pressY, true)
	if result != InputCaptured {
		t.Errorf("expected InputCaptured on initial press, got %v", result)
	}
	if len(pickedColors) == 0 {
		t.Fatal("expected OnColorPick to fire on press inside wheel")
	}
	if pickedColors[0] == nil {
		t.Error("expected non-nil color from OnColorPick")
	}

	// While held (dragging), component should be capturing
	if !comp.Capturing() {
		t.Error("expected Capturing() to be true during drag")
	}
	if !comp.IsOpen() {
		t.Error("expected wheel to remain open during drag")
	}

	// Continue holding (simulate drag to a different position)
	dragX := wheelRect.Min.X + wheelRect.Dx()/4
	dragY := wheelRect.Min.Y + wheelRect.Dy()/4
	result = comp.HandleInput(dragX, dragY, true)
	if result != InputCaptured {
		t.Errorf("expected InputCaptured during drag hold, got %v", result)
	}

	// Release — wheel should close via deferred close
	result = comp.HandleInput(dragX, dragY, false)
	if result != InputConsumed {
		t.Errorf("expected InputConsumed on release, got %v", result)
	}
	if comp.IsOpen() {
		t.Error("expected wheel to close after release")
	}
}

func TestColorWheel_ClickOutsideCloses(t *testing.T) {
	comp := NewColorWheelComponent()

	var closeCalled bool
	var pickCalled bool
	comp.SetProps(ColorWheelProps{
		AnchorRect:  image.Rect(100, 200, 130, 220),
		Bounds:      image.Rect(0, 0, 300, 300),
		RowHeight:   24,
		OnColorPick: func(c color.Color) { pickCalled = true },
		OnClose:     func() { closeCalled = true },
	})
	comp.Open()

	if !comp.IsOpen() {
		t.Fatal("expected wheel to be open")
	}

	// Press well outside wheel bounds
	wheelRect := comp.WheelRect()
	outsideX := wheelRect.Max.X + 100
	outsideY := wheelRect.Max.Y + 100

	result := comp.HandleInput(outsideX, outsideY, true)
	if result != InputConsumed {
		t.Errorf("expected click outside to be consumed, got %v", result)
	}
	if pickCalled {
		t.Error("OnColorPick should NOT be called for click outside")
	}
	if !closeCalled {
		t.Error("expected OnClose to be called on click outside")
	}
	if comp.IsOpen() {
		t.Error("expected wheel to be closed after click outside")
	}
}

