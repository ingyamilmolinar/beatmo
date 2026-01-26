package ui

import (
	"testing"
	"time"

	"github.com/ingyamilmolinar/tunkul/core/model"
)

func ensureNode(g *Game, i, j int, nodeType model.NodeType) *uiNode {
	n := g.nodeAt(i, j)
	if n == nil {
		n = g.tryAddNode(i, j, nodeType)
	}
	if n == nil {
		return nil
	}
	if m, ok := g.graph.GetNodeByID(n.ID); ok && m.Type != nodeType {
		m.Type = nodeType
		g.graph.Nodes[n.ID] = m
	}
	return n
}

func scheduleAbsForTest(g *Game, abs int) {
	// seqScheduleTime initializes its per-row counters on-demand. Many tests
	// drive scheduling without going through the Play button path, so ensure the
	// sequencer counters are sized up-front (starting at 0) to avoid inheriting
	// any UI preview offsets (nextBeatIdxs) that would skip abs=0.
	if g != nil && g.drum != nil {
		if len(g.seqNextIdxs) != len(g.drum.Rows) {
			g.seqNextIdxs = make([]int, len(g.drum.Rows))
		}
		if len(g.nextBeatIdxs) != len(g.drum.Rows) {
			g.nextBeatIdxs = make([]int, len(g.drum.Rows))
		}
	}
	setPlayStartForAbs(g, abs)
	g.seqScheduleTime()
}

// TestSilentVisibleNode_NoSound ensures visible-silent nodes are traversed but do not play audio.
func TestSilentVisibleNode_NoSound(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	g.pendingStartRow = -1

	// Build path: start(regular) -> silent -> regular
	start := ensureNode(g, 0, 0, model.NodeTypeRegular)
	mid := ensureNode(g, 1, 0, model.NodeTypeSilent)
	end := ensureNode(g, 2, 0, model.NodeTypeRegular)
	if start == nil || mid == nil || end == nil {
		t.Fatalf("failed to create nodes for silent path")
	}
	g.addEdge(start, mid)
	g.addEdge(mid, end)
	g.start = start
	g.graph.StartNodeID = start.ID
	g.updateBeatInfos()

	// Capture instrument triggers.
	plays := make(chan struct{}, 4)
	g.SetPlayFunc(func(string, float64, ...float64) { plays <- struct{}{} })

	scheduleAbsForTest(g, 0) // start node (regular)
	waitForChan(t, plays, 10000)

	// Advance to silent mid node: no sound expected.
	scheduleAbsForTest(g, 1)
	select {
	case <-plays:
		t.Fatalf("expected no sound at silent node")
	default:
	}

	// Advance to end regular node: sound expected.
	scheduleAbsForTest(g, 2)
	waitForChan(t, plays, 10000)
}

// TestNodeVolumeMultiplier applies a per-node volume and expects it to scale playback.
func TestNodeVolumeMultiplier(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	g.pendingStartRow = -1

	// Simple two-regular-node path
	n0 := ensureNode(g, 0, 0, model.NodeTypeRegular)
	n1 := ensureNode(g, 1, 0, model.NodeTypeRegular)
	if n0 == nil || n1 == nil {
		t.Fatalf("failed to create nodes for volume path")
	}
	g.addEdge(n0, n1)
	g.start = n0
	g.graph.StartNodeID = n0.ID
	g.updateBeatInfos()

	// Set node volume for n1 to 0.5x
	g.graph.SetNodeParams(n1.ID, model.NodeParams{Volume: 0.5})

	vols := make(chan float64, 2)
	g.SetPlayFunc(func(id string, v float64, when ...float64) { vols <- v })

	scheduleAbsForTest(g, 0)
	v0 := waitForChan(t, vols, 10000)
	scheduleAbsForTest(g, 1)
	v1 := waitForChan(t, vols, 10000)

	if v1 <= 0 || v0 <= 0 {
		t.Fatalf("unexpected zero volume values v0=%.3f v1=%.3f", v0, v1)
	}
	// Expect approximately half (row volume defaults to 1)
	want := 0.5 * v0
	if (v1-want) > 1e-6 || (want-v1) > 1e-6 {
		t.Fatalf("node volume not applied. want %.3f got %.3f (base %.3f)", want, v1, v0)
	}
}

// TestNodeLogic_DisableEveryOtherTrigger defines a node logic that disables
// playback on even triggers for a regular node in a loop.
func TestNodeLogic_DisableEveryOtherTrigger(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	g.pendingStartRow = -1

	// Loop with one audible node to isolate counts: silent -> regular -> silent -> back
	s0 := ensureNode(g, 0, 0, model.NodeTypeSilent)
	r1 := ensureNode(g, 1, 0, model.NodeTypeRegular)
	s2 := ensureNode(g, 2, 0, model.NodeTypeSilent)
	if s0 == nil || r1 == nil || s2 == nil {
		t.Fatalf("failed to create nodes for skip_every_n path")
	}
	g.addEdge(s0, r1)
	g.addEdge(r1, s2)
	g.addEdge(s2, s0)
	g.start = s0
	g.graph.StartNodeID = s0.ID
	g.updateBeatInfos()

	// Logic: skip every 2nd trigger of r1 (plays on 1,3,5,...).
	if n, ok := g.graph.GetNodeByID(r1.ID); ok {
		p := n.Params
		p.LogicKind = "skip_every_n"
		p.LogicN = 2
		g.graph.SetNodeParams(r1.ID, p)
	}

	plays := 0
	g.SetPlayFunc(func(string, float64, ...float64) { plays++ })

	// Find the absolute index of the 5th occurrence of r1 in the beat path (loop-safe).
	wantTriggers := 5
	lastAbs := -1
	triggers := 0
	for abs := 0; abs < 512; abs++ {
		bi := g.beatInfoAtRow(0, abs)
		if bi.NodeType == model.NodeTypeRegular && bi.NodeID == r1.ID {
			triggers++
			if triggers == wantTriggers {
				lastAbs = abs
				break
			}
		}
	}
	if lastAbs < 0 {
		t.Fatalf("could not find %d triggers for node %d (found %d)", wantTriggers, r1.ID, triggers)
	}

	// Drive the sequencer far enough to evaluate up to lastAbs (burst-capped at 8 per call).
	for i := 0; i < 64; i++ {
		scheduleAbsForTest(g, lastAbs)
		if len(g.seqNextIdxs) > 0 && g.seqNextIdxs[0] > lastAbs {
			break
		}
	}
	if len(g.seqNextIdxs) == 0 || g.seqNextIdxs[0] <= lastAbs {
		t.Fatalf("scheduler did not reach abs %d (seqNextIdxs=%v)", lastAbs, g.seqNextIdxs)
	}

	if plays == 0 {
		t.Fatalf("expected some playback, got 0")
	}
	if plays != 3 {
		t.Fatalf("expected 3 plays over 5 triggers with skip_every_n=2, got %d", plays)
	}
}

func TestRegularNodeLogicCallbackEveryOther(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	g.pendingStartRow = -1

	start := ensureNode(g, 0, 0, model.NodeTypeRegular)
	if start == nil {
		t.Fatalf("failed to create start node")
	}
	g.start = start
	g.graph.StartNodeID = start.ID
	g.addEdge(start, start)
	g.updateBeatInfos()

	g.graph.SetNodeLogic(start.ID, func(ctx model.NodeContext) model.NodeDecision {
		if ctx.TriggerCount%2 == 0 {
			disabled := false
			return model.NodeDecision{Enabled: &disabled}
		}
		return model.NodeDecision{}
	})

	plays := 0
	g.SetPlayFunc(func(string, float64, ...float64) { plays++ })

	for abs := 0; abs < 6; abs++ {
		scheduleAbsForTest(g, abs)
	}
	if plays != 3 {
		t.Fatalf("expected 3 plays over 6 triggers with logic callback, got %d", plays)
	}
}

func TestRegularNodeLogicCallbackAlwaysDisabled(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	g.pendingStartRow = -1

	start := ensureNode(g, 0, 0, model.NodeTypeRegular)
	if start == nil {
		t.Fatalf("failed to create start node")
	}
	g.start = start
	g.graph.StartNodeID = start.ID
	g.addEdge(start, start)
	g.updateBeatInfos()

	g.graph.SetNodeLogic(start.ID, func(ctx model.NodeContext) model.NodeDecision {
		disabled := false
		return model.NodeDecision{Enabled: &disabled}
	})

	plays := 0
	g.SetPlayFunc(func(string, float64, ...float64) { plays++ })

	for abs := 0; abs < 4; abs++ {
		scheduleAbsForTest(g, abs)
	}
	if plays != 0 {
		t.Fatalf("expected no plays with logic callback disabled, got %d", plays)
	}
}

func TestNodeLogicParamAdjustmentsApplied(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	start := ensureNode(g, 0, 0, model.NodeTypeRegular)
	if start == nil {
		t.Fatalf("failed to create start node")
	}
	g.start = start
	g.graph.StartNodeID = start.ID
	g.addEdge(start, start)
	g.updateBeatInfos()

	g.graph.SetNodeParams(start.ID, model.NodeParams{Volume: 1.2, Pitch: 2, Duration: 0.5})
	g.graph.SetNodeLogic(start.ID, func(ctx model.NodeContext) model.NodeDecision {
		return model.NodeDecision{
			VolumeMul:   0.5,
			PitchDelta:  3,
			DurationMul: 2,
		}
	})

	vols := make(chan float64, 2)
	g.SetPlayFunc(func(_ string, v float64, _ ...float64) { vols <- v })

	g.SetPlaying(true)
	scheduleAbsForMuteTest(g, 0)
	v0 := waitForChan(t, vols, 10000)

	info := g.beatInfoAtRow(0, 0)
	baseVol, basePitch, baseDur := g.evalNodeParamsOnly(0, 0, info)
	wantVol := baseVol * 0.5
	wantPitch := basePitch + 3
	wantDur := baseDur * 2

	if (v0-wantVol) > 1e-6 || (wantVol-v0) > 1e-6 {
		t.Fatalf("volume not adjusted by node logic: want %.3f got %.3f (base %.3f)", wantVol, v0, baseVol)
	}

	var gotPitch, gotDur float64
	found := false
	g.parityMu.Lock()
	for _, ev := range g.parityAudio {
		if ev.Row == 0 && ev.Abs == 0 {
			gotPitch = ev.Pitch
			gotDur = ev.Dur
			found = true
			break
		}
	}
	g.parityMu.Unlock()
	if !found {
		t.Fatalf("expected parity audio event for row=0 abs=0")
	}
	if (gotPitch-wantPitch) > 1e-6 || (wantPitch-gotPitch) > 1e-6 {
		t.Fatalf("pitch not adjusted by node logic: want %.3f got %.3f (base %.3f)", wantPitch, gotPitch, basePitch)
	}
	if (gotDur-wantDur) > 1e-6 || (wantDur-gotDur) > 1e-6 {
		t.Fatalf("duration not adjusted by node logic: want %.3f got %.3f (base %.3f)", wantDur, gotDur, baseDur)
	}
}

func TestNodeLogicParamCountsSkipDisabled(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	start := ensureNode(g, 0, 0, model.NodeTypeRegular)
	if start == nil {
		t.Fatalf("failed to create start node")
	}
	g.start = start
	g.graph.StartNodeID = start.ID
	g.addEdge(start, start)
	g.updateBeatInfos()

	seenCounts := make([]int, 0, 4)
	g.graph.SetNodeLogic(start.ID, func(ctx model.NodeContext) model.NodeDecision {
		seenCounts = append(seenCounts, ctx.TriggerCount)
		if ctx.TriggerCount%2 == 0 {
			disabled := false
			return model.NodeDecision{Enabled: &disabled}
		}
		return model.NodeDecision{VolumeMul: float64(ctx.TriggerCount)}
	})

	vols := make(chan float64, 4)
	g.SetPlayFunc(func(_ string, v float64, _ ...float64) { vols <- v })

	g.SetPlaying(true)
	scheduleAbsForMuteTest(g, 0)
	v1 := waitForChan(t, vols, 10000)

	scheduleAbsForMuteTest(g, 1)
	select {
	case v := <-vols:
		t.Fatalf("unexpected play on disabled trigger: vol=%.3f", v)
	case <-time.After(30 * time.Millisecond):
	}

	scheduleAbsForMuteTest(g, 2)
	v3 := waitForChan(t, vols, 10000)

	info := g.beatInfoAtRow(0, 0)
	baseVol, _, _ := g.evalNodeParamsOnly(0, 0, info)
	want1 := baseVol * 1
	want3 := baseVol * 3
	if (v1-want1) > 1e-6 || (want1-v1) > 1e-6 {
		t.Fatalf("first trigger volume mismatch: want %.3f got %.3f", want1, v1)
	}
	if (v3-want3) > 1e-6 || (want3-v3) > 1e-6 {
		t.Fatalf("third trigger volume mismatch: want %.3f got %.3f", want3, v3)
	}
	if len(seenCounts) < 3 {
		t.Fatalf("expected at least 3 logic callbacks, got %v", seenCounts)
	}
	if seenCounts[0] != 1 || seenCounts[1] != 2 || seenCounts[2] != 3 {
		t.Fatalf("unexpected trigger counts: %v", seenCounts)
	}
}
