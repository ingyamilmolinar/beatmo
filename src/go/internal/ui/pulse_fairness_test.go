//go:build test

package ui

import (
	"github.com/ingyamilmolinar/tunkul/core/model"
	"testing"
)

// TestEqualRowPulseSpeeds ensures that for equal-length segments on different
// rows, per-frame progress uses the same dt and therefore produces similar t.
func TestEqualRowPulseSpeeds(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	g.drum.SetBPM(120)

	// Create two equal 1-beat horizontal segments on rows 0 and 1.
	max := g.grid.MaxDiv()
	// Row 0
	n00 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n01 := g.tryAddNode(max, 0, model.NodeTypeRegular)
	g.addEdge(n00, n01)

	// Row 1
	g.drum.AddRow()
	n10 := g.tryAddNode(0, 1, model.NodeTypeRegular)
	n11 := g.tryAddNode(max, 1, model.NodeTypeRegular)
	g.addEdge(n10, n11)
	g.drum.Rows[1].Origin = n10.ID
	g.drum.Rows[1].Node = g.nodeByID(n10.ID)

	g.updateBeatInfos()
	g.playing = true
	g.spawnPulseFromRow(0, 0)
	g.spawnPulseFromRow(1, 0)
	if g.pulseForRow(0) == nil || g.pulseForRow(1) == nil {
		t.Fatalf("expected pulses on both rows")
	}

	// Run a few frames and check progress similarity.
	for i := 0; i < 10; i++ {
		_ = g.Update()
	}
	p0 := g.pulseForRow(0)
	p1 := g.pulseForRow(1)
	if p0 == nil || p1 == nil {
		t.Fatalf("pulses disappeared")
	}
	diff := p0.t - p1.t
	if diff < 0 {
		diff = -diff
	}
	if diff > 0.15 { // generous bound for timing noise under tests
		t.Fatalf("row pulse speeds diverged: t0=%.3f t1=%.3f", p0.t, p1.t)
	}
}
