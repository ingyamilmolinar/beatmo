package ui

import (
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// TestBPMFunctional60 verifies that, for a 1-beat-per-edge loop, scheduled
// indices for regular nodes advance by one beat (MaxDiv subdivisions) at BPM=60.
func TestBPMFunctional60(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(800, 600)

	beat := g.grid.MaxDiv()
	g.pendingStartRow = 0
	n0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n1 := g.tryAddNode(beat, 0, model.NodeTypeRegular)
	n2 := g.tryAddNode(beat, beat, model.NodeTypeRegular)
	n3 := g.tryAddNode(0, beat, model.NodeTypeRegular)
	g.addEdge(n0, n1)
	g.addEdge(n1, n2)
	g.addEdge(n2, n3)
	g.addEdge(n3, n0)
	g.pendingStartRow = -1
	g.updateBeatInfos()

	bpm := 60
	g.drum.SetBPM(bpm)
	g.SetAppliedBPMForTest(bpm)
	g.prevBPM = bpm
	g.SetPlaying(true)
	dt := 3 * time.Second
	target := int(dt.Seconds() * float64(bpm) / 60.0 * float64(beat))
	for i := 0; i < 20 && (len(g.seqNextIdxs) == 0 || g.seqNextIdxs[0] < target+1); i++ {
		setPlayStartForAbs(g, target)
		_ = g.Update()
	}
	if len(g.seqNextIdxs) == 0 {
		t.Fatalf("seqNextIdxs not initialized")
	}
	if got := g.seqNextIdxs[0]; got < target+1 {
		t.Fatalf("seqNextIdxs[0]=%d want >=%d (target=%d)", got, target+1, target)
	}
}

// TestBPMStressShortSegmentsHighBPM ensures short segments at high BPM don't
// skip indices; each scheduled event should advance by 1 subdivision.
func TestBPMStressShortSegmentsHighBPM(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(800, 600)

	beatUnit := 1
	buildRect := func(row, x, y int) {
		g.pendingStartRow = row
		n0 := g.tryAddNode(x, y, model.NodeTypeRegular)
		n1 := g.tryAddNode(x+beatUnit, y, model.NodeTypeRegular)
		n2 := g.tryAddNode(x+beatUnit, y+beatUnit, model.NodeTypeRegular)
		n3 := g.tryAddNode(x, y+beatUnit, model.NodeTypeRegular)
		g.addEdge(n0, n1)
		g.addEdge(n1, n2)
		g.addEdge(n2, n3)
		g.addEdge(n3, n0)
		g.pendingStartRow = -1
	}
	for len(g.drum.Rows) < 3 {
		g.drum.AddRow()
	}
	buildRect(0, 0, 0)
	buildRect(1, 5, 0)
	buildRect(2, 10, 0)
	g.updateBeatInfos()

	bpm := 240
	g.drum.SetBPM(bpm)
	g.SetAppliedBPMForTest(bpm)
	g.prevBPM = bpm
	g.SetPlaying(true)
	dt := 200 * time.Millisecond
	target := int(dt.Seconds() * float64(bpm) / 60.0 * float64(g.grid.MaxDiv()))
	for i := 0; i < 20; i++ {
		setPlayStartForAbs(g, target)
		_ = g.Update()
		if len(g.seqNextIdxs) >= 3 && g.seqNextIdxs[0] >= target+1 && g.seqNextIdxs[1] >= target+1 && g.seqNextIdxs[2] >= target+1 {
			break
		}
	}
	if len(g.seqNextIdxs) < 3 {
		t.Fatalf("seqNextIdxs length=%d want >=3", len(g.seqNextIdxs))
	}
	for row := 0; row < 3; row++ {
		if got := g.seqNextIdxs[row]; got < target+1 {
			t.Fatalf("row %d seqNextIdxs=%d want >=%d (target=%d)", row, got, target+1, target)
		}
	}
}

// TestHighlightAudioSync25ms validates that scheduled indices and UI highlight
// indices stay aligned for a simple loop (index parity, not wall time).
func TestHighlightAudioSync25ms(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(800, 600)

	// Use a subdivision loop where every index is a regular node so
	// scheduler and highlight indices should stay aligned.
	buildSubdivLoop(g)
	bpm := 90
	g.drum.SetBPM(bpm)
	g.SetAppliedBPMForTest(bpm)
	g.prevBPM = bpm
	g.SetPlaying(true)
	dt := 150 * time.Millisecond
	target := int(dt.Seconds() * float64(bpm) / 60.0 * float64(g.grid.MaxDiv()))
	for i := 0; i < 20; i++ {
		setPlayStartForAbs(g, target)
		if err := g.Update(); err != nil {
			t.Fatalf("update failed: %v", err)
		}
		if len(g.seqNextIdxs) > 0 && len(g.nextBeatIdxs) > 0 && g.seqNextIdxs[0] >= target+1 && g.nextBeatIdxs[0] >= target+1 {
			break
		}
	}
	if len(g.seqNextIdxs) == 0 || len(g.nextBeatIdxs) == 0 {
		t.Fatalf("missing scheduler/UI indices")
	}
	if g.seqNextIdxs[0] < target+1 || g.nextBeatIdxs[0] < target+1 {
		t.Fatalf("indices did not reach target: seqNext=%d nextBeat=%d target=%d", g.seqNextIdxs[0], g.nextBeatIdxs[0], target)
	}
	if g.nextBeatIdxs[0] != g.seqNextIdxs[0] {
		t.Fatalf("UI index mismatch: nextBeatIdxs=%d seqNextIdxs=%d", g.nextBeatIdxs[0], g.seqNextIdxs[0])
	}
}
