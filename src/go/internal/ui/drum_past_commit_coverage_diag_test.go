package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/timeline"
)

// Diagnostic (not an assertion of desired behavior): after real follow-mode
// playback, report for every visible PAST cell whether it is backed by an
// immutable Playback/Import commit. Cells that are NOT are the ones that will
// follow the live predictor and mutate on a circuit edit.
func TestDiag_PastCommitCoverageUnderFollow(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.SetPlayFunc(func(instrument.ID, float64, ...float64) {})
	g.Layout(1024, 720)

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	c := g.tryAddNode(1, 1, model.NodeTypeRegular)
	d := g.tryAddNode(0, 1, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, c)
	g.addEdge(c, d)
	g.addEdge(d, a)
	g.start = a
	g.graph.StartNodeID = a.ID

	g.drum.SetLength(16)
	g.drum.SetFollow(true)
	g.updateBeatInfos()
	g.refreshDrumRow()

	dst := ebiten.NewImage(1024, 720)
	g.drum.Draw(dst, nil, 0, nil, 0)
	g.drum.SetBPM(120)
	g.SetAppliedBPMForTest(120)
	pressPlay(t, g.drum)
	advancePlaybackByAbs(g, 3600) // long enough to slide the predictor window past 0
	g.refreshDrumRow()

	row := 0
	pastExclusive := g.nextBeatIdxs[row]
	off := g.drum.Offset
	winEnd := off + g.drum.Length
	pastEnd := pastExclusive
	if pastEnd > winEnd {
		pastEnd = winEnd
	}
	t.Logf("offset=%d pastExclusive=%d len=%d predWindowStart=%d predHorizon=%d frozenUpTo=%v",
		off, pastExclusive, g.drum.Length, g.engine.Predictor.WindowStart(), g.engine.Predictor.Horizon(), g.frozenUpToByRow)
	missing := 0
	for abs := off; abs < pastEnd; abs++ {
		_, _, kind, ok := g.timelineCommittedWithKind(row, abs)
		immutable := ok && (kind == timeline.CommitKindPlayback || kind == timeline.CommitKindImport)
		if !immutable {
			missing++
			t.Logf("  VISIBLE PAST abs=%d has NO immutable commit (ok=%v kind=%v)", abs, ok, kind)
		}
	}
	t.Logf("visible-past cells without immutable commit: %d", missing)
}
