// src/go/internal/ui/drumview_wheel_popup.go
package ui

import "image"

// openSynthKnobWheelPopup opens the mobile scroll-wheel for synth knob idx,
// wired to the named instrument instance. It closes any existing popups first,
// then opens the wheel through the portal system.
func (dv *DrumView) openSynthKnobWheelPopup(idx int, instID string) {
	if idx < 0 || idx >= len(dv.instEditorKnobs) || idx >= len(dv.instEditorBindings) {
		return
	}
	dv.CloseAllPopups()

	def := dv.instEditorBindings[idx].def
	k := dv.instEditorKnobs[idx]
	var badge *KnobStepBadge
	if idx < len(dv.instEditorStepBadges) {
		badge = dv.instEditorStepBadges[idx]
	}
	discrete := len(def.Enum) > 0 || def.Step > 0

	anchor := dv.instEditorKnobs[idx].Rect()
	binding := WheelBinding{
		Knob:     k,
		Badge:    badge,
		Def:      def,
		Discrete: discrete,
		Title:    func() string { return def.Label },
		OnChange: func() {
			dv.syncSliderFromKnob(idx)
			dv.propagateSynthSliderValue(idx, instID)
		},
		OnCommit: func() {
			dv.requestSynthMirror(instID)
			dv.commitInstrumentParams(instID)
		},
		OnResolution: func() { dv.persistKnobStep(badge) },
		OpenEditor:   func() { dv.openSynthParamEditor(idx, instID) },
		Accent:       nil, // renderer falls back to the primary token
	}
	dv.synthWheelPopup.Open(binding, anchor, dv.Bounds, dv.headerH)
	dv.openSynthWheelPortal()
}

// SynthWheelPopupRect returns the screen-space bounding rectangle of the
// mobile synth-knob scroll-wheel popup, or an empty rectangle when the popup
// is nil or not open. Used by (*Game).SubjectRect for screenshot cropping.
func (dv *DrumView) SynthWheelPopupRect() image.Rectangle {
	if dv.synthWheelPopup == nil || !dv.synthWheelPopup.IsOpen() {
		return image.Rectangle{}
	}
	return dv.synthWheelPopup.Rect()
}

// closeSynthKnobWheelPopup closes the mobile scroll-wheel for synth knobs.
// Navigating away persists the pending edit (Accept commits + closes; no-op if
// already closed).
func (dv *DrumView) closeSynthKnobWheelPopup() {
	if dv.synthWheelPopup != nil {
		dv.synthWheelPopup.Accept()
	}
	dv.closeSynthWheelPortal()
}
