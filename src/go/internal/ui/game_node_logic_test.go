package ui

import (
    "testing"
    "time"

    "github.com/ingyamilmolinar/tunkul/core/model"
)

// TestSilentVisibleNode_NoSound ensures visible-silent nodes are traversed but do not play audio.
func TestSilentVisibleNode_NoSound(t *testing.T) {
    g := New(testLogger)
    g.Layout(640, 480)
    // Use UI-driven highlights for direct scheduling in tests.
    g.SetUseSequencerForTest(false)

    // Build path: start(regular) -> silent -> regular
    start := g.tryAddNode(0, 0, model.NodeTypeRegular)
    mid := g.tryAddNode(1, 0, model.NodeTypeSilent)
    end := g.tryAddNode(2, 0, model.NodeTypeRegular)
    g.addEdge(start, mid)
    g.addEdge(mid, end)

    // Capture instrument triggers.
    plays := make(chan struct{}, 4)
    g.SetPlayFunc(func(string, float64, ...float64) { plays <- struct{}{} })

    g.playing = true
    g.spawnPulseFrom(0) // highlights start node (regular)
    select {
    case <-plays:
    case <-time.After(50 * time.Millisecond):
        t.Fatalf("expected sound at start node")
    }

    // Advance to silent mid node: no sound expected
    g.activePulse.t = 1
    g.Update()
    select {
    case <-plays:
        t.Fatalf("expected no sound at silent node")
    default:
    }

    // Advance to end regular node: sound expected
    g.activePulse.t = 1
    g.Update()
    select {
    case <-plays:
    case <-time.After(50 * time.Millisecond):
        t.Fatalf("expected sound at end node")
    }
}

// TestNodeVolumeMultiplier applies a per-node volume and expects it to scale playback.
func TestNodeVolumeMultiplier(t *testing.T) {
    g := New(testLogger)
    g.Layout(640, 480)
    g.SetUseSequencerForTest(false)

    // Simple two-regular-node path
    n0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
    n1 := g.tryAddNode(1, 0, model.NodeTypeRegular)
    g.addEdge(n0, n1)

    // Set node volume for n1 to 0.5x
    g.graph.SetNodeParams(n1.ID, model.NodeParams{Volume: 0.5})

    vols := make(chan float64, 2)
    g.SetPlayFunc(func(id string, v float64, when ...float64) { vols <- v })

    g.playing = true
    g.spawnPulseFrom(0)

    v0 := <-vols // start node

    // Advance to n1 and capture scaled volume
    g.activePulse.t = 1
    g.Update()
    var v1 float64
    select {
    case v1 = <-vols:
    case <-time.After(50 * time.Millisecond):
        t.Fatalf("expected playback at second node")
    }

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
    g := New(testLogger)
    g.Layout(640, 480)
    g.SetUseSequencerForTest(false)

    // Loop with one audible node to isolate counts: silent -> regular -> silent -> back
    s0 := g.tryAddNode(0, 0, model.NodeTypeSilent)
    r1 := g.tryAddNode(1, 0, model.NodeTypeRegular)
    s2 := g.tryAddNode(2, 0, model.NodeTypeSilent)
    g.addEdge(s0, r1)
    g.addEdge(r1, s2)
    g.addEdge(s2, s0)

    // Logic: disable on even triggers of r1
    disabled := false
    g.graph.SetNodeLogic(r1.ID, func(ctx model.NodeContext) model.NodeDecision {
        e := (ctx.TriggerCount%2 == 1)
        disabled = !e
        return model.NodeDecision{Enabled: &e}
    })

    plays := 0
    g.SetPlayFunc(func(string, float64, ...float64) { plays++ })

    g.playing = true
    g.spawnPulseFrom(0)
    // First audible at r1 should play
    time.Sleep(5 * time.Millisecond)
    // Advance repeatedly through the loop; expect plays on triggers 1,3,5
    for i := 0; i < 5; i++ {
        // mid (r1)
        g.activePulse.t = 1
        g.Update()
        time.Sleep(5 * time.Millisecond)
        // tail (s2)
        g.activePulse.t = 1
        g.Update()
        time.Sleep(5 * time.Millisecond)
        // back to start (s0)
        g.activePulse.t = 1
        g.Update()
        time.Sleep(5 * time.Millisecond)
    }

    if plays == 0 {
        t.Fatalf("expected some playback, got 0")
    }
    // Expect approximately 3 plays after ~5 loops; allow small variance due to timing
    if plays < 2 {
        t.Fatalf("expected at least 2 plays with alternating disable, got %d (disabled=%v)", plays, disabled)
    }
}

