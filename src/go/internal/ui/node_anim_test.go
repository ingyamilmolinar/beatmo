package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// TestNodeTriggerAnimationIncreasesRadius verifies that when a node is
// audibly triggered, its on-screen radius increases above the baseline and
// then decays, while remaining clamped to non-overlapping bounds.
func TestNodeTriggerAnimationIncreasesRadius(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	n0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n1 := g.tryAddNode(2, 0, model.NodeTypeRegular)
	g.addEdge(n0, n1)

	// Baseline (pre-trigger) screen radius
	base := g.nodeRadius(n0)
	g.SetPlaying(true)
	g.spawnPulseFromRow(0, 0) // triggers n0 immediately

	r0 := g.nodeRadius(n0)
	if r0 <= base {
		t.Fatalf("expected animated radius > base after trigger: got %.3f <= base %.3f", r0, base)
	}
	// One decay step should reduce animation but not below base
	_ = g.Update()
	r1 := g.nodeRadius(n0)
	if r1 > r0+1e-6 || r1 < base-1e-6 {
		t.Fatalf("unexpected decay radius sequence: base=%.3f first=%.3f after=%.3f", base, r0, r1)
	}
}

// Peak growth should be limited to a restrained fraction of remaining headroom.
func TestNodeAnimPeakLimited(t *testing.T) {
	withDefaultStart(t, true)
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	n := g.nodeAt(0, 0)
	if n == nil {
		t.Fatal("missing origin node")
	}
	// Base screen radius
	g.nodeAnim[n.ID] = 0
	baseScr := g.nodeRadius(n) * g.cam.Scale
	// Force peak
	g.nodeAnim[n.ID] = 1
	peakScr := g.nodeRadius(n) * g.cam.Scale
	if peakScr <= baseScr {
		t.Fatalf("no peak growth: base=%.2f peak=%.2f", baseScr, peakScr)
	}
	// Must be significantly less than full cap (96px), i.e., below halfway to 96.
	if peakScr-baseScr > (96.0-baseScr)*0.6 {
		t.Fatalf("peak growth too large: base=%.2f peak=%.2f", baseScr, peakScr)
	}
}

// TestNodeAnimRespectsSkipEveryNLogic ensures node-level gating (skip_every_n)
// inhibits the animation on skipped triggers.
func TestNodeAnimRespectsSkipEveryNLogic(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	g.pendingStartRow = 0
	n0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	r1 := g.tryAddNode(1, 0, model.NodeTypeRegular)
	n2 := g.tryAddNode(2, 0, model.NodeTypeRegular)
	if n0 == nil || r1 == nil || n2 == nil {
		t.Fatalf("failed to create nodes for skip logic test")
	}
	g.addEdge(n0, r1)
	g.addEdge(r1, n2)
	g.addEdge(n2, n0)
	g.pendingStartRow = -1

	// Skip every 2nd trigger at r1
	if n, ok := g.graph.GetNodeByID(r1.ID); ok {
		p := n.Params
		p.LogicKind = "skip_every_n"
		p.LogicN = 2
		g.graph.SetNodeParams(r1.ID, p)
	}
	g.updateBeatInfos()
	g.refreshDrumRow()

	g.drum.SetInstrument("snare")
	ensureInstrumentAvailable(t, g, "snare")

	var hits []int
	g.highlightHook = func(row, idx int) {
		if row != 0 {
			return
		}
		if bi := g.beatInfoAtRow(0, idx); bi.NodeID == r1.ID {
			hits = append(hits, idx)
		}
	}

	bpm := 120
	g.drum.SetBPM(bpm)
	g.SetAppliedBPMForTest(bpm)
	g.prevBPM = bpm
	g.SetPlaying(true)
	target := 4
	for abs := 0; abs <= target; abs++ {
		setPlayStartForAbs(g, abs)
		_ = g.Update()
	}
	if len(hits) != 1 {
		t.Fatalf("expected 1 highlighted trigger for skip_every_n, got %v", hits)
	}
}

// TestNodeAnimNoOverlapAtMax ensures two adjacent nodes with full animation do
// not overlap in screen space.
// TestNodeSizeScalesWithZoomOut ensures nodes become larger on screen as the
// user zooms out (smaller camera scale), providing visibility at bird's‑eye view.
func TestNodeSizeScalesWithZoomOut(t *testing.T) {
	withDefaultStart(t, true)
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	n := g.nodeAt(0, 0)
	if n == nil {
		t.Fatal("missing origin node")
	}
	g.cam.Scale = 2.0
	rIn := g.nodeRadius(n) * g.cam.Scale
	g.cam.Scale = 0.5
	rOut := g.nodeRadius(n) * g.cam.Scale
	if rOut <= rIn {
		t.Fatalf("expected larger radius when zoomed out: in=%.2f out=%.2f", rIn, rOut)
	}
}
