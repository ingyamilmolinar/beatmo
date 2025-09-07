package ui

import (
    "testing"
    "github.com/ingyamilmolinar/tunkul/core/model"
)

// Test that placing a node on a pass-through intersection (where an edge runs)
// does not change the beat path or drum view state.
func TestPlaceNodeOnPassThroughDoesNotChangeBeat(t *testing.T) {
    g := New(testLogger)
    g.Layout(640, 480)

    // Build a 2x2 square loop: (0,0)->(2,0)->(2,2)->(0,2)->(0,0)
    a := g.tryAddNode(0, 0, model.NodeTypeRegular)
    b := g.tryAddNode(2, 0, model.NodeTypeRegular)
    c := g.tryAddNode(2, 2, model.NodeTypeRegular)
    d := g.tryAddNode(0, 2, model.NodeTypeRegular)
    g.addEdge(a, b)
    g.addEdge(b, c)
    g.addEdge(c, d)
    g.addEdge(d, a)

    // Capture initial beat path and drum view state.
    g.updateBeatInfos()
    initial := append([]model.BeatInfo(nil), g.beatInfos...)
    if len(g.drum.Rows) == 0 {
        t.Fatalf("no drum rows")
    }
    steps0 := append([]bool(nil), g.drum.Rows[0].Steps...)
    length0 := g.drum.Length

    // Place a node on a pass-through intersection along edge (0,0)->(2,0): at (1,0).
    g.tryAddNode(1, 0, model.NodeTypeRegular)

    // Verify beat path unchanged.
    if len(g.beatInfos) != len(initial) {
        t.Fatalf("beat path length changed: %d -> %d", len(initial), len(g.beatInfos))
    }
    for i := range initial {
        if initial[i].NodeID != g.beatInfos[i].NodeID || initial[i].NodeType != g.beatInfos[i].NodeType || initial[i].I != g.beatInfos[i].I || initial[i].J != g.beatInfos[i].J {
            t.Fatalf("beat info changed at %d: before=%+v after=%+v", i, initial[i], g.beatInfos[i])
        }
    }
    // Drum steps and length unchanged.
    if g.drum.Length != length0 {
        t.Fatalf("drum length changed: %d -> %d", length0, g.drum.Length)
    }
    if len(g.drum.Rows[0].Steps) != len(steps0) {
        t.Fatalf("steps len changed: %d -> %d", len(steps0), len(g.drum.Rows[0].Steps))
    }
    for i := range steps0 {
        if steps0[i] != g.drum.Rows[0].Steps[i] {
            t.Fatalf("steps changed at %d: before=%v after=%v", i, steps0[i], g.drum.Rows[0].Steps[i])
        }
    }
}

