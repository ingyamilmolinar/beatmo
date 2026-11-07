//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Desktop mouse contract: shift+press a node arms the link arrow and release on another makes the edge,
// shift+right release deletes an edge, right-click deletes a node, shift+press on empty space marquees, and
// the grid guide never draws while a link drag or marquee owns the gesture.

// Unconnected, so edge creation is asserted from a clean slate.
func buildLinkDragCircuit(t *testing.T) (*Game, *uiNode, *uiNode) {
	t.Helper()
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	g.updateBeatInfos()
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(8, 0, model.NodeTypeRegular)
	g.start = a
	g.graph.StartNodeID = a.ID
	g.updateBeatInfos()
	g.cam.Scale = 2.0
	g.cam.OffsetX = 200
	g.cam.OffsetY = 60
	return g, a, b
}

func TestShiftDragNodeToNodeCreatesDirectedEdge(t *testing.T) {
	g, a, b := buildLinkDragCircuit(t)
	ax, ay := nodeScreenCenter(g, a)
	bx, by := nodeScreenCenter(g, b)
	shiftDrag(g, ax, ay, bx, by)

	if _, ok := g.graph.Edges[[2]model.NodeID{a.ID, b.ID}]; !ok {
		t.Fatalf("shift-drag a→b must create the directed edge a→b")
	}
	if _, ok := g.graph.Edges[[2]model.NodeID{b.ID, a.ID}]; ok {
		t.Fatalf("shift-drag a→b must not create the reverse edge b→a")
	}
	if len(g.nodes) != 2 {
		t.Fatalf("link drag must not create nodes (have %d)", len(g.nodes))
	}
	if n := len(g.graph.AllGroups()); n != 0 {
		t.Fatalf("shift-press ON a node must never arm the marquee (groups=%d)", n)
	}
	if g.linkDrag.active {
		t.Fatalf("link drag must be cleared after release")
	}
}

func TestShiftPressOnNodeArmsHelperArrowAndTracksCursor(t *testing.T) {
	g, a, b := buildLinkDragCircuit(t)
	ax, ay := nodeScreenCenter(g, a)
	bx, by := nodeScreenCenter(g, b)

	pos := [2]int{ax, ay}
	down := true
	restore := SetInputForTest(
		func() (int, int) { return pos[0], pos[1] },
		func(btn ebiten.MouseButton) bool { return down && btn == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return k == ebiten.KeyShiftLeft },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	defer restore()

	advanceFrames(g, 2) // press on node a with shift held
	if !g.linkDrag.active || g.linkDrag.from != a {
		t.Fatalf("shift+press on a node must arm linkDrag from that node (active=%v from=%v)",
			g.linkDrag.active, g.linkDrag.from)
	}

	pos = [2]int{bx, by}
	advanceFrames(g, 2) // drag toward node b while held
	// The arrow tip is the grid-snapped cursor position.
	wantX, wantY := b.X, b.Y
	if g.linkDrag.toX != wantX || g.linkDrag.toY != wantY {
		t.Fatalf("helper arrow tip = (%v,%v), want cursor-snapped world (%v,%v)",
			g.linkDrag.toX, g.linkDrag.toY, wantX, wantY)
	}

	down = false
	advanceFrames(g, 2) // release on b completes the connection
	if _, ok := g.graph.Edges[[2]model.NodeID{a.ID, b.ID}]; !ok {
		t.Fatalf("release on second node must create edge a→b")
	}
}

func TestShiftRightReleaseOnNodeDeletesEdge(t *testing.T) {
	g, a, b := buildLinkDragCircuit(t)
	g.addEdge(a, b)
	if _, ok := g.graph.Edges[[2]model.NodeID{a.ID, b.ID}]; !ok {
		t.Fatalf("setup: edge a→b must exist")
	}
	ax, ay := nodeScreenCenter(g, a)
	bx, by := nodeScreenCenter(g, b)

	pos := [2]int{ax, ay}
	left, right := true, false
	restore := SetInputForTest(
		func() (int, int) { return pos[0], pos[1] },
		func(btn ebiten.MouseButton) bool {
			switch btn {
			case ebiten.MouseButtonLeft:
				return left
			case ebiten.MouseButtonRight:
				return right
			}
			return false
		},
		func(k ebiten.Key) bool { return k == ebiten.KeyShiftLeft },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	defer restore()

	advanceFrames(g, 2) // shift+left press on a arms link drag
	if !g.linkDrag.active {
		t.Fatalf("setup: link drag must be armed")
	}
	pos = [2]int{bx, by}
	advanceFrames(g, 2)       // drag to b
	left, right = false, true // release left with right held = delete-edge gesture
	advanceFrames(g, 2)
	if _, ok := g.graph.Edges[[2]model.NodeID{a.ID, b.ID}]; ok {
		t.Fatalf("shift-drag release with right held must delete the edge a→b")
	}
}

func TestRightClickOnNodeDeletesIt(t *testing.T) {
	g, a, _ := buildLinkDragCircuit(t)
	ax, ay := nodeScreenCenter(g, a)
	right := true
	restore := SetInputForTest(
		func() (int, int) { return ax, ay },
		func(btn ebiten.MouseButton) bool { return right && btn == ebiten.MouseButtonRight },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	defer restore()
	advanceFrames(g, 2)
	right = false
	advanceFrames(g, 1)
	if g.nodeAt(0, 0) != nil {
		t.Fatalf("plain right-click on a node must delete it")
	}
}

func TestShiftPressOnEmptySpaceStillMarquees(t *testing.T) {
	g, _, _ := buildLinkDragCircuit(t)
	// Far from both nodes and their (absent) edges: marquee, not link drag.
	x1, y1 := guideScreenAt(g, 30*g.grid.Unit(), 20*g.grid.Unit())
	shiftDrag(g, x1, y1, x1+80, y1+60)
	if g.linkDrag.active {
		t.Fatalf("shift-press on empty space must not arm link drag")
	}
	if len(g.nodes) != 2 {
		t.Fatalf("marquee gesture must not create nodes (have %d)", len(g.nodes))
	}
}

// A ghost square riding the arrow tip buries the connect feedback, and this gesture can never place a node.
func TestGuideChromeHiddenDuringLinkDrag(t *testing.T) {
	g, a, b := buildLinkDragCircuit(t)
	ax, ay := nodeScreenCenter(g, a)
	// Far enough from both nodes to be a free-cell guide target, where the ghost would draw.
	mx, my := guideScreenAt(g, (a.X+b.X)/2, a.Y)

	pos := [2]int{ax, ay}
	down := true
	restore := SetInputForTest(
		func() (int, int) { return pos[0], pos[1] },
		func(btn ebiten.MouseButton) bool { return down && btn == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return k == ebiten.KeyShiftLeft },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	defer restore()
	advanceFrames(g, 2) // shift+press on node a arms the link drag
	pos = [2]int{mx, my}
	advanceFrames(g, 2) // drag to the midpoint, still held
	if !g.linkDrag.active {
		t.Fatalf("setup: link drag must be armed")
	}

	if g.guideLayerVisible() {
		t.Fatalf("guide layer must be hidden while a link drag is active (helper arrow owns the gesture)")
	}

	gt := g.guideTargetAt(mx, my)
	cx := int(g.cam.OffsetX + g.grid.Unit()*float64(gt.I)*g.cam.Scale)
	cy := int(g.cam.OffsetY+g.grid.Unit()*float64(gt.J)*g.cam.Scale) + gridTopOffset()
	// Matched by its exact glow colours, so nearby small rects (the coordinate badge) cannot trip it.
	ghostFill := WithAlpha(genColorVizGlow, genAlphaAccentOverlay)
	ghostLine := WithAlpha(genColorVizGlow, genAlphaStrong)
	var ghost []image.Rectangle
	orig := drawRect
	drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		if (c == ghostFill || c == ghostLine) && r.Dx() <= 60 && r.Dy() <= 60 && image.Pt(cx, cy).In(r.Inset(-2)) {
			ghost = append(ghost, r)
		}
		orig(dst, r, c, filled)
	}
	defer func() { drawRect = orig }()
	img := ebiten.NewImage(1280, 720)
	g.Draw(img)
	if len(ghost) != 0 {
		t.Fatalf("ghost-node preview must not render during a link drag (%d ghost rects at target)", len(ghost))
	}
}

// A shift-drag from empty space selects and never places.
func TestGuideChromeHiddenDuringMarquee(t *testing.T) {
	g, _, _ := buildLinkDragCircuit(t)
	x1, y1 := guideScreenAt(g, 30*g.grid.Unit(), 20*g.grid.Unit())
	pos := [2]int{x1, y1}
	down := true
	restore := SetInputForTest(
		func() (int, int) { return pos[0], pos[1] },
		func(btn ebiten.MouseButton) bool { return down && btn == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return k == ebiten.KeyShiftLeft },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	defer restore()
	advanceFrames(g, 2) // shift+press on empty arms the marquee
	pos = [2]int{x1 + 80, y1 + 60}
	advanceFrames(g, 2)
	if !g.marquee.active {
		t.Fatalf("setup: marquee must be active")
	}
	if g.guideLayerVisible() {
		t.Fatalf("guide layer must be hidden while the marquee is active")
	}
}

// The crosshair snaps to a node from outside its small sprite rect, and a plain click there selects it; the
// shift gesture must honour the same snap from both the source and destination annulus.
func TestShiftDragConnectsWithGuideSnap(t *testing.T) {
	g, a, b := buildLinkDragCircuit(t)

	// Both preconditions are asserted so this cannot degrade into the dead-centre case.
	r := g.guideSnapRadiusWorld()
	ax, ay := guideScreenAt(g, a.X+0.6*r, a.Y-0.6*r)
	if n := g.nodeAtScreen(ax, ay); n != nil {
		t.Fatalf("setup: press point must be outside the node sprite (hit %v)", n.ID)
	}
	if gt := g.guideTargetAt(ax, ay); gt.Kind != GuideNode || gt.Node != a {
		t.Fatalf("setup: press point must guide-snap to node a (got kind=%v node=%v)", gt.Kind, gt.Node)
	}
	// Same construction for the release point near node b.
	bx, by := guideScreenAt(g, b.X+0.6*r, b.Y+0.6*r)
	if n := g.nodeAtScreen(bx, by); n != nil {
		t.Fatalf("setup: release point must be outside node b's sprite (hit %v)", n.ID)
	}
	if gt := g.guideTargetAt(bx, by); gt.Kind != GuideNode || gt.Node != b {
		t.Fatalf("setup: release point must guide-snap to node b (got kind=%v node=%v)", gt.Kind, gt.Node)
	}

	pos := [2]int{ax, ay}
	down := true
	restore := SetInputForTest(
		func() (int, int) { return pos[0], pos[1] },
		func(btn ebiten.MouseButton) bool { return down && btn == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return k == ebiten.KeyShiftLeft },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	defer restore()

	advanceFrames(g, 2) // shift+press inside a's snap annulus
	if g.marquee.active {
		t.Fatalf("shift+press guide-snapped to a node must not arm the marquee")
	}
	if !g.linkDrag.active || g.linkDrag.from != a {
		t.Fatalf("shift+press guide-snapped to node a must arm the helper arrow from it (active=%v from=%v)",
			g.linkDrag.active, g.linkDrag.from)
	}

	pos = [2]int{bx, by}
	advanceFrames(g, 2) // aim at b's annulus: the arrow tip must lock onto b
	if g.linkDrag.toX != b.X || g.linkDrag.toY != b.Y {
		t.Fatalf("helper arrow tip = (%v,%v), want locked onto node b (%v,%v)",
			g.linkDrag.toX, g.linkDrag.toY, b.X, b.Y)
	}

	down = false
	advanceFrames(g, 2) // release inside b's annulus completes the connection
	if _, ok := g.graph.Edges[[2]model.NodeID{a.ID, b.ID}]; !ok {
		t.Fatalf("release guide-snapped to node b must create edge a→b")
	}
	if len(g.nodes) != 2 {
		t.Fatalf("guide-snapped connect must not create nodes (have %d)", len(g.nodes))
	}
}

// Selecting a node opens the sidebar over the grid's left strip; a shift press on a node beneath it must close
// the panel and arm the link drag, not be consumed by the sidebar.
func TestShiftDragFromSelectedNodeUnderSidebar(t *testing.T) {
	g, a, b := buildLinkDragCircuit(t)
	// Place node a inside the sidebar panel strip (x < sidebarMinW).
	g.cam.OffsetX = 100
	ax, ay := nodeScreenCenter(g, a)
	bx, by := nodeScreenCenter(g, b)
	if ax >= sidebarMinW {
		t.Fatalf("setup: node a must render inside the sidebar panel strip (x=%d >= %d)", ax, sidebarMinW)
	}

	guideClickAt(g, ax, ay)
	if !g.sidebar.IsOpen() || g.sel != a {
		t.Fatalf("setup: clicking node a must select it and open the sidebar (open=%v sel=%v)",
			g.sidebar.IsOpen(), g.sel)
	}

	// Step 2: shift+press on the same node, drag to b, release.
	pos := [2]int{ax, ay}
	down := true
	restore := SetInputForTest(
		func() (int, int) { return pos[0], pos[1] },
		func(btn ebiten.MouseButton) bool { return down && btn == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return k == ebiten.KeyShiftLeft },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	defer restore()
	advanceFrames(g, 2)
	if !g.linkDrag.active || g.linkDrag.from != a {
		t.Fatalf("shift+press on the selected node must arm the helper arrow even under the open sidebar (active=%v from=%v)",
			g.linkDrag.active, g.linkDrag.from)
	}
	pos = [2]int{bx, by}
	advanceFrames(g, 2)
	down = false
	advanceFrames(g, 2)
	if _, ok := g.graph.Edges[[2]model.NodeID{a.ID, b.ID}]; !ok {
		t.Fatalf("release on node b must create edge a→b")
	}
}
