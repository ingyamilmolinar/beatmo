package ui

import "github.com/hajimehoshi/ebiten/v2"

// Marks every color-baking cache dirty, like OnLocaleChanged; the rebuild costs one frame like a resize.
func (g *Game) OnThemeChanged() {
	if g == nil {
		return
	}
	// Sprites are keyed by color and simply miss; clearing frees the stale textures.
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
	// The profile caches token colors at build time, so it must be rebuilt.
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

// Repainted AFTER the pane dividers, which are drawn above both panes and would otherwise cross
// these window-bounded panels. Scrims are not repeated.
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
	// Only when the card leaves the drum pane, so the normal case costs no second draw.
	if nm := g.drum.navMenuComp; nm != nil && nm.IsOpen() && !nm.card.Empty() && !nm.card.In(g.drum.Bounds) {
		nm.Draw(screen)
		// Or the repaint hides the stacked picker while it still takes taps.
		if jm := p.OverlayByID("jump-menu"); jm != nil {
			jm.Draw(screen)
		}
	}
	// Same for the shared value wheel opened from the grid pane (node values).
	if vw := g.drum.valueWheelPopup; vw != nil && vw.IsOpen() && !vw.Rect().In(g.drum.Bounds) {
		vw.Draw(screen)
	}
	// Its wheels sit on the divider when it drops from the logs time field.
	if lc := g.drum.logsComp; lc != nil && lc.picker != nil && lc.picker.IsOpen() && g.drum.IsLogsPanelOpen() {
		if r := lc.picker.rect; !r.Empty() && !r.In(g.drum.Bounds) && r.Max.Y > g.drum.Bounds.Min.Y {
			lc.picker.Draw(screen)
		}
	}
}
