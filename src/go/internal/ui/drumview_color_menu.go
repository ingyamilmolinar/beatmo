package ui

import (
	"image"
)

// buildColorMenu rebuilds the color picker dropdown for the selected row.
// After the component computes its bounds via rebuildWheel(), this syncs
// dv.colorWheelRect for legacy callers and rebuilds the wheel image.
func (dv *DrumView) buildColorMenu() {
	dv.colorWheelRect = image.Rect(0, 0, 0, 0)
	if dv.colorMenuRow < 0 || dv.colorMenuRow >= len(dv.rowColorBtns()) {
		return
	}

	// The component's SetProps+Open already computed the wheel rect.
	// Sync it to the legacy field.
	if dv.colorWheelComp != nil && dv.colorWheelComp.IsOpen() {
		dv.colorWheelRect = dv.colorWheelComp.WheelRect()
	}

	dv.logger.Debugf("[COLOR] build wheel: rect=%v", dv.colorWheelRect)
	dv.rebuildColorWheelImage()
}
