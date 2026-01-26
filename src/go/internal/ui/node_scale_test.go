package ui

import "testing"

// Nodes should shrink on zoom-in and grow on zoom-out in screen pixels.
func TestNodeSizeRespondsToZoom(t *testing.T) {
	withDefaultStart(t, true)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	n := g.nodeAt(0, 0)
	if n == nil {
		t.Fatal("missing origin node")
	}

	// Zoom in
	g.cam.Scale = 3.0
	rIn := g.nodeRadius(n) * g.cam.Scale

	// Medium
	g.cam.Scale = 1.0
	rMid := g.nodeRadius(n) * g.cam.Scale
	if rMid <= rIn {
		t.Fatalf("expected mid >= in: mid=%.2f in=%.2f", rMid, rIn)
	}

	// Zoom out
	g.cam.Scale = 0.5
	rOut := g.nodeRadius(n) * g.cam.Scale
	if rOut <= rMid {
		t.Fatalf("expected out > mid: out=%.2f mid=%.2f", rOut, rMid)
	}
}
