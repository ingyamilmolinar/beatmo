package ui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ingyamilmolinar/tunkul/core/model"
)

func TestDrumView_SampleHighTomReaddParity(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	g.drum.SetFollow(false)
	g.drum.SetLength(192)

	// Use testdata file instead of external tunkul.json to avoid dependency on repo state
	jsonPath := filepath.Join("testdata", "future_cache_loop.json")
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("read testdata json: %v", err)
	}
	if err := g.Import(data); err != nil {
		t.Fatalf("import testdata json: %v", err)
	}
	g.refreshDrumRow()

	row := -1
	const targetInstrument = "sample-high-tom-9-wonder"
	for i, r := range g.drum.Rows {
		if r.Instrument == targetInstrument {
			row = i
			break
		}
	}
	if row < 0 {
		t.Fatalf("instrument %s not found in drum rows", targetInstrument)
	}

	pressPlay(t, g.drum)
	advancePlayback(g, 250*time.Millisecond)

	start := g.nextBeatIdxs[row]
	var (
		targetAbs int
		origNode  *uiNode
		graphNode model.Node
		preds     []model.NodeID
		succs     []model.NodeID
		found     bool
		searchAbs = start
	)
	for tries := 0; tries < 12; tries++ {
		targetAbs, origNode = futureNodeForRow(g, row, searchAbs, g.grid.MaxDiv()*8)
		if targetAbs < 0 || origNode == nil {
			t.Fatalf("failed to locate candidate node after %d attempts (start=%d offset=%d)", tries, searchAbs, g.drum.Offset)
		}
		var ok bool
		graphNode, ok = g.graph.GetNodeByID(origNode.ID)
		if !ok {
			t.Fatalf("graph node %d missing", origNode.ID)
		}
		preds, succs = collectNeighbors(g, origNode.ID)

		g.deleteNode(origNode)
		g.updateBeatInfos()
		g.refreshDrumRow()

		g.engine.Predictor.Ensure(targetAbs + g.grid.MaxDiv()*4)
		if !g.engine.Predictor.VisibleAt(row, targetAbs) {
			found = true
			break
		}

		// Restore node and try a later future beat.
		restore := g.tryAddNode(origNode.I, origNode.J, graphNode.Type)
		for _, id := range preds {
			if n := g.nodeByID(id); n != nil {
				g.addEdge(n, restore)
			}
		}
		for _, id := range succs {
			if n := g.nodeByID(id); n != nil {
				g.addEdge(restore, n)
			}
		}
		g.graph.SetNodeParams(restore.ID, graphNode.Params)
		g.updateBeatInfos()
		searchAbs = targetAbs + 1
	}
	if !found {
		t.Fatalf("could not find future node whose predictor toggles off after delete")
	}

	re := g.tryAddNode(origNode.I, origNode.J, graphNode.Type)
	for _, id := range preds {
		if n := g.nodeByID(id); n != nil {
			g.addEdge(n, re)
		}
	}
	for _, id := range succs {
		if n := g.nodeByID(id); n != nil {
			g.addEdge(re, n)
		}
	}
	g.graph.SetNodeParams(re.ID, graphNode.Params)
	g.updateBeatInfos()

	g.drum.Offset = targetAbs - g.drum.Length/4
	if g.drum.Offset < 0 {
		g.drum.Offset = 0
	}

	advancePlayback(g, 100*time.Millisecond)

	horizon := g.drum.Offset + g.drum.Length + g.grid.MaxDiv()*4
	g.engine.Predictor.Ensure(horizon)
	g.refreshDrumRow()

	rel := targetAbs - g.drum.Offset
	if rel < 0 || rel >= len(g.drum.Rows[row].Steps) {
		t.Fatalf("target rel=%d out of range offset=%d len=%d", rel, g.drum.Offset, len(g.drum.Rows[row].Steps))
	}
	got := g.drum.Rows[row].Steps[rel]
	want := g.engine.Predictor.VisibleAt(row, targetAbs)
	if got != want {
		t.Fatalf("parity mismatch row=%d abs=%d rel=%d want=%v got=%v freeze=%v next=%v offset=%d",
			row, targetAbs, rel, want, got, g.frozenUpToByRow, g.nextBeatIdxs, g.drum.Offset)
	}
}

func futureNodeForRow(g *Game, row, start, lookahead int) (int, *uiNode) {
	if row < 0 || row >= len(g.drum.Rows) {
		return -1, nil
	}
	if start < g.drum.Offset {
		start = g.drum.Offset
	}
	end := start + lookahead
	for abs := start; abs < end; abs++ {
		info := g.beatInfoAtRow(row, abs)
		if info.NodeID == model.InvalidNodeID || info.NodeType != model.NodeTypeRegular {
			continue
		}
		if n := g.nodeByID(info.NodeID); n != nil {
			return abs, n
		}
	}
	return -1, nil
}

func collectNeighbors(g *Game, id model.NodeID) (preds []model.NodeID, succs []model.NodeID) {
	for edge := range g.graph.Edges {
		if edge[1] == id {
			preds = append(preds, edge[0])
		}
		if edge[0] == id {
			succs = append(succs, edge[1])
		}
	}
	return preds, succs
}
