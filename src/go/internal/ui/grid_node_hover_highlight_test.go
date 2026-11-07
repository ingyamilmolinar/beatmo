package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// The stroked rings of drawRectHoverGlow; no pixel reads, so it runs under real Ebiten too.
func countNodeHoverRingRects(t *testing.T, want color.Color, cx, cy, tol int, draw func()) int {
	t.Helper()
	count := 0
	orig := drawRect
	drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		if !filled && !r.Empty() {
			mx := (r.Min.X + r.Max.X) / 2
			my := (r.Min.Y + r.Max.Y) / 2
			if abs(mx-cx) <= tol && abs(my-cy) <= tol && colorMatchesRow(c, want) {
				count++
			}
		}
		orig(dst, r, c, filled)
	}
	defer func() { drawRect = orig }()
	draw()
	return count
}

// An interceptor, not pixel reads, so the real-GPU path is exercised too.
func TestGridNodeHoverDrawsFocusRing(t *testing.T) {
	assertDefaultParityState(t)
	g, _, cx, cy := setupHoveredNode(t, model.NodeTypeRegular, true)
	rowCol := g.drum.Rows[0].Color
	rings := countNodeHoverRingRects(t, rowCol, cx, cy, 10, func() { g.Draw(ebiten.NewImage(g.winW, g.winH)) })
	if rings < 2 {
		t.Fatalf("hovered node drew %d own-color focus-ring outlines, want >=2 (the crisp hover ring is missing)", rings)
	}
}

// Halo eased to ~full and zoomed in far enough for the outer rings to render.
func setupHoveredNode(t *testing.T, nt model.NodeType, attachRow bool) (*Game, *uiNode, int, int) {
	t.Helper()
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	assertDefaultSimpleDraw(t, g)
	g.simpleDraw = false
	if len(g.drum.Rows) == 0 {
		g.drum.AddRow()
	}
	g.pendingStartRow = -1
	g.quietFrames = 0

	n := g.nodeAt(0, 0)
	if n == nil {
		n = g.tryAddNode(0, 0, nt)
	}
	if n == nil {
		t.Fatal("failed to add node")
	}
	if mn, ok := g.graph.Nodes[n.ID]; ok {
		mn.Type = nt
		g.graph.Nodes[n.ID] = mn
	}
	if attachRow {
		g.nodeRows[n.ID] = 0
	} else {
		delete(g.nodeRows, n.ID)
	}

	rPxOf := func() int {
		x1, _, x2, _ := g.nodeScreenRect(n)
		return int((x2 - x1) * 0.5)
	}
	for i := 0; i < 6 && rPxOf() < 9; i++ {
		g.cam.Scale *= 1.4
	}

	// Ease the hover halo to full.
	g.hover = n
	for i := 0; i < 30; i++ {
		g.advanceNodeHoverGlow()
	}

	x1, y1, x2, y2 := g.nodeScreenRect(n)
	return g, n, int((x1 + x2) * 0.5), int((y1 + y2) * 0.5)
}

// In the node's own colour, never white: a row node in its row colour, any other in the border colour.
func TestGridNodeHoverBloomsInOwnColorNotWhite(t *testing.T) {
	assertDefaultParityState(t)

	t.Run("row node hovers in its row color", func(t *testing.T) {
		g, _, cx, cy := setupHoveredNode(t, model.NodeTypeRegular, true)
		rowCol := g.drum.Rows[0].Color

		screen := ebiten.NewImage(g.winW, g.winH)
		own := countNodeHoverRingRects(t, rowCol, cx, cy, 4, func() { g.Draw(screen) })
		if own < 2 {
			t.Fatalf("row-node hover: got %d bloom rings in the row color, want >=2", own)
		}
		white := countNodeHoverRingRects(t, colHighlight, cx, cy, 4, func() { g.Draw(ebiten.NewImage(g.winW, g.winH)) })
		if white != 0 {
			t.Fatalf("row-node hover drew %d white bloom rings; want the node's own color, not white", white)
		}
	})

	t.Run("non-row node hovers in the node border color, not white", func(t *testing.T) {
		g, _, cx, cy := setupHoveredNode(t, model.NodeTypeRegular, false)

		white := countNodeHoverRingRects(t, colHighlight, cx, cy, 4, func() { g.Draw(ebiten.NewImage(g.winW, g.winH)) })
		if white != 0 {
			t.Fatalf("non-row node hover drew %d white (colHighlight) bloom rings; want the node's own color, not white", white)
		}
		own := countNodeHoverRingRects(t, genColorNodeBorder, cx, cy, 4, func() { g.Draw(ebiten.NewImage(g.winW, g.winH)) })
		if own < 2 {
			t.Fatalf("non-row node hover: got %d bloom rings in the node's own (border) color, want >=2", own)
		}
	})

	t.Run("mute node hovers in own color, not white", func(t *testing.T) {
		g, _, cx, cy := setupHoveredNode(t, model.NodeTypeMute, false)

		white := countNodeHoverRingRects(t, colHighlight, cx, cy, 4, func() { g.Draw(ebiten.NewImage(g.winW, g.winH)) })
		muteWhite := countNodeHoverRingRects(t, colMuteHighlight, cx, cy, 4, func() { g.Draw(ebiten.NewImage(g.winW, g.winH)) })
		if white != 0 || muteWhite != 0 {
			t.Fatalf("mute node hover drew white bloom rings (colHighlight=%d colMuteHighlight=%d); want the node's own color", white, muteWhite)
		}
		own := countNodeHoverRingRects(t, genColorNodeBorder, cx, cy, 4, func() { g.Draw(ebiten.NewImage(g.winW, g.winH)) })
		if own < 2 {
			t.Fatalf("mute node hover: got %d bloom rings in the node's own color, want >=2", own)
		}
	})
}

// A real cursor makes the node the hover target and eases its halo up.
func TestGridNodeHoverFromRealCursorHighlights(t *testing.T) {
	withDefaultStart(t, true)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(400, 400)

	n := g.nodeAt(0, 0)
	if n == nil {
		t.Fatal("missing origin node")
	}
	x1, y1, x2, y2 := g.nodeScreenRect(n)
	mx := int((x1 + x2) / 2)
	my := int((y1 + y2) / 2)

	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 400, 400 },
	)
	defer restore()

	for i := 0; i < 30; i++ {
		g.Update()
	}

	if g.hover != n {
		t.Fatalf("cursor over node: g.hover=%v, want the hovered node", g.hover)
	}
	if g.hoverGlowNode != n {
		t.Fatalf("cursor over node: hoverGlowNode=%v, want the hovered node", g.hoverGlowNode)
	}
	if g.hoverGlowP < 0.9 {
		t.Fatalf("cursor over node: hoverGlowP=%.2f after 30 frames, want ~1 (node not highlighting on hover)", g.hoverGlowP)
	}
}

// The input override is restored on Cleanup.
func hoverCursorOverOriginNode(t *testing.T, g *Game, frames int) *uiNode {
	t.Helper()
	n := g.nodeAt(0, 0)
	if n == nil {
		t.Fatal("missing origin node")
	}
	x1, y1, x2, y2 := g.nodeScreenRect(n)
	mx := int((x1 + x2) / 2)
	my := int((y1 + y2) / 2)
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 400, 400 },
	)
	t.Cleanup(restore)
	for i := 0; i < frames; i++ {
		g.Update()
	}
	return n
}

// Hover keys off the input class (desktop pointer), not the perf fast path, which is the browser default.
func TestGridNodeHoverWorksUnderFastPath(t *testing.T) {
	withDefaultStart(t, true)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(400, 400)
	g.SetPerfFastPath(true) // simulate the browser / WASM default

	n := hoverCursorOverOriginNode(t, g, 30)

	if g.hover != n {
		t.Fatalf("fast-path (browser) cursor over node: g.hover=%v, want the node — grid-node hover is disabled under fastPath", g.hover)
	}
	if g.hoverGlowNode != n || g.hoverGlowP < 0.9 {
		t.Fatalf("fast-path: hover halo not rising (hoverGlowNode=%v hoverGlowP=%.2f), want the node at ~1", g.hoverGlowNode, g.hoverGlowP)
	}
}

// Under the browser fast path the hover must PAINT the bloom, not merely set state.
func TestGridNodeHoverBloomPaintsUnderFastPath(t *testing.T) {
	withDefaultStart(t, true)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	assertDefaultSimpleDraw(t, g)
	g.simpleDraw = false
	g.SetPerfFastPath(true) // simulate browser / WASM default
	if len(g.drum.Rows) == 0 {
		g.drum.AddRow()
	}

	n := g.nodeAt(0, 0)
	if n == nil {
		t.Fatal("missing origin node")
	}
	g.nodeRows[n.ID] = 0
	rowCol := g.drum.Rows[0].Color

	// Zoom in so the node is large enough for the outer bloom rings.
	rPxOf := func() int { x1, _, x2, _ := g.nodeScreenRect(n); return int((x2 - x1) * 0.5) }
	for i := 0; i < 6 && rPxOf() < 9; i++ {
		g.cam.Scale *= 1.4
	}

	x1, y1, x2, y2 := g.nodeScreenRect(n)
	mx := int((x1 + x2) / 2)
	my := int((y1 + y2) / 2)
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()
	for i := 0; i < 30; i++ {
		g.Update()
	}

	cx, cy := mx, my
	own := countNodeHoverRingRects(t, rowCol, cx, cy, 6, func() { g.Draw(ebiten.NewImage(g.winW, g.winH)) })
	if own < 2 {
		t.Fatalf("fast-path hover: %d own-color bloom rings painted at the node, want >=2 (highlight not visible in browser)", own)
	}
}

// Touch has no persistent pointer, so a stale cursor must never latch a hover on mobile.
func TestGridNodeHoverSuppressedOnMobile(t *testing.T) {
	withDefaultStart(t, true)
	withSmallScreen(t, true) // mobile screen class → touch, no pointer hover
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(400, 400)

	hoverCursorOverOriginNode(t, g, 10)

	if g.hover != nil {
		t.Fatalf("mobile: g.hover=%v, want nil (no persistent pointer hover on touch)", g.hover)
	}
}
