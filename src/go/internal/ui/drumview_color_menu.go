package ui

// buildColorMenu finalizes the swatch-grid color picker for the selected row.
//
// The picker is the ColorWheelComponent (a swatch grid restricted to the
// curated instrument palette); it computes and owns its own panel rect via
// rebuildWheel() when SetProps+Open runs. There is no separate bitmap wheel to
// rebuild anymore — this hook simply guards the selection and logs.
func (dv *DrumView) buildColorMenu() {
	if dv.colorMenuRow < 0 || dv.colorMenuRow >= len(dv.rowColorBtns()) {
		return
	}
	if dv.colorWheelComp != nil && dv.colorWheelComp.IsOpen() {
		dv.logger.Debugf("[color] build picker: rect=%v", dv.colorWheelComp.WheelRect())
	}
}
