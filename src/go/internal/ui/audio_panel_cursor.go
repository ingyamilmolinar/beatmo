package ui

import "image"

// Wave/Spectrum only: desktop tracks hover; mobile is driven by cursorScrubHandler, so a lifted
// finger stays latched. cursorPosition() honours test overrides and the touch layer.
func (z *EQPanelZone) updateAudioPanelCursor() {
	tab := z.tabState.ActiveTab()
	if tab != TabWave && tab != TabSpectrum {
		z.cursorActive = false
		z.cursorPinned = false
		return
	}
	cr := z.bodyRect()
	if cr.Empty() {
		z.cursorActive = false
		return
	}
	if Profile().IsMobile() {
		return
	}
	mx, my := cursorPosition()
	if image.Pt(mx, my).In(cr) {
		z.cursorX = mx
		z.cursorActive = true
	} else {
		z.cursorActive = false
	}
}

type cursorScrubHandler struct{ zone *EQPanelZone }

// Clears the previous gesture's pin.
func (h *cursorScrubHandler) OnPress(x, y int) InputResult {
	if h.zone == nil {
		return InputIgnored
	}
	h.zone.cursorX = x
	h.zone.cursorActive = true
	h.zone.cursorPinned = false
	return InputCaptured
}

// OnDrag tracks the touch as the user scrubs along the panel.
func (h *cursorScrubHandler) OnDrag(x, y int) {
	if h.zone == nil {
		return
	}
	h.zone.cursorX = x
	h.zone.cursorActive = true
}

// Mobile keeps the readout after the finger lifts so the value can be read.
func (h *cursorScrubHandler) OnRelease(x, y int) {
	if h.zone == nil {
		return
	}
	h.zone.cursorPinned = true
}

// OnWheel is unused for scrubbing.
func (h *cursorScrubHandler) OnWheel(x, y, steps int) InputResult { return InputIgnored }
