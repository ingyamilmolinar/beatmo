//go:build test

// The screenshot scene is a REGRESSION SURFACE, so it has to actually show the
// thing it is named for. `scene_crop_test.go` only checks subject bounds, and a
// scene whose Setup silently no-ops still captures a perfectly valid-looking
// PNG — which is exactly what happened here on the first capture: the cursor
// was placed before rebuildMenu had produced any rows, so SetSelected had
// nothing to select and the scene photographed a menu with no ring.

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestInstMenuKeyCursorSceneShowsTheRing(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	advanceFrames(g, 2)

	if err := RunScene(g, "instrument_menu_keycursor"); err != nil {
		t.Fatalf("RunScene: %v", err)
	}
	advanceFrames(g, 5)

	c := g.drum.instMenuComp
	if c == nil || !c.IsOpen() {
		t.Fatal("scene did not leave the instrument menu open")
	}
	sel := c.MenuKeyNavForTest().Selected()
	if sel == MenuKeyNavNoSelection {
		t.Fatal("the scene placed no keyboard cursor, so the capture shows no focus " +
			"ring — the scene is not a regression surface for the feature it names")
	}
	ids := c.FilteredInstIDsForTest()
	if sel < 0 || sel >= len(ids) {
		t.Fatalf("cursor %d is outside the %d-row list", sel, len(ids))
	}
	if ids[sel] == c.ActiveInstrumentID() {
		t.Errorf("the cursor sits on the APPLIED instrument %q, so the focus ring and "+
			"the active-row highlight land on the same row and the capture cannot "+
			"show they are distinguishable", ids[sel])
	}

	screen := ebiten.NewImage(1280, 720)
	rec := &drawCallRecorder{}
	rec.record(t, func() { g.Draw(screen) })
	if len(focusRings(rec)) == 0 {
		t.Error("the scene draws no focus ring")
	}
}
