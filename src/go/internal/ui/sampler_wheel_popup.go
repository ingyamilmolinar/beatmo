// src/go/internal/ui/sampler_wheel_popup.go
package ui

import (
	"image"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// openSamplerKnobWheelPopup opens the mobile scroll-wheel for sampler knob idx,
// wired to the sampler's shared knob value-model. It closes any existing popups
// first, then opens the wheel through the portal system. Mirrors the synth tab's
// openSynthKnobWheelPopup; the sampler differs only in that its ParamDef is
// synthesized from samplerKnobScale (it has no per-knob binding array) and every
// sampler knob is continuous (Discrete=false).
func (p *samplerPanel) openSamplerKnobWheelPopup(idx int) {
	dv := p.dv
	if idx < 0 || idx >= samplerKnobCount || idx >= len(dv.sampler.knobs) {
		return
	}
	k := dv.sampler.knobs[idx]
	if k == nil {
		return
	}
	dv.CloseAllPopups()

	sc := samplerKnobScale(idx)
	def := audio.ParamDef{Name: samplerStepPrefName(idx), Min: sc.Min, Max: sc.Max, Unit: sc.Unit}
	var badge *KnobStepBadge
	if idx < len(dv.sampler.knobStepBadges) {
		badge = dv.sampler.knobStepBadges[idx]
	}

	anchor := k.Rect()
	binding := WheelBinding{
		Knob:     k,
		Badge:    badge,
		Def:      def,
		Discrete: false, // every sampler knob is continuous (endless + step badge)
		Title:    func() string { return samplerKnobPlainEnglish(idx) },
		OnChange: func() { dv.applySamplerKnob(idx) },
		// One undo step per gesture, committed on release — the same site the
		// rotary path commits at (samplerKnobHitAdapter.OnRelease).
		OnCommit:     func() { dv.commitSamplerEdit() },
		OnResolution: func() { dv.persistKnobStep(badge) },
		OpenEditor:   func() { dv.openSamplerParamEditor(idx) },
		Accent:       nil, // renderer falls back to the primary token
	}
	dv.samplerWheelPopup.Open(binding, anchor, dv.Bounds, dv.headerH)
	dv.openSamplerWheelPortal()
}

// SamplerWheelPopupRect returns the screen-space bounding rectangle of the
// mobile sampler-knob scroll-wheel popup, or an empty rectangle when the popup
// is nil or not open. Parity with SynthWheelPopupRect (screenshot cropping).
func (p *samplerPanel) SamplerWheelPopupRect() image.Rectangle {
	dv := p.dv
	if dv.samplerWheelPopup == nil || !dv.samplerWheelPopup.IsOpen() {
		return image.Rectangle{}
	}
	return dv.samplerWheelPopup.Rect()
}

// samplerMobileKnobButtonRect computes the compact pop-up button rect for
// sampler knob idx on mobile. The button sits in the dial's vertical band but
// spans the cell width so a long localized caption fits; the value caption is
// drawn separately BELOW the dial band (so captionH=0 here). Mirrors the synth
// tab's mobileKnobButtonRect via the shared knobValuePillRect.
func (p *samplerPanel) samplerMobileKnobButtonRect(idx int) image.Rectangle {
	dv := p.dv
	s := &dv.sampler
	if idx < 0 || idx >= len(s.knobs) || s.knobs[idx] == nil {
		return image.Rectangle{}
	}
	dialR := s.knobs[idx].Rect()
	if dialR.Empty() {
		return image.Rectangle{}
	}
	cell := s.knobCells[idx]
	if cell.Empty() {
		cell = dialR
	}
	band := image.Rect(cell.Min.X, dialR.Min.Y, cell.Max.X, dialR.Max.Y)
	// Size to the VALUE label — that's what the pill renders (the name
	// lives in the caption band below).
	return knobValuePillRect(band, samplerKnobValueText(idx, s), 0)
}

// closeSamplerKnobWheelPopup closes the mobile scroll-wheel for sampler knobs.
// Navigating away persists the pending edit (Accept commits + closes; no-op if
// already closed).
func (p *samplerPanel) closeSamplerKnobWheelPopup() {
	dv := p.dv
	if dv.samplerWheelPopup != nil {
		dv.samplerWheelPopup.Accept()
	}
	dv.closeSamplerWheelPortal()
}

// ── *DrumView forwarding shims (Rung A relocation onto *samplerPanel) ──────

func (dv *DrumView) openSamplerKnobWheelPopup(idx int) { dv.sp().openSamplerKnobWheelPopup(idx) }

// SamplerWheelPopupRect returns the screen-space bounding rectangle of the
// mobile sampler-knob scroll-wheel popup. Exported forwarding shim onto the
// relocated *samplerPanel method.
func (dv *DrumView) SamplerWheelPopupRect() image.Rectangle { return dv.sp().SamplerWheelPopupRect() }

func (dv *DrumView) samplerMobileKnobButtonRect(idx int) image.Rectangle {
	return dv.sp().samplerMobileKnobButtonRect(idx)
}

func (dv *DrumView) closeSamplerKnobWheelPopup() { dv.sp().closeSamplerKnobWheelPopup() }
