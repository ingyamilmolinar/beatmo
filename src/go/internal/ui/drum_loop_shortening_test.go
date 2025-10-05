package ui

import (
	"testing"
	"time"

	"github.com/ingyamilmolinar/tunkul/core/model"
)

// Shortening a loop by inserting a node in the middle (auto-stitch) should
// immediately update the drum row to show hits on every subdivision.
func TestDrumRowLoopShorteningFillsSteps(t *testing.T) {
	g := New(testLogger)
	g.SetUseSequencerForTest(true)
	g.Layout(1024, 720)
	g.drum.follow = false

	g.drum.Length = 16
	g.drum.Offset = 0

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(2, 0, model.NodeTypeRegular)
	g.start = a
	g.graph.StartNodeID = a.ID
	if len(g.drum.Rows) == 0 {
		t.Fatalf("expected default drum row")
	}
	g.drum.Rows[0].Origin = a.ID
	g.drum.Rows[0].Node = a

	g.addEdge(a, b)
	g.addEdge(b, a)
	g.updateBeatInfos()
	g.refreshDrumRow()

	// Baseline: with a gap of one invisible grid intersection, hits alternate.
	initial := append([]bool(nil), g.drum.Rows[0].Steps...)
	if len(initial) < 4 {
		t.Fatalf("unexpected initial window length: %d", len(initial))
	}
	if !(initial[0] && !initial[1] && initial[2]) {
		t.Fatalf("expected alternating baseline pattern, got %v", initial[:4])
	}

	// Start playback long enough to record history for the alternating pattern.
	g.drum.SetBPM(120)
	g.drum.playPressed = true
	until := time.Now().Add(120 * time.Millisecond)
	for time.Now().Before(until) {
		_ = g.Update()
		time.Sleep(2 * time.Millisecond)
	}

	// Live edit: insert a node at (1,0). Auto-stitch splits the edge so every
	// subdivision now lands on a regular node.
	_ = g.tryAddNode(1, 0, model.NodeTypeRegular)

	// Allow the UI/game loop to process predictor rebuilds and refresh rows.
	until = time.Now().Add(120 * time.Millisecond)
	for time.Now().Before(until) {
		_ = g.Update()
		time.Sleep(2 * time.Millisecond)
	}

	base := 0
	if len(g.nextBeatIdxs) > 0 {
		base = g.nextBeatIdxs[0]
	}
	g.drum.Offset = base
	g.refreshDrumRow()
	row := g.drum.Rows[0].Steps
	if len(row) == 0 {
		t.Fatalf("unexpected empty row after edit")
	}
	span := 8
	if span > len(row) {
		span = len(row)
	}
	for i := 0; i < span; i++ {
		if !row[i] {
			t.Fatalf("step %d remained silent after loop shortening: %v", i, row[:span])
		}
	}
}

// Past cells must stay immutable when a loop is shortened; only future
// predictions may change.
func TestDrumRowLoopShorteningPreservesHistory(t *testing.T) {
	g := New(testLogger)
	g.SetUseSequencerForTest(true)
	g.Layout(1024, 720)
	g.drum.follow = false

	g.drum.Length = 16
	g.drum.Offset = 0

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(2, 0, model.NodeTypeRegular)
	g.start = a
	g.graph.StartNodeID = a.ID
	g.drum.Rows[0].Origin = a.ID
	g.drum.Rows[0].Node = a

	g.addEdge(a, b)
	g.addEdge(b, a)
	g.updateBeatInfos()
	g.refreshDrumRow()

	g.drum.SetBPM(120)
	g.drum.playPressed = true
	stop := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(stop) {
		_ = g.Update()
		time.Sleep(2 * time.Millisecond)
	}

	if len(g.nextBeatIdxs) == 0 {
		t.Fatal("missing nextBeatIdxs")
	}
	past := g.nextBeatIdxs[0]
	before := append([]bool(nil), g.drum.Rows[0].Steps...)

	_ = g.tryAddNode(1, 0, model.NodeTypeRegular)
	stop = time.Now().Add(120 * time.Millisecond)
	for time.Now().Before(stop) {
		_ = g.Update()
		time.Sleep(2 * time.Millisecond)
	}

	g.refreshDrumRow()
	after := g.drum.Rows[0].Steps
	limit := minInt(len(before), len(after))
	for j := 0; j < limit; j++ {
		abs := g.drum.Offset + j
		if abs < past && before[j] != after[j] {
			t.Fatalf("history changed at abs=%d: before=%v after=%v", abs, before[j], after[j])
		}
	}
}
