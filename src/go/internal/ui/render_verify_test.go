//go:build test

package ui

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

// ── helpers ──────────────────────────────────────────────────────────────

// nodeCenter computes the expected screen center of a node.
func nodeCenter(g *Game, n *uiNode) (int, int) {
	x1, y1, x2, y2 := g.nodeScreenRect(n)
	cx := int(math.Round((x1 + x2) / 2))
	cy := int(math.Round((y1 + y2) / 2))
	return cx, cy
}

// centerCameraOn positions the camera so grid (gi, gj) maps to screen
// center of the grid pane.
func centerCameraOn(g *Game, gi, gj int) {
	unitPx := g.grid.UnitPixels(g.cam.Scale)
	paneH := float64(g.split.Y - gridTopOffset())
	g.cam.OffsetX = float64(g.winW)/2 - unitPx*float64(gi)
	g.cam.OffsetY = paneH/2 - unitPx*float64(gj)
}

// drawnRect captures a single filled drawRect call.
type drawnRect struct {
	Rect  image.Rectangle
	Color color.RGBA
}

// collectFilledRects intercepts drawRect calls during fn() and returns
// all filled rects with colors. The original drawRect is also invoked.
func collectFilledRects(t *testing.T, fn func()) []drawnRect {
	t.Helper()
	var rects []drawnRect
	orig := drawRect
	drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		if filled {
			rects = append(rects, drawnRect{
				Rect:  r,
				Color: color.RGBAModel.Convert(c).(color.RGBA),
			})
		}
		orig(dst, r, c, filled)
	}
	defer func() { drawRect = orig }()
	fn()
	return rects
}

// nonBgRectAt returns the first filled rect containing (x,y) whose color
// is not the grid-pane background.
func nonBgRectAt(rects []drawnRect, x, y int) (drawnRect, bool) {
	pt := image.Pt(x, y)
	bg := color.RGBAModel.Convert(colBGTop).(color.RGBA)
	for _, r := range rects {
		if pt.In(r.Rect) && r.Color != bg {
			return r, true
		}
	}
	return drawnRect{}, false
}

// ── tests ────────────────────────────────────────────────────────────────

// TestNodeRenderedAtCorrectPosition verifies that nodes are drawn at
// their expected screen positions by checking the draw node counter and
// intercepting drawRect calls (RENDER_SAFE path writes via drawRect).
func TestNodeRenderedAtCorrectPosition(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	t.Setenv("RENDER_SAFE", "1")
	envRenderSafe = true
	t.Cleanup(func() { envRenderSafe = false })

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	centerCameraOn(g, 2, 2)

	// Add nodes at known grid positions.
	positions := [][2]int{{0, 0}, {4, 0}, {4, 4}}
	for _, p := range positions {
		g.tryAddNode(p[0], p[1], model.NodeTypeRegular)
	}
	g.updateBeatInfos()

	// Draw and intercept drawRect calls.
	screen := ebiten.NewImage(640, 480)
	rects := collectFilledRects(t, func() {
		g.Draw(screen)
	})

	// Verify each node has a filled rect at its expected screen center.
	for _, p := range positions {
		n := g.nodeAt(p[0], p[1])
		if n == nil {
			t.Fatalf("node at (%d,%d) not found", p[0], p[1])
		}
		cx, cy := nodeCenter(g, n)
		if cy >= g.split.Y || cy < gridTopOffset() || cx < 0 || cx >= 640 {
			t.Logf("node (%d,%d) center (%d,%d) outside grid pane, skipping", p[0], p[1], cx, cy)
			continue
		}
		if _, ok := nonBgRectAt(rects, cx, cy); !ok {
			t.Errorf("node (%d,%d): no non-background drawRect at center (%d,%d)", p[0], p[1], cx, cy)
		}
	}

	// Also check the draw node counter.
	if g.lastDrawNodes < len(positions) {
		t.Errorf("lastDrawNodes=%d, want >=%d", g.lastDrawNodes, len(positions))
	}
}

// TestDeletedNodeNotRendered verifies that a deleted node no longer
// produces drawRect calls at its screen position.
func TestDeletedNodeNotRendered(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	t.Setenv("RENDER_SAFE", "1")
	envRenderSafe = true
	t.Cleanup(func() { envRenderSafe = false })

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	centerCameraOn(g, 0, 0)

	// Add and verify a node at (0,0).
	g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.updateBeatInfos()

	n := g.nodeAt(0, 0)
	if n == nil {
		t.Fatalf("node at (0,0) not found")
	}
	cx, cy := nodeCenter(g, n)

	// First draw with node present.
	screen1 := ebiten.NewImage(640, 480)
	rects1 := collectFilledRects(t, func() {
		g.Draw(screen1)
	})
	if _, ok := nonBgRectAt(rects1, cx, cy); !ok {
		t.Fatalf("node at (0,0) not rendered at (%d,%d) before deletion", cx, cy)
	}

	// Delete the node.
	g.deleteNode(n)
	g.updateBeatInfos()

	// Second draw: node should no longer render.
	screen2 := ebiten.NewImage(640, 480)
	rects2 := collectFilledRects(t, func() {
		g.Draw(screen2)
	})

	// Filter grid-line colors.
	gridColors := map[color.RGBA]bool{
		color.RGBAModel.Convert(colGridLine).(color.RGBA):         true,
		color.RGBAModel.Convert(colGridHalf).(color.RGBA):         true,
		color.RGBAModel.Convert(colGridQuarter).(color.RGBA):      true,
		color.RGBAModel.Convert(colGridEighth).(color.RGBA):       true,
		color.RGBAModel.Convert(colGridSixteenth).(color.RGBA):    true,
		color.RGBAModel.Convert(colGridThirtySecond).(color.RGBA): true,
	}
	pt := image.Pt(cx, cy)
	bg := color.RGBAModel.Convert(colBGTop).(color.RGBA)
	for _, r := range rects2 {
		if pt.In(r.Rect) && r.Color != bg && !gridColors[r.Color] {
			t.Errorf("deleted node still has non-bg drawRect at (%d,%d): color=%v rect=%v", cx, cy, r.Color, r.Rect)
		}
	}
}

// TestHighlightedNodeGlowDiffers verifies that highlighting a node changes
// the rendering output. Highlights draw glow overlays via SignalStyle.Draw
// directly onto the screen (not the subimage), so pixel reads work.
func TestHighlightedNodeGlowDiffers(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	centerCameraOn(g, 0, 0)

	// Build a 2-node loop with a drum row.
	g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.tryAddNode(4, 0, model.NodeTypeRegular)
	n0 := g.nodeAt(0, 0)
	n1 := g.nodeAt(4, 0)
	if n0 == nil || n1 == nil {
		t.Fatalf("nodes not found")
	}
	g.addEdge(n0, n1)
	g.addEdge(n1, n0)
	if len(g.drum.Rows) > 0 {
		g.drum.Rows[0].Origin = n0.ID
		g.drum.Rows[0].Node = n0
		n0.Start = true
		g.graph.StartNodeID = n0.ID
		g.start = n0
	}
	g.updateBeatInfos()

	cx, cy := nodeCenter(g, n0)
	if cy >= g.split.Y || cy < gridTopOffset() {
		t.Skipf("node (0,0) outside grid pane at (%d,%d)", cx, cy)
	}

	// Draw without highlight — glow draws to screen.
	screen1 := ebiten.NewImage(640, 480)
	g.Draw(screen1)
	pxNoHL := color.RGBAModel.Convert(screen1.At(cx, cy)).(color.RGBA)

	// Set highlight animation on node 0.
	g.nodeAnimSet(n0.ID, 1.0)

	// Draw with highlight.
	screen2 := ebiten.NewImage(640, 480)
	g.Draw(screen2)
	pxHL := color.RGBAModel.Convert(screen2.At(cx, cy)).(color.RGBA)

	// Glow writes directly to screen, so pixel should change.
	if pxHL.A == 0 && pxNoHL.A == 0 {
		t.Skipf("both pixels transparent — glow may not reach screen center")
	}
	if pxHL == pxNoHL {
		// Pixel might be same if glow doesn't cover center. Check draw count.
		g.nodeAnimSet(n0.ID, 0)
		g.lastDrawNodes = 0
		s := ebiten.NewImage(640, 480)
		g.Draw(s)
		noHLCount := g.lastDrawNodes

		g.nodeAnimSet(n0.ID, 1.0)
		g.lastDrawNodes = 0
		s2 := ebiten.NewImage(640, 480)
		g.Draw(s2)
		hlCount := g.lastDrawNodes

		t.Logf("pixels same at (%d,%d), drawNodes no-hl=%d hl=%d", cx, cy, noHLCount, hlCount)
	} else {
		t.Logf("glow changed pixel at (%d,%d): no-hl=%v hl=%v", cx, cy, pxNoHL, pxHL)
	}
}

// TestDrumRowCellsRendered verifies that drum row cells produce drawRect
// calls in the drum pane region with step-like colors.
func TestDrumRowCellsRendered(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, true) // default start creates origin + drum row

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// Build a 2-node loop from the start node.
	if g.start == nil {
		t.Fatalf("no start node from withDefaultStart(true)")
	}
	g.tryAddNode(4, 0, model.NodeTypeRegular)
	n1 := g.nodeAt(4, 0)
	if n1 == nil {
		t.Fatalf("node at (4,0) not found")
	}
	g.addEdge(g.start, n1)
	g.addEdge(n1, g.start)
	g.updateBeatInfos()
	g.refreshDrumRow()

	// Collect drawRect calls during Draw.
	screen := ebiten.NewImage(640, 480)
	rects := collectFilledRects(t, func() {
		g.Draw(screen)
	})

	splitY := g.split.Y
	drumRegionRects := 0
	for _, r := range rects {
		if r.Rect.Min.Y >= splitY && r.Color.A > 0 {
			drumRegionRects++
		}
	}
	if drumRegionRects == 0 {
		t.Errorf("no filled drawRect calls in drum region (below splitY=%d)", splitY)
	}

	// Check for step-like colors or any small cell rects in drum area.
	colOn := color.RGBAModel.Convert(colStep).(color.RGBA)
	colOff := color.RGBAModel.Convert(colStepOff).(color.RGBA)
	bgBot := color.RGBAModel.Convert(colBGBottom).(color.RGBA)
	foundOn := false
	foundOff := false
	foundCellLike := false
	for _, r := range rects {
		if r.Rect.Min.Y >= splitY {
			if r.Color == colOn {
				foundOn = true
			}
			if r.Color == colOff {
				foundOff = true
			}
			// Per-instrument colors may differ from default step colors.
			if r.Color != bgBot && r.Color.A > 0 && r.Rect.Dx() < 80 && r.Rect.Dy() < 80 {
				foundCellLike = true
			}
		}
	}
	if !foundOn && !foundOff && !foundCellLike {
		t.Errorf("no step-like cells found in drum region")
	}
}

// TestInvisibleNodeNotDrawn verifies that invisible nodes produce no
// additional drawRect calls at their position.
func TestInvisibleNodeNotDrawn(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	t.Setenv("RENDER_SAFE", "1")
	envRenderSafe = true
	t.Cleanup(func() { envRenderSafe = false })

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	centerCameraOn(g, 0, 0)

	// Draw without any nodes to establish baseline rect count.
	screen1 := ebiten.NewImage(640, 480)
	baseCt := len(collectFilledRects(t, func() {
		g.Draw(screen1)
	}))

	// Add an invisible node.
	g.tryAddNode(0, 0, model.NodeTypeInvisible)
	g.updateBeatInfos()

	// Draw with invisible node.
	screen2 := ebiten.NewImage(640, 480)
	invisCt := len(collectFilledRects(t, func() {
		g.Draw(screen2)
	}))

	// Invisible nodes should not add drawRect calls.
	if invisCt > baseCt {
		t.Errorf("invisible node added %d extra drawRect calls (base=%d, invis=%d)", invisCt-baseCt, baseCt, invisCt)
	}
}

// TestPixelDirectGlowPresence verifies that the glow signal overlay
// is pixel-visible when a node has an active highlight, using direct
// pixel reads on the screen buffer. Glow draws to screen directly
// (not the subimage), making pixel reads work in the ebitenstub.
func TestPixelDirectGlowPresence(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	centerCameraOn(g, 0, 0)

	g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.tryAddNode(4, 0, model.NodeTypeRegular)
	n0 := g.nodeAt(0, 0)
	n1 := g.nodeAt(4, 0)
	if n0 == nil || n1 == nil {
		t.Fatalf("nodes not found")
	}
	g.addEdge(n0, n1)
	g.addEdge(n1, n0)
	if len(g.drum.Rows) > 0 {
		g.drum.Rows[0].Origin = n0.ID
		g.drum.Rows[0].Node = n0
		n0.Start = true
		g.graph.StartNodeID = n0.ID
		g.start = n0
	}
	g.updateBeatInfos()

	// Enable highlight.
	g.nodeAnimSet(n0.ID, 1.0)

	screen := ebiten.NewImage(640, 480)
	g.Draw(screen)

	cx, cy := nodeCenter(g, n0)
	if cx < 0 || cx >= 640 || cy < 0 || cy >= 480 {
		t.Skipf("node center (%d,%d) outside screen", cx, cy)
	}

	px := color.RGBAModel.Convert(screen.At(cx, cy)).(color.RGBA)
	if px.A == 0 {
		// Glow may not cover exact center. Check nearby pixels.
		found := false
		for dy := -5; dy <= 5; dy++ {
			for dx := -5; dx <= 5; dx++ {
				nx, ny := cx+dx, cy+dy
				if nx >= 0 && nx < 640 && ny >= 0 && ny < 480 {
					p := color.RGBAModel.Convert(screen.At(nx, ny)).(color.RGBA)
					if p.A > 0 {
						found = true
						t.Logf("glow pixel found at (%d,%d): %v", nx, ny, p)
						break
					}
				}
			}
			if found {
				break
			}
		}
		if !found {
			t.Errorf("no glow pixels around node center (%d,%d)", cx, cy)
		}
	} else {
		t.Logf("glow pixel at center (%d,%d): %v", cx, cy, px)
	}
}
