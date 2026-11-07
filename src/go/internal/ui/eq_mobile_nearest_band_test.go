//go:build test

package ui

import "testing"

// A press between two packed handles picks the NEAREST band: the 26 px radii overlap at a ~39 px pitch.
func TestMobileEQPressSelectsNearestBand(t *testing.T) {
	restore := SetForceSmallScreen(t, true)
	defer restore()

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)
	dv := g.drum
	dv.SetMobileEQMode(true)
	z := dv.eqPanelZone
	z.SetActiveTab(TabEQ)
	z.Layout(z.PanelRect())

	x4, y4 := z.eqBandHandlePos(4)
	x5, _ := z.eqBandHandlePos(5)
	if x5 <= x4 {
		t.Fatalf("band handles not laid out left-to-right: x4=%d x5=%d", x4, x5)
	}
	// Nearest is band 5, yet still inside band 4's circle.
	px := (x4+x5)/2 + 2
	if dx := px - x4; dx*dx >= Profile().EQHandleRadius*Profile().EQHandleRadius {
		t.Skipf("bands 4/5 don't overlap at this width (pitch %d, radius %d)", x5-x4, Profile().EQHandleRadius)
	}

	h := &curveHandleHitAdapter{zone: z}
	if got := h.OnPress(px, y4); got != InputCaptured {
		t.Fatalf("press not captured: %v", got)
	}
	if z.curveDragBand != 5 {
		t.Fatalf("press nearest to band 5 grabbed band %d", z.curveDragBand)
	}
}
