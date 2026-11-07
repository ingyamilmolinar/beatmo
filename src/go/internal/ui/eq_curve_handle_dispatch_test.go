//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

// The tree's press loop over HitIndex.At, so z-order bugs between a catch-all and its own controls show.
func dispatchPressViaHitIndex(idx *HitIndex, x, y int) (tag string, res InputResult) {
	for _, hit := range idx.At(x, y) {
		if hit.Handler == nil {
			continue
		}
		r := hit.Handler.OnPress(x, y)
		if r != InputIgnored {
			return hit.Tag, r
		}
	}
	return "", InputIgnored
}

// As DrumViewTree publishes it.
func publishEQHitIndex(z *EQPanelZone) *HitIndex {
	idx := &HitIndex{}
	idx.Update(z.ID(), z.HitAreas())
	return idx
}

// Through the real z-order loop the handle press reaches the curve adapter, never the eq-panel catch-all, which
// consumes and stops dispatch.
func TestEQBandHandlePressReachesCurveHandler(t *testing.T) {
	restore := noInputForTest()
	defer restore()

	z, _ := newTestEQPanelZone(nil)
	rect := image.Rect(0, 200, 800, 400)
	z.Layout(rect)

	idx := publishEQHitIndex(z)

	// The handle position the renderer and hit test share.
	hx, hy := z.eqBandHandlePos(5)

	tag, res := dispatchPressViaHitIndex(idx, hx, hy)
	if tag != "eq-curve-area" || res != InputCaptured {
		t.Fatalf("press on band-5 handle was dispatched to %q (result=%d); "+
			"want %q captured. The opaque catch-all is swallowing the EQ curve "+
			"handle press.", tag, res, "eq-curve-area")
	}
	if z.curveDragBand != 5 {
		t.Fatalf("expected curveDragBand=5 after dispatched press, got %d", z.curveDragBand)
	}
}

// The same for the HP/LP handles.
func TestEQFilterHandlePressReachesCurveHandler(t *testing.T) {
	restore := noInputForTest()
	defer restore()

	z, _, fs := newTestEQPanelZoneWithFilters(nil)
	fs.hpfEnabled = true
	fs.hpfCutoff = 200
	rect := image.Rect(0, 200, 800, 400)
	z.Layout(rect)

	idx := publishEQHitIndex(z)

	// HPF handle centre — computed the same way drawEQCurve draws it.
	r := z.eqPlotRect()
	hx := freqToX(fs.hpfCutoff, r)
	hy := z.curveYAtX(hx)

	tag, res := dispatchPressViaHitIndex(idx, hx, hy)
	if tag != "eq-curve-area" || res != InputCaptured {
		t.Fatalf("press on HPF handle was dispatched to %q (result=%d); "+
			"want %q captured. The opaque catch-all is swallowing the HP/LP "+
			"handle press.", tag, res, "eq-curve-area")
	}
	if z.curveDragFilter != hooks.FilterHPF {
		t.Fatalf("expected curveDragFilter=%q after dispatched press, got %q", "hpf", z.curveDragFilter)
	}
}
