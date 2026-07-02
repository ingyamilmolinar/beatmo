package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// synth_focus_graph.go — the Synth tab's right pane.
//
// The pane splits into two stacked bands:
//   top — "Your sound" (drawSynthMirror): the real output wave, the stable
//         end-signal anchor.
//   bot — the focus graph (drawSynthFocusGraph): a property-native picture of
//         the SELECTED knob (filter -> frequency curve, envelope -> ADSR, …),
//         drawn large, updating live while that knob is dragged.
//
// Replaces the old per-knob concept-viz band: one big graph for the one knob
// the user is looking at, instead of twelve tiny ones competing for room.

// splitSynthRightPane carves the preview rect into a top mirror band and a
// bottom focus band. The focus band is the star (>= the mirror). A 1px gap
// separates them. When the pane is too short to host a legible mirror, the
// mirror collapses to empty and the focus band takes the whole rect.
func splitSynthRightPane(r image.Rectangle) (mirror, focus image.Rectangle) {
	const minBand = 40
	if r.Dy() < minBand*2 {
		return image.Rectangle{}, r
	}
	// Mirror gets ~42%, focus ~58% (focus is bigger). Clamp the mirror to a
	// legible floor so a tall pane doesn't over-feed the anchor.
	mirrorH := r.Dy() * 42 / 100
	if mirrorH < minBand {
		mirrorH = minBand
	}
	mid := r.Min.Y + mirrorH
	mirror = image.Rect(r.Min.X, r.Min.Y, r.Max.X, mid)
	focus = image.Rect(r.Min.X, mid+1, r.Max.X, r.Max.Y)
	return mirror, focus
}

// synthFocusRenderers maps each param GROUP to the PROPERTY-NATIVE renderer the
// focus graph draws large. Unlike the (retired) per-knob conceptRenderers — which
// drew the output wave for every timbre knob — the focus graph shows each knob in
// its own domain: a filter as a frequency curve, drive as a transfer curve, etc.
// The grab-bag groups (core/generic/voice) and unknown groups fall back to a
// level-scaled output wave so volume/gain/pan changes stay visible.
var synthFocusRenderers = map[string]conceptRenderer{
	"osc":      conceptOsc,
	"filter":   conceptFilter,
	"env":      conceptEnvelope,
	"fm":       conceptFM,
	"post":     conceptPost,
	"pitch":    conceptMotion,
	"pitchenv": conceptMotion,
	"lfo":      conceptMotion,
	"burst":    conceptMotion,
	// filtenv (filter envelope) and unison/ensemble have no dedicated domain
	// curve yet. The level wave is the INTENTIONAL choice (not the silent
	// fallback): the env's depth/decay and the ensemble's voices/detune/mix all
	// scale the displayed output, so the selected knob stays responsive.
	"filtenv": conceptFocusLevelWave,
	"unison":  conceptFocusLevelWave,
	// kick (the configurable KICK stage): like "core"/"voice", the kick knobs
	// (harmonics/decay/pitch-drop/punch/tail) all scale the percussive output, so
	// the level wave keeps the selected knob responsive (no dedicated curve yet).
	"kick":    conceptFocusLevelWave,
	"core":    conceptFocusLevelWave,
	"generic": conceptFocusLevelWave,
	"voice":   conceptFocusLevelWave,
	"":        conceptFocusLevelWave,
}

// synthFocusRendererForGroup is TOTAL: every group resolves to a non-nil
// renderer; unknown groups fall back to the level wave (NOT a misleading filter
// curve). TestSynthFocusRenderer_EveryGroupExplicit keeps the fallback from
// silently absorbing a real new group.
func synthFocusRendererForGroup(group string) conceptRenderer {
	if r, ok := synthFocusRenderers[group]; ok {
		return r
	}
	return conceptFocusLevelWave
}

// conceptFocusLevelWave renders the instrument's output wave with its DISPLAY
// amplitude scaled by the selected knob's value fraction within [Min,Max], so a
// level/gain/volume knob visibly grows/shrinks the wave (the grab-bag knobs have
// no rich domain of their own, but they DO change the end signal's level). A
// faint ghost draws the wave at the pre-drag value's amplitude.
func conceptFocusLevelWave(dst *ebiten.Image, rect image.Rectangle, instID string, def audio.ParamDef, ghost map[string]float64) {
	if rect.Dx() < 8 || rect.Dy() < 6 {
		return
	}
	conceptBgFill(dst, rect)
	const cycles, pts = 3, 64
	wave := audio.RenderInstrumentPreviewWave(instID, nil, cycles, pts)
	if len(wave) < 2 {
		return
	}
	zoom := autoZoomWindow(wave, cycles)
	if len(zoom) >= 2 {
		wave = zoom
	}

	midY := (rect.Min.Y + rect.Max.Y) / 2
	half := float64(rect.Dy()-2) / 2

	frac := 1.0
	if def.Name != "" && def.Max > def.Min {
		merged := conceptMergedParams(instID)
		f := (merged[def.Name] - def.Min) / (def.Max - def.Min)
		if f < 0 {
			f = 0
		} else if f > 1 {
			f = 1
		}
		frac = 0.25 + 0.75*f
	}
	yAt := func(amp float64) func(float64) int {
		return func(v float64) int {
			if v > 1 {
				v = 1
			} else if v < -1 {
				v = -1
			}
			return int(float64(midY) - v*half*amp)
		}
	}

	if ghost != nil && def.Name != "" && def.Max > def.Min {
		if gv, ok := ghost[def.Name]; ok {
			gf := (gv - def.Min) / (def.Max - def.Min)
			if gf < 0 {
				gf = 0
			} else if gf > 1 {
				gf = 1
			}
			gAmp := 0.25 + 0.75*gf
			if gAmp != frac {
				conceptDrawFilledCurve(dst, rect, wave, yAt(gAmp), midY, nil, colConceptGhost)
			}
		}
	}
	conceptDrawFilledCurve(dst, rect, wave, yAt(frac), midY, colConceptFill, colConceptStroke)
}

// drawSynthFocusGraph paints the bottom band of the right pane: a large,
// property-native picture of the SELECTED knob for instID, with a caption naming
// the knob + its plain-English purpose. Dispatches by the knob's group through
// synthFocusRendererForGroup. Renders nothing (beyond chrome) when no section /
// knob resolves.
func (dv *DrumView) drawSynthFocusGraph(dst *ebiten.Image, rect image.Rectangle, instID string) {
	if rect.Dx() < 32 || rect.Dy() < 24 {
		return
	}
	drawRoundedRect(dst, rect, TokenSurface1(), RadiusSM, true)
	drawRoundedRect(dst, rect, TokenBorderSubtle(), RadiusSM, false)

	sec := dv.synthSelectedSection()
	if sec == nil {
		return
	}
	kIdx := dv.synthSelectedKnobIdx(instID, sec)
	if kIdx < 0 || kIdx >= len(dv.instEditorBindings) {
		return
	}
	def := dv.instEditorBindings[kIdx].def

	pad := SpaceSM
	captionScale := FontSizeCaption / FontSizeBody
	captionH := int(float64(TextHeight()) * captionScale)

	label := def.Label
	if label == "" {
		label = def.Name
	}
	caption := label
	if purpose := synthKnobPurpose(def); purpose != "" {
		caption = label + " — " + purpose
	}
	DrawTextColorAtScale(dst, caption, rect.Min.X+pad, rect.Min.Y+pad/2, colConceptStroke, captionScale)

	graphR := image.Rect(rect.Min.X+pad, rect.Min.Y+pad+captionH, rect.Max.X-pad, rect.Max.Y-pad)
	if graphR.Dx() < 8 || graphR.Dy() < 10 {
		return
	}
	synthFocusRendererForKnob(def)(dst, graphR, instID, def, dv.synthConceptGhost(kIdx))
}

// isSynthPitchKnob reports whether a knob changes PITCH rather than timbre. The
// cycle-normalized timbre renderers (conceptOsc/conceptFilter/…) can't show
// pitch — they normalize to a fixed number of cycles — so these route to
// conceptPitchWave, a real-pitch output window whose on-screen period tracks the
// knob.
func isSynthPitchKnob(name string) bool {
	switch name {
	case "osc_octave", "osc_detune", "pitch", "tune", "fm_base", "fm_base_freq", "fundamental", "base_freq":
		return true
	}
	return false
}

// synthFocusRendererForKnob picks the focus renderer for a SPECIFIC knob: pitch
// knobs get the pitch wave; everything else routes by group. This is the entry
// point drawSynthFocusGraph uses.
func synthFocusRendererForKnob(def audio.ParamDef) conceptRenderer {
	if isSynthPitchKnob(def.Name) {
		return conceptPitchWave
	}
	if isFMTimeDomainKnob(def.Name) {
		return conceptFMEnvelope
	}
	if def.Name == "fm_op1_level" && fmOp1IsCarrier(def) {
		return conceptFocusLevelWave
	}
	return synthFocusRendererForGroup(def.Group)
}

// fmOp1IsCarrier reports whether operator 1's LEVEL knob (fm_op1_level) maps to
// OUTPUT AMPLITUDE — i.e. whether raising it makes the sound louder, so the
// level-scaled output wave (conceptFocusLevelWave) would be an honest picture.
//
// Evidence (internal/audio/preview_render.go:412-456, the FM operator matrix):
// op1 (index 0) is the CARRIER in three of the four algorithms (0 default 2-op,
// 2 3-op chain, 3 4-op stack), but in every one of those the carrier is emitted
// via op(0, phaseMod) which NEVER multiplies by levels[0] — fm_op1_level is
// simply unused there, so it changes neither amplitude nor timbre. In algorithm
// 1 (Parallel) op1's level is just ONE of four summed carrier mix weights
// (preview_render.go:423-438), so it is not the sole amplitude control either.
// Across every algorithm fm_op1_level is therefore NOT a reliable output-
// amplitude knob, so we return false: it falls through to conceptFM (the FM
// output wave with operators forced audible) rather than painting a misleading
// level-scaled amplitude. Task 3 should KEEP fm_op1_level as a sweep exemption.
func fmOp1IsCarrier(def audio.ParamDef) bool {
	return false
}

// conceptPitchWave renders a fixed-time window of the REAL note
// (audio.RenderInstrumentPreview, whose period responds to pitch), so the wave's
// number of cycles on screen tracks pitch: higher pitch ⇒ denser wiggles.
// Amplitude-normalized via mirrorTraceGain for legibility. Pitch knobs only;
// timbre knobs use their domain-native renderers.
//
// The ghost (pre-drag pitch) is OMITTED here on purpose: there is no override
// form of RenderInstrumentPreview, so a ghost would require mutating+restoring
// the live instrument param mid-draw (a read-only operation). The
// responsiveness guarantee only needs the live render to track the knob, so we
// keep the draw side-effect-free.
func conceptPitchWave(dst *ebiten.Image, rect image.Rectangle, instID string, def audio.ParamDef, ghost map[string]float64) {
	if rect.Dx() < 8 || rect.Dy() < 6 {
		return
	}
	conceptBgFill(dst, rect)
	const windowMs = 24
	pcm := audio.RenderInstrumentPreview(instID, windowMs)
	if len(pcm) < 2 {
		return
	}
	midY := (rect.Min.Y + rect.Max.Y) / 2
	half := float64(rect.Dy()-2) / 2
	gain := mirrorTraceGain(pcm)
	yAt := func(v float64) int {
		v *= gain
		if v > 1 {
			v = 1
		} else if v < -1 {
			v = -1
		}
		return int(float64(midY) - v*half)
	}
	conceptDrawFilledCurve(dst, rect, pcm, yAt, midY, colConceptFill, colConceptStroke)
}
