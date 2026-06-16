package ui

// decayAnims drives the per-frame decay of all DrumView animation
// counters. State-only — no drawing. Called from (*DrumView).Draw.
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
		// Bidirectional sync: BPM text input in DrumView.Update() may set
		// dv.bpmErrorAnim directly; propagate the higher value between
		// DrumView and zone so the toolbar cache sees it.
		if dv.bpmErrorAnim > dv.transportZone.bpmErrorAnim {
			dv.transportZone.bpmErrorAnim = dv.bpmErrorAnim
		} else {
			dv.bpmErrorAnim = dv.transportZone.bpmErrorAnim
		}
		decay(&dv.bpmErrorAnim)
		dv.transportZone.bpmErrorAnim = dv.bpmErrorAnim
	}
	// Stage 5: tick per-knob synth concept ghosts (fade-out after release).
	dv.advanceSynthGhost()
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
