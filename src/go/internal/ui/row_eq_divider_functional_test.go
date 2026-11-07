//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// On WASM the cursor-hover probe is off, and it gated the only code that set the EQ divider's hover; the pill
// must glow like the main splitter, which reads the cursor in Draw.
func TestRowEQDivider_GlowsWhenCursorProbeDisabled(t *testing.T) {
	restore := SetRuntimeProfileForTest(browserRuntimeProfile())
	defer restore()
	if !RuntimeProf().SkipCursorHover {
		t.Fatalf("precondition: browser profile should disable the cursor-hover probe")
	}

	g, mx, my, pressed, px, py := driveRealEQDivider(t)
	*pressed = false
	*mx, *my = px, py
	screen := ebiten.NewImage(1280, 720)
	for i := 0; i < 15; i++ {
		_ = g.Update()
		g.Draw(screen)
	}
	if p := g.drum.rowEQDividerLayer().handle.HoverAnim(); p <= 0 {
		t.Fatalf("EQ pill did NOT glow on hover at (%d,%d) with the cursor-hover probe disabled (browser): hoverAnim=%v — its hover relies on the gated layoutResizeZone.Update, unlike the main splitter",
			px, py, p)
	}
}

// With a production-floored audio panel; the caller's mx/my/pressed feed the real input functions, so Update
// dispatches through the whole stack.
func driveRealEQDivider(t *testing.T) (g *Game, mx, my *int, pressed *bool, px, py int) {
	t.Helper()
	assertDefaultParityState(t)
	// A leaked mobile profile makes eqDividerRowIdx return -1; the divider is a desktop feature.
	forceSmallScreenForTest = false
	SetTouchScreenSize(0, 0)
	UpdateProfile()
	t.Cleanup(func() { SetTouchScreenSize(0, 0); UpdateProfile() })
	eqPanelHeightForTest = 190
	t.Cleanup(func() { eqPanelHeightForTest = 0 })

	g = New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)

	mx, my, pressed = new(int), new(int), new(bool)
	restore := SetInputForTest(
		func() (int, int) { return *mx, *my },
		func(b ebiten.MouseButton) bool { return *pressed && b == ebiten.MouseButtonLeft },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 1280, 720 },
	)
	t.Cleanup(restore)

	for i := 0; i < 3; i++ {
		_ = g.Update()
	}

	dv := g.drum
	idx := dv.eqDividerRowIdx()
	if idx < 0 {
		t.Fatalf("no EQ divider present")
	}
	// Otherwise this is not the production scenario.
	widgetBoundaryY := dv.widgets.rowPos[idx+1] + dv.Bounds.Min.Y
	if dv.eqRect.Min.Y >= widgetBoundaryY {
		t.Fatalf("audio panel not floored (top=%d, widget boundary=%d); functional scenario not reproduced",
			dv.eqRect.Min.Y, widgetBoundaryY)
	}
	hr := dv.layoutHandler.rowHandleRect(idx)
	return g, mx, my, pressed, (hr.Min.X + hr.Max.X) / 2, (hr.Min.Y + hr.Max.Y) / 2
}

// Pressing the pill starts a resize, and the panel top follows the cursor both ways, shrinking below the
// analysis-tab floor too.
func TestRowEQDivider_RealDragResizesPanel(t *testing.T) {
	g, mx, my, pressed, px, py := driveRealEQDivider(t)
	dv := g.drum

	// Press on the visible pill, through the real dispatch.
	*mx, *my = px, py
	*pressed = true
	_ = g.Update()
	if !dv.layoutHandler.dragging {
		t.Fatalf("pressing the visible divider pill at (%d,%d) did NOT start a resize drag — unusable", px, py)
	}

	// Drag DOWN to shrink the panel (below the floor it was pinned at).
	dragTo := func(targetY int) {
		// Move in small steps like a real pointer.
		from := *my
		steps := 6
		for s := 1; s <= steps; s++ {
			*my = from + (targetY-from)*s/steps
			_ = g.Update()
		}
	}
	startTop := dv.eqRect.Min.Y
	dragTo(py + 80) // pull the divider down
	shrunkTop := dv.eqRect.Min.Y
	if shrunkTop <= startTop {
		t.Fatalf("dragging the divider down did not shrink the panel: top %d -> %d", startTop, shrunkTop)
	}
	// The panel top must follow the cursor (the user drags it where they point).
	if d := shrunkTop - *my; d < -3 || d > 3 {
		t.Errorf("after drag-down the panel top %d does not track the cursor y=%d", shrunkTop, *my)
	}

	// Within the clamp range, so the top tracks exactly.
	dragTo(py - 40)
	grownTop := dv.eqRect.Min.Y
	if grownTop >= startTop {
		t.Fatalf("dragging the divider up did not grow the panel: top stayed %d (start %d)", grownTop, startTop)
	}
	if d := grownTop - *my; d < -3 || d > 3 {
		t.Errorf("after drag-up the panel top %d does not track the cursor y=%d", grownTop, *my)
	}

	*pressed = false
	_ = g.Update()
}

// Picked up by the live frame loop, not only a direct Update() call.
func TestRowEQDivider_RealHoverRegisters(t *testing.T) {
	g, mx, my, pressed, px, py := driveRealEQDivider(t)
	dv := g.drum
	idx := dv.eqDividerRowIdx()

	*pressed = false
	*mx, *my = px, py
	_ = g.Update()
	if dv.layoutHoverAxis != "row" || dv.layoutHoverIdx != idx {
		t.Fatalf("hovering the visible divider pill at (%d,%d) did not register through the real loop (axis=%q idx=%d)",
			px, py, dv.layoutHoverAxis, dv.layoutHoverIdx)
	}
}

// Update (hover detection) then Draw (glow animation) with the cursor on the pill: the cross-component path a
// hand-set hover never exercised.
func TestRowEQDivider_RealHoverGlows(t *testing.T) {
	g, mx, my, pressed, px, py := driveRealEQDivider(t)
	dv := g.drum
	layer := dv.rowEQDividerLayer()

	*pressed = false
	*mx, *my = px, py // cursor on the visible pill
	screen := ebiten.NewImage(1280, 720)
	for i := 0; i < 15; i++ {
		_ = g.Update()
		g.Draw(screen)
	}

	if layer.handle.HoverAnim() <= 0 {
		t.Fatalf("hovering the EQ pill at (%d,%d) via the REAL Update+Draw loop did not start the glow (hoverAnim=%v); layoutHover=%q/%d eqIdx=%d",
			px, py, layer.handle.HoverAnim(), dv.layoutHoverAxis, dv.layoutHoverIdx, dv.eqDividerRowIdx())
	}
}
