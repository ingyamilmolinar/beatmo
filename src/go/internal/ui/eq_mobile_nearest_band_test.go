//go:build test

package ui

import "testing"

// On mobile, a press between two packed band handles must select the
// NEAREST band. The precise-handle radius loop ran before the forgiving
// column grab, and with a 26px hit radius against a ~39px band pitch the
// radii overlap — so a press just right of the midpoint (nearest to band
// i+1, still inside band i's circle) grabbed band i instead (the mobile
// EQ handle-overlap item, 2026-07-04 critique).
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
	// Just right of the midpoint: nearest band is 5, but still within
	// band 4's hit circle when the radii overlap.
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
