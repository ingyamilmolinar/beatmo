// src/go/internal/ui/eq_wheel_popup.go
package ui

import (
	"image"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// eqDBMin/eqDBMax bound the EQ band gain domain, matching eqDBSpec()'s clamp;
// eqDBSpan is their derived width, used to map the endless knob's normalized
// [0,1] value onto real dB units.
const (
	eqDBMin  = -12.0
	eqDBMax  = 12.0
	eqDBSpan = eqDBMax - eqDBMin
)

// openEQKnobWheelPopup opens the precision wheel popup for EQ band's dB gain.
// Works on desktop AND mobile (unlike the synth/sampler wheels, which are
// mobile-only). The wheel edits EQPanelZone.bandGainsDB — the same source of
// truth as the curve-drag handles — via applyBandGainLive; one undo step is
// committed on release through commitEQBand. The center-box tap re-opens the
// inline numeric editor so text entry is preserved.
func (dv *DrumView) openEQKnobWheelPopup(band int) {
	z := dv.eqPanelZone
	if z == nil || band < 0 || band >= len(z.bandGainsDB) || dv.eqWheelPopup == nil {
		return
	}
	dv.CloseAllPopups()

	badge := dv.eqWheelStepBadge
	// Lazily restore the persisted step-resolution rung on first open, not in
	// the ctor: the ctor may run before SetKnobStepSink installs the global
	// sink, so an eager read there would always miss.
	if !dv.eqWheelStepRestored {
		if persisted, ok := dv.knobStepPref("eq_band_gain"); ok && badge != nil {
			badge.SetStep(persisted)
		}
		dv.eqWheelStepRestored = true
	}
	def := audio.ParamDef{Name: "eq_band_gain", Label: eqBandLabel(band), Min: eqDBMin, Max: eqDBMax, Unit: "dB"}

	// Transient knob seeded from the current gain; endless accumulation in real
	// dB units, one StepMul (badge rung) per notch.
	k := NewKnob((z.bandGainsDB[band] - eqDBMin) / eqDBSpan)
	k.Scale = KnobScale{Min: eqDBMin, Max: eqDBMax}
	k.Endless = true
	if badge != nil {
		k.StepMul = badge.Step()
	}

	anchor := z.dbReadoutRect(band)
	binding := WheelBinding{
		Knob:     k,
		Badge:    badge,
		Def:      def,
		Discrete: false,
		Title:    func() string { return eqBandLabel(band) },
		OnChange: func() {
			db := eqDBMin + k.Value*eqDBSpan
			z.applyBandGainLive(band, db)
		},
		OnCommit:     func() { dv.commitEQBand() },
		OnResolution: func() { dv.persistKnobStep(badge) },
		OpenEditor:   func() { z.openEQDBEditor(band) },
		Accent:       nil,
	}
	dv.eqWheelPopup.Open(binding, anchor, dv.Bounds, dv.headerH)
	dv.openEQWheelPortal()
}

// closeEQKnobWheelPopup closes the EQ precision wheel popup. Navigating away
// (CloseAllPopups on tab switch / opening another control) persists the pending
// edit — Accept commits it and closes; a no-op when the popup is already closed.
func (dv *DrumView) closeEQKnobWheelPopup() {
	if dv.eqWheelPopup != nil {
		dv.eqWheelPopup.Accept()
	}
	dv.closeEQWheelPortal()
}

// EQWheelPopupRect returns the screen-space rect of the EQ wheel popup (empty
// when nil/closed). Parity with SamplerWheelPopupRect for screenshot cropping.
func (dv *DrumView) EQWheelPopupRect() image.Rectangle {
	if dv.eqWheelPopup == nil || !dv.eqWheelPopup.IsOpen() {
		return image.Rectangle{}
	}
	return dv.eqWheelPopup.Rect()
}

// eqBandLabel returns a short human label for EQ band index (e.g. "1k").
// Reuses the zone's existing per-band ISO center-frequency labels
// (eqCenterLabels, drumview_eq_popup.go) — the same strings already drawn on
// the readout cells and used to build the mute/dB hit-area tags — rather than
// introducing a second table.
func eqBandLabel(band int) string {
	if band >= 0 && band < len(eqCenterLabels) {
		return eqCenterLabels[band]
	}
	return ""
}
