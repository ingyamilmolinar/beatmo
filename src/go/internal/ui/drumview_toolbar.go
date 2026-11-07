package ui

func (dv *DrumView) decayAnims() {
	decay := func(v *float64) {
		*v *= 0.85
		if *v < 0.01 {
			*v = 0
		}
	}
	// Transport animations are decayed by the zone's Update() when available.
	if dv.transportZone == nil {
		decay(&dv.playAnim)
		decay(&dv.stopAnim)
		decay(&dv.bpmDecAnim)
		decay(&dv.bpmIncAnim)
		decay(&dv.uploadAnim)
		decay(&dv.bpmErrorAnim)
	} else {
		// Take the higher value so the toolbar cache sees errors raised on either side.
		if dv.bpmErrorAnim > dv.transportZone.bpmErrorAnim {
			dv.transportZone.bpmErrorAnim = dv.bpmErrorAnim
		} else {
			dv.bpmErrorAnim = dv.transportZone.bpmErrorAnim
		}
		decay(&dv.bpmErrorAnim)
		dv.transportZone.bpmErrorAnim = dv.bpmErrorAnim
	}
	// DrumView-specific animations.
	decay(&dv.lenDecAnim)
	decay(&dv.lenIncAnim)
	decay(&dv.saveAnim)
	for i := range dv.rowFireDecay {
		decay(&dv.rowFireDecay[i])
	}
	// Reset delete confirmation after ~2s timeout
	if dv.deleteConfirmRow >= 0 && (dv.frame-dv.deleteConfirmFrame) >= 120 {
		dv.deleteConfirmRow = -1
		dv.markRowControlsDirty()
	}
}
