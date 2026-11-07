//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// The coordinate badge (z=40) must never paint over the node sidebar (z=60): the z table and the real draw
// order are both pinned.

// Strictly below the sidebar and the popups.
func TestCoordBadgeBelowSidebarZ(t *testing.T) {
	if !(GZCoordBadge < GZLongPress && GZCoordBadge < GZSidebar) {
		t.Fatalf("badge z=%d must be below popup z values long=%d sidebar=%d",
			GZCoordBadge, GZLongPress, GZSidebar)
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
	// The long-press popup sits between them and must also composite over the badge.
	lp := idx("grid-longpress")
	if lp < 0 {
		t.Fatalf("missing popup layer: longpress idx=%d", lp)
	}
	if bi >= lp {
		t.Errorf("badge draws at or after a popup (would overpaint): badge@%d longpress@%d", bi, lp)
	}
}

// Holds both drawRect and drawRoundedRect calls, so the badge fill and the sidebar fill share one timeline.
type orderedDraw struct {
	rect    image.Rectangle
	col     color.RGBA
	filled  bool
	rounded bool
}

// The originals still run. Both primitives are captured because the badge is a drawRect and the panel a
// drawRoundedRect, and their call order is the composite order.
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

// If the badge is drawn at all, the sidebar fill overlapping it must be drawn after it. Desktop is the case
// that matters: only the mobile guard hides the badge under an open sidebar.
func TestCoordBadgeDrawnBeforeSidebarPanel(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	// Desktop keeps the badge visible under an open sidebar.
	g.Layout(1280, 720)
	if Profile().IsMobile() {
		t.Fatalf("expected desktop profile after Layout(1280,720); IsMobile=true")
	}

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	if n == nil {
		t.Fatal("tryAddNode returned nil")
	}
	g.updateBeatInfos()

	sidebarW := g.sidebar.width
	if sidebarW <= 0 {
		t.Fatalf("sidebar width not positive: %d", sidebarW)
	}
	// The card is clamped to the grid pane, so a node near the bottom flips it above the node, right over the
	// badge.
	gridH := g.split.GridH(g.winH)
	g.cam.OffsetX = float64(sidebarW / 3)
	g.cam.OffsetY = float64(gridH - 40 - gridTopOffset())

	// Activate the badge for this node and open the sidebar over it.
	g.sel = n
	g.coordBadgeNode = n
	g.coordBadgeFrame = g.frame
	g.sidebar.Open(n)
	if !g.sidebar.IsOpen() {
		t.Fatal("sidebar did not open")
	}

	// Otherwise the "paints over" assertion is vacuous.
	bx1, by1, bx2, _ := g.nodeScreenRect(g.coordBadgeNode)
	bcx := int((bx1 + bx2) * 0.5)
	bcy := int(by1) - 16
	if bcy < 2 {
		bcy = 2
	}
	// The real formatter and width metric: a restated format drifts from the badge.
	badgeText := g.coordBadgeText(g.coordBadgeNode)
	tw := coordBadgePillWidth(badgeText)
	badgePill := image.Rect(bcx-tw/2, bcy-1, bcx+tw/2, bcy+13)
	panelRect := g.sidebar.rects[sbPanel]
	if panelRect.Empty() {
		t.Fatal("sidebar panel rect should be non-empty once open")
	}
	if badgePill.Intersect(panelRect).Empty() {
		t.Fatalf("test misconfigured: badge pill %v does not overlap sidebar panel %v; "+
			"reposition the camera so the badge lands under the sidebar", badgePill, panelRect)
	}

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
		// A badge not drawn at all cannot overpaint the sidebar either, so this passes; the z test still holds.
		t.Logf("badge pill fill not drawn while sidebar open — badge cannot overpaint sidebar; " +
			"ordering assertion vacuously satisfied (structural guard is TestCoordBadgeBelowSidebarZ)")
		return
	}

	sidebarAfter := false
	for i := badgeIdx + 1; i < len(seq); i++ {
		d := seq[i]
		if d.rounded && d.filled && d.col == wantPanelFill && !d.rect.Intersect(badgePill).Empty() {
			sidebarAfter = true
			break
		}
	}
	if !sidebarAfter {
		// A sidebar fill BEFORE the badge is the bug: the badge drawn last, over it.
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
