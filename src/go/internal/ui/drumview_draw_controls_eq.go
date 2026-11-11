package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// markRowControlsDirty invalidates the row controls cache.
func (dv *DrumView) markRowControlsDirty() {
	dv.rowControlsCacheDirty = true
	if dv.rowRackZone != nil {
		dv.rowRackZone.MarkDirty()
	}
}

// computeRowControlsBounds returns the bounding rectangle for all visible row controls.
// Delegates to RowRackZone when available.
func (dv *DrumView) computeRowControlsBounds() image.Rectangle {
	if dv.rowRackZone != nil {
		return dv.rowRackZone.computeControlsBounds()
	}
	return image.Rectangle{}
}

// drawEQ delegates to the EQPanelZone for backward-compat test access.
func (dv *DrumView) drawEQ(dst *ebiten.Image) {
	if dv.eqPanelZone != nil {
		dv.eqPanelZone.Draw(dst)
		// Sync zone band values back to DrumView for backward-compat test access.
		dv.eqLastBands = append(dv.eqLastBands[:0], dv.eqPanelZone.eqBandVals...)
	}
}
