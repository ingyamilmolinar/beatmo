package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Build a small loop: s0 -> a -> s1 -> b -> s2 -> s0
func buildPrevChain(g *Game) (s0, a, b *uiNode) {
	s0 = g.tryAddNode(0, 0, model.NodeTypeSilent)
	a = g.tryAddNode(1, 0, model.NodeTypeRegular)
	s1 := g.tryAddNode(2, 0, model.NodeTypeSilent)
	b = g.tryAddNode(3, 0, model.NodeTypeRegular)
	s2 := g.tryAddNode(4, 0, model.NodeTypeSilent)
	g.addEdge(s0, a)
	g.addEdge(a, s1)
	g.addEdge(s1, b)
	g.addEdge(b, s2)
	g.addEdge(s2, s0)
	return s0, a, b
}

func TestRuntimeTriggerIfPrevTriggered(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	s0, a, b := buildPrevChain(g)
	g.start = s0
	g.graph.StartNodeID = s0.ID
	// Make A skip every 2nd trigger so B can observe both prev-triggered states.
	if n, ok := g.graph.GetNodeByID(a.ID); ok {
		p := n.Params
		p.LogicKind = "skip_every_n"
		p.LogicN = 2
		g.graph.SetNodeParams(a.ID, p)
	}
	// B triggers only if previous regular triggered.
	if n, ok := g.graph.GetNodeByID(b.ID); ok {
		p := n.Params
		p.LogicKind = "trigger_if_prev_triggered"
		g.graph.SetNodeParams(b.ID, p)
	}
	g.updateBeatInfos()

	idxA := make([]int, 0, 2)
	idxB := make([]int, 0, 2)
	for i := 0; i < 64 && (len(idxA) < 2 || len(idxB) < 2); i++ {
		info := g.beatInfoAtRow(0, i)
		if info.NodeID == a.ID && info.NodeType == model.NodeTypeRegular {
			idxA = append(idxA, i)
		}
		if info.NodeID == b.ID && info.NodeType == model.NodeTypeRegular {
			idxB = append(idxB, i)
		}
	}
	if len(idxA) < 2 || len(idxB) < 2 {
		t.Fatalf("missing repeated a/b in beat path: idxA=%v idxB=%v", idxA, idxB)
	}
	horizon := idxB[1] + 1
	g.engine.Predictor.Ensure(horizon)

	if got := g.engine.Predictor.AudibleAt(0, idxA[0]); !got {
		t.Fatalf("expected a audible at abs=%d", idxA[0])
	}
	if got := g.engine.Predictor.AudibleAt(0, idxA[1]); got {
		t.Fatalf("expected a suppressed at abs=%d (skip_every_n=2)", idxA[1])
	}
	if got := g.engine.Predictor.AudibleAt(0, idxB[0]); !got {
		t.Fatalf("expected b audible at abs=%d when prev a triggered", idxB[0])
	}
	if got := g.engine.Predictor.AudibleAt(0, idxB[1]); got {
		t.Fatalf("expected b suppressed at abs=%d when prev a skipped", idxB[1])
	}
}

func TestRuntimeTriggerIfPrevSkipped(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	s0, a, b := buildPrevChain(g)
	g.start = s0
	g.graph.StartNodeID = s0.ID
	// Make A skip every 2nd trigger so B can observe both prev-triggered states.
	if n, ok := g.graph.GetNodeByID(a.ID); ok {
		p := n.Params
		p.LogicKind = "skip_every_n"
		p.LogicN = 2
		g.graph.SetNodeParams(a.ID, p)
	}
	// B triggers only if previous regular skipped.
	if n, ok := g.graph.GetNodeByID(b.ID); ok {
		p := n.Params
		p.LogicKind = "trigger_if_prev_skipped"
		g.graph.SetNodeParams(b.ID, p)
	}
	g.updateBeatInfos()

	idxA := make([]int, 0, 2)
	idxB := make([]int, 0, 2)
	for i := 0; i < 64 && (len(idxA) < 2 || len(idxB) < 2); i++ {
		info := g.beatInfoAtRow(0, i)
		if info.NodeID == a.ID && info.NodeType == model.NodeTypeRegular {
			idxA = append(idxA, i)
		}
		if info.NodeID == b.ID && info.NodeType == model.NodeTypeRegular {
			idxB = append(idxB, i)
		}
	}
	if len(idxA) < 2 || len(idxB) < 2 {
		t.Fatalf("missing repeated a/b in beat path: idxA=%v idxB=%v", idxA, idxB)
	}
	horizon := idxB[1] + 1
	g.engine.Predictor.Ensure(horizon)

	if got := g.engine.Predictor.AudibleAt(0, idxA[0]); !got {
		t.Fatalf("expected a audible at abs=%d", idxA[0])
	}
	if got := g.engine.Predictor.AudibleAt(0, idxA[1]); got {
		t.Fatalf("expected a suppressed at abs=%d (skip_every_n=2)", idxA[1])
	}
	if got := g.engine.Predictor.AudibleAt(0, idxB[0]); got {
		t.Fatalf("expected b suppressed at abs=%d when prev a triggered", idxB[0])
	}
	if got := g.engine.Predictor.AudibleAt(0, idxB[1]); !got {
		t.Fatalf("expected b audible at abs=%d when prev a skipped", idxB[1])
	}
}
