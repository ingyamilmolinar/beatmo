package ui

import (
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/tunkul/core/model"
)

// Build two nodes 1 subdivision apart with bidirectional edges.
func buildBiDirPair(g *Game, step int) (a, b *uiNode) {
	g.pendingStartRow = 0
	a = g.tryAddNode(0, 0, model.NodeTypeRegular)
	b = g.tryAddNode(step, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, a)
	g.pendingStartRow = -1
	g.start = a
	g.graph.StartNodeID = a.ID
	g.updateBeatInfos()
	return
}

// Temporal spacing must remain constant across subdivision increases.
func TestSubdivChange8to16KeepsTemporalSpacing(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	// Freeze input
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	if err := g.SetSubdivisions(8); err != nil {
		t.Fatalf("set 8: %v", err)
	}
	a, b := buildBiDirPair(g, 1)
	// Use a high-ish BPM to shorten test run while keeping stability.
	bpm := 240.0
	div8 := g.grid.MaxDiv()
	if div8 <= 0 {
		t.Fatalf("invalid div8=%d", div8)
	}
	delta8 := b.I - a.I
	beats8 := float64(delta8) / float64(div8)

	// Increase subdivisions to 16; nodes should be remapped to 2-step spacing.
	if err := g.SetSubdivisions(16); err != nil {
		t.Fatalf("set 16: %v", err)
	}
	// Sanity: coordinates scaled
	if a.I != 0 || b.I != 2 {
		t.Fatalf("node remap mismatch after 8->16: a.I=%d b.I=%d", a.I, b.I)
	}
	div16 := g.grid.MaxDiv()
	if div16 <= 0 {
		t.Fatalf("invalid div16=%d", div16)
	}
	delta16 := b.I - a.I
	beats16 := float64(delta16) / float64(div16)

	// Expected interval corresponds to 1/8th note, unchanged across subdiv.
	want := (60.0 / bpm) / 8.0
	got8 := beats8 * (60.0 / bpm)
	got16 := beats16 * (60.0 / bpm)
	tol := 1e-9
	if math.Abs(got8-want) > tol {
		t.Fatalf("baseline interval off: got=%.6fs want=%.6fs", got8, want)
	}
	if math.Abs(got16-want) > tol {
		t.Fatalf("post-change interval off: got=%.6fs want=%.6fs", got16, want)
	}
}

// Temporal spacing must remain constant across subdivision decreases when nodes align.
func TestSubdivChange16to8KeepsTemporalSpacing(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()
	if err := g.SetSubdivisions(16); err != nil {
		t.Fatalf("set 16: %v", err)
	}
	a, b := buildBiDirPair(g, 2) // aligned for 16->8 (multiples of 2)
	bpm := 180.0
	div16 := g.grid.MaxDiv()
	if div16 <= 0 {
		t.Fatalf("invalid div16=%d", div16)
	}
	delta16 := b.I - a.I
	beats16 := float64(delta16) / float64(div16)

	if err := g.SetSubdivisions(8); err != nil {
		t.Fatalf("set 8: %v", err)
	}
	if a.I != 0 || b.I != 1 {
		t.Fatalf("node remap mismatch after 16->8: a.I=%d b.I=%d", a.I, b.I)
	}
	div8 := g.grid.MaxDiv()
	if div8 <= 0 {
		t.Fatalf("invalid div8=%d", div8)
	}
	delta8 := b.I - a.I
	beats8 := float64(delta8) / float64(div8)

	want := (60.0 / bpm) / 8.0
	got16 := beats16 * (60.0 / bpm)
	got8 := beats8 * (60.0 / bpm)
	tol := 1e-9
	if math.Abs(got16-want) > tol {
		t.Fatalf("baseline interval off: got=%.6fs want=%.6fs", got16, want)
	}
	if math.Abs(got8-want) > tol {
		t.Fatalf("post-change interval off: got=%.6fs want=%.6fs", got8, want)
	}
}
