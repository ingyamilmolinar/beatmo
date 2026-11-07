// src/go/internal/ui/sampler_wheel_popup.go
package ui

import (
	"image"
)

// Every sampler knob is continuous, and its ParamDef is synthesized from samplerKnobScale.
func (p *samplerPanel) openSamplerKnobWheelPopup(idx int) {
	dv := p.dv
	if idx < 0 || idx >= samplerKnobCount || idx >= len(dv.sampler.knobs) {
		return
	}
	k := dv.sampler.knobs[idx]
	if k == nil {
		return
	}
	// Flush the wheel-nudge commit so the popup snapshots the committed value.
	dv.samplerWheelNudge.flush()
	dv.CloseAllPopups()

	def := dv.samplerKnobDef(idx)
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
		Title:    func() string { return samplerKnobNameText(idx, &dv.sampler) },
		Gloss:    func() string { return samplerKnobPlainEnglish(idx) },
		OnChange: func() { dv.applySamplerKnob(idx) },
		// One undo step per gesture, on release.
		OnCommit:     func() { dv.commitSamplerEdit() },
		OnResolution: func() { dv.persistKnobStep(badge) },
		Accent:       nil, // renderer falls back to the primary token
	}
	dv.samplerWheelPopup.Open(binding, anchor, dv.Bounds, dv.headerH)
	dv.openSamplerWheelPortal()
}

// Empty when nil or closed.
func (p *samplerPanel) SamplerWheelPopupRect() image.Rectangle {
	dv := p.dv
	if dv.samplerWheelPopup == nil || !dv.samplerWheelPopup.IsOpen() {
		return image.Rectangle{}
	}
	return dv.samplerWheelPopup.Rect()
}

// Navigating away keeps the pending edit (Accept commits and closes).
func (p *samplerPanel) closeSamplerKnobWheelPopup() {
	dv := p.dv
	if dv.samplerWheelPopup != nil {
		dv.samplerWheelPopup.Accept()
	}
	dv.closeSamplerWheelPortal()
}

// ── *DrumView forwarding shims (Rung A relocation onto *samplerPanel) ──────

func (dv *DrumView) openSamplerKnobWheelPopup(idx int) { dv.sp().openSamplerKnobWheelPopup(idx) }

func (dv *DrumView) SamplerWheelPopupRect() image.Rectangle { return dv.sp().SamplerWheelPopupRect() }

func (dv *DrumView) closeSamplerKnobWheelPopup() { dv.sp().closeSamplerKnobWheelPopup() }
