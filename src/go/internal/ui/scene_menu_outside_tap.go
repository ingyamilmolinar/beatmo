//go:build !noscenes

package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// A REAL tap through the whole touch pipeline, including the release-time GestureTap the grid acts on.
func sceneRealTouchTap(g *Game, p image.Point) {
	down := true
	restore := SetTouchForTest(
		func() []ebiten.TouchID {
			if down {
				return []ebiten.TouchID{1}
			}
			return nil
		},
		func(ebiten.TouchID) (int, int) { return p.X, p.Y },
	)
	defer restore()
	for i := 0; i < 2; i++ {
		_ = g.Update()
	}
	down = false
	for i := 0; i < 6; i++ {
		_ = g.Update()
	}
}

// Clear of avoid, the top-right cluster, nodes and group badges.
func sceneEmptyGridPointOutside(g *Game, avoid image.Rectangle) (image.Point, bool) {
	grid := g.split.GridRect(g.winW, g.winH)
	midY := (gridTopOffset() + grid.Max.Y) / 2
	for dy := 0; dy < grid.Dy()/2; dy += 13 {
		for _, y := range []int{midY + dy, midY - dy} {
			for x := grid.Min.X + 40; x < grid.Max.X-40; x += 17 {
				p := image.Pt(x, y)
				if p.In(avoid.Inset(-8)) || !g.split.InGridPane(x, y) || y < gridTopOffset()+20 ||
					p.In(g.split.InputBounds()) || g.inTopRightButtonZone(x, y) || g.nodeAtScreen(x, y) != nil {
					continue
				}
				if _, ok := g.groupBadgeAt(x, y); ok {
					continue
				}
				return p, true
			}
		}
	}
	return image.Point{}, false
}

// The tap must close the menu and create NO node.
func menuOutsideTapGridSceneSetup(g *Game) {
	_ = g.Update()
	r := g.gridThemeButtonRect()
	sceneRealTouchTap(g, image.Pt(r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2))
	g.Draw(ebiten.NewImage(g.winW, g.winH)) // the overlay lays out in Draw
	ov, ok := g.drum.portal().OverlayByID(themeOverlayID).(*ThemeOverlay)
	if !ok {
		g.logger.Errorf("[scene] menu_outside_tap_grid: theme editor did not open")
		return
	}
	p, ok := sceneEmptyGridPointOutside(g, ov.Rect())
	if !ok {
		g.logger.Errorf("[scene] menu_outside_tap_grid: no empty grid point outside %v", ov.Rect())
		return
	}
	before, wasOpen := len(g.graph.Nodes), g.drum.portal().Has(themeOverlayID)
	sceneRealTouchTap(g, p)
	g.logger.Infof("[scene] menu_outside_tap_grid: theme open before=%v; tapped %v, theme open after=%v, nodes %d -> %d",
		wasOpen, p, g.drum.portal().Has(themeOverlayID), before, len(g.graph.Nodes))
}
