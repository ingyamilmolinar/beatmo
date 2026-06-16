//go:build test

package ui

import (
	"fmt"
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// This file is the regression guard for the ORIGINAL bug behind the GridTree
// work: the node coordinate badge (the (i,j) pill drawn above a selected node)
// painting ON TOP OF the node sidebar/pop-up menu. The structural fix routes
// the grid pane through a z-ordered tree (GridTree) where the badge is a
// z=GZCoordBadge(40) layer and the sidebar is z=GZSidebar(60); the tree draws
// ascending-z, so the sidebar composites OVER the badge instead of being
// overpainted by it. These tests FAIL if anyone reintroduces the bug by
// moving the badge above the sidebar in z, or by reverting the tree dispatch
// so the badge is drawn after the sidebar.

// TestCoordBadgeBelowSidebarZ is the structural guard for the reported bug
// (coordinate badge painting over the node pop-up menu). The badge Layer
// must sit strictly below the sidebar and the popups so GridTree.Draw
// composites them on top of the badge.
func TestCoordBadgeBelowSidebarZ(t *testing.T) {
	if !(GZCoordBadge < GZLongPress && GZCoordBadge < GZMoveConfirm && GZCoordBadge < GZSidebar) {
		t.Fatalf("badge z=%d must be below popup z values long=%d moveConfirm=%d sidebar=%d",
			GZCoordBadge, GZLongPress, GZMoveConfirm, GZSidebar)
	}
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)

	got := g.gridTree.LayersForTest()
	idx := func(id string) int {
		for i, l := range got {
			if l.ID() == id {
				return i
			}
		}
		return -1
	}
	bi, si := idx("grid-coord-badge"), idx("grid-sidebar")
	if bi < 0 || si < 0 {
		t.Fatalf("missing layers: badge idx=%d sidebar idx=%d", bi, si)
	}
	if bi >= si {
		t.Errorf("badge draws at or after sidebar (would overpaint): badge@%d sidebar@%d", bi, si)
	}
	// Also pin the long-press + move-confirm popups, which sit between the
	// badge and the sidebar in z and must likewise composite over the badge.
	lp, mc := idx("grid-longpress"), idx("grid-move-confirm")
	if lp < 0 || mc < 0 {
		t.Fatalf("missing popup layers: longpress idx=%d moveConfirm idx=%d", lp, mc)
	}
	if bi >= lp || bi >= mc {
		t.Errorf("badge draws at or after a popup (would overpaint): badge@%d longpress@%d moveConfirm@%d", bi, lp, mc)
	}
}

// orderedDraw is one entry in the unified, ordered draw timeline captured
// during a drawGridPane pass. It records both drawRect and drawRoundedRect
// calls so the badge fill (drawRect) and the sidebar panel fill
// (drawRoundedRect) land in the SAME chronological slice.
type orderedDraw struct {
	rect    image.Rectangle
	col     color.RGBA
	filled  bool
	rounded bool
}

// collectOrderedDraws intercepts BOTH drawRect and drawRoundedRect during
// fn() and returns every call in invocation order. The originals are still
// invoked so the real draw side-effects happen. We need both primitives in
// one timeline because the badge pill is a drawRect while the sidebar panel
// is a drawRoundedRect — comparing their relative call order is exactly the
// z-composite order GridTree.Draw produces.
func collectOrderedDraws(t *testing.T, fn func()) []orderedDraw {
	t.Helper()
	var seq []orderedDraw
	origRect := drawRect
	origRound := drawRoundedRect
	drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		seq = append(seq, orderedDraw{
			rect:   r,
			col:    color.RGBAModel.Convert(c).(color.RGBA),
			filled: filled,
		})
		origRect(dst, r, c, filled)
	}
	drawRoundedRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, radius int, filled bool) {
		seq = append(seq, orderedDraw{
			rect:    r,
			col:     color.RGBAModel.Convert(c).(color.RGBA),
			filled:  filled,
			rounded: true,
		})
		origRound(dst, r, c, radius, filled)
	}
	defer func() {
		drawRect = origRect
		drawRoundedRect = origRound
	}()
	fn()
	return seq
}

// TestCoordBadgeDrawnBeforeSidebarPanel is the runtime draw-order guard. It
// drives drawGridPane on DESKTOP with the coordinate badge ACTIVE and the
// sidebar OPEN over the same node, then asserts that — IF the badge is drawn
// at all — the sidebar panel fill that overlaps the badge is drawn AFTER the
// badge, so the sidebar composites on top of (and hides) the badge.
//
// Approach (A) from the task: draw-order interception. We intercept drawRect
// (the badge pill fill, color genColorVizPillFill) and drawRoundedRect (the
// sidebar panel fill, colPanelBG over the panel rect) into one ordered
// timeline. A naive screen.At() pixel read can't be used here: the badge
// draws into the grid-pane SubImage (clip=true) and so is invisible to a
// parent screen.At(), while the sidebar (clip=false) draws to the unclipped
// screen — a pixel read would only ever see the sidebar and trivially pass
// without exercising the ordering (task note (B)).
//
// Desktop is the meaningful case: the desktop badge guard does NOT suppress
// the badge when the sidebar is open (only the mobile guard does), so the
// badge IS drawn while the sidebar is open — exactly the bug scenario the
// z-order tree must neutralize. If a future change suppresses the badge
// entirely under an open sidebar, that ALSO satisfies "the badge can't
// overpaint the sidebar"; that case is handled below with a comment.
func TestCoordBadgeDrawnBeforeSidebarPanel(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	// Desktop screen class (width 1280) — desktop badge guard keeps the badge
	// visible while the sidebar is open.
	g.Layout(1280, 720)
	if Profile().IsMobile() {
		t.Fatalf("expected desktop profile after Layout(1280,720); IsMobile=true")
	}

	// Add a node and position the camera so the node (and the badge above it)
	// sit on the LEFT, under the left-anchored sidebar panel (0,0,w,gridH).
	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	if n == nil {
		t.Fatal("tryAddNode returned nil")
	}
	g.updateBeatInfos()

	sidebarW := g.sidebar.width
	if sidebarW <= 0 {
		t.Fatalf("sidebar width not positive: %d", sidebarW)
	}
	// Map grid (0,0) to a small screen X well inside the sidebar's left band
	// and to a Y a comfortable distance below the grid top so the badge (which
	// sits ~16px above the node) stays on-screen. Grid (0,0) maps to the raw
	// camera offset, so set the offset directly to the target screen point.
	g.cam.OffsetX = float64(sidebarW / 3)
	g.cam.OffsetY = float64(gridTopOffset() + 120)

	// Activate the badge for this node and open the sidebar over it.
	g.sel = n
	g.coordBadgeNode = n
	g.coordBadgeFrame = g.frame
	g.sidebar.Open(n)
	if !g.sidebar.IsOpen() {
		t.Fatal("sidebar did not open")
	}

	// Sanity: the badge's computed pill rect overlaps the sidebar panel rect,
	// otherwise the "sidebar paints over the badge" assertion would be vacuous.
	bx1, by1, bx2, _ := g.nodeScreenRect(g.coordBadgeNode)
	bcx := int((bx1 + bx2) * 0.5)
	bcy := int(by1) - 16
	if bcy < 2 {
		bcy = 2
	}
	badgeText := fmt.Sprintf("(%d, %d)", g.coordBadgeNode.I, g.coordBadgeNode.J)
	tw := len(badgeText)*7 + 8
	badgePill := image.Rect(bcx-tw/2, bcy-1, bcx+tw/2, bcy+13)
	panelRect := image.Rect(0, 0, sidebarW, g.split.Y-gridTopOffset())
	if badgePill.Intersect(panelRect).Empty() {
		t.Fatalf("test misconfigured: badge pill %v does not overlap sidebar panel %v; "+
			"reposition the camera so the badge lands under the sidebar", badgePill, panelRect)
	}

	// Drive the dispatch-only grid-pane draw through GridTree and capture the
	// ordered draw timeline.
	img := ebiten.NewImage(1280, 720)
	seq := collectOrderedDraws(t, func() {
		g.drawGridPane(img)
	})

	wantBadgeFill := color.RGBAModel.Convert(WithAlpha(genColorVizPillFill, genAlphaSidebarChip)).(color.RGBA)
	wantPanelFill := color.RGBAModel.Convert(colPanelBG).(color.RGBA)

	badgeIdx := -1
	for i, d := range seq {
		if !d.rounded && d.filled && d.col == wantBadgeFill {
			badgeIdx = i
			break
		}
	}

	if badgeIdx < 0 {
		// The badge was not drawn at all (e.g. a future guard suppressed it
		// while the sidebar is open). That ALSO means the badge cannot
		// overpaint the sidebar, so the regression is not present. Pass, but
		// log so the maintainer knows this path was taken (Test 1 remains the
		// load-bearing structural guard).
		t.Logf("badge pill fill not drawn while sidebar open — badge cannot overpaint sidebar; " +
			"ordering assertion vacuously satisfied (structural guard is TestCoordBadgeBelowSidebarZ)")
		return
	}

	// Find a sidebar panel fill (rounded, filled, colPanelBG) that overlaps
	// the badge and is drawn AFTER it.
	sidebarAfter := false
	for i := badgeIdx + 1; i < len(seq); i++ {
		d := seq[i]
		if d.rounded && d.filled && d.col == wantPanelFill && !d.rect.Intersect(badgePill).Empty() {
			sidebarAfter = true
			break
		}
	}
	if !sidebarAfter {
		// Did a sidebar panel fill overlapping the badge appear BEFORE it
		// (the bug — badge drawn last, over the sidebar)? Diagnose.
		for i := 0; i < badgeIdx; i++ {
			d := seq[i]
			if d.rounded && d.filled && d.col == wantPanelFill && !d.rect.Intersect(badgePill).Empty() {
				t.Fatalf("BUG REINTRODUCED: sidebar panel fill drawn BEFORE the coordinate badge "+
					"(panel@%d badge@%d) — the badge overpaints the sidebar", i, badgeIdx)
			}
		}
		t.Fatalf("no sidebar panel fill (colPanelBG) overlapping the badge found after the badge draw "+
			"(badge@%d, %d total draws) — cannot confirm the sidebar composites over the badge",
			badgeIdx, len(seq))
	}
}
