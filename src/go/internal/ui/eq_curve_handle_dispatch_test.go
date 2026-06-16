//go:build test

package ui

import (
	"image"
	"testing"
)

// dispatchPressViaHitIndex replicates the DrumView tree's core press-dispatch
// loop (drumview_tree.go: iterate HitIndex.At() results in z-order, calling
// OnPress, stopping at the first handler that does NOT return InputIgnored).
// It returns the tag + result of the handler that won the press.
//
// Driving the press through the real HitIndex (rather than calling a single
// HitArea.Handler.OnPress directly, as TestEQPanelZoneCurveHandleDragSyncsSlider
// does) is what exposes z-ordering bugs between a zone's opaque catch-all and
// its per-control hit areas.
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

// publishEQHitIndex publishes a laid-out EQ panel zone's hit areas into a fresh
// HitIndex exactly the way DrumViewTree does in production
// (hitIndex.Update(zone.ID(), zone.HitAreas())).
func publishEQHitIndex(z *EQPanelZone) *HitIndex {
	idx := &HitIndex{}
	idx.Update(z.ID(), z.HitAreas())
	return idx
}

// TestEQBandHandlePressReachesCurveHandler verifies that a press landing exactly
// on an EQ band handle, dispatched through the real HitIndex z-order loop,
// reaches the curve handle adapter rather than being swallowed by the panel's
// opaque "eq-panel-capture" catch-all.
//
// Regression: the "eq-curve-area" hit area was registered at the same z-index
// (130) as the catch-all and appended after it, so HitIndex.At's stable sort
// kept the catch-all first; its OnPress returns InputConsumed, stopping the
// dispatch before the curve adapter ever ran. Result: EQ band handles (and the
// HP/LP handles, see the sibling test) did not respond to user input.
func TestEQBandHandlePressReachesCurveHandler(t *testing.T) {
	restore := noInputForTest()
	defer restore()

	z, _ := newTestEQPanelZone(nil)
	rect := image.Rect(0, 200, 800, 400)
	z.Layout(rect)

	idx := publishEQHitIndex(z)

	// Band 5 handle centre — the single source of truth shared by renderer
	// and hit-test, so the press lands exactly where the handle is drawn.
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

// TestEQFilterHandlePressReachesCurveHandler verifies the same for the HP/LP
// (high-pass / low-pass) draggable handles: a press on the HPF handle,
// dispatched through the real HitIndex, must reach the curve handle adapter and
// begin a filter drag rather than being consumed by the catch-all.
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
	if z.curveDragFilter != "hpf" {
		t.Fatalf("expected curveDragFilter=%q after dispatched press, got %q", "hpf", z.curveDragFilter)
	}
}
