package ui

func (dv *DrumView) buildColorMenu() {
	if dv.colorMenuRow < 0 || dv.colorMenuRow >= len(dv.rowColorBtns()) {
		return
	}
	if dv.colorWheelComp != nil && dv.colorWheelComp.IsOpen() {
		dv.logger.Debugf("[color] build picker: rect=%v", dv.colorWheelComp.WheelRect())
	}
}
