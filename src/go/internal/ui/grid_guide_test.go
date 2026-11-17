//go:build test

package ui

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// buildGuideCircuit creates two nodes at (0,0) and (0,span) joined by one
// vertical edge, with a deterministic camera. Mirrors the canonical
// functional-test constructor (see buildGroupLoopCircuit, game_groups_test.go).
func buildGuideCircuit(t *testing.T, span int) (*Game, *uiNode, *uiNode) {
	t.Helper()
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	g.updateBeatInfos()
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(0, span, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.start = a
	g.graph.StartNodeID = a.ID
	g.updateBeatInfos()
	g.cam.Scale = 2.0
	g.cam.OffsetX = 200
	g.cam.OffsetY = 60
	return g, a, b
}

// guideScreenAt converts world coords into the screen coords guideTargetAt expects.
func guideScreenAt(g *Game, wx, wy float64) (int, int) {
	return int(math.Round(g.cam.OffsetX + wx*g.cam.Scale)),
		int(math.Round(g.cam.OffsetY+wy*g.cam.Scale)) + gridTopOffset()
}

func TestGuideTargetNodeWins(t *testing.T) {
	g, a, _ := buildGuideCircuit(t, 12)
	// A point diagonally off node a, inside the snap radius but not on the edge.
	r := g.guideSnapRadiusWorld()
	sx, sy := guideScreenAt(g, 0.5*r, -0.5*r)
	got := g.guideTargetAt(sx, sy)
	if got.Kind != GuideNode || got.Node != a {
		t.Fatalf("want GuideNode(a), got kind=%v node=%v", got.Kind, got.Node)
	}
	if got.I != a.I || got.J != a.J {
		t.Fatalf("node target cell = (%d,%d), want (%d,%d)", got.I, got.J, a.I, a.J)
	}
}

func TestGuideSmallestDenominatorRule(t *testing.T) {
	g, _, _ := buildGuideCircuit(t, 12)
	u := g.grid.Unit()

	// Zoomed in: radius < 1 unit → exact nearest cell. Cursor at 4.2 units
	// along the edge picks cell 4 = 1/3 of the span.
	g.cam.Scale = float64(Profile().DensityValues().GuideSnapRadius) / (0.9 * u)
	sx, sy := guideScreenAt(g, 0.2*u, 4.2*u)
	got := g.guideTargetAt(sx, sy)
	if got.Kind != GuideEdge || got.J != 4 || got.Num != 1 || got.Den != 3 {
		t.Fatalf("zoomed in: got kind=%v cell=(%d,%d) frac=%d/%d, want edge (0,4) 1/3",
			got.Kind, got.I, got.J, got.Num, got.Den)
	}

	// Zoomed out: radius ≈ 4 units. Cursor at 4.4 units now also has cell 6
	// (= 1/2, denominator 2) in reach — the simpler fraction wins.
	g.cam.Scale = float64(Profile().DensityValues().GuideSnapRadius) / (4.0 * u)
	sx, sy = guideScreenAt(g, 0.2*u, 4.4*u)
	got = g.guideTargetAt(sx, sy)
	if got.Kind != GuideEdge || got.J != 6 || got.Num != 1 || got.Den != 2 {
		t.Fatalf("zoomed out: got kind=%v cell=(%d,%d) frac=%d/%d, want edge (0,6) 1/2",
			got.Kind, got.I, got.J, got.Num, got.Den)
	}
}

func TestGuideThirdsUnrepresentableOnSpan8(t *testing.T) {
	g, _, _ := buildGuideCircuit(t, 8)
	u := g.grid.Unit()
	// Tight radius (0.5u): only cell 3 is reachable near 8/3 ≈ 2.67 → the
	// honest fraction is 3/8 (no exact third exists on a span-8 edge).
	g.cam.Scale = float64(Profile().DensityValues().GuideSnapRadius) / (0.5 * u)
	sx, sy := guideScreenAt(g, 0.2*u, (8.0/3.0)*u)
	got := g.guideTargetAt(sx, sy)
	if got.Kind != GuideEdge || got.J != 3 || got.Num != 3 || got.Den != 8 {
		t.Fatalf("tight: got kind=%v cell=(%d,%d) frac=%d/%d, want edge (0,3) 3/8",
			got.Kind, got.I, got.J, got.Num, got.Den)
	}
	// Wider radius (0.9u): cell 2 (= 1/4, denominator 4) comes into reach and
	// beats 3/8 despite being farther from the cursor.
	g.cam.Scale = float64(Profile().DensityValues().GuideSnapRadius) / (0.9 * u)
	sx, sy = guideScreenAt(g, 0.2*u, (8.0/3.0)*u)
	got = g.guideTargetAt(sx, sy)
	if got.Kind != GuideEdge || got.J != 2 || got.Num != 1 || got.Den != 4 {
		t.Fatalf("wide: got kind=%v cell=(%d,%d) frac=%d/%d, want edge (0,2) 1/4",
			got.Kind, got.I, got.J, got.Num, got.Den)
	}
}

func TestGuideNearestEdgeWins(t *testing.T) {
	g, _, _ := buildGuideCircuit(t, 12)
	u := g.grid.Unit()
	// Second parallel vertical edge at I=6.
	c := g.tryAddNode(6, 0, model.NodeTypeRegular)
	d := g.tryAddNode(6, 12, model.NodeTypeRegular)
	g.addEdge(c, d)
	// Cursor 2u from the I=0 edge, 4u from the I=6 edge; radius 2.5u.
	g.cam.Scale = float64(Profile().DensityValues().GuideSnapRadius) / (2.5 * u)
	sx, sy := guideScreenAt(g, 2.0*u, 5.0*u)
	got := g.guideTargetAt(sx, sy)
	if got.Kind != GuideEdge || got.I != 0 || got.J != 6 {
		t.Fatalf("got kind=%v cell=(%d,%d), want the nearer I=0 edge's 1/2 cell (0,6)",
			got.Kind, got.I, got.J)
	}
}

func TestGuideFreeCellFarFromCircuit(t *testing.T) {
	g, _, _ := buildGuideCircuit(t, 12)
	u := g.grid.Unit()
	r := g.guideSnapRadiusWorld()
	// Well beyond the snap radius of every node and edge, whatever Step is.
	wx, wy := 3*r+40*u, 3*r+40*u
	sx, sy := guideScreenAt(g, wx, wy)
	got := g.guideTargetAt(sx, sy)
	_, _, wantI, wantJ := g.grid.Snap(wx, wy)
	if got.Kind != GuideCell || got.I != wantI || got.J != wantJ {
		t.Fatalf("got kind=%v cell=(%d,%d), want free cell (%d,%d)",
			got.Kind, got.I, got.J, wantI, wantJ)
	}
}

func TestGuideSpanOneEdgeNeverEdgeTarget(t *testing.T) {
	g, _, _ := buildGuideCircuit(t, 1)
	u := g.grid.Unit()
	for _, along := range []float64{0.2, 0.5, 0.8} {
		sx, sy := guideScreenAt(g, 0.1*u, along*u)
		if got := g.guideTargetAt(sx, sy); got.Kind == GuideEdge {
			t.Fatalf("span-1 edge produced GuideEdge at along=%v", along)
		}
	}
}

func TestGuideOccupiedCellResolvesToNode(t *testing.T) {
	// A node parked on an interior cell of a hand-built UI edge (defensive
	// case: normal placement auto-stitches, but imports/origin flows can
	// leave this shape). Built WITHOUT buildGuideCircuit: the nodes must be
	// created BEFORE any edge exists so no stitch splits the spanning edge —
	// and no g.addEdge call, so g.edges holds exactly the hand-built edge.
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	g.updateBeatInfos()
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(0, 12, model.NodeTypeRegular)
	c := g.tryAddNode(0, 6, model.NodeTypeRegular) // no edges yet → no stitch
	g.edges = append(g.edges, uiEdge{A: a, B: b})  // spanning edge passing over c
	g.cam.Scale = 2.0
	g.cam.OffsetX = 200
	g.cam.OffsetY = 60
	u := g.grid.Unit()
	// Radius 1.5u; cursor 1.2u perpendicular, along-axis 5.0. The reachable
	// cells are 4..6; cell 6 (1/2, den 2) wins the smallest-denominator rule
	// but is occupied by c → GuideNode(c). c itself is hypot(1.2,1.0) ≈ 1.56u
	// from the cursor — outside the radius — so the node-search phase cannot
	// claim it first; only the occupied-cell branch can.
	g.cam.Scale = float64(Profile().DensityValues().GuideSnapRadius) / (1.5 * u)
	sx, sy := guideScreenAt(g, 1.2*u, 5.0*u)
	got := g.guideTargetAt(sx, sy)
	if got.Kind != GuideNode || got.Node != c {
		t.Fatalf("occupied cell: got kind=%v node=%v, want GuideNode(c)", got.Kind, got.Node)
	}
}
