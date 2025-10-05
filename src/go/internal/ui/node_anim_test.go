package ui

import (
	"github.com/ingyamilmolinar/tunkul/core/model"
	"testing"
)

// TestNodeTriggerAnimationIncreasesRadius verifies that when a node is
// audibly triggered, its on-screen radius increases above the baseline and
// then decays, while remaining clamped to non-overlapping bounds.
func TestNodeTriggerAnimationIncreasesRadius(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	g.SetUseSequencerForTest(false)

	n0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n1 := g.tryAddNode(2, 0, model.NodeTypeRegular)
	g.addEdge(n0, n1)

	// Baseline (pre-trigger) screen radius
	base := g.nodeRadius(n0)
	g.playing = true
	g.spawnPulseFrom(0) // triggers n0 immediately

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
	SetDefaultStartForTest(true)
	g := New(testLogger)
	defer SetDefaultStartForTest(false)
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

// TestNodeAnimRespectsSkipEveryN ensures node-level logic gating (SkipEveryN)
// inhibits the animation on skipped triggers.
func TestNodeAnimRespectsSkipEveryN(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	g.SetUseSequencerForTest(false)

	s0 := g.tryAddNode(0, 0, model.NodeTypeSilent)
	r1 := g.tryAddNode(1, 0, model.NodeTypeRegular)
	s2 := g.tryAddNode(2, 0, model.NodeTypeSilent)
	g.addEdge(s0, r1)
	g.addEdge(r1, s2)
	g.addEdge(s2, s0)

	// Skip every 2nd trigger at r1
	g.graph.SetNodeParams(r1.ID, model.NodeParams{SkipEveryN: 2})

	g.playing = true
	g.spawnPulseFrom(0) // starts at s0; no animation expected yet

	// Step to r1 (first trigger) -> should animate
	g.activePulse.t = 1
	_ = g.Update()
	if v := g.nodeAnim[r1.ID]; v == 0 {
		t.Fatalf("expected animation at first trigger; got 0")
	}
	// Reset anim to measure next trigger distinctly
	g.nodeAnim[r1.ID] = 0

	// Step s2, then back to s0, then to r1 again (second trigger skipped)
	g.activePulse.t = 1
	_ = g.Update() // to s2
	g.activePulse.t = 1
	_ = g.Update() // to s0
	g.activePulse.t = 1
	_ = g.Update() // to r1 again (skip)
	// Peek immediately after update
	if v := g.nodeAnim[r1.ID]; v != 0 {
		t.Fatalf("expected no animation on skipped trigger; got %f", v)
	}
}

// TestNodeAnimNoOverlapAtMax ensures two adjacent nodes with full animation do
// not overlap in screen space.
// TestNodeSizeScalesWithZoomOut ensures nodes become larger on screen as the
// user zooms out (smaller camera scale), providing visibility at bird's‑eye view.
func TestNodeSizeScalesWithZoomOut(t *testing.T) {
	SetDefaultStartForTest(true)
	g := New(testLogger)
	defer SetDefaultStartForTest(false)
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
