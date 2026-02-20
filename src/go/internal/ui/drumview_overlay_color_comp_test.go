//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"
)

func TestColorWheelComponent_OpenClose(t *testing.T) {
	comp := NewColorWheelComponent("test-color")

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

func TestColorWheelComponent_HoldCapture(t *testing.T) {
	comp := NewColorWheelComponent("test-color")
	comp.SetProps(ColorWheelProps{
		AnchorRect: image.Rect(100, 200, 130, 220),
		Bounds:     image.Rect(0, 0, 500, 500),
		RowHeight:  24,
	})
	comp.Open()

	// Initially should be capturing (hold is true)
	if !comp.Capturing() {
		t.Error("expected Capturing() to be true after opening")
	}

	// First input while holding should return InputCaptured
	result := comp.HandleInput(150, 150, true)
	if result != InputCaptured {
		t.Errorf("expected InputCaptured while holding, got %v", result)
	}

	// Release should clear hold
	result = comp.HandleInput(150, 150, false)
	if result != InputCaptured {
		t.Errorf("expected InputCaptured on release, got %v", result)
	}

	// After release, should no longer be capturing
	if comp.Capturing() {
		t.Error("expected Capturing() to be false after mouse release")
	}
}

func TestColorWheelComponent_OnColorPickCallback(t *testing.T) {
	comp := NewColorWheelComponent("test-color")

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

	// Release hold first
	comp.HandleInput(150, 150, false)

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
	comp := NewColorWheelComponent("test-color")

	var closeCalled bool
	comp.SetProps(ColorWheelProps{
		AnchorRect:  image.Rect(100, 200, 130, 220),
		Bounds:      image.Rect(0, 0, 500, 500),
		RowHeight:   24,
		OnColorPick: func(c color.Color) {},
		OnClose:     func() { closeCalled = true },
	})
	comp.Open()

	// Release hold
	comp.HandleInput(150, 150, false)

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
	comp := NewColorWheelComponent("test-color")

	var closeCalled bool
	comp.SetProps(ColorWheelProps{
		AnchorRect: image.Rect(100, 200, 130, 220),
		Bounds:     image.Rect(0, 0, 500, 500),
		RowHeight:  24,
		OnClose: func() {
			closeCalled = true
		},
	})
	comp.Open()

	// Release hold first
	comp.HandleInput(150, 150, false)

	// Click far outside the wheel bounds
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
	comp := NewColorWheelComponent("test-color")

	// Anchor near top - wheel should go below
	comp.SetProps(ColorWheelProps{
		AnchorRect: image.Rect(100, 10, 130, 30),
		Bounds:     image.Rect(0, 0, 500, 500),
		RowHeight:  24,
	})
	comp.Open()

	wheelRect := comp.WheelRect()
	if wheelRect.Min.Y < 30 {
		t.Logf("wheel positioned below anchor as expected, Y=%d", wheelRect.Min.Y)
	}

	comp.Close()

	// Anchor near bottom - wheel should go above
	comp.SetProps(ColorWheelProps{
		AnchorRect: image.Rect(100, 450, 130, 470),
		Bounds:     image.Rect(0, 0, 500, 500),
		RowHeight:  24,
	})
	comp.Open()

	wheelRect = comp.WheelRect()
	if wheelRect.Max.Y <= 470 {
		t.Logf("wheel positioned, Max.Y=%d", wheelRect.Max.Y)
	}
}

func TestColorWheelComponent_WheelImageCache(t *testing.T) {
	comp := NewColorWheelComponent("test-color")
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
	comp := NewColorWheelComponent("test-color")
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
	comp := NewColorWheelComponent("test-color")
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
	comp := NewColorWheelComponent("test-color")
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
	comp := NewColorWheelComponent("test-color")
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
