package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/utils"
)

// Touch: only while a finger is down, or the crosshair sticks at the last lift. Hidden during a
// link drag or marquee, which never place and whose helper arrow it would bury.
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

// Screen-constant chrome so nothing grows when zooming in; the ghost previews a real node, so it
// scales like one.
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

	// Move mode draws its own ghost; a node target needs none.
	if t.Kind != GuideNode && !g.modes.Is(GridModeMove) {
		gr := g.grid.NodeRadius(g.cam.Scale) * g.cam.Scale
		ghostRect := image.Rect(int(sx-gr), int(sy-gr), int(sx+gr), int(sy+gr))
		drawRect(dst, ghostRect, WithAlpha(genColorVizGlow, genAlphaAccentOverlay), true)
		drawRect(dst, ghostRect, WithAlpha(genColorVizGlow, genAlphaStrong), false)
	}
}

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
