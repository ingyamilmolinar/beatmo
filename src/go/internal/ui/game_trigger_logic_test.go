package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// ---------- TestApplyNodeDecision ----------

func TestApplyNodeDecision(t *testing.T) {
	assertDefaultParityState(t)

	t.Run("VolumeMul doubles volume", func(t *testing.T) {
		v, p, d := applyNodeDecision(0.5, 0, 1, model.NodeDecision{VolumeMul: 2})
		if v != 1.0 {
			t.Fatalf("volume: got %f want 1.0", v)
		}
		if p != 0 {
			t.Fatalf("pitch changed: got %f want 0", p)
		}
		if d != 1 {
			t.Fatalf("duration changed: got %f want 1", d)
		}
	})

	t.Run("PitchDelta adds to pitch", func(t *testing.T) {
		v, p, d := applyNodeDecision(1, 3, 1, model.NodeDecision{PitchDelta: 5})
		if p != 8 {
			t.Fatalf("pitch: got %f want 8", p)
		}
		if v != 1 {
			t.Fatalf("volume changed: got %f want 1", v)
		}
		if d != 1 {
			t.Fatalf("duration changed: got %f want 1", d)
		}
	})

	t.Run("DurationMul halves duration", func(t *testing.T) {
		v, p, d := applyNodeDecision(1, 0, 2, model.NodeDecision{DurationMul: 0.5})
		if d != 1.0 {
			t.Fatalf("duration: got %f want 1.0", d)
		}
		if v != 1 {
			t.Fatalf("volume changed: got %f want 1", v)
		}
		if p != 0 {
			t.Fatalf("pitch changed: got %f want 0", p)
		}
	})

	t.Run("zero values cause no change", func(t *testing.T) {
		v, p, d := applyNodeDecision(0.7, 3.5, 1.2, model.NodeDecision{})
		if v != 0.7 {
			t.Fatalf("volume: got %f want 0.7", v)
		}
		if p != 3.5 {
			t.Fatalf("pitch: got %f want 3.5", p)
		}
		if d != 1.2 {
			t.Fatalf("duration: got %f want 1.2", d)
		}
	})

	t.Run("combined modifiers", func(t *testing.T) {
		v, p, d := applyNodeDecision(1, 0, 1, model.NodeDecision{
			VolumeMul:   0.5,
			PitchDelta:  -3,
			DurationMul: 2,
		})
		if v != 0.5 {
			t.Fatalf("volume: got %f want 0.5", v)
		}
		if p != -3 {
			t.Fatalf("pitch: got %f want -3", p)
		}
		if d != 2 {
			t.Fatalf("duration: got %f want 2", d)
		}
	})
}

// ---------- TestDeterministicRoll ----------

func TestDeterministicRoll(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	t.Run("same inputs same output", func(t *testing.T) {
		r1 := g.deterministicRoll(0, 5, model.NodeID(42))
		r2 := g.deterministicRoll(0, 5, model.NodeID(42))
		if r1 != r2 {
			t.Fatalf("not deterministic: %f != %f", r1, r2)
		}
	})

	t.Run("different inputs produce different outputs", func(t *testing.T) {
		base := g.deterministicRoll(0, 0, model.NodeID(1))
		diffRow := g.deterministicRoll(1, 0, model.NodeID(1))
		diffIdx := g.deterministicRoll(0, 1, model.NodeID(1))
		diffID := g.deterministicRoll(0, 0, model.NodeID(2))

		if base == diffRow {
			t.Fatalf("different row produced same result")
		}
		if base == diffIdx {
			t.Fatalf("different idx produced same result")
		}
		if base == diffID {
			t.Fatalf("different id produced same result")
		}
	})

	t.Run("result in [0, 1)", func(t *testing.T) {
		for row := 0; row < 10; row++ {
			for idx := 0; idx < 10; idx++ {
				r := g.deterministicRoll(row, idx, model.NodeID(row*10+idx))
				if r < 0 || r >= 1 {
					t.Fatalf("roll out of [0,1): row=%d idx=%d got %f", row, idx, r)
				}
			}
		}
	})
}

// ---------- TestSeqShouldTriggerNode_NoLogic ----------

func TestSeqShouldTriggerNode_NoLogic(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, a)
	g.start = a
	g.graph.StartNodeID = a.ID
	g.updateBeatInfos()

	n, ok := g.graph.GetNodeByID(a.ID)
	if !ok {
		t.Fatal("node not found")
	}
	info := model.BeatInfo{NodeID: a.ID, NodeType: model.NodeTypeRegular}
	counts := map[model.NodeID]int{}

	// No logic (kind="") should always trigger.
	for i := 0; i < 10; i++ {
		if !g.seqShouldTriggerNode(0, i, info, n, counts, nil) {
			t.Fatalf("expected trigger at idx=%d with no logic", i)
		}
	}
}

// ---------- TestSeqShouldTriggerNode_EveryN ----------

func TestSeqShouldTriggerNode_EveryN(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, a)
	g.start = a
	g.graph.StartNodeID = a.ID

	// Set every_n_triggers with N=3.
	if nd, ok := g.graph.GetNodeByID(a.ID); ok {
		p := nd.Params
		p.LogicKind = "every_n_triggers"
		p.LogicN = 3
		g.graph.SetNodeParams(a.ID, p)
	}
	g.updateBeatInfos()

	n, ok := g.graph.GetNodeByID(a.ID)
	if !ok {
		t.Fatal("node not found")
	}
	info := model.BeatInfo{NodeID: a.ID, NodeType: model.NodeTypeRegular}
	counts := map[model.NodeID]int{}

	// Call 9 times. Triggers should happen on the 3rd, 6th, 9th calls
	// (when count % 3 == 0).
	var results []bool
	for i := 0; i < 9; i++ {
		results = append(results, g.seqShouldTriggerNode(0, i, info, n, counts, nil))
	}

	expected := []bool{false, false, true, false, false, true, false, false, true}
	for i, want := range expected {
		if results[i] != want {
			t.Fatalf("idx=%d: got %v want %v (results=%v)", i, results[i], want, results)
		}
	}
}

// ---------- TestSeqShouldTriggerNode_SkipEveryN ----------

func TestSeqShouldTriggerNode_SkipEveryN(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, a)
	g.start = a
	g.graph.StartNodeID = a.ID

	// Set skip_every_n with N=2.
	if nd, ok := g.graph.GetNodeByID(a.ID); ok {
		p := nd.Params
		p.LogicKind = "skip_every_n"
		p.LogicN = 2
		g.graph.SetNodeParams(a.ID, p)
	}
	g.updateBeatInfos()

	n, ok := g.graph.GetNodeByID(a.ID)
	if !ok {
		t.Fatal("node not found")
	}
	info := model.BeatInfo{NodeID: a.ID, NodeType: model.NodeTypeRegular}
	counts := map[model.NodeID]int{}

	// Call 6 times. Skips every 2nd call (count%2==0 means skip).
	// count after increment: 1, 2, 3, 4, 5, 6
	// skip when count%2==0:  no, yes, no, yes, no, yes
	var results []bool
	for i := 0; i < 6; i++ {
		results = append(results, g.seqShouldTriggerNode(0, i, info, n, counts, nil))
	}

	expected := []bool{true, false, true, false, true, false}
	for i, want := range expected {
		if results[i] != want {
			t.Fatalf("idx=%d: got %v want %v (results=%v)", i, results[i], want, results)
		}
	}
}

// ---------- TestSeqShouldTriggerNode_Probability ----------

func TestSeqShouldTriggerNode_Probability(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, a)
	g.start = a
	g.graph.StartNodeID = a.ID
	g.updateBeatInfos()

	info := model.BeatInfo{NodeID: a.ID, NodeType: model.NodeTypeRegular}
	counts := map[model.NodeID]int{}

	t.Run("p=0 never triggers", func(t *testing.T) {
		if nd, ok := g.graph.GetNodeByID(a.ID); ok {
			p := nd.Params
			p.LogicKind = "probability"
			p.LogicP = 0
			g.graph.SetNodeParams(a.ID, p)
		}
		n, _ := g.graph.GetNodeByID(a.ID)
		for i := 0; i < 20; i++ {
			if g.seqShouldTriggerNode(0, i, info, n, counts, nil) {
				t.Fatalf("p=0 triggered at idx=%d", i)
			}
		}
	})

	t.Run("p=1 always triggers", func(t *testing.T) {
		if nd, ok := g.graph.GetNodeByID(a.ID); ok {
			p := nd.Params
			p.LogicKind = "probability"
			p.LogicP = 1
			g.graph.SetNodeParams(a.ID, p)
		}
		n, _ := g.graph.GetNodeByID(a.ID)
		for i := 0; i < 20; i++ {
			if !g.seqShouldTriggerNode(0, i, info, n, counts, nil) {
				t.Fatalf("p=1 did not trigger at idx=%d", i)
			}
		}
	})

	t.Run("p=0.5 deterministic based on hash", func(t *testing.T) {
		if nd, ok := g.graph.GetNodeByID(a.ID); ok {
			p := nd.Params
			p.LogicKind = "probability"
			p.LogicP = 0.5
			g.graph.SetNodeParams(a.ID, p)
		}
		n, _ := g.graph.GetNodeByID(a.ID)

		// Run twice and verify deterministic: same inputs produce same results.
		var first, second []bool
		for i := 0; i < 20; i++ {
			first = append(first, g.seqShouldTriggerNode(0, i, info, n, counts, nil))
		}
		for i := 0; i < 20; i++ {
			second = append(second, g.seqShouldTriggerNode(0, i, info, n, counts, nil))
		}
		for i := range first {
			if first[i] != second[i] {
				t.Fatalf("non-deterministic at idx=%d: %v vs %v", i, first[i], second[i])
			}
		}

		// With p=0.5 we expect a mix of true and false over 20 samples.
		trueCount := 0
		for _, v := range first {
			if v {
				trueCount++
			}
		}
		if trueCount == 0 || trueCount == 20 {
			t.Fatalf("p=0.5 produced all same results (%d/20 true)", trueCount)
		}
	})
}

// ---------- TestSeqShouldTriggerNode_NonAudibleTypes ----------

func TestSeqShouldTriggerNode_NonAudibleTypes(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	// Create a regular node just to have a valid ID in the graph.
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, a)
	g.start = a
	g.graph.StartNodeID = a.ID
	g.updateBeatInfos()

	n, _ := g.graph.GetNodeByID(a.ID)
	counts := map[model.NodeID]int{}

	t.Run("invisible never triggers", func(t *testing.T) {
		info := model.BeatInfo{NodeID: a.ID, NodeType: model.NodeTypeInvisible}
		if g.seqShouldTriggerNode(0, 0, info, n, counts, nil) {
			t.Fatal("invisible node should not trigger")
		}
	})

	t.Run("silent never triggers", func(t *testing.T) {
		info := model.BeatInfo{NodeID: a.ID, NodeType: model.NodeTypeSilent}
		if g.seqShouldTriggerNode(0, 0, info, n, counts, nil) {
			t.Fatal("silent node should not trigger")
		}
	})

	t.Run("regular triggers", func(t *testing.T) {
		info := model.BeatInfo{NodeID: a.ID, NodeType: model.NodeTypeRegular}
		if !g.seqShouldTriggerNode(0, 0, info, n, counts, nil) {
			t.Fatal("regular node should trigger with no logic")
		}
	})

	t.Run("mute triggers", func(t *testing.T) {
		info := model.BeatInfo{NodeID: a.ID, NodeType: model.NodeTypeMute}
		if !g.seqShouldTriggerNode(0, 0, info, n, counts, nil) {
			t.Fatal("mute node should trigger with no logic")
		}
	})
}

// ---------- TestEvalNodeParamsOnly ----------

func TestEvalNodeParamsOnly(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, a)
	g.start = a
	g.graph.StartNodeID = a.ID
	g.drum.Rows[0].Origin = a.ID
	g.drum.Rows[0].Node = a

	t.Run("regular node with custom params", func(t *testing.T) {
		// Set node params.
		if nd, ok := g.graph.GetNodeByID(a.ID); ok {
			p := nd.Params
			p.Volume = 0.5
			p.Pitch = 2
			p.Duration = 0.8
			g.graph.SetNodeParams(a.ID, p)
		}
		// Set row volume.
		g.drum.Rows[0].Volume = 0.6
		g.updateBeatInfos()

		info := model.BeatInfo{NodeID: a.ID, NodeType: model.NodeTypeRegular}
		vol, pitch, dur := g.evalNodeParamsOnly(0, 0, info)

		// vol = row volume (0.6) * node volume (0.5) = 0.3
		wantVol := 0.6 * 0.5
		if vol < wantVol-0.001 || vol > wantVol+0.001 {
			t.Fatalf("volume: got %f want %f", vol, wantVol)
		}
		if pitch != 2 {
			t.Fatalf("pitch: got %f want 2", pitch)
		}
		if dur < 0.799 || dur > 0.801 {
			t.Fatalf("duration: got %f want 0.8", dur)
		}
	})

	t.Run("silent node returns defaults", func(t *testing.T) {
		info := model.BeatInfo{NodeID: a.ID, NodeType: model.NodeTypeSilent}
		vol, pitch, dur := g.evalNodeParamsOnly(0, 0, info)
		if vol != 0 {
			t.Fatalf("silent vol: got %f want 0", vol)
		}
		if pitch != 0 {
			t.Fatalf("silent pitch: got %f want 0", pitch)
		}
		if dur != 1 {
			t.Fatalf("silent dur: got %f want 1", dur)
		}
	})

	t.Run("invisible node returns defaults", func(t *testing.T) {
		info := model.BeatInfo{NodeID: a.ID, NodeType: model.NodeTypeInvisible}
		vol, pitch, dur := g.evalNodeParamsOnly(0, 0, info)
		if vol != 0 {
			t.Fatalf("invisible vol: got %f want 0", vol)
		}
		if pitch != 0 {
			t.Fatalf("invisible pitch: got %f want 0", pitch)
		}
		if dur != 1 {
			t.Fatalf("invisible dur: got %f want 1", dur)
		}
	})
}

// ---------- TestIncrementLogicTriggerCount ----------

func TestIncrementLogicTriggerCount(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	id1 := model.NodeID(100)
	id2 := model.NodeID(200)

	t.Run("increments from zero", func(t *testing.T) {
		c := g.incrementLogicTriggerCount(0, id1)
		if c != 1 {
			t.Fatalf("first increment: got %d want 1", c)
		}
	})

	t.Run("subsequent increments", func(t *testing.T) {
		c2 := g.incrementLogicTriggerCount(0, id1)
		c3 := g.incrementLogicTriggerCount(0, id1)
		if c2 != 2 {
			t.Fatalf("second increment: got %d want 2", c2)
		}
		if c3 != 3 {
			t.Fatalf("third increment: got %d want 3", c3)
		}
	})

	t.Run("different node IDs are independent", func(t *testing.T) {
		c := g.incrementLogicTriggerCount(0, id2)
		if c != 1 {
			t.Fatalf("different node first increment: got %d want 1", c)
		}
	})

	t.Run("different rows are independent", func(t *testing.T) {
		c := g.incrementLogicTriggerCount(1, id1)
		if c != 1 {
			t.Fatalf("different row first increment: got %d want 1", c)
		}
	})
}

// ---------- TestSeqShouldTriggerNode_EveryN_Mute ----------

func TestSeqShouldTriggerNode_EveryN_Mute(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	a := g.tryAddNode(0, 0, model.NodeTypeMute)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, a)
	g.start = a
	g.graph.StartNodeID = a.ID

	// Set every_n_triggers with N=2 on a mute node.
	if nd, ok := g.graph.GetNodeByID(a.ID); ok {
		p := nd.Params
		p.LogicKind = "every_n_triggers"
		p.LogicN = 2
		g.graph.SetNodeParams(a.ID, p)
	}
	g.updateBeatInfos()

	n, ok := g.graph.GetNodeByID(a.ID)
	if !ok {
		t.Fatal("node not found")
	}

	// For mute nodes, every_n_triggers uses cycle-based logic instead of
	// counts. Build a snapshot with a 2-node path so cycleLen=2.
	path := []model.BeatInfo{
		{NodeID: a.ID, NodeType: model.NodeTypeMute},
		{NodeID: b.ID, NodeType: model.NodeTypeRegular},
	}
	snap := &seqPathSnapshot{
		beatInfosByRow: [][]model.BeatInfo{path},
	}

	info := model.BeatInfo{NodeID: a.ID, NodeType: model.NodeTypeMute}
	counts := map[model.NodeID]int{}

	// cycleLen=2, so cycle = idx/2.
	// N=2: triggers when (cycle+1)%2==0, i.e. cycle is odd (1, 3, 5, ...).
	// idx=0 -> cycle=0 -> (0+1)%2=1!=0 -> false
	// idx=1 -> cycle=0 -> false
	// idx=2 -> cycle=1 -> (1+1)%2=0 -> true
	// idx=3 -> cycle=1 -> true
	// idx=4 -> cycle=2 -> (2+1)%2=1!=0 -> false
	// idx=5 -> cycle=2 -> false
	expected := []bool{false, false, true, true, false, false}
	for i, want := range expected {
		got := g.seqShouldTriggerNode(0, i, info, n, counts, snap)
		if got != want {
			t.Fatalf("idx=%d: got %v want %v", i, got, want)
		}
	}
}
