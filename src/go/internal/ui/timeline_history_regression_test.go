package ui

import (
	"testing"

	"github.com/ingyamilmolinar/tunkul/core/model"
)

// Regression for live circuit edits during playback: past commits should
// survive updateBeatInfos so the DrumView timeline does not drop history.
func TestTimelineHistorySurvivesPathEdit(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(960, 600)

	// Base square loop on row 0.
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(2, 0, model.NodeTypeRegular)
	c := g.tryAddNode(2, 2, model.NodeTypeRegular)
	d := g.tryAddNode(0, 2, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, c)
	g.addEdge(c, d)
	g.addEdge(d, a)
	g.start = a
	g.graph.StartNodeID = a.ID
	if len(g.drum.Rows) == 0 {
		t.Fatalf("drum rows not initialized")
	}
	g.drum.Rows[0].Origin = a.ID
	g.drum.Rows[0].Node = a
	g.drum.SetLength(192)
	g.updateBeatInfos()

	// Simulate playback progress: freeze history up to abs=7 and store commits.
	g.frozenUpToByRow = []int{7}
	for abs := 0; abs <= 7; abs++ {
		bi := g.beatInfoAtRow(0, abs)
		g.recordTimelineCommit(0, abs, abs%2 == 0, bi.NodeType)
	}
	g.nextBeatIdxs = []int{25}
	g.refreshDrumRow()
	before := g.dumpRowState(0)
	if before.FrozenUpTo != 7 {
		t.Fatalf("expected frozenUpTo 7 got %d", before.FrozenUpTo)
	}
	if len(before.Timeline.PastMask) == 0 || !before.Timeline.PastMask[7-before.Timeline.Offset] {
		t.Fatalf("expected commit at abs=7 before edit")
	}

	// Live edit: detour B->X->Y->C to change the path shape.
	x := g.tryAddNode(3, 0, model.NodeTypeRegular)
	y := g.tryAddNode(3, 1, model.NodeTypeRegular)
	g.deleteEdge(b, c)
	g.addEdge(b, x)
	g.addEdge(x, y)
	g.addEdge(y, c)

	g.updateBeatInfos()
	after := g.dumpRowState(0)

	limit := before.FrozenUpTo
	if after.FrozenUpTo < limit {
		limit = after.FrozenUpTo
	}
	for abs := before.Timeline.Offset; abs <= limit; abs++ {
		rel := abs - after.Timeline.Offset
		if rel < 0 || rel >= len(after.Timeline.PastMask) {
			t.Fatalf("past commit abs=%d fell outside window (offset=%d len=%d)", abs, after.Timeline.Offset, len(after.Timeline.PastMask))
		}
		if !after.Timeline.PastMask[rel] {
			t.Fatalf("past commit at abs=%d dropped after edit (rel=%d)", abs, rel)
		}
	}
}

// Mirrors live_edit_sync.browser.test.js: during playback, editing the circuit
// must not drop committed past timeline entries.
func TestTimelineHistoryLiveEditPlayback(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(960, 600)

	// Base square loop (row 0).
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(2, 0, model.NodeTypeRegular)
	c := g.tryAddNode(2, 2, model.NodeTypeRegular)
	d := g.tryAddNode(0, 2, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, c)
	g.addEdge(c, d)
	g.addEdge(d, a)
	g.start = a
	g.graph.StartNodeID = a.ID

	// Secondary loop (row 1) to mirror browser harness shape.
	e := g.tryAddNode(6, 0, model.NodeTypeRegular)
	f := g.tryAddNode(7, 0, model.NodeTypeRegular)
	gNode := g.tryAddNode(7, 1, model.NodeTypeRegular)
	g.addEdge(e, f)
	g.addEdge(f, gNode)
	g.addEdge(gNode, e)
	g.drum.AddRow()
	g.drum.Rows[1].Origin = e.ID
	g.drum.Rows[1].Node = e

	g.drum.SetLength(192)
	g.updateBeatInfos()
	g.refreshDrumRow()
	g.drum.SetBPM(120)
	g.SetAppliedBPMForTest(120)

	// Start playback and deterministically build some history.
	g.SetPlaying(true)
	targetAbs := 10
	for abs := 0; abs <= targetAbs; abs++ {
		scheduleAbsForMuteTest(g, abs)
	}
	g.elapsedBeats = targetAbs
	g.refreshDrumRow()
	before := g.dumpRowState(0)
	if before.FrozenUpTo < 4 {
		t.Fatalf("insufficient past built before edit; frozen=%d", before.FrozenUpTo)
	}

	// Live edit: detour B->X->Y->C while playback runs.
	x := g.tryAddNode(3, 0, model.NodeTypeRegular)
	y := g.tryAddNode(3, 1, model.NodeTypeRegular)
	g.deleteEdge(b, c)
	g.addEdge(b, x)
	g.addEdge(x, y)
	g.addEdge(y, c)
	g.updateBeatInfos()
	g.refreshDrumRow()

	for abs := targetAbs + 1; abs <= targetAbs+6; abs++ {
		scheduleAbsForMuteTest(g, abs)
	}
	g.elapsedBeats = targetAbs + 6
	g.refreshDrumRow()

	after := g.dumpRowState(0)
	if after.FrozenUpTo < before.FrozenUpTo {
		t.Fatalf("unexpected freeze regression: before=%d after=%d", before.FrozenUpTo, after.FrozenUpTo)
	}
	limit := before.FrozenUpTo
	if after.FrozenUpTo < limit {
		limit = after.FrozenUpTo
	}
	for abs := before.Timeline.Offset; abs <= limit; abs++ {
		if abs >= len(before.Timeline.PastMask) {
			break
		}
		if !before.Timeline.PastMask[abs] {
			continue
		}
		if abs < 0 {
			continue
		}
		if abs >= len(after.Timeline.PastMask) || !after.Timeline.PastMask[abs] {
			t.Fatalf("past commit at abs=%d dropped after live edit (before freeze=%d after freeze=%d)", abs, before.FrozenUpTo, after.FrozenUpTo)
		}
	}
}
