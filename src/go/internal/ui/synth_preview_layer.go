package ui

import (
	"image"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// synth_preview_layer.go — right-half preview pane for the Synth tab.
// Three stacked plots (Phase 4 audio-panel redesign):
//
//   1. Osc       — one cycle of the current carrier wave shape so the
//                  kid sees "this is the source the knobs are
//                  modulating".
//   2. Envelope  — ADSR curve drawn from the recipe's attack / decay
//                  / sustain / release knobs.
//   3. Filter    — biquad magnitude response (sourced from the active
//                  EQ for the current channel).
//
// The pane is rendered to a sub-rect carved off the right of the
// content area in drawSynthTab. layoutSynthSections receives the
// reduced rowR so the knob cards never overlap the preview.

// synthPreviewWidth is the horizontal slice reserved for the right-
// half preview pane on desktop layouts. Returns 0 on mobile (no
// horizontal split — the preview disappears in favour of vertical
// knob stacks).
func synthPreviewWidth(contentR image.Rectangle, mobile bool) int {
	if mobile {
		return 0
	}
	const target = 280
	avail := contentR.Dx() / 3
	if avail > target {
		return target
	}
	if avail < 160 {
		return 0
	}
	return avail
}

// drawSynthPreviewPane paints the three-plot preview into rect. The
// caller (drawSynthTab) is responsible for choosing the rect — this
// function does not mutate any DrumView state.
//
// `instID` is the resolved instrument id used for the active-channel
// filter lookup. `decayMul` is the row's current decay knob value
// (1.0 = default, 0.5 = half, 2.0 = double) and drives the ADSR plot.
func drawSynthPreviewPane(dst *ebiten.Image, rect image.Rectangle, instID string, decayMul float64) {
	if rect.Dx() < 32 || rect.Dy() < 60 {
		return
	}
	drawRoundedRect(dst, rect, TokenSurface1(), 8, true)
	drawRoundedRect(dst, rect, TokenBorderSubtle(), 8, false)

	// Three equal-height plot rows with a small gap between.
	pad := SpaceSM
	captionScale := FontSizeCaption / FontSizeBody
	captionH := int(float64(TextHeight()) * captionScale)
	rowH := (rect.Dy() - pad*4 - captionH*3) / 3
	if rowH < 16 {
		return
	}

	y := rect.Min.Y + pad
	// 1) Osc
	DrawTextColorAtScale(dst, "OSC", rect.Min.X+pad, y, TokenTextSecondary(), captionScale)
	oscR := image.Rect(rect.Min.X+pad, y+captionH, rect.Max.X-pad, y+captionH+rowH)
	drawSynthOscPlot(dst, oscR, instID)
	dimPlotIfBypassed(dst, oscR, instID, "osc_enabled")
	y = oscR.Max.Y + pad

	// 2) Envelope (ADSR)
	DrawTextColorAtScale(dst, "ADSR", rect.Min.X+pad, y, TokenTextSecondary(), captionScale)
	envR := image.Rect(rect.Min.X+pad, y+captionH, rect.Max.X-pad, y+captionH+rowH)
	drawSynthADSRPlot(dst, envR, instID, decayMul)
	dimPlotIfBypassed(dst, envR, instID, "env_enabled")
	y = envR.Max.Y + pad

	// 3) Filter
	DrawTextColorAtScale(dst, "FILTER", rect.Min.X+pad, y, TokenTextSecondary(), captionScale)
	fltR := image.Rect(rect.Min.X+pad, y+captionH, rect.Max.X-pad, y+captionH+rowH)
	drawSynthFilterPlot(dst, fltR, instID)
	dimPlotIfBypassed(dst, fltR, instID, "filter_enabled")
}

// dimPlotIfBypassed paints a translucent scrim over a preview plot when the
// modular stage gated by paramName is disabled, so the preview visually
// matches the bypassed section card. No-op for non-modular instruments
// (synthStageEnabled returns true when the param is absent).
func dimPlotIfBypassed(dst *ebiten.Image, rect image.Rectangle, instID, paramName string) {
	if synthStageEnabled(instID, paramName) {
		return
	}
	drawRect(dst, rect, WithAlpha(TokenSurface1(), AlphaStrong), true)
}

// drawSynthOscPlot paints a 1-cycle preview of the carrier wave. The
// current implementation samples a sine wave at 256 points — most
// builtin recipes use sine-family wavetables, and a real wavetable
// preview path would require exposing the recipe's selected shape
// through audio.SynthParams. The sine baseline is still a useful
// "this is the seed shape your knobs are reshaping" cue.
func drawSynthOscPlot(dst *ebiten.Image, rect image.Rectangle, instID string) {
	if rect.Dx() < 8 || rect.Dy() < 6 {
		return
	}
	drawRect(dst, rect, WithAlpha(genColorVizScopeBg, AlphaOverlay), true)
	const samples = 256
	width := rect.Dx()
	if width > samples {
		width = samples
	}
	// Read the instrument's selected oscillator shape (modular voice) so the
	// preview reflects the chosen generator; non-modular recipes fall back to
	// the sine seed shape (oscType 0).
	oscType, fmRatio, fmDepth := synthOscParamsFor(instID)
	midY := rect.Min.Y + rect.Dy()/2
	halfH := float64(rect.Dy()) / 2.0
	col := WithAlpha(colWaveTrace, AlphaStrong)
	for c := 0; c < width-1; c++ {
		t0 := float64(c) / float64(width)
		t1 := float64(c+1) / float64(width)
		y0 := midY - int(synthOscSample(oscType, t0, fmRatio, fmDepth)*(halfH*0.85))
		y1 := midY - int(synthOscSample(oscType, t1, fmRatio, fmDepth)*(halfH*0.85))
		if y0 > y1 {
			y0, y1 = y1, y0
		}
		drawRect(dst, image.Rect(rect.Min.X+c, y0, rect.Min.X+c+1, y1+1), col, true)
	}
	// Centre line.
	drawRect(dst, image.Rect(rect.Min.X, midY, rect.Max.X, midY+1), WithAlpha(genColorBorder, AlphaSubtle), true)
}

// synthOscSample returns one sample of the previewed waveform at phase t∈[0,1)
// for the given oscillator type (0=sine 1=saw 2=square 3=triangle 4=FM). The
// FM case approximates a 2-operator phase-modulated carrier using the supplied
// modulator ratio + depth. Output is in [-1,1].
func synthOscSample(oscType int, t, fmRatio, fmDepth float64) float64 {
	switch oscType {
	case 1: // saw
		return 2*t - 1
	case 2: // square
		if t < 0.5 {
			return 1
		}
		return -1
	case 3: // triangle
		return 1 - 4*math.Abs(t-0.5)
	case 4: // FM (2-op phase modulation)
		return math.Sin(2*math.Pi*t + fmDepth*math.Sin(2*math.Pi*fmRatio*t))
	case 5, 6: // noise (white / pink) — visual scribble only
		// Deterministic per-phase hash so the preview shows a jagged noise
		// trace (consecutive phases decorrelate). Pink (6) is drawn a touch
		// quieter to hint at its softer high end.
		n := uint32(t*4096.0)*2654435761 + 0x9E3779B9
		n ^= n >> 15
		v := float64(int32(n)) / float64(int32(1)<<30) // ~[-2,2)
		if v > 1 {
			v = 1
		} else if v < -1 {
			v = -1
		}
		if oscType == 6 {
			v *= 0.6
		}
		return v
	default: // sine
		return math.Sin(2 * math.Pi * t)
	}
}

// synthOscParamsFor resolves the oscillator preview params for an instrument.
// For the modular voice it reads the merged osc_type / fm op-2 ratio+depth; for
// every other recipe it returns the sine default so the seed-shape cue stays.
func synthOscParamsFor(instID string) (oscType int, fmRatio, fmDepth float64) {
	if instID == "" {
		return 0, 1, 0
	}
	recipeID := audio.RecipeForInstrument(instID)
	if recipeID == "" {
		return 0, 1, 0
	}
	merged := audio.MergeRecipeDefaults(recipeID, audio.GetInstrumentParams(instID))
	if _, ok := merged["osc_type"]; !ok {
		return 0, 1, 0 // not a modular voice
	}
	oscType = int(merged["osc_type"] + 0.5)
	fmRatio = merged["fm_op2_ratio"]
	if fmRatio <= 0 {
		fmRatio = 1
	}
	fmDepth = merged["fm_op2_depth"]
	return oscType, fmRatio, fmDepth
}

// drawSynthADSRPlot paints a four-stage ADSR envelope. For the unified modular
// voice it reads the real amp_attack/decay/sustain/release params so the plot
// tracks every envelope stage the user edits. For the bespoke drum/FM recipes —
// which expose only a Decay knob — it falls back to the representative shape
// (fixed attack/release, decay scaled by decayMul, sustain held at 0.6).
func drawSynthADSRPlot(dst *ebiten.Image, rect image.Rectangle, instID string, decayMul float64) {
	if rect.Dx() < 8 || rect.Dy() < 6 {
		return
	}
	drawRect(dst, rect, WithAlpha(genColorVizScopeBg, AlphaOverlay), true)

	// Stage durations (relative) + sustain level. Modular voices supply the
	// real ADSR; other recipes use the decay-knob approximation.
	attack := 0.05
	if decayMul <= 0 {
		decayMul = 1.0
	}
	decay := 0.30 * decayMul
	sustainLen := 0.20
	release := 0.30
	sustainLevel := 0.6
	if a, d, s, r, ok := synthADSRParamsFor(instID); ok {
		// Seconds map directly to relative widths; a small constant sustain
		// hold keeps the plateau visible even when the held stage is short.
		attack, decay, release = a, d, r
		sustainLen = 0.25
		sustainLevel = s
	}
	total := attack + decay + sustainLen + release
	if total <= 0 {
		total = 1
	}
	xA := rect.Min.X + int(float64(rect.Dx())*attack/total)
	xD := rect.Min.X + int(float64(rect.Dx())*(attack+decay)/total)
	xS := rect.Min.X + int(float64(rect.Dx())*(attack+decay+sustainLen)/total)
	yTop := rect.Min.Y + 2
	yBot := rect.Max.Y - 2
	if sustainLevel < 0 {
		sustainLevel = 0
	} else if sustainLevel > 1 {
		sustainLevel = 1
	}
	ySustain := yBot - int(float64(yBot-yTop)*sustainLevel)

	colA := WithAlpha(colAccent, AlphaStrong)
	drawSynthADSRSegment(dst, rect.Min.X, yBot, xA, yTop, colA)
	drawSynthADSRSegment(dst, xA, yTop, xD, ySustain, colA)
	drawRect(dst, image.Rect(xD, ySustain, xS, ySustain+1), colA, true)
	drawSynthADSRSegment(dst, xS, ySustain, rect.Max.X, yBot, colA)
	drawRect(dst, image.Rect(rect.Min.X, yBot, rect.Max.X, yBot+1), WithAlpha(genColorBorder, AlphaSubtle), true)
}

// drawSynthADSRSegment draws a straight 1px line between two points
// inside the ADSR plot. Implemented as a series of 1×1 rects rather
// than calling out to ebiten/vector because the existing render
// pipeline uses drawRect exclusively.
func drawSynthADSRSegment(dst *ebiten.Image, x0, y0, x1, y1 int, col interface {
	RGBA() (r, g, b, a uint32)
}) {
	if x1 < x0 {
		x0, x1, y0, y1 = x1, x0, y1, y0
	}
	dx := x1 - x0
	dy := y1 - y0
	if dx == 0 {
		return
	}
	for x := 0; x <= dx; x++ {
		y := y0 + dy*x/dx
		drawRect(dst, image.Rect(x0+x, y, x0+x+1, y+1), col, true)
	}
}

// synthVoiceExposesParam reports whether recipeID declares a VISIBLE (non-hidden)
// ParamDef named paramName. Phase-8A appends the modular pipeline stage params
// (amp_*, filter_*, osc_*) to migrated drum/FM recipes in the HIDDEN group so the
// binding/persistence can drive them, but the Synth preview must treat those
// recipes as before (no full ADSR / no synth filter) until Phase 8B surfaces the
// unified stage layout. Only the modular voice exposes these as visible knobs.
func synthVoiceExposesParam(recipeID, paramName string) bool {
	if recipeID == "" {
		return false
	}
	reg, ok := audio.RecipeRegistrations()[recipeID]
	if !ok || reg == nil {
		return false
	}
	for _, d := range reg.Params {
		if d.Name == paramName {
			return d.Group != audio.SynthHiddenGroup
		}
	}
	return false
}

// synthADSRParamsFor resolves the modular voice's amp envelope params for an
// instrument. ok is false for non-modular recipes (which have no amp_* knobs),
// so the caller keeps the decay-knob approximation.
func synthADSRParamsFor(instID string) (a, d, s, r float64, ok bool) {
	if instID == "" {
		return 0, 0, 0, 0, false
	}
	recipeID := audio.RecipeForInstrument(instID)
	if recipeID == "" {
		return 0, 0, 0, 0, false
	}
	// Phase-8A: migrated drum/FM recipes now carry amp_* in their schema (HIDDEN
	// group, deferred to the Phase-8B unified Synth tab), but their amp ADSR stage
	// is OFF by default — the preview must still read "no full ADSR" for them.
	// Gate on a VISIBLE amp_attack knob, which only the modular voice exposes.
	if !synthVoiceExposesParam(recipeID, "amp_attack") {
		return 0, 0, 0, 0, false
	}
	merged := audio.MergeRecipeDefaults(recipeID, audio.GetInstrumentParams(instID))
	if _, has := merged["amp_attack"]; !has {
		return 0, 0, 0, 0, false
	}
	return merged["amp_attack"], merged["amp_decay"], merged["amp_sustain"], merged["amp_release"], true
}

// synthFilterParamsFor resolves the modular voice's filter stage for an
// instrument. ok is false for non-modular recipes.
func synthFilterParamsFor(instID string) (filterType int, cutoff, q float64, ok bool) {
	if instID == "" {
		return 0, 0, 0, false
	}
	recipeID := audio.RecipeForInstrument(instID)
	if recipeID == "" {
		return 0, 0, 0, false
	}
	// Phase-8A: gate on a VISIBLE filter_cutoff knob (modular voice only); migrated
	// drum/FM recipes carry filter_* in the HIDDEN group with the stage OFF by
	// default, so their preview stays on the channel-EQ fallback until Phase 8B.
	if !synthVoiceExposesParam(recipeID, "filter_cutoff") {
		return 0, 0, 0, false
	}
	merged := audio.MergeRecipeDefaults(recipeID, audio.GetInstrumentParams(instID))
	if _, has := merged["filter_cutoff"]; !has {
		return 0, 0, 0, false
	}
	return int(merged["filter_type"] + 0.5), merged["filter_cutoff"], merged["filter_resonance"], true
}

// drawSynthFilterPlot paints the magnitude response of the instrument's filter.
// For the modular voice it computes the actual synth biquad response from
// filter_type/cutoff/resonance (audio.ModularFilterResponse) so the curve
// reflects the voice's own filter; for other recipes it falls back to the
// channel EQ stack (audio.GetChannelEQResponse), consistent with the EQ tab.
func drawSynthFilterPlot(dst *ebiten.Image, rect image.Rectangle, instID string) {
	if rect.Dx() < 8 || rect.Dy() < 6 {
		return
	}
	drawRect(dst, rect, WithAlpha(genColorVizScopeBg, AlphaOverlay), true)
	var resp []audio.FreqResponsePoint
	if ft, cutoff, q, ok := synthFilterParamsFor(instID); ok {
		resp = audio.ModularFilterResponse(ft, cutoff, q, 48000, 96, 20, 22000)
	} else {
		ch := instID
		if ch == "" {
			ch = "main"
		}
		resp = audio.GetChannelEQResponse(ch, 96, 20, 22000)
	}
	midY := rect.Min.Y + rect.Dy()/2
	if len(resp) == 0 {
		drawRect(dst, image.Rect(rect.Min.X, midY, rect.Max.X, midY+1), WithAlpha(genColorBorder, AlphaSubtle), true)
		return
	}
	const dbSpan = 24.0
	half := float64(rect.Dy()) / 2.0
	col := WithAlpha(genColorVizCurve, AlphaStrong)
	const minHz = 20.0
	const maxHz = 22000.0
	logMin := math.Log10(minHz)
	logMax := math.Log10(maxHz)
	prevX, prevY := -1, midY
	for _, p := range resp {
		hz := p.FreqHz
		if hz < minHz || hz > maxHz {
			continue
		}
		frac := (math.Log10(hz) - logMin) / (logMax - logMin)
		x := rect.Min.X + int(frac*float64(rect.Dx()))
		db := p.GainDB
		if db > dbSpan {
			db = dbSpan
		} else if db < -dbSpan {
			db = -dbSpan
		}
		y := midY - int((db/dbSpan)*half)
		if prevX < 0 {
			prevX, prevY = x, y
			continue
		}
		y0, y1 := prevY, y
		if y0 > y1 {
			y0, y1 = y1, y0
		}
		drawRect(dst, image.Rect(prevX, y0, x+1, y1+1), col, true)
		prevX, prevY = x, y
	}
	drawRect(dst, image.Rect(rect.Min.X, midY, rect.Max.X, midY+1), WithAlpha(genColorBorder, AlphaSubtle), true)
}
