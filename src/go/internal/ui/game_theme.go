package ui

import "github.com/hajimehoshi/ebiten/v2"

// OnThemeChanged is the whole-Game theme chokepoint, mirroring
// DrumView.OnLocaleChanged: every cache that BAKES a color is marked dirty so
// the next Draw rebuilds it from the freshly rebuilt palette. Cheap — flags
// only; the rebuild burst on the following frame is the same as a resize.
// Registered with OnThemeChange in New(); runs on the UI goroutine.
func (g *Game) OnThemeChanged() {
	if g == nil {
		return
	}
	// Grid pane: tile (grid lines), node layer + sprites (node fill/border),
	// edge cache (edge color). Sprites are keyed by color and simply miss;
	// clearing the map frees the stale textures.
	g.gridTileSubSig = 0
	g.gridTile = nil
	g.nodeLayerDirty = true
	g.nodeLayerGraphSig = 0
	g.nodeSpriteCache = map[spriteKey]*ebiten.Image{}
	g.edgeCacheColorSig = 0
	g.edgesDirty = true
	// Subdivision line styles copy their token at construction.
	if g.grid != nil {
		g.grid.RefreshLineColors()
	}
	// The add-row "+" strip sprite is keyed by size/state only.
	invalidateAddRowBtnCache()
	// The layout profile caches token colors and styles (PlayIconColor,
	// ScrollbarStyle, …) at build time; drop it so Profile() rebuilds from
	// the live tokens on the next call and the zones re-copy them on their
	// invalidated layout pass.
	activeProfile = nil
	dv := g.drum
	if dv == nil {
		return
	}
	dv.invalidateRowCaches()
	dv.invalidateLabelCaches()
	dv.markRowControlsDirty()
	if dv.eqPanelZone != nil {
		dv.eqPanelZone.Invalidate()
		if cz := dv.eqPanelZone.ChainZone(); cz != nil {
			cz.Invalidate()
		}
	}
	if dv.rowRackZone != nil {
		dv.rowRackZone.Invalidate()
	}
	if dv.transportZone != nil {
		dv.transportZone.Invalidate()
	}
	if dv.timelineZone != nil {
		dv.timelineZone.Invalidate()
		dv.timelineZone.InvalidateRowCaches()
	}
	if dv.synthPanel != nil {
		dv.synthPanel.sprites.releaseAll()
	}
}

// drawThemeOverlaysAboveDivider re-paints the window-bounded portal panels —
// the theme editor (and the swatch picker stacked on it), and the navigation
// menu when it crosses the divider — AFTER the pane dividers. Without this pass
// the splitter lines, drawn last "above both panes", would cross them. Scrims
// are not repeated (both draw only chrome here).
func (g *Game) drawThemeOverlaysAboveDivider(screen *ebiten.Image) {
	if g == nil || g.drum == nil {
		return
	}
	p := g.drum.portal()
	if p == nil {
		return
	}
	if ov, ok := p.OverlayByID(themeOverlayID).(*ThemeOverlay); ok && ov != nil {
		ov.DrawPanel(screen)
	}
	if pk := p.OverlayByID(themePickerID); pk != nil {
		pk.Draw(screen)
	}
	// The navigation menu is window-bounded too; repaint it only when its card
	// actually crosses outside the drum pane (a short pane), so the normal case
	// costs no second draw.
	if nm := g.drum.navMenuComp; nm != nil && nm.IsOpen() && !nm.card.Empty() && !nm.card.In(g.drum.Bounds) {
		nm.Draw(screen)
		// …and the jump-size picker stacked on it, or the repaint hides it
		// while it still takes taps (mirrors the theme picker above).
		if jm := p.OverlayByID("jump-menu"); jm != nil {
			jm.Draw(screen)
		}
	}
	// Same for the shared value wheel opened from the grid pane (node values).
	if vw := g.drum.valueWheelPopup; vw != nil && vw.IsOpen() && !vw.Rect().In(g.drum.Bounds) {
		vw.Draw(screen)
	}
	// …and the logs date-time picker, whose HH / MM wheels sit on the divider
	// when it drops from the logs panel's time field.
	if lc := g.drum.logsComp; lc != nil && lc.picker != nil && lc.picker.IsOpen() && g.drum.IsLogsPanelOpen() {
		if r := lc.picker.rect; !r.Empty() && !r.In(g.drum.Bounds) && r.Max.Y > g.drum.Bounds.Min.Y {
			lc.picker.Draw(screen)
		}
	}
}
