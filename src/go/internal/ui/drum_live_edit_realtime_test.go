package ui

import (
	"testing"

	"github.com/ingyamilmolinar/tunkul/core/model"
)

// Graph edits must reflect immediately in the highlighted cell (current beat)
// without waiting extra frames.
func TestDrumView_ImmediateHighlightReflectsNodeChange(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(800, 600)
	g.drum.SetFollow(true)
	g.drum.SetLength(16)

	// Simple 3-step loop.
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	c := g.tryAddNode(2, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, c)
	g.addEdge(c, a)
	g.start = a
	g.graph.StartNodeID = a.ID
	g.drum.Rows[0].Origin = a.ID
	g.drum.Rows[0].Node = a

	g.updateBeatInfos()
	g.refreshDrumRow()
	g.drum.TrackBeat(0)

	if !g.drum.Rows[0].Steps[0] {
		t.Fatalf("expected current cell ON before edit")
	}

	// Make current node invisible and ensure the change is visible immediately.
	n := g.graph.Nodes[a.ID]
	n.Type = model.NodeTypeInvisible
	g.graph.Nodes[a.ID] = n
	g.cacheNode(a.ID)
	g.notifyPredictorNode(a.ID)
	g.updateBeatInfos()
	g.refreshDrumRow()
	g.drum.TrackBeat(0)

	if g.drum.Rows[0].Steps[0] {
		t.Fatalf("current cell did not turn OFF immediately after edit")
	}

	// Restore to regular and verify it turns back ON in the same frame.
	n.Type = model.NodeTypeRegular
	g.graph.Nodes[a.ID] = n
	g.cacheNode(a.ID)
	g.notifyPredictorNode(a.ID)
	g.updateBeatInfos()
	g.refreshDrumRow()
	g.drum.TrackBeat(0)

	if !g.drum.Rows[0].Steps[0] {
		t.Fatalf("current cell did not turn ON immediately after reverting")
	}
}

// Repeated remove/re-add should flip the highlighted cell in the same update
// while keeping TrackBeat centered between past and future.
func TestDrumView_RemoveReaddImmediate(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(800, 600)
	g.drum.SetFollow(true)
	g.drum.SetLength(16)

	// 0->1->2 loop.
	nodes := []*uiNode{
		g.tryAddNode(0, 0, model.NodeTypeRegular),
		g.tryAddNode(1, 0, model.NodeTypeRegular),
		g.tryAddNode(2, 0, model.NodeTypeRegular),
	}
	g.addEdge(nodes[0], nodes[1])
	g.addEdge(nodes[1], nodes[2])
	g.addEdge(nodes[2], nodes[0])
	g.start = nodes[0]
	g.graph.StartNodeID = nodes[0].ID
	g.drum.Rows[0].Origin = nodes[0].ID
	g.drum.Rows[0].Node = nodes[0]

	g.updateBeatInfos()
	g.refreshDrumRow()

	for cycle := 0; cycle < 3; cycle++ {
		cur := cycle % 3
		bi := g.beatInfoAtRow(0, cur)
		g.applySequencerHighlight(0, cur, bi)
		g.drum.TrackBeat(cur)

		idx := cur - g.drum.Offset
		if idx < 0 || idx >= len(g.drum.Rows[0].Steps) {
			t.Fatalf("cur outside window: cur=%d offset=%d len=%d", cur, g.drum.Offset, len(g.drum.Rows[0].Steps))
		}
		if !g.drum.Rows[0].Steps[idx] {
			t.Fatalf("expected ON before edit at cur=%d", cur)
		}

		// Flip node at cur invisible and expect OFF immediately.
		n := g.nodeAt(cur, 0)
		if n == nil {
			t.Fatalf("missing node at cur=%d", cur)
		}
		m := g.graph.Nodes[n.ID]
		m.Type = model.NodeTypeInvisible
		g.graph.Nodes[n.ID] = m
		g.cacheNode(n.ID)
		g.notifyPredictorNode(n.ID)
		g.updateBeatInfos()
		g.refreshDrumRow()
		g.drum.TrackBeat(cur)

		if g.drum.Rows[0].Steps[idx] {
			t.Fatalf("invisible not reflected immediately at cur=%d", cur)
		}

		// Restore to regular and expect ON immediately.
		m.Type = model.NodeTypeRegular
		g.graph.Nodes[n.ID] = m
		g.cacheNode(n.ID)
		g.notifyPredictorNode(n.ID)
		g.updateBeatInfos()
		g.refreshDrumRow()
		g.drum.TrackBeat(cur)

		if !g.drum.Rows[0].Steps[idx] {
			t.Fatalf("restore not reflected immediately at cur=%d", cur)
		}

	}
}
