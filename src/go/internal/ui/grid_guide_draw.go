package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/utils"
)

// guideLayerVisible gates the grid-guide crosshair layer. Desktop: whenever
// the pointer is inside the grid pane (same gate as the cursor label). Touch:
// only while a finger is down — otherwise the crosshair would stick at the
// last lift position. Hidden outright while a link drag or marquee owns the
// gesture: those gestures never place a node, and the ghost/crosshair would
// bury the link-drag helper arrow (drawn at GZEdges) under placement chrome.
func (g *Game) guideLayerVisible() bool {
	if g.modalOverlayActive() || g.landscapeUnsupported() {
		return false
	}
	if g.linkDrag.active || g.marquee.active {
		return false
	}
	mx, my := cursorPosition()
	if my < gridTopOffset() || !g.split.InGridPane(mx, my) {
		return false
	}
	if touchOverrideActive || g.guideAim.active {
		return true
	}
	return Profile().ShowCursorLabel
}

// drawGridGuide renders the placement guide: a screen-constant crosshair
// through the snapped target, fraction ticks + a brightened line on the
// active edge, and a ghost-node preview at the commit cell. All chrome except
// the ghost is screen-constant so nothing grows when zooming in; the ghost
// previews a real node, so it scales with zoom like one (see NodeRadius).
func (g *Game) drawGridGuide(dst *ebiten.Image) {
	if !g.guideLayerVisible() {
		return
	}
	ctx := &g.gridDrawCtx
	mx, my := cursorPosition()
	t := g.guideTargetCached(mx, my)
	unitPx := ctx.unitPx
	top := float64(gridTopOffset())
	sx := ctx.offX + unitPx*float64(t.I)
	sy := ctx.offY + unitPx*float64(t.J) + top
	gridRect := ctx.gridRect
	var idm ebiten.GeoM
	cross := WithAlpha(colAccentDim, genAlphaMedium)
	DrawLineCam(dst, sx, float64(gridRect.Min.Y), sx, float64(gridRect.Max.Y), &idm, cross, 1)
	DrawLineCam(dst, float64(gridRect.Min.X), sy, float64(gridRect.Max.X), sy, &idm, cross, 1)

	if t.Kind == GuideEdge && t.Edge != nil {
		g.drawGuideEdgeTicks(dst, t, &idm)
	}

	// Ghost node preview at the commit cell. Move mode draws its own ghost
	// (drawGridMoveMode); a node target needs no preview — the node exists.
	if t.Kind != GuideNode && !g.modes.Is(GridModeMove) {
		gr := g.grid.NodeRadius(g.cam.Scale) * g.cam.Scale
		ghostRect := image.Rect(int(sx-gr), int(sy-gr), int(sx+gr), int(sy+gr))
		drawRect(dst, ghostRect, WithAlpha(genColorVizGlow, genAlphaAccentOverlay), true)
		drawRect(dst, ghostRect, WithAlpha(genColorVizGlow, genAlphaStrong), false)
	}
}

// drawGuideEdgeTicks brightens the active edge and marks its simple-fraction
// positions (reduced denominator 2, 3, or 4) with small screen-constant ticks.
func (g *Game) drawGuideEdgeTicks(dst *ebiten.Image, t GuideTarget, idm *ebiten.GeoM) {
	ctx := &g.gridDrawCtx
	unitPx := ctx.unitPx
	top := float64(gridTopOffset())
	a, b := t.Edge.A, t.Edge.B
	ax := ctx.offX + unitPx*float64(a.I)
	ay := ctx.offY + unitPx*float64(a.J) + top
	bx := ctx.offX + unitPx*float64(b.I)
	by := ctx.offY + unitPx*float64(b.J) + top
	bright := WithAlpha(colAccentBright, genAlphaStrong)
	DrawLineCam(dst, ax, ay, bx, by, idm, bright, 1)

	vertical := a.I == b.I
	span := abs(b.I - a.I)
	if vertical {
		span = abs(b.J - a.J)
	}
	const tickPx = 4.0
	for _, den := range [3]int{2, 3, 4} {
		if span%den != 0 {
			continue
		}
		for num := 1; num < den; num++ {
			if utils.GCD(num, den) != 1 {
				continue // 2/4 etc. already drawn at a smaller denominator
			}
			if vertical {
				fy := ay + unitPx*float64((b.J-a.J)*num/den)
				DrawLineCam(dst, ax-tickPx, fy, ax+tickPx, fy, idm, bright, 1)
			} else {
				fx := ax + unitPx*float64((b.I-a.I)*num/den)
				DrawLineCam(dst, fx, ay-tickPx, fx, ay+tickPx, idm, bright, 1)
			}
		}
	}
}
