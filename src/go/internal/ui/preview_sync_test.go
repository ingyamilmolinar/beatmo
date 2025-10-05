package ui

import (
	"github.com/ingyamilmolinar/tunkul/core/model"
	"testing"
)

// computeExpectedPreview computes the next N preview cells starting at baseIdx
// using the current live playback state as seed to ensure predictions match playback.
func computeExpectedPreview(g *Game, row, baseIdx, n int) []bool {
	exp := make([]bool, 0, n)
	// Seed from live state
	simCounts := map[model.NodeID]int{}
	if m, ok := g.nodeTriggerCountsByRow[row]; ok {
		for id, c := range m {
			simCounts[id] = c
		}
	}
	simLastTrig := map[model.NodeID]bool{}
	if m, ok := g.lastTriggeredByRow[row]; ok {
		for id, v := range m {
			simLastTrig[id] = v
		}
	}
	simLastFired := model.InvalidNodeID
	if row < len(g.lastFiredNodeByRow) {
		simLastFired = g.lastFiredNodeByRow[row]
	}
	simGate := 0
	if row < len(g.muteUntilByRow) {
		simGate = g.muteUntilByRow[row]
	}
	// Loop info for seam mask
	loop := false
	start := 0
	seg := 0
	if row < len(g.isLoopByRow) && g.isLoopByRow[row] {
		loop = true
		start = g.loopStartByRow[row]
		seg = loopSegmentLen(g.beatInfosByRow[row], start)
	}
	// Advance from live base to window start if needed
	liveBase := 0
	if g.nextBeatIdxs != nil && row < len(g.nextBeatIdxs) {
		liveBase = g.nextBeatIdxs[row]
	}
	if baseIdx > liveBase {
		for k := liveBase; k < baseIdx; k++ {
			bi := g.beatInfoAtRow(row, k)
			if bi.NodeType != model.NodeTypeRegular && bi.NodeType != model.NodeTypeMute {
				continue
			}
			_, triggered := g.simEvalAudible(row, k, bi, simCounts, &simLastFired, simLastTrig, &simGate)
			if bi.NodeType == model.NodeTypeRegular {
				if triggered {
					simLastFired = bi.NodeID
					simLastTrig[bi.NodeID] = true
				} else {
					simLastTrig[bi.NodeID] = false
				}
			} else {
				simLastTrig[bi.NodeID] = triggered
			}
		}
	}
	// Build expected booleans
	for i := 0; i < n; i++ {
		abs := baseIdx + i
		bi := g.beatInfoAtRow(row, abs)
		if bi.NodeType != model.NodeTypeRegular && bi.NodeType != model.NodeTypeMute {
			exp = append(exp, false)
			continue
		}
		audible, triggered := g.simEvalAudible(row, abs, bi, simCounts, &simLastFired, simLastTrig, &simGate)
		on := audible
		if loop && seg > 0 && abs >= start+1 {
			if (abs-(start+1))%seg == 0 {
				if bi.NodeType == model.NodeTypeInvisible {
					on = false
				}
			}
		}
		exp = append(exp, on)
		// Update sim state based on audible (pre-mask) to match playback
		if bi.NodeType == model.NodeTypeRegular {
			if triggered {
				simLastFired = bi.NodeID
				simLastTrig[bi.NodeID] = true
			} else {
				simLastTrig[bi.NodeID] = false
			}
		} else if bi.NodeType == model.NodeTypeMute {
			simLastTrig[bi.NodeID] = triggered
		}
	}
	return exp
}

func TestPreviewSyncMatchesPlayback_EveryN(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, a)
	if n, ok := g.graph.GetNodeByID(a.ID); ok {
		p := n.Params
		p.LogicKind = "every_n_triggers"
		p.LogicN = 2
		g.graph.SetNodeParams(a.ID, p)
	}
	g.start = a
	g.graph.StartNodeID = a.ID
	g.updateBeatInfos()
	// Simulate a few playback steps to build live state
	g.playing = true
	g.spawnPulseFromRow(0, 0)
	for step := 0; step < 4 && g.activePulse != nil; step++ {
		_ = g.advancePulse(g.activePulse)
	}
	// Align preview window to next beat and compare
	base := g.nextBeatIdxs[0]
	g.drum.Offset = base
	g.refreshDrumRow()
	got := g.drum.Rows[0].Steps
	want := computeExpectedPreview(g, 0, base, minInt(6, len(got)))
	for i := 0; i < len(want); i++ {
		if got[i] != want[i] {
			t.Fatalf("mismatch at +%d: got %v want %v (base=%d)", i, got[i], want[i], base)
		}
	}
}

func TestPreviewSyncMatchesPlayback_PrevTriggered(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	c := g.tryAddNode(2, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, c)
	g.addEdge(c, a)
	if n, ok := g.graph.GetNodeByID(c.ID); ok {
		p := n.Params
		p.LogicKind = "trigger_if_prev_triggered"
		g.graph.SetNodeParams(c.ID, p)
	}
	g.start = a
	g.graph.StartNodeID = a.ID
	g.updateBeatInfos()
	g.playing = true
	g.spawnPulseFromRow(0, 0)
	for step := 0; step < 5 && g.activePulse != nil; step++ {
		_ = g.advancePulse(g.activePulse)
	}
	base := g.nextBeatIdxs[0]
	g.drum.Offset = base
	g.refreshDrumRow()
	got := g.drum.Rows[0].Steps
	want := computeExpectedPreview(g, 0, base, minInt(6, len(got)))
	for i := 0; i < len(want); i++ {
		if got[i] != want[i] {
			t.Fatalf("mismatch at +%d: got %v want %v (base=%d)", i, got[i], want[i], base)
		}
	}
}

func TestPreviewSyncMatchesPlayback_ComplexLogic(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	// Build loop A->B->C->D->A
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	c := g.tryAddNode(2, 0, model.NodeTypeRegular)
	d := g.tryAddNode(3, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, c)
	g.addEdge(c, d)
	g.addEdge(d, a)
	// Rules:
	// A: every 3rd trigger
	if n, ok := g.graph.GetNodeByID(a.ID); ok {
		p := n.Params
		p.LogicKind = "every_n_triggers"
		p.LogicN = 3
		g.graph.SetNodeParams(a.ID, p)
	}
	// B: skip every 2nd trigger
	if n, ok := g.graph.GetNodeByID(b.ID); ok {
		p := n.Params
		p.LogicKind = "skip_every_n"
		p.LogicN = 2
		g.graph.SetNodeParams(b.ID, p)
	}
	// C: trigger if previous triggered
	if n, ok := g.graph.GetNodeByID(c.ID); ok {
		p := n.Params
		p.LogicKind = "trigger_if_prev_triggered"
		g.graph.SetNodeParams(c.ID, p)
	}
	// D: trigger if previous skipped
	if n, ok := g.graph.GetNodeByID(d.ID); ok {
		p := n.Params
		p.LogicKind = "trigger_if_prev_skipped"
		g.graph.SetNodeParams(d.ID, p)
	}
	g.start = a
	g.graph.StartNodeID = a.ID
	g.updateBeatInfos()
	// Advance playback several cycles
	g.playing = true
	g.spawnPulseFromRow(0, 0)
	for step := 0; step < 20 && g.activePulse != nil; step++ {
		_ = g.advancePulse(g.activePulse)
	}
	// Check three different preview alignments
	for _, delta := range []int{0, 3, 8} {
		base := g.nextBeatIdxs[0] + delta
		g.drum.Offset = base
		g.refreshDrumRow()
		got := g.drum.Rows[0].Steps
		want := computeExpectedPreview(g, 0, base, minInt(12, len(got)))
		for i := 0; i < len(want); i++ {
			if got[i] != want[i] {
				t.Fatalf("complex: base=%d mismatch at +%d: got %v want %v\npreview=%v\n", base, i, got[i], want[i], got[:len(want)])
			}
		}
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
