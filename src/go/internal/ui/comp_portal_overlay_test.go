//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// mockCompOverlayable implements compOverlayable for testing.
type mockCompOverlayable struct {
	open       bool
	bounds     image.Rectangle
	capturing  bool
	lastInputX int
	lastInputY int
	lastPress  bool
	inputCalls int
	wheelCalls int
	drawCalls  int
}

func (m *mockCompOverlayable) IsOpen() bool                 { return m.open }
func (m *mockCompOverlayable) Close()                       { m.open = false }
func (m *mockCompOverlayable) InputBounds() image.Rectangle { return m.bounds }
func (m *mockCompOverlayable) Capturing() bool              { return m.capturing }
func (m *mockCompOverlayable) Draw(_ *ebiten.Image) {
	m.drawCalls++
}
func (m *mockCompOverlayable) HandleInput(x, y int, pressed bool) InputResult {
	m.lastInputX = x
	m.lastInputY = y
	m.lastPress = pressed
	m.inputCalls++
	if m.capturing {
		return InputCaptured
	}
	return InputConsumed
}
func (m *mockCompOverlayable) HandleWheel(x, y, steps int) InputResult {
	m.wheelCalls++
	return InputConsumed
}

func TestCompPortalOverlayHitAreas(t *testing.T) {
	comp := &mockCompOverlayable{
		open:   true,
		bounds: image.Rect(10, 20, 110, 120),
	}
	overlay := &compPortalOverlay{comp: comp, tag: "test-comp"}

	areas := overlay.HitAreas()
	if len(areas) != 1 {
		t.Fatalf("expected 1 hit area, got %d", len(areas))
	}
	if areas[0].Tag != "test-comp" {
		t.Errorf("expected tag 'test-comp', got %q", areas[0].Tag)
	}
	if areas[0].Rect != comp.bounds {
		t.Errorf("expected rect %v, got %v", comp.bounds, areas[0].Rect)
	}
}

func TestCompPortalOverlayEmptyBounds(t *testing.T) {
	comp := &mockCompOverlayable{
		open:   true,
		bounds: image.Rectangle{},
	}
	overlay := &compPortalOverlay{comp: comp, tag: "empty"}

	areas := overlay.HitAreas()
	if len(areas) != 0 {
		t.Fatalf("expected 0 hit areas for empty bounds, got %d", len(areas))
	}
}

func TestCompPortalOverlayShouldClose(t *testing.T) {
	comp := &mockCompOverlayable{open: true}
	overlay := &compPortalOverlay{comp: comp, tag: "sc"}

	if overlay.ShouldClose() {
		t.Fatal("should not close when comp is open")
	}
	comp.open = false
	if !overlay.ShouldClose() {
		t.Fatal("should close when comp is closed")
	}
}

func TestCompHitHandlerRouting(t *testing.T) {
	comp := &mockCompOverlayable{
		open:   true,
		bounds: image.Rect(0, 0, 100, 100),
	}
	handler := &compHitHandler{comp: comp}

	// OnPress
	result := handler.OnPress(50, 50)
	if result != InputConsumed {
		t.Errorf("expected InputConsumed from OnPress, got %v", result)
	}
	if comp.lastInputX != 50 || comp.lastInputY != 50 || !comp.lastPress {
		t.Error("OnPress did not route correctly")
	}

	// OnDrag
	handler.OnDrag(60, 70)
	if comp.lastInputX != 60 || comp.lastInputY != 70 || !comp.lastPress {
		t.Error("OnDrag did not route correctly")
	}

	// OnRelease
	handler.OnRelease(80, 90)
	if comp.lastInputX != 80 || comp.lastInputY != 90 || comp.lastPress {
		t.Error("OnRelease did not route correctly")
	}

	// OnWheel
	result = handler.OnWheel(50, 50, 3)
	if result != InputConsumed {
		t.Errorf("expected InputConsumed from OnWheel, got %v", result)
	}
	if comp.wheelCalls != 1 {
		t.Errorf("expected 1 wheel call, got %d", comp.wheelCalls)
	}
}

func TestCompHitHandlerCapture(t *testing.T) {
	comp := &mockCompOverlayable{
		open:      true,
		bounds:    image.Rect(0, 0, 100, 100),
		capturing: true,
	}
	handler := &compHitHandler{comp: comp}

	result := handler.OnPress(50, 50)
	if result != InputCaptured {
		t.Errorf("expected InputCaptured when comp is capturing, got %v", result)
	}
}

func TestSliderPopupPortalOverlay(t *testing.T) {
	var val float64
	popup := NewSliderPopup(SliderPopupConfig{
		ID:       "test-popup",
		GetValue: func() float64 { return val },
		SetValue: func(v float64) { val = v },
	})

	overlay := &sliderPopupPortalOverlay{popup: popup, tag: "vol-popup"}

	// Closed: no hit areas.
	areas := overlay.HitAreas()
	if len(areas) != 0 {
		t.Fatalf("expected 0 hit areas when closed, got %d", len(areas))
	}
	if !overlay.ShouldClose() {
		t.Fatal("should close when popup is closed")
	}

	// Open the popup.
	popup.Open(image.Rect(100, 100, 120, 120), image.Rect(0, 0, 800, 600), 40)
	if overlay.ShouldClose() {
		t.Fatal("should not close when popup is open")
	}

	areas = overlay.HitAreas()
	if len(areas) != 1 {
		t.Fatalf("expected 1 hit area when open, got %d", len(areas))
	}
	if areas[0].Tag != "vol-popup" {
		t.Errorf("expected tag 'vol-popup', got %q", areas[0].Tag)
	}
}

func TestDvOverlayPortal(t *testing.T) {
	open := true
	inputCalls := 0
	wheelCalls := 0
	drawCalls := 0
	rect := image.Rect(10, 10, 200, 300)

	overlay := &dvOverlayPortal{
		id:       "test-dv",
		isOpenFn: func() bool { return open },
		rectFn:   func() image.Rectangle { return rect },
		inputFn: func(x, y int, pressed bool) InputResult {
			inputCalls++
			return InputConsumed
		},
		wheelFn: func(x, y, steps int) InputResult {
			wheelCalls++
			return InputConsumed
		},
		drawFn: func(_ *ebiten.Image) {
			drawCalls++
		},
	}

	// Hit areas.
	areas := overlay.HitAreas()
	if len(areas) != 1 {
		t.Fatalf("expected 1 hit area, got %d", len(areas))
	}
	if areas[0].Rect != rect {
		t.Errorf("expected rect %v, got %v", rect, areas[0].Rect)
	}

	// Input routing.
	handler := areas[0].Handler
	handler.OnPress(50, 50)
	if inputCalls != 1 {
		t.Errorf("expected 1 input call, got %d", inputCalls)
	}
	handler.OnDrag(60, 60)
	if inputCalls != 2 {
		t.Errorf("expected 2 input calls, got %d", inputCalls)
	}
	handler.OnRelease(70, 70)
	if inputCalls != 3 {
		t.Errorf("expected 3 input calls, got %d", inputCalls)
	}

	// Wheel routing.
	handler.OnWheel(50, 50, 3)
	if wheelCalls != 1 {
		t.Errorf("expected 1 wheel call, got %d", wheelCalls)
	}

	// ShouldClose.
	if overlay.ShouldClose() {
		t.Fatal("should not close when open")
	}
	open = false
	if !overlay.ShouldClose() {
		t.Fatal("should close when not open")
	}

	// Empty rect → no hit areas.
	rect = image.Rectangle{}
	areas = overlay.HitAreas()
	if len(areas) != 0 {
		t.Fatalf("expected 0 hit areas for empty rect, got %d", len(areas))
	}
}

func TestPortalUpdater(t *testing.T) {
	idx := &HitIndex{}
	portal := NewOverlayPortal(idx)
	portal.SetScreenBounds(image.Rect(0, 0, 800, 600))

	updateCalls := 0
	overlay := &updaterOverlay{
		testPortalOverlay: testPortalOverlay{},
		updateFn:          func() { updateCalls++ },
	}

	portal.Open(PortalEntry{
		ID:      "updater",
		Overlay: overlay,
	})

	portal.Update()
	if updateCalls != 1 {
		t.Errorf("expected 1 update call, got %d", updateCalls)
	}

	portal.Update()
	if updateCalls != 2 {
		t.Errorf("expected 2 update calls, got %d", updateCalls)
	}
}

// updaterOverlay implements both PortalOverlay and PortalUpdater.
type updaterOverlay struct {
	testPortalOverlay
	updateFn func()
}

func (o *updaterOverlay) Update() {
	if o.updateFn != nil {
		o.updateFn()
	}
}
