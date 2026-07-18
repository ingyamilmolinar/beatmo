//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

// guideClickAt simulates press+release at a fixed screen position through the
// real Update loop (mirrors shiftDrag in game_marquee_test.go, without
// Shift). Named distinctly from origin_select_test.go's clickAt (same
// signature, different semantics: that one runs a single Update per phase
// with no settle frames) to avoid a redeclaration collision — both files
// compile together under -tags test.
func guideClickAt(g *Game, x, y int) {
	down := false
	restore := SetInputForTest(
		func() (int, int) { return x, y },
		func(b ebiten.MouseButton) bool { return down && b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	defer restore()
	advanceFrames(g, 2)
	down = true
	advanceFrames(g, 2)
	down = false
	advanceFrames(g, 2)
}

func TestGuideClickInsertsNodeAtThirdAndStitches(t *testing.T) {
	g, a, b := buildGuideCircuit(t, 12)
	u := g.grid.Unit()
	g.cam.Scale = float64(Profile().DensityValues().GuideSnapRadius) / (0.9 * u)
	sx, sy := guideScreenAt(g, 0.3*u, 4.2*u)
	guideClickAt(g, sx, sy)

	n := g.nodeAt(0, 4)
	if n == nil {
		t.Fatalf("expected a node at (0,4) — 1/3 of the a→b span")
	}
	if _, ok := g.graph.Edges[[2]model.NodeID{a.ID, b.ID}]; ok {
		t.Fatalf("a→b must be split by the auto-stitch")
	}
	if _, ok := g.graph.Edges[[2]model.NodeID{a.ID, n.ID}]; !ok {
		t.Fatalf("missing stitched edge a→new")
	}
	if _, ok := g.graph.Edges[[2]model.NodeID{n.ID, b.ID}]; !ok {
		t.Fatalf("missing stitched edge new→b")
	}

	// One undo removes the node AND restores the original edge (one group).
	g.performUndo()
	if g.nodeAt(0, 4) != nil {
		t.Fatalf("undo must remove the inserted node")
	}
	if _, ok := g.graph.Edges[[2]model.NodeID{a.ID, b.ID}]; !ok {
		t.Fatalf("undo must restore the original a→b edge")
	}
}

func TestGuideClickNearNodeSelectsInsteadOfCreating(t *testing.T) {
	g, a, _ := buildGuideCircuit(t, 12)
	r := g.guideSnapRadiusWorld()
	// Diagonally off node a, inside snap radius, clear of the edge line.
	sx, sy := guideScreenAt(g, 0.5*r, -0.5*r)
	before := len(g.nodes)
	guideClickAt(g, sx, sy)
	if len(g.nodes) != before {
		t.Fatalf("click near a node must select, not create (nodes %d→%d)", before, len(g.nodes))
	}
	if g.sel != a {
		t.Fatalf("node a must be selected")
	}
	if !g.sidebar.IsOpen() {
		t.Fatalf("selecting a node by click opens the sidebar")
	}
}

// TestGuideMoveModeSnapsGhostTargetOffEdgeCancels: move placement is
// edge-only, so a tap that the guide resolves to a free cell — even one
// snapped precisely onto the grid in the same column as the moved node's own
// edge — cancels the mode instead of placing there. (0,20) sits outside the
// a→b span [0,12], so it is a GuideCell, not a GuideEdge interior point.
func TestGuideMoveModeSnapsGhostTargetOffEdgeCancels(t *testing.T) {
	g, _, b := buildGuideCircuit(t, 12)
	u := g.grid.Unit()
	g.cam.Scale = float64(Profile().DensityValues().GuideSnapRadius) / (0.9 * u)
	origI, origJ := b.I, b.J
	g.modes.Enter(GridModeMove, b)
	sx, sy := guideScreenAt(g, 0.2*u, 20.1*u)
	g.handleTapInGrid(sx, sy)
	if g.modes.Is(GridModeMove) {
		t.Fatal("expected move mode canceled after a free-cell (off-edge) tap")
	}
	if b.I != origI || b.J != origJ {
		t.Fatalf("expected b to stay at (%d,%d), got (%d,%d)", origI, origJ, b.I, b.J)
	}
}

// dragTouch simulates a touch press at (x1,y1), drag to (x2,y2), release,
// with the guide-aim touch seam armed (SetInputForTest forces the real touch
// override off inside updateTouchOverride, so the seam stands in for it).
func dragTouch(g *Game, x1, y1, x2, y2 int) {
	restoreTouch := SetGuideAimTouchForTest(true)
	defer restoreTouch()
	pos := [2]int{x1, y1}
	down := false
	restore := SetInputForTest(
		func() (int, int) { return pos[0], pos[1] },
		func(b ebiten.MouseButton) bool { return down && b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	defer restore()
	advanceFrames(g, 2)
	down = true
	advanceFrames(g, 2) // press edge: arm the visual aim if near a circuit element
	pos = [2]int{x2, y2}
	advanceFrames(g, 3) // drag → pans the camera (visual aim clears past the slop)
	down = false
	advanceFrames(g, 2) // release edge: a moved gesture never places a node
}

// TestGuideAimDragPansInsteadOfPlacing pins the unified "movement = pan
// everywhere" model. A drag that STARTS on/near a circuit element used to arm
// the guide-aim gesture, suppress panning, and place a node at the aimed cell on
// release. That contradicted "click-and-hold always pans" and made the
// pan-vs-create-node behaviour depend on where the drag happened to start (over
// content vs empty space) — half the reported mobile conflict. Now the aim is
// visual only: the same drag pans the camera and places nothing. Precise
// placement still works, but only via a stationary tap (see TestGuideTapInGridSnapsThroughGuide).
func TestGuideAimDragPansInsteadOfPlacing(t *testing.T) {
	g, _, _ := buildGuideCircuit(t, 12)
	u := g.grid.Unit()
	g.cam.Scale = float64(Profile().DensityValues().GuideSnapRadius) / (0.9 * u)
	x1, y1 := guideScreenAt(g, 0.5*u, 7.0*u) // touch-down near the edge
	x2, y2 := guideScreenAt(g, 0.3*u, 4.1*u) // drag toward the 1/3 cell
	offXBefore, offYBefore := g.cam.OffsetX, g.cam.OffsetY
	before := len(g.nodes)
	dragTouch(g, x1, y1, x2, y2)
	if g.cam.OffsetX == offXBefore && g.cam.OffsetY == offYBefore {
		t.Fatalf("a drag starting near a circuit element must pan the camera, not aim-place")
	}
	if len(g.nodes) != before {
		t.Fatalf("a drag must never place a node (%d→%d); only a stationary tap does", before, len(g.nodes))
	}
	if g.nodeAt(0, 4) != nil {
		t.Fatalf("release of a moved gesture must NOT place a node at the aimed cell (0,4)")
	}
}

func TestGuideDragFromEmptySpaceStillPans(t *testing.T) {
	g, _, _ := buildGuideCircuit(t, 12)
	u := g.grid.Unit()
	r := g.guideSnapRadiusWorld()
	// Horizontally clear of the circuit (perp > 3r from the I=0 edge) but at
	// small y so the touch stays inside the grid pane regardless of Step.
	x1, y1 := guideScreenAt(g, 30*u+3*r, 2.0*u)
	x2, y2 := x1-60, y1+20
	before := len(g.nodes)
	offXBefore := g.cam.OffsetX
	dragTouch(g, x1, y1, x2, y2)
	if g.cam.OffsetX == offXBefore {
		t.Fatalf("drag from empty space must still pan the camera")
	}
	if len(g.nodes) != before {
		t.Fatalf("pan must not create nodes (%d→%d)", before, len(g.nodes))
	}
}

func TestGuideTapInGridSnapsThroughGuide(t *testing.T) {
	g, _, _ := buildGuideCircuit(t, 12)
	u := g.grid.Unit()
	g.cam.Scale = float64(Profile().DensityValues().GuideSnapRadius) / (0.9 * u)
	sx, sy := guideScreenAt(g, 0.3*u, 4.2*u)
	g.handleTapInGrid(sx, sy) // the GestureTap direct handler
	if g.nodeAt(0, 4) == nil {
		t.Fatalf("grid tap must resolve through the guide to (0,4)")
	}
}

func TestGuideAimDoesNotArmOverOpenGroupMenu(t *testing.T) {
	g, a, b := buildGuideCircuit(t, 12)
	gid, err := g.graph.CreateGroup("", []model.NodeID{a.ID, b.ID})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	g.groupMenu.OpenAt(gid, 100, 100)
	u := g.grid.Unit()
	g.cam.Scale = float64(Profile().DensityValues().GuideSnapRadius) / (0.9 * u)
	x1, y1 := guideScreenAt(g, 0.5*u, 7.0*u)
	x2, y2 := guideScreenAt(g, 0.3*u, 4.1*u)
	before := len(g.nodes)
	dragTouch(g, x1, y1, x2, y2)
	if len(g.nodes) != before {
		t.Fatalf("aim gesture must not place nodes while the group menu is open (%d→%d)", before, len(g.nodes))
	}
}

// TestGuideAimDisarmsMidGesture pins the mid-gesture disarm branch of
// updateGuideAim. Simulating a literal second finger isn't feasible under
// SetInputForTest, so this exercises the same disarm condition list via the
// sidebar-open branch (added by item 1 alongside g.groupMenu.IsOpen()).
func TestGuideAimDisarmsMidGesture(t *testing.T) {
	g, _, _ := buildGuideCircuit(t, 12)
	u := g.grid.Unit()
	g.cam.Scale = float64(Profile().DensityValues().GuideSnapRadius) / (0.9 * u)
	x1, y1 := guideScreenAt(g, 0.5*u, 7.0*u)
	restoreTouch := SetGuideAimTouchForTest(true)
	defer restoreTouch()
	pos := [2]int{x1, y1}
	down := false
	restore := SetInputForTest(
		func() (int, int) { return pos[0], pos[1] },
		func(b ebiten.MouseButton) bool { return down && b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	defer restore()
	advanceFrames(g, 2)
	down = true
	advanceFrames(g, 2)
	if !g.guideAim.active {
		t.Fatalf("setup: aim must be armed after press near the edge")
	}
	pos = [2]int{x1 - 30, y1 - 40} // past slop → moved
	advanceFrames(g, 2)
	g.sidebar.Open(g.nodes[0]) // any disarm condition mid-gesture
	advanceFrames(g, 1)
	if g.guideAim.active {
		t.Fatalf("aim must disarm when a disarm condition (sidebar open) appears mid-gesture")
	}
	before := len(g.nodes)
	down = false
	advanceFrames(g, 2)
	if len(g.nodes) != before {
		t.Fatalf("release after mid-gesture disarm must not commit (%d→%d)", before, len(g.nodes))
	}
}
