//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// firstVisibleFMControlRect returns the global index and centre point of the
// first on-window FM control in the realistic modular synth tab (a section that
// overflows, so the ControlGrid scrolls).
func firstVisibleFMControlRect(t *testing.T, g *Game) (idx, cx, cy int) {
	t.Helper()
	s := fmSection(t, g)
	controls := g.drum.SynthTabControls()
	for _, i := range s.KnobIdxs() {
		if i >= 0 && i < len(controls) && controls[i] != nil && !controls[i].Rect().Empty() {
			r := controls[i].Rect()
			return i, r.Min.X + r.Dx()/2, r.Min.Y + r.Dy()/2
		}
	}
	t.Fatal("no visible FM control rect")
	return -1, 0, 0
}

// driveDrumInput drives one real DrumView.Update frame with the pointer at
// (x,y) and the given left-button state, through the production input path.
func driveDrumInput(g *Game, x, y int, down bool) {
	restore := SetInputForTest(
		func() (int, int) { return x, y },
		func(b ebiten.MouseButton) bool { return down && b == ebiten.MouseButtonLeft },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return g.winW, g.winH },
	)
	g.drum.Update()
	restore()
}

// TestSynthControl_VerticalDragScrollsAndDoesNotOpenPopup: a press that begins
// on a control and sweeps vertically past the tap slop must SCROLL the section
// grid and must NOT open the value-wheel popup. This is the mobile "swipe
// up/down" gesture — currently broken because OnPress opens the popup outright.
func TestSynthControl_VerticalDragScrollsAndDoesNotOpenPopup(t *testing.T) {
	g := modularSynthGameRealistic(t)
	grid := g.drum.SynthSectionGrid(synthSectionFM)
	if grid == nil || !grid.HasScroll() {
		t.Skip("FM section does not overflow at this layout; nothing to scroll")
	}
	_, cx, cy := firstVisibleFMControlRect(t, g)
	before := grid.Scroll().VS.First

	// Press on the control, then sweep UP well past the tap slop across a few
	// frames (content follows the finger → First increases), then release.
	driveDrumInput(g, cx, cy, true)
	for dy := 20; dy <= 120; dy += 20 {
		driveDrumInput(g, cx, cy-dy, true)
	}
	driveDrumInput(g, cx, cy-120, false)

	if got := grid.Scroll().VS.First; got == before {
		t.Fatalf("vertical drag on a control did not scroll the grid: First stayed %d", before)
	}
	if g.drum.synthWheelPopup != nil && g.drum.synthWheelPopup.IsOpen() {
		t.Fatal("a scroll gesture opened the value-wheel popup; it must stay closed")
	}
}

// TestSynthControl_PopupOpensOnReleaseNotPress: a stationary tap on a control
// opens the popup on RELEASE, not on the physical press (so the press frame can
// still become a scroll). Guards the capture-on-press / decide-on-release model.
func TestSynthControl_PopupOpensOnReleaseNotPress(t *testing.T) {
	g := modularSynthGameRealistic(t)
	if g.drum.synthWheelPopup != nil && g.drum.synthWheelPopup.IsOpen() {
		t.Fatal("popup unexpectedly open before the tap")
	}
	_, cx, cy := firstVisibleFMControlRect(t, g)

	driveDrumInput(g, cx, cy, true) // press — must NOT open yet
	if g.drum.synthWheelPopup != nil && g.drum.synthWheelPopup.IsOpen() {
		t.Fatal("popup opened on the press frame; it must wait for release (so a swipe can scroll)")
	}
	driveDrumInput(g, cx, cy, false) // release without moving — tap opens it
	if g.drum.synthWheelPopup == nil || !g.drum.synthWheelPopup.IsOpen() {
		t.Fatal("a stationary tap did not open the value-wheel popup on release")
	}
}
