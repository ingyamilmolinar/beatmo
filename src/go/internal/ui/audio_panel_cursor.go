package ui

import "image"

// updateAudioPanelCursor refreshes the EQPanelZone's cursor state from the
// current input source. It is called once per Update() at the top of the
// frame so the crosshair/readout drawn later in Draw() always matches the
// latest pointer.
//
// Behavior matrix:
//
//	Tab is not Wave or Spectrum   → cursor cleared (no draw)
//	Tab is Wave/Spectrum + desktop → cursor tracks mouse hover; clears when
//	                                  the mouse leaves contentRect().
//	Tab is Wave/Spectrum + mobile  → handled by cursorScrubHandler (registered
//	                                  by rebuildHitAreas). This function does
//	                                  nothing per-frame on mobile so a finger-
//	                                  up stays latched until the next press.
//
// Note: we deliberately use cursorPosition() (the package-level function
// variable, which honors test overrides and the touch-override layer) rather
// than ebiten.CursorPosition directly. Tests inject input via
// SetInputForTest and the override falls through here.
func (z *EQPanelZone) updateAudioPanelCursor() {
	tab := z.tabState.ActiveTab()
	if tab != TabWave && tab != TabSpectrum {
		z.cursorActive = false
		z.cursorPinned = false
		return
	}
	cr := z.contentRect()
	if cr.Empty() {
		z.cursorActive = false
		return
	}
	if Profile().IsMobile() {
		// Touch path: the scrub hit area drives cursorX/cursorActive on
		// press/drag and sets cursorPinned on release. Nothing to do here.
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

// cursorScrubHandler dispatches mobile touch press/drag/release events to the
// owning EQPanelZone's cursor state. Registered as a hit-area handler over
// the panel's content rect on the Wave and Spectrum tabs only.
type cursorScrubHandler struct{ zone *EQPanelZone }

// OnPress activates the cursor at the press position. It clears any prior
// "pinned" state so the latched cursor from the previous gesture is replaced
// by the new live one.
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

// OnRelease latches the cursor in place. On mobile we keep the readout
// visible after the finger lifts so the user can read the value they aimed
// at — a press elsewhere (or a tab switch) clears it.
func (h *cursorScrubHandler) OnRelease(x, y int) {
	if h.zone == nil {
		return
	}
	h.zone.cursorPinned = true
}

// OnWheel is unused for scrubbing.
func (h *cursorScrubHandler) OnWheel(x, y, steps int) InputResult { return InputIgnored }
