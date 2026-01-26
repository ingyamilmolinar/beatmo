package ui

import (
	"math/rand"
	"testing"

	"github.com/ingyamilmolinar/tunkul/core/model"
	"github.com/ingyamilmolinar/tunkul/internal/timeline"
)

// Property test: during chaotic live edits while "playing", every cell in the
// DrumView window must match the engine predictor for the same absolute index.
// This exercises graph -> predictor -> drumview caches under repeated
// delete/re-add and logic toggles.
func TestDrumView_LiveEditRandomizedKeepsParity(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1024, 720)
	g.drum.SetFollow(true)
	g.drum.SetLength(128)

	// Base loop (8 nodes) to give room for edits.
	nodes := make([]*uiNode, 8)
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

	g.drum.SetBPM(120)
	g.updateBeatInfos()
	g.refreshDrumRow()
	if len(g.nextBeatIdxs) == 0 {
		g.nextBeatIdxs = []int{0}
	}
	g.SetPlaying(true)

	rng := rand.New(rand.NewSource(2)) // deterministic

	// Helpers ---------------------------------------------------------------
	addNodeAt := func(i int) *uiNode {
		return g.tryAddNode(i, 0, model.NodeTypeRegular)
	}
	hasEdge := func(a, b *uiNode) bool {
		_, ok := g.graph.Edges[[2]model.NodeID{a.ID, b.ID}]
		return ok
	}
	reconnect := func(pos int) {
		left := g.nodeAt(pos-1, 0)
		right := g.nodeAt((pos+1)%16, 0)
		cur := g.nodeAt(pos, 0)
		if cur == nil {
			cur = addNodeAt(pos)
		}
		if left != nil && !hasEdge(left, cur) {
			g.addEdge(left, cur)
		}
		if right != nil && !hasEdge(cur, right) {
			g.addEdge(cur, right)
		}
	}
	mutate := func() {
		pos := rng.Intn(10) + 2 // stay away from start node
		switch rng.Intn(4) {
		case 0: // delete then re-add
			if n := g.nodeAt(pos, 0); n != nil {
				g.deleteNode(n)
			}
			reconnect(pos)
		case 1: // toggle node type
			if n := g.nodeAt(pos, 0); n != nil {
				if node, ok := g.graph.GetNodeByID(n.ID); ok {
					if node.Type == model.NodeTypeRegular {
						node.Type = model.NodeTypeMute
					} else {
						node.Type = model.NodeTypeRegular
					}
					g.graph.Nodes[n.ID] = node
					g.cacheNode(n.ID)
					g.notifyPredictorNode(n.ID)
				}
			}
		case 2: // probability tweak
			if n := g.nodeAt(pos, 0); n != nil {
				if node, ok := g.graph.GetNodeByID(n.ID); ok {
					params := node.Params
					params.LogicKind = "probability"
					params.LogicP = 0.2 + rng.Float64()*0.6
					g.graph.SetNodeParams(n.ID, params)
					g.cacheNode(n.ID)
					g.notifyPredictorNode(n.ID)
				}
			}
		default: // detour insert
			left := g.nodeAt(pos, 0)
			right := g.nodeAt(pos+1, 0)
			if left != nil && right != nil && hasEdge(left, right) {
				g.deleteEdge(left, right)
				mid := addNodeAt(pos + 1)
				if mid != nil {
					g.addEdge(left, mid)
					g.addEdge(mid, right)
				}
			}
		}
		g.updateBeatInfos()
	}

	checkWindow := func() {
		horizon := g.drum.Offset + g.drum.Length + g.grid.MaxDiv()*4
		g.engine.Predictor.Ensure(horizon)
		g.refreshDrumRow()
		if len(g.drum.Rows) == 0 {
			t.Fatalf("drum rows empty")
		}
		r := g.drum.Rows[0]
		for j := 0; j < len(r.Steps); j++ {
			abs := g.drum.Offset + j
			bi := g.beatInfoAtRow(0, abs)
			// Immutable playback/import history is the source of truth; fall
			// back to predictor for everything else.
			if v, _, kind, ok := g.timelineCommittedWithKind(0, abs); ok && (kind == timeline.CommitKindPlayback || kind == timeline.CommitKindImport) {
				want := v
				got := r.Steps[j]
				if got != want {
					t.Fatalf("parity mismatch abs=%d (immutable) want=%v got=%v kind=%v playing=%v offset=%d len=%d", abs, want, got, kind, g.Playing(), g.drum.Offset, g.drum.Length)
				}
				continue
			}
			want := false
			switch bi.NodeType {
			case model.NodeTypeRegular:
				want = g.engine.Predictor.VisibleAt(0, abs)
			case model.NodeTypeMute:
				want = g.engine.Predictor.TriggeredAt(0, abs)
			default:
				want = false
			}
			got := r.Steps[j]
			if got != want {
				if v, typ, kind, ok := g.timelineCommittedWithKind(0, abs); ok {
					t.Fatalf("parity mismatch abs=%d type=%v want=%v got=%v commit=%v typ=%v kind=%v playing=%v offset=%d len=%d", abs, bi.NodeType, want, got, v, typ, kind, g.Playing(), g.drum.Offset, g.drum.Length)
				}
				t.Fatalf("parity mismatch abs=%d type=%v want=%v got=%v commit=none playing=%v offset=%d len=%d", abs, bi.NodeType, want, got, g.Playing(), g.drum.Offset, g.drum.Length)
			}
		}
	}

	// Main loop: advance playhead manually, mutate ahead of it, assert parity.
	for step := 0; step < 20; step++ {
		if step%5 == 0 {
			mutate()
		}
		cur := g.nextBeatIdxs[0]
		info := g.beatInfoAtRow(0, cur)
		g.applySequencerHighlight(0, cur, info)
		// Drift follow window.
		g.drum.TrackBeat(cur)
		checkWindow()
		g.nextBeatIdxs[0] = cur + 1
	}
}
