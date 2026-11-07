package ui

import (
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// The mini-timeline cursor (displayBeat) and the drum view's auto-scroll (TrackBeat) derive from ONE
// playhead: round(displayBeat * div) == playheadAbsSubdiv(). Sampled from different clocks they drift apart.
func TestPlayheadSingleSourceOfTruth(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.SetPlayFunc(func(instrument.ID, float64, ...float64) {})
	g.Layout(1024, 720)
	g.drum.SetFollow(true)
	g.drum.SetLength(16)
	g.drum.SetBPM(120)

	nodes := make([]*uiNode, 4)
	for i := 0; i < len(nodes); i++ {
		nodes[i] = g.tryAddNode(i, 0, model.NodeTypeRegular)
		if i > 0 {
			g.addEdge(nodes[i-1], nodes[i])
		}
	}
	g.addEdge(nodes[len(nodes)-1], nodes[0])
	g.start = nodes[0]
	g.graph.StartNodeID = nodes[0].ID
	g.drum.Rows[0].Origin = nodes[0].ID
	g.drum.Rows[0].Node = nodes[0]
	g.updateBeatInfos()
	g.refreshDrumRow()

	dst := ebiten.NewImage(1024, 720)
	g.drum.Draw(dst, nil, 0, nil, 0)
	pressPlay(t, g.drum)
	if err := g.Update(); err != nil {
		t.Fatalf("update at play: %v", err)
	}
	advancePlaybackByAbs(g, 200*g.grid.MaxDiv())
	g.drum.Draw(dst, nil, 0, nil, 0)

	div := float64(max1(g.grid.MaxDiv()))
	cursorBeats := g.displayBeat()
	expectedSubdiv := int(math.Round(cursorBeats * div))
	actualSubdiv := g.playheadAbsSubdiv()

	if absInt(actualSubdiv-expectedSubdiv) > 1 {
		t.Fatalf("playhead source-of-truth violation: g.playheadAbsSubdiv()=%d, round(g.displayBeat()*div)=%d (displayBeat=%.4f beats, div=%d) — these must agree within ±1 subdivision",
			actualSubdiv, expectedSubdiv, cursorBeats, int(div))
	}
}

// The auto-scroll actually uses the helper, not a MAX over several clocks.
func TestTrackBeatConsumesCanonicalPlayhead(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.SetPlayFunc(func(instrument.ID, float64, ...float64) {})
	g.Layout(1024, 720)
	g.drum.SetFollow(true)
	g.drum.SetLength(16)
	g.drum.SetBPM(120)

	nodes := make([]*uiNode, 4)
	for i := 0; i < len(nodes); i++ {
		nodes[i] = g.tryAddNode(i, 0, model.NodeTypeRegular)
		if i > 0 {
			g.addEdge(nodes[i-1], nodes[i])
		}
	}
	g.addEdge(nodes[len(nodes)-1], nodes[0])
	g.start = nodes[0]
	g.graph.StartNodeID = nodes[0].ID
	g.drum.Rows[0].Origin = nodes[0].ID
	g.drum.Rows[0].Node = nodes[0]
	g.updateBeatInfos()
	g.refreshDrumRow()

	dst := ebiten.NewImage(1024, 720)
	g.drum.Draw(dst, nil, 0, nil, 0)
	pressPlay(t, g.drum)
	if err := g.Update(); err != nil {
		t.Fatalf("update at play: %v", err)
	}
	advancePlaybackByAbs(g, 200*g.grid.MaxDiv())
	g.drum.Draw(dst, nil, 0, nil, 0)

	// cur - length*frac.
	frac := RuntimeProf().RibbonPlayheadFrac
	if frac <= 0 || frac >= 1 {
		frac = 0.70
	}
	cur := g.playheadAbsSubdiv()
	expectedOffset := cur - int(math.Round(float64(g.drum.Length)*frac))
	if expectedOffset < 0 {
		expectedOffset = 0
	}
	// TrackBeat's ±1-cell dead-zone is the only allowed slack.
	diff := absInt(g.drum.Offset - expectedOffset)
	if diff > 2 {
		t.Fatalf("drum.Offset (%d) diverged from canonical-playhead-derived target (%d) by %d cells — TrackBeat is consuming a different playhead clock than playheadAbsSubdiv. cur=%d length=%d frac=%.2f",
			g.drum.Offset, expectedOffset, diff, cur, g.drum.Length, frac)
	}
}
