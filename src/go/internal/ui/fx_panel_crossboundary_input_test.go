//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// The FX panel opens from a drum-tree row but extends over the audio tree's EQ panel; as a top-z portal it
// must own every point of its rect, never the EQ catch-all beneath. Real input through the RootTree.

// Tall and scrollable; returns a point inside both the FX and EQ panels.
func openTallFXPanel(t *testing.T, dv *DrumView) image.Point {
	t.Helper()
	if len(dv.Rows) == 0 || dv.Rows[0] == nil {
		t.Fatal("no row 0")
	}
	instID := dv.Rows[0].Instrument
	// Every effect type, so the content overflows.
	for _, et := range audio.EffectTypeOrder() {
		audio.AddInsertEffect(instID, et, nil)
	}
	t.Cleanup(func() {
		for len(audio.GetInsertEffects(instID)) > 0 {
			audio.RemoveInsertEffect(instID, 0)
		}
	})

	dv.OpenFXPanel(0)
	dv.layoutForTest() // build + publish portal hit areas

	fxR := dv.fxPanelRect
	eqR := dv.eqPanelZone.PanelRect()
	if fxR.Empty() {
		t.Fatal("FX panel rect is empty after open")
	}
	if eqR.Empty() {
		t.Fatal("EQ panel rect is empty — panel not laid out")
	}
	overlap := fxR.Intersect(eqR)
	if overlap.Empty() {
		t.Fatalf("FX panel (%v) does not overlap EQ panel (%v) — repro precondition unmet", fxR, eqR)
	}
	if !dv.fxHasScroll() {
		t.Fatalf("FX panel is not scrollable — repro precondition unmet")
	}
	return image.Pt(overlap.Min.X+overlap.Dx()/2, overlap.Min.Y+overlap.Dy()/2)
}

// The audio tree is dispatched first, and its catch-all must not swallow a wheel meant for the panel above it.
func TestFXPanelWheelScrollsInsideEQPanelRegion(t *testing.T) {
	dv := newTestDrumViewWithRows(t, 6)
	dv.layoutForTest()

	pt := openTallFXPanel(t, dv)

	before := dv.fxScrollOffset()
	for i := 0; i < 4; i++ {
		injectWheelAt(t, dv, pt.X, pt.Y, 0, -1) // scroll down
	}
	after := dv.fxScrollOffset()
	if after == before {
		t.Fatalf("wheel inside FX panel over the EQ-panel region did not scroll it: "+
			"offset stayed %d at point %v (panel must own every point in its rect)", before, pt)
	}
}

// A press there belongs to the panel portal, never to the EQ panel painted under it; detected as the audio
// tree not handling it.
func TestFXPanelPressInsideEQPanelRegionNotEatenByEQ(t *testing.T) {
	dv := newTestDrumViewWithRows(t, 6)
	dv.layoutForTest()

	pt := openTallFXPanel(t, dv)

	injectPressAt(t, dv, pt.X, pt.Y)
	audioHandled := dv.audioTree.InputHandled()
	panelOpen := dv.IsFXPanelOpen()
	injectRelease(t, dv, pt.X, pt.Y)

	if audioHandled {
		t.Fatalf("press inside FX panel at %v was consumed by the EQ (audio) subtree — "+
			"it leaked to the panel painted beneath instead of the top-z FX overlay", pt)
	}
	if !panelOpen {
		t.Fatalf("press inside FX panel at %v dismissed the panel", pt)
	}
}
