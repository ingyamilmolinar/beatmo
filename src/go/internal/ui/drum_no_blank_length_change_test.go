package ui

import (
	"testing"

	"github.com/ingyamilmolinar/tunkul/core/model"
)

// Changing DrumView length during active playback must not blank the rows.
// Steps must be recomputed immediately and caches updated without clearing.
func TestNoBlankOnLengthChangeDuringPlayback(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(1024, 720)

	// Row 0 rectangle
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

	// Row 1 triangle
	e := g.tryAddNode(4, 0, model.NodeTypeRegular)
	f := g.tryAddNode(5, 0, model.NodeTypeRegular)
	h := g.tryAddNode(5, 1, model.NodeTypeRegular)
	g.addEdge(e, f)
	g.addEdge(f, h)
	g.addEdge(h, e)
	g.drum.AddRow()
	g.drum.Rows[1].Origin = e.ID
	g.drum.Rows[1].Node = g.nodeByID(e.ID)

	g.drum.SetLength(16)
	g.updateBeatInfos()
	g.refreshDrumRow()

	// Start playing and let it run a bit.
	g.drum.SetBPM(120)
	g.SetAppliedBPMForTest(120)
	g.SetPlaying(true)
	for abs := 0; abs <= 8; abs++ {
		scheduleAbsForMuteTest(g, abs)
	}
	g.elapsedBeats = 8
	g.refreshDrumRow()

	// Press + to increase length while playing.
	pressLenInc(t, g.drum)
	_ = g.Update()

	// Steps must be recomputed and contain at least one visible cell per row.
	for row := range g.drum.Rows {
		steps := g.drum.Rows[row].Steps
		if len(steps) != g.drum.Length {
			t.Fatalf("row %d steps len=%d want=%d", row, len(steps), g.drum.Length)
		}
		any := false
		for _, v := range steps {
			if v {
				any = true
				break
			}
		}
		if !any {
			t.Fatalf("row %d became blank after length change during playback", row)
		}
	}
}
