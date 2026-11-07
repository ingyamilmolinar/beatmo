//go:build test

package ui

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// The window matches the beat duration, so the grid node glow stays in step with the cell highlight.
func TestApplySequencerHighlightSetsNodeWindow(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	start := g.tryAddNode(0, 0, model.NodeTypeRegular)
	next := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(start, next)
	g.addEdge(next, start)
	g.start = start
	g.graph.StartNodeID = start.ID
	g.drum.Rows[0].Origin = start.ID
	g.drum.Rows[0].Node = start
	g.updateBeatInfos()
	g.refreshDrumRow()

	g.SetPlaying(true)
	if len(g.nextBeatIdxs) != len(g.drum.Rows) {
		g.nextBeatIdxs = make([]int, len(g.drum.Rows))
	}

	// Override audio.Now() so the highlight window is set.
	restoreNow := audio.SetNowForTest(func() float64 { return 1.0 })
	defer restoreNow()

	row := 0
	idx := 0
	info := g.beatInfoAtRow(row, idx)
	g.applySequencerHighlight(row, idx, info)

	hlStart, hlEnd, ok := g.nodeHighlightUntil(info.NodeID)
	if !ok {
		t.Fatal("expected nodeHighlightUntil to be set after applySequencerHighlight")
	}

	// Window should start at audio.Now() (1.0) and span one beat.
	bpm := g.state.AppliedBPM()
	expectedBeatSec := 60.0 / float64(bpm)

	if math.Abs(hlStart-1.0) > 0.001 {
		t.Fatalf("highlight window start = %f, want ~1.0", hlStart)
	}
	duration := hlEnd - hlStart
	if math.Abs(duration-expectedBeatSec) > 0.001 {
		t.Fatalf("highlight window duration = %f, want beat duration %f (BPM=%d)", duration, expectedBeatSec, bpm)
	}
}

// With no audio clock the grid node falls back to exponential decay.
func TestApplySequencerHighlightNoWindowWhenAudioNowZero(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	start := g.tryAddNode(0, 0, model.NodeTypeRegular)
	next := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(start, next)
	g.addEdge(next, start)
	g.start = start
	g.graph.StartNodeID = start.ID
	g.drum.Rows[0].Origin = start.ID
	g.drum.Rows[0].Node = start
	g.updateBeatInfos()
	g.refreshDrumRow()

	g.SetPlaying(true)
	if len(g.nextBeatIdxs) != len(g.drum.Rows) {
		g.nextBeatIdxs = make([]int, len(g.drum.Rows))
	}

	// Do NOT override audio.Now() — it returns 0 in stub mode.
	row := 0
	idx := 0
	info := g.beatInfoAtRow(row, idx)
	g.applySequencerHighlight(row, idx, info)

	_, _, ok := g.nodeHighlightUntil(info.NodeID)
	if ok {
		t.Fatal("nodeHighlightUntil should NOT be set when audio.Now() returns 0")
	}

	// The animation should still be set for exponential decay fallback.
	anim := g.nodeAnimGet(info.NodeID)
	if anim != 1 {
		t.Fatalf("nodeAnim = %f, want 1 (fallback decay)", anim)
	}
}
