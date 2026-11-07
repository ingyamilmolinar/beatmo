// src/go/internal/ui/eq_wheel_popup.go
package ui

import (
	"image"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// Must match eqDBSpec()'s clamp; eqDBSpan maps the knob's normalized value onto dB.
const (
	eqDBMin  = -12.0
	eqDBMax  = 12.0
	eqDBSpan = eqDBMax - eqDBMin
)

// Desktop AND mobile (the synth/sampler wheels are mobile-only). It edits bandGainsDB, the curve handles'
// source of truth, live; one undo step lands on release.
func (dv *DrumView) openEQKnobWheelPopup(band int) {
	z := dv.eqPanelZone
	if z == nil || band < 0 || band >= len(z.bandGainsDB) || dv.eqWheelPopup == nil {
		return
	}
	dv.CloseAllPopups()

	badge := dv.eqWheelStepBadge
	// Restored lazily on first open: the ctor may run before SetKnobStepSink installs the sink.
	if !dv.eqWheelStepRestored {
		if persisted, ok := dv.knobStepPref("eq_band_gain"); ok && badge != nil {
			badge.SetStep(persisted)
		}
		dv.eqWheelStepRestored = true
	}
	def := audio.ParamDef{Name: "eq_band_gain", Label: eqBandLabel(band), Min: eqDBMin, Max: eqDBMax, Unit: "dB"}

	// Endless accumulation in real dB units, one StepMul (badge rung) per notch.
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
		Gloss:    func() string { return PlainEnglish("Gain") },
		OnChange: func() {
			db := eqDBMin + k.Value*eqDBSpan
			z.applyBandGainLive(band, db)
		},
		OnCommit:     func() { dv.commitEQBand() },
		OnResolution: func() { dv.persistKnobStep(badge) },
		Accent:       nil,
	}
	dv.eqWheelPopup.Open(binding, anchor, dv.Bounds, dv.headerH)
	dv.openEQWheelPortal()
}

// Navigating away keeps the pending edit: Accept commits it and closes. No-op when already closed.
func (dv *DrumView) closeEQKnobWheelPopup() {
	if dv.eqWheelPopup != nil {
		dv.eqWheelPopup.Accept()
	}
	dv.closeEQWheelPortal()
}

func (dv *DrumView) EQWheelPopupRect() image.Rectangle {
	if dv.eqWheelPopup == nil || !dv.eqWheelPopup.IsOpen() {
		return image.Rectangle{}
	}
	return dv.eqWheelPopup.Rect()
}

// Reuses eqCenterLabels, the strings already drawn on the readout cells, rather than a second table.
func eqBandLabel(band int) string {
	if band >= 0 && band < len(eqCenterLabels) {
		return eqCenterLabels[band]
	}
	return ""
}
