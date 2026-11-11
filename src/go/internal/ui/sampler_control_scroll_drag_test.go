//go:build test

package ui

import (
	"testing"
)

// firstVisibleSamplerControl returns the centre of the first on-window sampler
// control dial.
func firstVisibleSamplerControl(t *testing.T, g *Game) (cx, cy int) {
	t.Helper()
	for _, r := range visibleSamplerKnobRects(g.drum) {
		return r.Min.X + r.Dx()/2, r.Min.Y + r.Dy()/2
	}
	t.Fatal("no visible sampler control rect")
	return 0, 0
}

// TestSamplerControl_VerticalDragScrollsAndDoesNotOpenPopup: on the Sampler tab
// a sweep that starts on a control must scroll the grid and NOT open the
// value-wheel popup — the same "swipe up/down" fix as the Synth tab.
func TestSamplerControl_VerticalDragScrollsAndDoesNotOpenPopup(t *testing.T) {
	g := newMobileSamplerTabGameForTestSize(t, 390, 844)
	grid := g.drum.sampler.knobGrid
	if grid == nil || !grid.HasScroll() {
		t.Skip("sampler grid does not overflow at this layout")
	}
	cx, cy := firstVisibleSamplerControl(t, g)
	before := grid.Scroll().VS.First

	driveDrumInput(g, cx, cy, true)
	for dy := 20; dy <= 120; dy += 20 {
		driveDrumInput(g, cx, cy-dy, true)
	}
	driveDrumInput(g, cx, cy-120, false)

	if got := grid.Scroll().VS.First; got == before {
		t.Fatalf("vertical drag on a sampler control did not scroll the grid: First stayed %d", before)
	}
	if g.drum.samplerWheelPopup != nil && g.drum.samplerWheelPopup.IsOpen() {
		t.Fatal("a scroll gesture opened the sampler value-wheel popup; it must stay closed")
	}
}

// TestSamplerControl_PopupOpensOnReleaseNotPress: a stationary tap opens the
// sampler popup on RELEASE, not on the press frame.
func TestSamplerControl_PopupOpensOnReleaseNotPress(t *testing.T) {
	g := newMobileSamplerTabGameForTestSize(t, 390, 844)
	cx, cy := firstVisibleSamplerControl(t, g)

	driveDrumInput(g, cx, cy, true)
	if g.drum.samplerWheelPopup != nil && g.drum.samplerWheelPopup.IsOpen() {
		t.Fatal("sampler popup opened on the press frame; it must wait for release so a swipe can scroll")
	}
	driveDrumInput(g, cx, cy, false)
	if g.drum.samplerWheelPopup == nil || !g.drum.samplerWheelPopup.IsOpen() {
		t.Fatal("a stationary tap did not open the sampler value-wheel popup on release")
	}
}
