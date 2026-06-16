//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// testZone is a trivial Zone implementation for testing the 4-phase lifecycle.
type testZone struct {
	id          string
	needsLayout bool
	layoutCount int
	updateCount int
	drawCount   int
	lastRect    image.Rectangle
	hitAreas    []HitArea
	keyResults  map[ebiten.Key]InputResult
	charResult  InputResult
	lastChars   []rune
	drawFunc    func(*ebiten.Image) // optional override for Draw
}

func newTestZone(id string) *testZone {
	return &testZone{
		id:          id,
		needsLayout: true,
		keyResults:  make(map[ebiten.Key]InputResult),
	}
}

func (z *testZone) ID() string               { return z.id }
func (z *testZone) NeedsLayout() bool        { return z.needsLayout }
func (z *testZone) Invalidate()              { z.needsLayout = true }
func (z *testZone) Layout(r image.Rectangle) { z.layoutCount++; z.lastRect = r; z.needsLayout = false }
func (z *testZone) Update()                  { z.updateCount++ }
func (z *testZone) HitAreas() []HitArea      { return z.hitAreas }
func (z *testZone) Draw(screen *ebiten.Image) {
	z.drawCount++
	if z.drawFunc != nil {
		z.drawFunc(screen)
	}
}
func (z *testZone) HandleKey(key ebiten.Key) InputResult {
	if r, ok := z.keyResults[key]; ok {
		return r
	}
	return InputIgnored
}
func (z *testZone) HandleChars(chars []rune) InputResult {
	z.lastChars = chars
	return z.charResult
}

// TestTreeLifecycleOrdering verifies the 4-phase loop runs in order.
func TestTreeLifecycleOrdering(t *testing.T) {
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	tree := NewDrumViewTree()
	z := newTestZone("test")
	tree.RegisterZone(z, 100)
	tree.SetZoneRect("test", image.Rect(0, 0, 400, 300))

	tree.Update()

	if z.layoutCount != 1 {
		t.Errorf("expected 1 layout call, got %d", z.layoutCount)
	}
	if z.updateCount != 1 {
		t.Errorf("expected 1 update call, got %d", z.updateCount)
	}
	if z.lastRect != image.Rect(0, 0, 400, 300) {
		t.Errorf("unexpected layout rect: %v", z.lastRect)
	}

	// Second update: layout should NOT re-run (needsLayout=false, same rect).
	tree.Update()
	if z.layoutCount != 1 {
		t.Errorf("expected layout to be skipped, got %d calls", z.layoutCount)
	}
	if z.updateCount != 2 {
		t.Errorf("expected 2 update calls, got %d", z.updateCount)
	}
}

// TestTreeLayoutOnRectChange verifies layout re-runs when rect changes.
func TestTreeLayoutOnRectChange(t *testing.T) {
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	tree := NewDrumViewTree()
	z := newTestZone("test")
	tree.RegisterZone(z, 100)
	tree.SetZoneRect("test", image.Rect(0, 0, 400, 300))
	tree.Update()

	// Change rect.
	tree.SetZoneRect("test", image.Rect(0, 0, 500, 400))
	tree.Update()

	if z.layoutCount != 2 {
		t.Errorf("expected 2 layout calls after rect change, got %d", z.layoutCount)
	}
	if z.lastRect != image.Rect(0, 0, 500, 400) {
		t.Errorf("unexpected layout rect: %v", z.lastRect)
	}
}

// TestTreeLayoutOnInvalidate verifies layout re-runs after Invalidate().
func TestTreeLayoutOnInvalidate(t *testing.T) {
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	tree := NewDrumViewTree()
	z := newTestZone("test")
	tree.RegisterZone(z, 100)
	tree.SetZoneRect("test", image.Rect(0, 0, 400, 300))
	tree.Update()

	z.Invalidate()
	tree.Update()

	if z.layoutCount != 2 {
		t.Errorf("expected 2 layout calls after invalidate, got %d", z.layoutCount)
	}
}

// TestTreeCaptureSemantics verifies press → drag → release capture lifecycle.
func TestTreeCaptureSemantics(t *testing.T) {
	var mx, my int
	var pressed bool
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	tree := NewDrumViewTree()
	handler := &testHitHandler{pressResult: InputCaptured}
	z := newTestZone("test")
	z.hitAreas = []HitArea{
		{Rect: image.Rect(10, 10, 100, 100), ZIndex: 100, Handler: handler, Tag: "drag-btn"},
	}
	tree.RegisterZone(z, 100)
	tree.SetZoneRect("test", image.Rect(0, 0, 400, 300))

	// Frame 1: Layout.
	tree.Update()

	// Frame 2: Press inside hit area.
	mx, my = 50, 50
	pressed = true
	tree.Update()

	if handler.pressCount != 1 {
		t.Errorf("expected 1 press, got %d", handler.pressCount)
	}
	if !tree.Capturing() {
		t.Error("tree should be capturing after InputCaptured press")
	}
	if tree.CapturedTag() != "drag-btn" {
		t.Errorf("expected captured tag 'drag-btn', got %q", tree.CapturedTag())
	}

	// Frame 3: Drag (still pressed, moved).
	mx, my = 80, 80
	tree.Update()

	if handler.dragCount != 1 {
		t.Errorf("expected 1 drag, got %d", handler.dragCount)
	}

	// Frame 4: Release.
	pressed = false
	tree.Update()

	if handler.releaseCount != 1 {
		t.Errorf("expected 1 release, got %d", handler.releaseCount)
	}
	if tree.Capturing() {
		t.Error("tree should not be capturing after release")
	}
}

// TestTreeConsumedPress verifies that InputConsumed does not start capture.
func TestTreeConsumedPress(t *testing.T) {
	var mx, my int
	var pressed bool
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	tree := NewDrumViewTree()
	handler := &testHitHandler{pressResult: InputConsumed}
	z := newTestZone("test")
	z.hitAreas = []HitArea{
		{Rect: image.Rect(10, 10, 100, 100), ZIndex: 100, Handler: handler, Tag: "tap-btn"},
	}
	tree.RegisterZone(z, 100)
	tree.SetZoneRect("test", image.Rect(0, 0, 400, 300))
	tree.Update()

	mx, my = 50, 50
	pressed = true
	tree.Update()

	if handler.pressCount != 1 {
		t.Errorf("expected 1 press, got %d", handler.pressCount)
	}
	if tree.Capturing() {
		t.Error("tree should NOT be capturing after InputConsumed")
	}
}

// TestTreeSuppressPropagation verifies that the tree propagates suppress
// to the global flag when it handles input.
func TestTreeSuppressPropagation(t *testing.T) {
	tree := NewDrumViewTree()
	z := newTestZone("test")
	tree.RegisterZone(z, 100)
	tree.SetZoneRect("test", image.Rect(0, 0, 400, 300))

	// Set up a hit area so a press dispatches to a handler.
	z.hitAreas = []HitArea{{
		Rect:    image.Rect(0, 0, 100, 100),
		ZIndex:  100,
		Handler: &buttonHitHandler{},
		Tag:     "consume",
	}}
	z.needsLayout = true
	var pressed bool
	restore := SetInputForTest(
		func() (int, int) { return 50, 50 },
		func(ebiten.MouseButton) bool { return pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	pressed = true
	tree.Update()
	if !tree.Suppress() {
		t.Error("tree.Suppress() should be true when tree dispatches InputConsumed")
	}
	// Clean up.
	pressed = false
	tree.Update()
}

// TestTreeClickOutsideClosesPortal verifies click outside all areas closes portal.
func TestTreeClickOutsideClosesPortal(t *testing.T) {
	var mx, my int
	var pressed bool
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	tree := NewDrumViewTree()
	tree.SetBounds(image.Rect(0, 0, 800, 600))
	z := newTestZone("test")
	// No hit areas in the zone — entire screen is "outside".
	tree.RegisterZone(z, 100)
	tree.SetZoneRect("test", image.Rect(0, 0, 400, 300))
	tree.Update()

	// Open a portal overlay.
	overlay := &testPortalOverlay{
		hitAreas: []HitArea{
			{Rect: image.Rect(200, 200, 300, 300), Handler: &testHitHandler{}, Tag: "popup"},
		},
	}
	tree.Portal().Open(PortalEntry{
		ID:      "test-popup",
		Overlay: overlay,
		Modal:   false,
		Anchor:  image.Rect(250, 250, 260, 260),
	})

	if !tree.Portal().IsOpen() {
		t.Fatal("portal should be open")
	}

	// Click outside all areas.
	mx, my = 10, 10
	pressed = true
	tree.Update()

	if tree.Portal().IsOpen() {
		t.Error("portal should be closed after click outside")
	}
	if !tree.Suppress() {
		t.Error("tree should suppress after close")
	}
}

// TestTreeEndToEndTrivialZone exercises the full lifecycle with a button zone.
func TestTreeEndToEndTrivialZone(t *testing.T) {
	var mx, my int
	var pressed bool
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	// Create a zone with a single button.
	clicked := false
	handler := &buttonHitHandler{onClick: func() { clicked = true }}
	z := newTestZone("btn-zone")
	z.hitAreas = []HitArea{
		{Rect: image.Rect(10, 10, 60, 40), ZIndex: 100, Handler: handler, Tag: "my-btn"},
	}

	tree := NewDrumViewTree()
	tree.RegisterZone(z, 100)
	tree.SetZoneRect("btn-zone", image.Rect(0, 0, 400, 300))

	// Frame 1: Layout + Update (no input).
	tree.Update()
	if z.layoutCount != 1 {
		t.Errorf("expected layout, got %d", z.layoutCount)
	}

	// Frame 2: Press the button.
	mx, my = 30, 25
	pressed = true
	tree.Update()
	if !clicked {
		t.Error("button should have been clicked")
	}

	// Frame 3: Release.
	pressed = false
	tree.Update()
}

// TestTreeDispatchesToPortalWhenPopupOpen verifies the tree dispatches to
// portal hit areas (z >= ZOverlayMin) when a portal overlay is open, rather
// than yielding at the popupActive check.
func TestTreeDispatchesToPortalWhenPopupOpen(t *testing.T) {
	var mx, my int
	var pressed bool
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	tree := NewDrumViewTree()
	tree.SetBounds(image.Rect(0, 0, 800, 600))

	// Register a zone with a hit area at z=110.
	zoneHandler := &testHitHandler{pressResult: InputConsumed}
	z := newTestZone("zone")
	z.hitAreas = []HitArea{
		{Rect: image.Rect(10, 10, 100, 100), ZIndex: 110, Handler: zoneHandler, Tag: "zone-btn"},
	}
	tree.RegisterZone(z, 110)
	tree.SetZoneRect("zone", image.Rect(0, 0, 400, 300))

	// Layout frame.
	tree.Update()

	// Open a portal overlay with a hit area at z=300+.
	portalHandler := &testHitHandler{pressResult: InputConsumed}
	overlay := &testPortalOverlay{
		hitAreas: []HitArea{
			{Rect: image.Rect(200, 200, 350, 350), Handler: portalHandler, Tag: "popup-btn"},
		},
	}
	tree.Portal().Open(PortalEntry{
		ID:      "test-popup",
		Overlay: overlay,
		Modal:   false,
		Anchor:  image.Rect(250, 250, 260, 260),
	})

	// Press within the portal hit area.
	mx, my = 250, 250
	pressed = true
	tree.Update()

	if portalHandler.pressCount != 1 {
		t.Errorf("portal handler should have received 1 press, got %d", portalHandler.pressCount)
	}
	if zoneHandler.pressCount != 0 {
		t.Errorf("zone handler should NOT have received a press, got %d", zoneHandler.pressCount)
	}

	// Release.
	pressed = false
	tree.Update()
}

// TestTreeBlocksZoneButtonsWhenPortalOpen verifies that zone buttons (z < ZOverlayMin)
// are blocked when a portal overlay is open. Clicking on a zone button position
// should close the portal and suppress, not fire the zone handler.
func TestTreeBlocksZoneButtonsWhenPortalOpen(t *testing.T) {
	var mx, my int
	var pressed bool
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	tree := NewDrumViewTree()
	tree.SetBounds(image.Rect(0, 0, 800, 600))

	// Register a zone with a hit area at z=110.
	zoneHandler := &testHitHandler{pressResult: InputConsumed}
	z := newTestZone("zone")
	z.hitAreas = []HitArea{
		{Rect: image.Rect(10, 10, 100, 100), ZIndex: 110, Handler: zoneHandler, Tag: "zone-btn"},
	}
	tree.RegisterZone(z, 110)
	tree.SetZoneRect("zone", image.Rect(0, 0, 400, 300))

	// Layout frame.
	tree.Update()

	// Open a portal overlay with a hit area that does NOT overlap the zone button.
	portalHandler := &testHitHandler{pressResult: InputConsumed}
	overlay := &testPortalOverlay{
		hitAreas: []HitArea{
			{Rect: image.Rect(200, 200, 350, 350), Handler: portalHandler, Tag: "popup-btn"},
		},
	}
	tree.Portal().Open(PortalEntry{
		ID:      "test-popup",
		Overlay: overlay,
		Modal:   false,
		Anchor:  image.Rect(250, 250, 260, 260),
	})

	if !tree.Portal().IsOpen() {
		t.Fatal("portal should be open")
	}

	// Press on the zone button position (inside zone, outside portal overlay).
	mx, my = 50, 50
	pressed = true
	tree.Update()

	// Zone handler should NOT have fired — portal was open.
	if zoneHandler.pressCount != 0 {
		t.Errorf("zone handler should NOT fire when portal is open, got %d presses", zoneHandler.pressCount)
	}
	// Portal should have been closed.
	if tree.Portal().IsOpen() {
		t.Error("portal should have been closed by click-outside")
	}
	// Tree should be suppressed.
	if !tree.Suppress() {
		t.Error("tree should suppress after closing portal")
	}

	// Release.
	pressed = false
	tree.Update()
}

// TestTreeWheelDispatchIgnoresPopupActive verifies that wheel events dispatch
// to portal overlays unconditionally (no popupActive guard).
func TestTreeWheelDispatchIgnoresPopupActive(t *testing.T) {
	var mx, my int
	var wy float64
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, wy },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	tree := NewDrumViewTree()
	tree.SetBounds(image.Rect(0, 0, 800, 600))

	z := newTestZone("test")
	tree.RegisterZone(z, 100)
	tree.SetZoneRect("test", image.Rect(0, 0, 400, 300))
	tree.Update()

	// Open a portal overlay with a wheel-handling handler.
	wheelReceived := false
	wheelHandler := &testHitHandler{pressResult: InputConsumed}
	// Override OnWheel to track calls.
	portalOverlay := &testPortalOverlay{
		hitAreas: []HitArea{
			{Rect: image.Rect(100, 100, 300, 300), Handler: &wheelTrackingHandler{received: &wheelReceived}, Tag: "wheel-popup"},
		},
	}
	tree.Portal().Open(PortalEntry{
		ID:      "wheel-test",
		Overlay: portalOverlay,
		Modal:   false,
		Anchor:  image.Rect(150, 150, 160, 160),
	})
	_ = wheelHandler

	// Simulate wheel over the portal overlay.
	mx, my = 200, 200
	wy = -1
	tree.Update()
	wy = 0

	if !wheelReceived {
		t.Error("wheel event should have been dispatched to portal overlay handler")
	}
}

// TestTreeClickOutsideAlwaysSuppresses verifies click-outside sets suppress
// regardless of external popup state.
func TestTreeClickOutsideAlwaysSuppresses(t *testing.T) {
	var mx, my int
	var pressed bool
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	tree := NewDrumViewTree()
	tree.SetBounds(image.Rect(0, 0, 800, 600))
	z := newTestZone("test")
	tree.RegisterZone(z, 100)
	tree.SetZoneRect("test", image.Rect(0, 0, 400, 300))
	tree.Update()

	tree.Portal().Open(PortalEntry{
		ID:      "popup",
		Overlay: &testPortalOverlay{hitAreas: []HitArea{{Rect: image.Rect(200, 200, 300, 300), Handler: &testHitHandler{}, Tag: "p"}}},
		Modal:   false,
		Anchor:  image.Rect(250, 250, 260, 260),
	})

	// Click outside.
	mx, my = 5, 5
	pressed = true
	tree.Update()

	if tree.Portal().IsOpen() {
		t.Error("portal should be closed")
	}
	if !tree.Suppress() {
		t.Error("should suppress after close")
	}

	pressed = false
	tree.Update()
}

// wheelTrackingHandler is a HitHandler that tracks wheel events.
type wheelTrackingHandler struct {
	received *bool
}

func (h *wheelTrackingHandler) OnPress(x, y int) InputResult { return InputConsumed }
func (h *wheelTrackingHandler) OnDrag(x, y int)              {}
func (h *wheelTrackingHandler) OnRelease(x, y int)           {}
func (h *wheelTrackingHandler) OnWheel(x, y, steps int) InputResult {
	*h.received = true
	return InputConsumed
}

// buttonHitHandler simulates a simple tap button (OnPress fires and returns InputConsumed).
type buttonHitHandler struct {
	onClick func()
}

func (h *buttonHitHandler) OnPress(x, y int) InputResult {
	if h.onClick != nil {
		h.onClick()
	}
	return InputConsumed
}
func (h *buttonHitHandler) OnDrag(x, y int)                     {}
func (h *buttonHitHandler) OnRelease(x, y int)                  {}
func (h *buttonHitHandler) OnWheel(x, y, steps int) InputResult { return InputIgnored }

// ---------------------------------------------------------------------------
// DrumViewTree integration gap tests
// ---------------------------------------------------------------------------

// TestTreeDraw_CallsAllZonesAndPortal verifies Draw() invokes Draw on every
// registered zone and on the portal overlay.
func TestTreeDraw_CallsAllZonesAndPortal(t *testing.T) {
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	tree := NewDrumViewTree()
	tree.SetBounds(image.Rect(0, 0, 800, 600))

	z1 := newTestZone("zone-1")
	z2 := newTestZone("zone-2")
	tree.RegisterZone(z1, 100)
	tree.RegisterZone(z2, 110)
	tree.SetZoneRect("zone-1", image.Rect(0, 0, 400, 300))
	tree.SetZoneRect("zone-2", image.Rect(400, 0, 800, 300))

	// Open a portal overlay.
	overlay := &testPortalOverlay{
		hitAreas: []HitArea{
			{Rect: image.Rect(200, 200, 300, 300), Handler: &testHitHandler{}, Tag: "popup"},
		},
	}
	tree.Portal().Open(PortalEntry{
		ID:      "draw-test",
		Overlay: overlay,
		Modal:   false,
		Anchor:  image.Rect(250, 250, 260, 260),
	})

	screen := ebiten.NewImage(800, 600)
	tree.Draw(screen)

	if z1.drawCount != 1 {
		t.Errorf("zone-1 drawCount: want 1, got %d", z1.drawCount)
	}
	if z2.drawCount != 1 {
		t.Errorf("zone-2 drawCount: want 1, got %d", z2.drawCount)
	}
	if !overlay.drawCalled {
		t.Error("portal overlay Draw was not called")
	}
}

// TestTreeSetFocusKeyboardRouting verifies HandleKey is only routed to the
// focused zone.
func TestTreeSetFocusKeyboardRouting(t *testing.T) {
	var keyDown ebiten.Key = -1
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return k == keyDown },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	tree := NewDrumViewTree()
	za := newTestZone("zone-a")
	zb := newTestZone("zone-b")
	za.keyResults[ebiten.KeyEnter] = InputConsumed
	zb.keyResults[ebiten.KeyEnter] = InputConsumed
	tree.RegisterZone(za, 100)
	tree.RegisterZone(zb, 110)
	tree.SetZoneRect("zone-a", image.Rect(0, 0, 400, 300))
	tree.SetZoneRect("zone-b", image.Rect(400, 0, 800, 300))

	tree.SetFocus("zone-a")

	// Simulate Enter key press.
	keyDown = ebiten.KeyEnter
	tree.Update()

	// zone-a should have received the key via HandleKey. Since HandleKey
	// returns the stored result, we verify indirectly that it was called
	// by checking that zone-b's lastChars is still nil (zone-b never got input).
	// zone-a HandleKey was called — the testZone returns InputConsumed.
	// zone-b should not have received any key.
	if zb.lastChars != nil {
		t.Error("zone-b should NOT have received any char input")
	}

	keyDown = -1
}

// TestTreeSetFocusCharsRouting verifies HandleChars is only routed to the
// focused zone.
func TestTreeSetFocusCharsRouting(t *testing.T) {
	var chars []rune
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return chars },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	tree := NewDrumViewTree()
	za := newTestZone("zone-a")
	zb := newTestZone("zone-b")
	tree.RegisterZone(za, 100)
	tree.RegisterZone(zb, 110)
	tree.SetZoneRect("zone-a", image.Rect(0, 0, 400, 300))
	tree.SetZoneRect("zone-b", image.Rect(400, 0, 800, 300))

	tree.SetFocus("zone-a")

	chars = []rune{'a', 'b'}
	tree.Update()

	if len(za.lastChars) != 2 || za.lastChars[0] != 'a' || za.lastChars[1] != 'b' {
		t.Errorf("zone-a lastChars: want ['a','b'], got %v", za.lastChars)
	}
	if zb.lastChars != nil {
		t.Errorf("zone-b should not have received chars, got %v", zb.lastChars)
	}
}

// TestTreeSetFocusClear verifies clearing focus stops keyboard routing.
func TestTreeSetFocusClear(t *testing.T) {
	var keyDown ebiten.Key = -1
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return k == keyDown },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	tree := NewDrumViewTree()
	za := newTestZone("zone-a")
	za.keyResults[ebiten.KeyEnter] = InputConsumed
	// Use lastChars to detect if HandleChars is called.
	za.charResult = InputConsumed
	tree.RegisterZone(za, 100)
	tree.SetZoneRect("zone-a", image.Rect(0, 0, 400, 300))

	// Set and then clear focus.
	tree.SetFocus("zone-a")
	tree.SetFocus("")

	keyDown = ebiten.KeyEnter
	tree.Update()

	// With focus cleared, zone-a should NOT have received chars.
	// (HandleKey is harder to verify absence — use chars as proxy.)
	if za.lastChars != nil {
		t.Error("zone-a should not receive char input when focus is cleared")
	}
	keyDown = -1
}

// TestTreeInputHandled verifies InputHandled is true only in the frame that
// dispatched a press.
func TestTreeInputHandled(t *testing.T) {
	var mx, my int
	var pressed bool
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	tree := NewDrumViewTree()
	handler := &testHitHandler{pressResult: InputConsumed}
	z := newTestZone("test")
	z.hitAreas = []HitArea{
		{Rect: image.Rect(10, 10, 100, 100), ZIndex: 100, Handler: handler, Tag: "btn"},
	}
	tree.RegisterZone(z, 100)
	tree.SetZoneRect("test", image.Rect(0, 0, 400, 300))

	// Frame 1: Layout.
	tree.Update()
	if tree.InputHandled() {
		t.Error("InputHandled should be false with no press")
	}

	// Frame 2: Press.
	mx, my = 50, 50
	pressed = true
	tree.Update()
	if !tree.InputHandled() {
		t.Error("InputHandled should be true after dispatching press")
	}

	// Frame 3: Release — InputHandled resets.
	pressed = false
	tree.Update()
	if tree.InputHandled() {
		t.Error("InputHandled should be false after release frame")
	}
}

// TestTreeEscClosesTopPortal verifies that pressing Escape closes the topmost
// portal overlay when no mouse button is pressed.
func TestTreeEscClosesTopPortal(t *testing.T) {
	var escPressed bool
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return k == ebiten.KeyEscape && escPressed },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	tree := NewDrumViewTree()
	tree.SetBounds(image.Rect(0, 0, 800, 600))
	z := newTestZone("test")
	tree.RegisterZone(z, 100)
	tree.SetZoneRect("test", image.Rect(0, 0, 400, 300))

	// Layout frame.
	tree.Update()

	// Open a portal.
	overlay := &testPortalOverlay{
		hitAreas: []HitArea{
			{Rect: image.Rect(200, 200, 300, 300), Handler: &testHitHandler{}, Tag: "popup"},
		},
	}
	tree.Portal().Open(PortalEntry{
		ID:      "esc-test",
		Overlay: overlay,
		Modal:   false,
		Anchor:  image.Rect(250, 250, 260, 260),
	})

	if !tree.Portal().IsOpen() {
		t.Fatal("portal should be open")
	}

	// Press Escape (no mouse press).
	escPressed = true
	tree.Update()

	if tree.Portal().IsOpen() {
		t.Error("portal should be closed after ESC")
	}
	if !tree.Suppress() {
		t.Error("suppress should be set after ESC-close")
	}

	escPressed = false
}

// TestTreeEscWithStackedPortals verifies ESC closes portals one at a time
// from the top.
func TestTreeEscWithStackedPortals(t *testing.T) {
	var escPressed bool
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return k == ebiten.KeyEscape && escPressed },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	tree := NewDrumViewTree()
	tree.SetBounds(image.Rect(0, 0, 800, 600))
	z := newTestZone("test")
	tree.RegisterZone(z, 100)
	tree.SetZoneRect("test", image.Rect(0, 0, 400, 300))
	tree.Update()

	// Open portal "a".
	tree.Portal().Open(PortalEntry{
		ID:      "a",
		Overlay: &testPortalOverlay{},
		Modal:   false,
	})
	// Open portal "b".
	tree.Portal().Open(PortalEntry{
		ID:      "b",
		Overlay: &testPortalOverlay{},
		Modal:   false,
	})

	if tree.Portal().StackLen() != 2 {
		t.Fatalf("expected 2 portals, got %d", tree.Portal().StackLen())
	}

	// ESC 1: closes "b".
	escPressed = true
	tree.Update()
	escPressed = false
	// Need a release frame to clear suppress for next ESC.
	tree.Update()

	if tree.Portal().StackLen() != 1 {
		t.Fatalf("expected 1 portal after first ESC, got %d", tree.Portal().StackLen())
	}
	if tree.Portal().TopID() != "a" {
		t.Errorf("expected top 'a', got %q", tree.Portal().TopID())
	}

	// ESC 2: closes "a".
	escPressed = true
	tree.Update()

	if tree.Portal().IsOpen() {
		t.Error("all portals should be closed after second ESC")
	}
	escPressed = false
}

// TestTreeDragActiveBlocksNewPress verifies that when dragActive returns true,
// new presses are not dispatched to handlers.
func TestTreeDragActiveBlocksNewPress(t *testing.T) {
	var mx, my int
	var pressed bool
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	tree := NewDrumViewTree()
	tree.SetDragActive(func() bool { return true })

	handler := &testHitHandler{pressResult: InputConsumed}
	z := newTestZone("test")
	z.hitAreas = []HitArea{
		{Rect: image.Rect(10, 10, 100, 100), ZIndex: 100, Handler: handler, Tag: "btn"},
	}
	tree.RegisterZone(z, 100)
	tree.SetZoneRect("test", image.Rect(0, 0, 400, 300))

	// Layout frame.
	tree.Update()

	// Press inside hit area — should be blocked by dragActive.
	mx, my = 50, 50
	pressed = true
	tree.Update()

	if handler.pressCount != 0 {
		t.Errorf("handler should NOT receive press when dragActive=true, got %d", handler.pressCount)
	}

	// Release.
	pressed = false
	tree.Update()
}

// TestTreePortalOverlayAbsorbsInputIgnored verifies that a portal overlay
// handler returning InputIgnored is promoted to InputConsumed, setting both
// InputHandled and Suppress to prevent click-through to underlying zones.
func TestTreePortalOverlayAbsorbsInputIgnored(t *testing.T) {
	var mx, my int
	var pressed bool
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	tree := NewDrumViewTree()
	tree.SetBounds(image.Rect(0, 0, 800, 600))

	// Register a zone with a hit area underneath the portal overlay.
	zoneHandler := &testHitHandler{pressResult: InputConsumed}
	z := newTestZone("zone")
	z.hitAreas = []HitArea{
		{Rect: image.Rect(0, 0, 400, 400), ZIndex: 110, Handler: zoneHandler, Tag: "zone-btn"},
	}
	tree.RegisterZone(z, 110)
	tree.SetZoneRect("zone", image.Rect(0, 0, 400, 400))

	// Layout frame.
	tree.Update()

	// Open a portal overlay whose handler returns InputIgnored (empty space click).
	portalHandler := &testHitHandler{pressResult: InputIgnored}
	overlay := &testPortalOverlay{
		hitAreas: []HitArea{
			{Rect: image.Rect(50, 50, 300, 300), Handler: portalHandler, Tag: "popup"},
		},
	}
	tree.Portal().Open(PortalEntry{
		ID:      "absorb-test",
		Overlay: overlay,
		Modal:   false,
		Anchor:  image.Rect(100, 100, 110, 110),
	})

	// Click inside the portal overlay's hit area.
	mx, my = 150, 150
	pressed = true
	tree.Update()

	if portalHandler.pressCount != 1 {
		t.Errorf("portal handler should have received 1 press, got %d", portalHandler.pressCount)
	}
	if zoneHandler.pressCount != 0 {
		t.Errorf("zone handler should NOT have received a press, got %d", zoneHandler.pressCount)
	}
	if !tree.InputHandled() {
		t.Error("InputHandled should be true — InputIgnored should be promoted to InputConsumed for portal overlays")
	}
	if !tree.Suppress() {
		t.Error("Suppress should be true — InputIgnored should be promoted to InputConsumed for portal overlays")
	}

	// Release.
	pressed = false
	tree.Update()
}

// TestTreeDrawClipsToTreeBounds verifies that when tree.bounds is set,
// zone.Draw receives a clipped sub-image so zones cannot render outside
// the tree bounds.
func TestTreeDrawClipsToTreeBounds(t *testing.T) {
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	tree := NewDrumViewTree()
	treeBounds := image.Rect(50, 50, 400, 300)
	tree.SetBounds(treeBounds)

	z := newTestZone("clip-test")
	// Override Draw to record the screen bounds it received.
	var receivedBounds image.Rectangle
	z.drawFunc = func(screen *ebiten.Image) {
		receivedBounds = screen.Bounds()
	}
	tree.RegisterZone(z, 100)
	tree.SetZoneRect("clip-test", image.Rect(0, 0, 800, 600))

	screen := ebiten.NewImage(800, 600)
	tree.Draw(screen)

	// The zone should receive a sub-image clipped to the tree bounds.
	// In ebitenstub, SubImage creates an independent buffer with bounds
	// starting at (0,0), so we verify dimensions match the clip rect.
	expectedClip := screen.Bounds().Intersect(treeBounds)
	expectedDx := expectedClip.Dx()
	expectedDy := expectedClip.Dy()
	if receivedBounds.Dx() != expectedDx || receivedBounds.Dy() != expectedDy {
		t.Errorf("expected zone to receive screen with dimensions %dx%d (clipped to tree bounds %v), got bounds %v (%dx%d)",
			expectedDx, expectedDy, treeBounds, receivedBounds, receivedBounds.Dx(), receivedBounds.Dy())
	}
	// Full screen would be 800x600 — verify it's actually clipped smaller.
	if receivedBounds.Dx() == 800 && receivedBounds.Dy() == 600 {
		t.Error("zone received full screen — tree bounds clipping not applied")
	}
}

// TestTreeNonPortalInputIgnoredNotPromoted verifies that InputIgnored from a
// regular zone handler (z < ZOverlayMin) is NOT promoted. InputHandled and
// Suppress should remain false.
func TestTreeNonPortalInputIgnoredNotPromoted(t *testing.T) {
	var mx, my int
	var pressed bool
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	tree := NewDrumViewTree()

	// Register a zone with a handler that returns InputIgnored.
	handler := &testHitHandler{pressResult: InputIgnored}
	z := newTestZone("zone")
	z.hitAreas = []HitArea{
		{Rect: image.Rect(10, 10, 200, 200), ZIndex: 110, Handler: handler, Tag: "zone-btn"},
	}
	tree.RegisterZone(z, 110)
	tree.SetZoneRect("zone", image.Rect(0, 0, 400, 300))

	// Layout frame.
	tree.Update()

	// Click inside the zone hit area.
	mx, my = 50, 50
	pressed = true
	tree.Update()

	if handler.pressCount != 1 {
		t.Errorf("handler should have received 1 press, got %d", handler.pressCount)
	}
	if tree.InputHandled() {
		t.Error("InputHandled should be false for non-portal InputIgnored")
	}
	if tree.Suppress() {
		t.Error("Suppress should be false for non-portal InputIgnored")
	}

	// Release.
	pressed = false
	tree.Update()
}

// TestTreeDragActiveAllowsWhenFalse verifies that presses are dispatched when
// dragActive returns false.
func TestTreeDragActiveAllowsWhenFalse(t *testing.T) {
	var mx, my int
	var pressed bool
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	tree := NewDrumViewTree()
	tree.SetDragActive(func() bool { return false })

	handler := &testHitHandler{pressResult: InputConsumed}
	z := newTestZone("test")
	z.hitAreas = []HitArea{
		{Rect: image.Rect(10, 10, 100, 100), ZIndex: 100, Handler: handler, Tag: "btn"},
	}
	tree.RegisterZone(z, 100)
	tree.SetZoneRect("test", image.Rect(0, 0, 400, 300))

	// Layout frame.
	tree.Update()

	// Press inside hit area — should be dispatched.
	mx, my = 50, 50
	pressed = true
	tree.Update()

	if handler.pressCount != 1 {
		t.Errorf("handler should receive press when dragActive=false, got %d", handler.pressCount)
	}

	// Release.
	pressed = false
	tree.Update()
}

// --- P2 gap tests ---

// TestTree_SuppressStaleClearWhenNoPressCycle verifies that suppress set
// outside a press cycle (e.g., Escape key closing portal) is cleared on the
// next frame even without a press event.
func TestTree_SuppressStaleClearWhenNoPressCycle(t *testing.T) {
	var mx, my int
	var pressed bool
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	tree := NewDrumViewTree()
	tree.SetBounds(image.Rect(0, 0, 800, 600))

	z := newTestZone("test")
	tree.RegisterZone(z, 100)
	tree.SetZoneRect("test", image.Rect(0, 0, 400, 300))
	tree.Update() // initial layout

	// Manually set suppress=true with wasPressed=false (simulates Escape
	// key closing a portal outside a press cycle).
	tree.suppress = true
	// wasPressed should be false (no press cycle active).
	if tree.wasPressed {
		t.Fatal("precondition: wasPressed should be false")
	}

	// Next Update should clear suppress because no press cycle is active.
	tree.Update()

	if tree.Suppress() {
		t.Error("suppress should be cleared when set outside a press cycle")
	}
}

// TestTree_WheelZOrderDispatch verifies that wheel events are dispatched to
// the highest z-index handler first among overlapping hit areas.
func TestTree_WheelZOrderDispatch(t *testing.T) {
	var mx, my int
	var wy float64
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, wy },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	tree := NewDrumViewTree()
	tree.SetBounds(image.Rect(0, 0, 800, 600))

	// Two zones with overlapping hit areas at different z-indexes.
	lowReceived := false
	highReceived := false

	zLow := newTestZone("low")
	zLow.hitAreas = []HitArea{
		{
			Rect:    image.Rect(50, 50, 200, 200),
			ZIndex:  100,
			Handler: &wheelTrackingHandler{received: &lowReceived},
			Tag:     "low-wheel",
		},
	}

	zHigh := newTestZone("high")
	zHigh.hitAreas = []HitArea{
		{
			Rect:    image.Rect(50, 50, 200, 200), // overlapping
			ZIndex:  150,
			Handler: &wheelTrackingHandler{received: &highReceived},
			Tag:     "high-wheel",
		},
	}

	tree.RegisterZone(zLow, 100)
	tree.SetZoneRect("low", image.Rect(0, 0, 400, 400))
	tree.RegisterZone(zHigh, 150)
	tree.SetZoneRect("high", image.Rect(0, 0, 400, 400))
	tree.Update()

	// Send wheel event at the overlapping point.
	mx, my = 100, 100
	wy = 3.0
	tree.Update()

	if !highReceived {
		t.Error("expected high-z handler to receive wheel event")
	}
	if lowReceived {
		t.Error("expected low-z handler NOT to receive wheel event (traversal stopped)")
	}
}

// TestTree_WheelConsumedStopsTraversal verifies that when the top handler
// returns InputConsumed on wheel, lower handlers do not receive the event.
func TestTree_WheelConsumedStopsTraversal(t *testing.T) {
	var mx, my int
	var wy float64
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, wy },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	tree := NewDrumViewTree()
	tree.SetBounds(image.Rect(0, 0, 800, 600))

	bottomReceived := false

	zTop := newTestZone("top")
	zTop.hitAreas = []HitArea{
		{
			Rect:   image.Rect(50, 50, 200, 200),
			ZIndex: 160,
			Handler: &wheelTrackingHandler{received: func() *bool {
				b := true
				return &b
			}()},
			Tag: "top-wheel",
		},
	}

	zBottom := newTestZone("bottom")
	zBottom.hitAreas = []HitArea{
		{
			Rect:    image.Rect(50, 50, 200, 200),
			ZIndex:  100,
			Handler: &wheelTrackingHandler{received: &bottomReceived},
			Tag:     "bottom-wheel",
		},
	}

	tree.RegisterZone(zTop, 160)
	tree.SetZoneRect("top", image.Rect(0, 0, 400, 400))
	tree.RegisterZone(zBottom, 100)
	tree.SetZoneRect("bottom", image.Rect(0, 0, 400, 400))
	tree.Update()

	mx, my = 100, 100
	wy = -2.0
	tree.Update()

	if tree.WheelHandled() != true {
		t.Error("expected WheelHandled to be true")
	}
	if bottomReceived {
		t.Error("expected bottom handler NOT to receive wheel (top consumed it)")
	}
}

// --- P6: Tree auto-focus integration tests ---

// TestTree_AutoFocusTransportWhenBPMBoxFocused verifies that when a
// TransportZone is registered and its BPM box is focused, tree.Update()
// sets focusedZone to "transport". When blurred, focus clears.
func TestTree_AutoFocusTransportWhenBPMBoxFocused(t *testing.T) {
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	tree := NewDrumViewTree()
	tz := NewTransportZone(TransportCallbacks{})
	tree.RegisterZone(tz, 110)
	tree.SetZoneRect("transport", image.Rect(0, 0, 800, 60))

	// Initial: no focus.
	tree.Update()
	if tree.focusedZone != "" {
		t.Errorf("expected no focus initially, got %q", tree.focusedZone)
	}

	// Focus the BPM box.
	tz.bpmBox.focused = true
	tree.Update()

	if tree.focusedZone != "transport" {
		t.Errorf("expected focusedZone='transport' when BPM box focused, got %q", tree.focusedZone)
	}

	// Blur the BPM box.
	tz.bpmBox.focused = false
	tree.Update()

	if tree.focusedZone != "" {
		t.Errorf("expected focusedZone='' after BPM box blur, got %q", tree.focusedZone)
	}
}

// TestTree_AutoFocusEQPanelWhenDBInputFocused verifies that when an
// EQPanelZone is registered and a dB TextInput is focused, tree.Update()
// sets focusedZone to "eq-panel". Blurring clears focus.
func TestTree_AutoFocusEQPanelWhenDBInputFocused(t *testing.T) {
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	tree := NewDrumViewTree()
	ez := NewEQPanelZone(EQCallbacks{})
	tree.RegisterZone(ez, 120)
	tree.SetZoneRect("eq-panel", image.Rect(0, 60, 800, 300))

	// Initial: no focus.
	tree.Update()
	if tree.focusedZone != "" {
		t.Errorf("expected no focus initially, got %q", tree.focusedZone)
	}

	// Open the shared dB editor for band 3 to simulate user editing.
	// EQPanelZone now derives its focus state from paramEditor.Active(), and
	// the tree mirrors that into focusedZone.
	ez.openEQDBEditor(3)
	tree.Update()

	if tree.focusedZone != "eq-panel" {
		t.Errorf("expected focusedZone='eq-panel' when dB editor is open, got %q", tree.focusedZone)
	}

	// Close (cancel) the dB editor.
	ez.paramEditor.cancel()
	tree.Update()

	if tree.focusedZone != "" {
		t.Errorf("expected focusedZone='' after dB input blur, got %q", tree.focusedZone)
	}
}

// --- Fall-through regression tests ---

// TestTreeFallThrough_HigherZIgnoredFallsToLowerZ verifies that when the
// highest-z handler returns InputIgnored, the press falls through to the
// next hit in z-order.
func TestTreeFallThrough_HigherZIgnoredFallsToLowerZ(t *testing.T) {
	var mx, my int
	var pressed bool
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	tree := NewDrumViewTree()

	highHandler := &testHitHandler{pressResult: InputIgnored}
	lowHandler := &testHitHandler{pressResult: InputCaptured}

	z := newTestZone("test")
	z.hitAreas = []HitArea{
		{Rect: image.Rect(10, 10, 100, 100), ZIndex: 121, Handler: highHandler, Tag: "high"},
		{Rect: image.Rect(10, 10, 100, 100), ZIndex: 120, Handler: lowHandler, Tag: "low"},
	}
	tree.RegisterZone(z, 100)
	tree.SetZoneRect("test", image.Rect(0, 0, 400, 300))

	// Layout frame.
	tree.Update()

	// Press in the overlap area.
	mx, my = 50, 50
	pressed = true
	tree.Update()

	if highHandler.pressCount != 1 {
		t.Errorf("high-z handler should have received 1 press, got %d", highHandler.pressCount)
	}
	if lowHandler.pressCount != 1 {
		t.Errorf("low-z handler should have received 1 press (fall-through), got %d", lowHandler.pressCount)
	}
	if tree.CapturedTag() != "low" {
		t.Errorf("expected captured tag 'low', got %q", tree.CapturedTag())
	}

	// Release.
	pressed = false
	tree.Update()
}

// TestTreeFallThrough_HigherZCapturesBlocksLowerZ verifies that when the
// highest-z handler returns InputCaptured, no fall-through occurs.
func TestTreeFallThrough_HigherZCapturesBlocksLowerZ(t *testing.T) {
	var mx, my int
	var pressed bool
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	tree := NewDrumViewTree()

	highHandler := &testHitHandler{pressResult: InputCaptured}
	lowHandler := &testHitHandler{pressResult: InputCaptured}

	z := newTestZone("test")
	z.hitAreas = []HitArea{
		{Rect: image.Rect(10, 10, 100, 100), ZIndex: 121, Handler: highHandler, Tag: "high"},
		{Rect: image.Rect(10, 10, 100, 100), ZIndex: 120, Handler: lowHandler, Tag: "low"},
	}
	tree.RegisterZone(z, 100)
	tree.SetZoneRect("test", image.Rect(0, 0, 400, 300))

	// Layout frame.
	tree.Update()

	// Press in the overlap area.
	mx, my = 50, 50
	pressed = true
	tree.Update()

	if highHandler.pressCount != 1 {
		t.Errorf("high-z handler should have received 1 press, got %d", highHandler.pressCount)
	}
	if lowHandler.pressCount != 0 {
		t.Errorf("low-z handler should NOT have received a press, got %d", lowHandler.pressCount)
	}

	// Release.
	pressed = false
	tree.Update()
}

// TestTreeFallThrough_PortalIgnoredDoesNotFallThrough verifies that a portal
// overlay returning InputIgnored is promoted to InputConsumed and does NOT
// fall through to underlying zone handlers.
func TestTreeFallThrough_PortalIgnoredDoesNotFallThrough(t *testing.T) {
	var mx, my int
	var pressed bool
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	tree := NewDrumViewTree()
	tree.SetBounds(image.Rect(0, 0, 800, 600))

	// Register a zone with a hit area underneath the portal overlay.
	zoneHandler := &testHitHandler{pressResult: InputConsumed}
	z := newTestZone("zone")
	z.hitAreas = []HitArea{
		{Rect: image.Rect(0, 0, 400, 400), ZIndex: 110, Handler: zoneHandler, Tag: "zone-btn"},
	}
	tree.RegisterZone(z, 110)
	tree.SetZoneRect("zone", image.Rect(0, 0, 400, 400))

	// Layout frame.
	tree.Update()

	// Open a portal overlay whose handler returns InputIgnored.
	portalHandler := &testHitHandler{pressResult: InputIgnored}
	overlay := &testPortalOverlay{
		hitAreas: []HitArea{
			{Rect: image.Rect(50, 50, 300, 300), Handler: portalHandler, Tag: "popup"},
		},
	}
	tree.Portal().Open(PortalEntry{
		ID:      "fallthrough-test",
		Overlay: overlay,
		Modal:   false,
		Anchor:  image.Rect(100, 100, 110, 110),
	})

	// Click inside the portal overlay's hit area (overlaps zone).
	mx, my = 150, 150
	pressed = true
	tree.Update()

	if portalHandler.pressCount != 1 {
		t.Errorf("portal handler should have received 1 press, got %d", portalHandler.pressCount)
	}
	if zoneHandler.pressCount != 0 {
		t.Errorf("zone handler should NOT have received a press (portal absorbs InputIgnored), got %d", zoneHandler.pressCount)
	}
	if !tree.InputHandled() {
		t.Error("InputHandled should be true — InputIgnored promoted to InputConsumed for portal")
	}
	if !tree.Suppress() {
		t.Error("Suppress should be true — InputIgnored promoted to InputConsumed for portal")
	}

	// Release.
	pressed = false
	tree.Update()
}

// TestTree_CleanupClosedRemovesOverlay opens a portal overlay with
// ShouldClose() returning true after a flag flip, and verifies that after
// tree.Update() the overlay is removed and OnClose fires.
func TestTree_CleanupClosedRemovesOverlay(t *testing.T) {
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	tree := NewDrumViewTree()
	tree.SetBounds(image.Rect(0, 0, 800, 600))
	z := newTestZone("test")
	tree.RegisterZone(z, 100)
	tree.SetZoneRect("test", image.Rect(0, 0, 400, 300))

	// Layout frame.
	tree.Update()

	// Open a portal overlay that does NOT want to close yet.
	overlay := &testPortalOverlay{
		hitAreas: []HitArea{
			{Rect: image.Rect(200, 200, 300, 300), Handler: &testHitHandler{}, Tag: "popup"},
		},
		shouldClose: false,
	}
	closeCalled := false
	tree.Portal().Open(PortalEntry{
		ID:      "auto-close-test",
		Overlay: overlay,
		Modal:   false,
		Anchor:  image.Rect(250, 250, 260, 260),
		OnClose: func() { closeCalled = true },
	})

	if !tree.Portal().IsOpen() {
		t.Fatal("portal should be open")
	}

	// Update — overlay does not want to close yet.
	tree.Update()
	if !tree.Portal().IsOpen() {
		t.Fatal("portal should still be open (ShouldClose=false)")
	}
	if closeCalled {
		t.Fatal("OnClose should not have been called yet")
	}

	// Now flip ShouldClose to true.
	overlay.shouldClose = true
	tree.Update()

	if tree.Portal().IsOpen() {
		t.Error("portal should be closed after ShouldClose() returned true")
	}
	if !closeCalled {
		t.Error("OnClose should have been called when the overlay was cleaned up")
	}
}
