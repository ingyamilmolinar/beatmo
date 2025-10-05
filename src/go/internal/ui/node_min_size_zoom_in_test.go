package ui

import (
	"github.com/ingyamilmolinar/tunkul/core/model"
	"testing"
)

// Ensure nodes remain reasonably clickable when zoomed in by enforcing a
// minimum on-screen radius, unless neighbors would overlap.
func TestNodeMinSizeZoomIn(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	// Single node to avoid overlap constraints
	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.cam.Scale = 3.0 // zoomed in
	scr := g.nodeRadius(n) * g.cam.Scale
	if scr < 10-0.5 { // floor ~10px radius at 3x zoom-in
		t.Fatalf("expected min on-screen radius >=10px, got %.2f", scr)
	}
}
