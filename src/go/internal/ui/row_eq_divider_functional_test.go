//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestRowEQDivider_GlowsWhenCursorProbeDisabled is the bug repro: on WASM the
// RuntimeProfile disables the cursor-hover probe (SkipCursorHover=true), which
// gates layoutResizeZone.Update — the ONLY thing that set the EQ divider's hover
// state. The main splitter reads the cursor directly in Draw and still glows, so
// the EQ pill must too. Drives the REAL Update+Draw loop with the cursor on the
// pill and asserts the glow ramps regardless of the probe gate.
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

// driveRealEQDivider builds a real Game with a production-FLOORED audio panel
// (eqPanelHeightForTest), settles the layout, and returns the game plus the
// visible divider-pill center the user would aim for. The mx/my/pressed vars
// the caller mutates are wired into the real input functions, so subsequent
// g.Update() calls dispatch through the WHOLE stack (legacy InputDispatcher →
// DrumView → DrumViewTree HitIndex → layoutResizeZone), exactly like a user.
func driveRealEQDivider(t *testing.T) (g *Game, mx, my *int, pressed *bool, px, py int) {
	t.Helper()
	assertDefaultParityState(t)
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
	// Precondition: the panel really is floored (top edge above the widget
	// boundary) — otherwise we're not exercising the production scenario.
	widgetBoundaryY := dv.widgets.rowPos[idx+1] + dv.Bounds.Min.Y
	if dv.eqRect.Min.Y >= widgetBoundaryY {
		t.Fatalf("audio panel not floored (top=%d, widget boundary=%d); functional scenario not reproduced",
			dv.eqRect.Min.Y, widgetBoundaryY)
	}
	hr := dv.layoutHandler.rowHandleRect(idx)
	return g, mx, my, pressed, (hr.Min.X + hr.Max.X) / 2, (hr.Min.Y + hr.Max.Y) / 2
}

// TestRowEQDivider_RealDragResizesPanel drives the full input loop and proves
// the divider is usable: pressing the pill starts a resize, and dragging the
// cursor moves the audio-panel top edge to follow it — both shrinking (below
// the analysis-tab floor) and growing.
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

	// Drag UP to grow the panel past where it started (a modest amount that
	// stays within the clamp range so the top tracks the cursor exactly).
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

// TestRowEQDivider_RealHoverRegisters proves a real cursor hover over the pill
// is picked up by the live frame loop (not just a direct Update() call), so the
// divider gives feedback and is discoverable.
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

// TestRowEQDivider_RealHoverGlows drives the FULL real frame loop — Update()
// (which runs the tree's hover detection) THEN Draw() (which advances the pill's
// glow animation) — with the cursor parked on the visible pill. The glow must
// actually ramp up. The unit animation test set dv.layoutHoverAxis by hand and
// only called the layer's Draw, so it never exercised this cross-component path.
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
