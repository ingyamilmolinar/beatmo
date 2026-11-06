// src/go/internal/ui/sampler_wheel_popup.go
package ui

import (
	"image"
)

// openSamplerKnobWheelPopup opens the mobile scroll-wheel for sampler knob idx,
// wired to the sampler's shared knob value-model. It closes any existing popups
// first, then opens the wheel through the portal system. (The synth tab's
// equivalent popup path was retired in Task 13 — synth controls now arm
// inline instead.) The sampler's ParamDef is synthesized from
// samplerKnobScale (it has no per-knob binding array) and every sampler knob
// is continuous (Discrete=false).
func (p *samplerPanel) openSamplerKnobWheelPopup(idx int) {
	dv := p.dv
	if idx < 0 || idx >= samplerKnobCount || idx >= len(dv.sampler.knobs) {
		return
	}
	k := dv.sampler.knobs[idx]
	if k == nil {
		return
	}
	// Flush any pending desktop wheel-nudge commit so the popup snapshots the
	// committed value.
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
		// Title is the param NAME ("Start", "Pitch"); the PlainEnglish gloss moves
		// to the title band's Gloss slot (Task 7 popup v2).
		Title:    func() string { return samplerKnobNameText(idx, &dv.sampler) },
		Gloss:    func() string { return samplerKnobPlainEnglish(idx) },
		OnChange: func() { dv.applySamplerKnob(idx) },
		// One undo step per gesture, committed on release — the same site the
		// rotary path commits at (samplerKnobHitAdapter.OnRelease).
		OnCommit:     func() { dv.commitSamplerEdit() },
		OnResolution: func() { dv.persistKnobStep(badge) },
		Accent:       nil, // renderer falls back to the primary token
	}
	dv.samplerWheelPopup.Open(binding, anchor, dv.Bounds, dv.headerH)
	dv.openSamplerWheelPortal()
}

// SamplerWheelPopupRect returns the screen-space bounding rectangle of the
// mobile sampler-knob scroll-wheel popup, or an empty rectangle when the popup
// is nil or not open. Used by (*Game).SubjectRect for screenshot cropping.
func (p *samplerPanel) SamplerWheelPopupRect() image.Rectangle {
	dv := p.dv
	if dv.samplerWheelPopup == nil || !dv.samplerWheelPopup.IsOpen() {
		return image.Rectangle{}
	}
	return dv.samplerWheelPopup.Rect()
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

func (dv *DrumView) closeSamplerKnobWheelPopup() { dv.sp().closeSamplerKnobWheelPopup() }
