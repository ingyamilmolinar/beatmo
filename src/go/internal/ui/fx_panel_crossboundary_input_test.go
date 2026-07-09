//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// fx_panel_crossboundary_input_test.go — the "popup that crosses a subtree
// boundary must still own its own input" contract.
//
// The FX (insert-effects) panel opens from a row in the drum-view subtree but
// its rect extends DOWN across the bottom EQ-panel region, which lives in the
// SEPARATE audio subtree. A portal overlay is globally top-of-z (z >= 300), so
// EVERY point inside the panel — including the part painted over the EQ panel —
// must be owned by the panel, not by the EQ panel's opaque catch-all beneath
// it. These tests drive the REAL per-frame input loop (dv.Update through the
// RootTree) at a point inside fxPanelRect ∩ eqPanelRect.

// openTallFXPanel adds enough distinct insert effects to row 0's instrument
// that the FX panel is tall AND scrollable, then opens it. Returns the point
// (in screen space) that lies inside BOTH the FX panel and the EQ panel — the
// cross-boundary overlap the bug lives in.
func openTallFXPanel(t *testing.T, dv *DrumView) image.Point {
	t.Helper()
	if len(dv.Rows) == 0 || dv.Rows[0] == nil {
		t.Fatal("no row 0")
	}
	instID := dv.Rows[0].Instrument
	// Add every registered effect type so the panel content overflows and a
	// scrollbar (fxScrollMaxPx > 0) is guaranteed.
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
	if dv.fxScrollMaxPx <= 0 {
		t.Fatalf("FX panel is not scrollable (fxScrollMaxPx=%d) — repro precondition unmet", dv.fxScrollMaxPx)
	}
	return image.Pt(overlap.Min.X+overlap.Dx()/2, overlap.Min.Y+overlap.Dy()/2)
}

// TestFXPanelWheelScrollsInsideEQPanelRegion is the reported bug: a wheel
// delivered inside the FX panel but over the EQ-panel region does nothing,
// because the EQ panel's wheel-consuming catch-all (higher-z audio subtree,
// dispatched first) swallows it before the FX panel portal sees it. The panel
// must scroll from ANY point inside its own rect.
func TestFXPanelWheelScrollsInsideEQPanelRegion(t *testing.T) {
	dv := newTestDrumViewWithRows(t, 6)
	dv.layoutForTest()

	pt := openTallFXPanel(t, dv)

	before := dv.fxScrollOffsetPx
	for i := 0; i < 4; i++ {
		injectWheelAt(t, dv, pt.X, pt.Y, 0, -1) // scroll down
	}
	after := dv.fxScrollOffsetPx
	if after == before {
		t.Fatalf("wheel inside FX panel over the EQ-panel region did not scroll it: "+
			"fxScrollOffsetPx stayed %d at point %v (panel must own every point in its rect)", before, pt)
	}
}

// TestFXPanelPressInsideEQPanelRegionNotEatenByEQ is the press-side twin: a
// press inside the FX panel over the EQ-panel region must be OWNED by the panel
// portal (globally top-z), never consumed by the EQ panel zone beneath it.
// Under the bug the audio subtree is dispatched first and its opaque
// wheel/press catch-all eats the press before the FX panel portal (in the
// lower drum-view subtree) is even offered it — so the click lands on the EQ
// panel that is painted UNDER the visible FX panel. We detect the leak by
// asserting the audio subtree did not handle the press.
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
