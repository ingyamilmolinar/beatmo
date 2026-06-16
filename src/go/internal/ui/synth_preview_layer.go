package ui

import (
	"image"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// synthMirrorDisplayCyclesMax caps how many cycles the "Your sound" trace shows
// on screen. The wave is rendered with more cycles (for clean auto-zoom math),
// but cramming them all in reads as a solid block — so the DISPLAY shows at most
// this many, generously spaced and legible.
const synthMirrorDisplayCyclesMax = 2

// splitSynthPreviewMirror carves the synth preview pane into a top mirror band
// (~55%) and a bottom seed-plots band (~45%). A 1px gap separates them so the
// two surfaces read as distinct. When the pane is too short to host both, the
// mirror band collapses to empty and the plots get the whole rect (drawSynthMirror
// no-ops on tiny rects).
func splitSynthPreviewMirror(r image.Rectangle) (mirror, plots image.Rectangle) {
	const minMirrorH = 36
	if r.Dy() < minMirrorH+minMirrorH {
		return image.Rectangle{}, r
	}
	mirrorH := r.Dy() * 55 / 100
	mid := r.Min.Y + mirrorH
	mirror = image.Rect(r.Min.X, r.Min.Y, r.Max.X, mid)
	plots = image.Rect(r.Min.X, mid+1, r.Max.X, r.Max.Y)
	return mirror, plots
}

// autoZoomCycles picks how many of a cycle-rendered wave's cycles to DISPLAY so
// the trace shows roughly a constant number of features (zero crossings). A busy,
// harmonic-rich wave (lots of crossings per cycle) zooms IN to fewer cycles so
// its shape reads; a smooth sine shows more. Result is clamped to
// [1, cyclesRendered].
func autoZoomCycles(wave []float64, cyclesRendered int) int {
	if cyclesRendered < 1 || len(wave) < 2 {
		return 1
	}
	cross := 0
	for i := 1; i < len(wave); i++ {
		if (wave[i-1] < 0) != (wave[i] < 0) {
			cross++
		}
	}
	perCycle := float64(cross) / float64(cyclesRendered)
	if perCycle < 0.5 {
		perCycle = 0.5
	}
	// Aim for ~8 crossings on screen — enough to show periodicity + shape detail
	// without smearing. (sine ≈4 cycles, a rich/FM wave ≈1.)
	const targetCrossings = 8.0
	n := int(math.Round(targetCrossings / perCycle))
	if n < 1 {
		n = 1
	}
	if n > cyclesRendered {
		n = cyclesRendered
	}
	return n
}

// autoZoomWindow returns the leading whole-cycle slice of a cycle-rendered wave
// chosen by autoZoomCycles — the zoomed-in view to draw.
func autoZoomWindow(wave []float64, cyclesRendered int) []float64 {
	if cyclesRendered < 1 || len(wave) == 0 {
		return wave
	}
	ptsPerCycle := len(wave) / cyclesRendered
	if ptsPerCycle < 1 {
		return wave
	}
	n := autoZoomCycles(wave, cyclesRendered) * ptsPerCycle
	if n < 1 || n > len(wave) {
		n = len(wave)
	}
	return wave[:n]
}

// mirrorDisplayWindow returns the leading slice of a cycle-rendered wave to
// DISPLAY in the "Your sound" trace: the auto-zoom window, but never more than
// synthMirrorDisplayCyclesMax cycles, so the trace stays breathable.
func mirrorDisplayWindow(wave []float64, cyclesRendered int) []float64 {
	if cyclesRendered < 1 || len(wave) == 0 {
		return wave
	}
	ptsPerCycle := len(wave) / cyclesRendered
	if ptsPerCycle < 1 {
		return wave
	}
	cycles := autoZoomCycles(wave, cyclesRendered)
	if cycles > synthMirrorDisplayCyclesMax {
		cycles = synthMirrorDisplayCyclesMax
	}
	n := cycles * ptsPerCycle
	if n < 1 || n > len(wave) {
		n = len(wave)
	}
	return wave[:n]
}

// mirrorTraceGain returns a vertical scale so the wave's peak reaches ~80% of
// the band half-height, making the SHAPE legible whatever the level. Clamped so
// a near-silent buffer isn't amplified into noise and a hot one isn't squashed.
func mirrorTraceGain(pcm []float64) float64 {
	peak := 0.0
	for _, v := range pcm {
		if a := math.Abs(v); a > peak {
			peak = a
		}
	}
	if peak < 1e-3 {
		return 1.0
	}
	g := 0.8 / peak
	if g < 0.3 {
		g = 0.3
	}
	if g > 12 {
		g = 12
	}
	return g
}

// drawSynthMirror paints the live final-output mirror: a faint ghost trace of
// the previous render under the current rendered note, with clipped columns
// flagged in red. Renders nothing (beyond chrome) until the mirror has PCM.
func drawSynthMirror(dst *ebiten.Image, rect image.Rectangle, mirror *synthMirror) {
	if rect.Dx() < 32 || rect.Dy() < 24 {
		return
	}
	drawRoundedRect(dst, rect, TokenSurface1(), RadiusSM, true)
	drawRoundedRect(dst, rect, TokenBorderSubtle(), RadiusSM, false)

	pad := SpaceSM
	captionScale := FontSizeCaption / FontSizeBody
	captionH := int(float64(TextHeight()) * captionScale)
	DrawTextColorAtScale(dst, i18n.T(i18n.KeyCapYourSound), rect.Min.X+pad, rect.Min.Y+pad/2, TokenTextSecondary(), captionScale)

	traceRect := image.Rect(rect.Min.X+pad, rect.Min.Y+pad+captionH, rect.Max.X-pad, rect.Max.Y-pad)
	if traceRect.Dx() < 8 || traceRect.Dy() < 6 {
		return
	}
	drawRect(dst, traceRect, WithAlpha(genColorVizScopeBg, AlphaOverlay), true)

	if mirror == nil {
		return
	}
	pcm, ghost := mirror.snapshot()
	// Auto-zoom: show only as many cycles as keep the shape legible (a busy wave
	// zooms in, a simple one shows more), so the trace never smears into a dense
	// block. Ghost is sliced to the SAME window so the two align.
	zoomLen := len(mirrorDisplayWindow(pcm, synthMirrorWaveCycles))
	if zoomLen > 0 && zoomLen <= len(pcm) {
		pcm = pcm[:zoomLen]
	}
	if zoomLen > 0 && zoomLen <= len(ghost) {
		ghost = ghost[:zoomLen]
	}
	midY := traceRect.Min.Y + traceRect.Dy()/2
	width := traceRect.Dx()
	// Normalize so the wave SHAPE fills the band whatever its level — a quiet
	// patch is still legible, and the gain/drive knobs still visibly change the
	// trace because they change the shape, not just the height.
	yGain := mirrorTraceGain(pcm)

	// Ghost (previous render) first, faint, so the live trace paints over it.
	if len(ghost) > 0 {
		drawWaveTrace(dst, ghost, traceRect, midY, width, WithAlpha(genColorBorder, AlphaSubtle), yGain, nil)
	}
	if len(pcm) > 0 {
		drawWaveTrace(dst, pcm, traceRect, midY, width, colWaveTrace, yGain, colSynthOscFill)
	}
	// Centre line.
	drawRect(dst, image.Rect(traceRect.Min.X, midY, traceRect.Max.X, midY+1), WithAlpha(genColorBorder, AlphaSubtle), true)
}

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

// Synthwave "outrun" fill colors for the Synth preview mini-graphs: a soft
// wash beneath each curve at AlphaSubtle. OSC fills to the centerline
// (colWaveTrace family, matching the wave trace); the ADSR envelope fills to
// its baseline (colAccent family, matching its line). Fixed package vars so
// drawRect serves them from pixelCache without per-column allocation.
var (
	colSynthOscFill = WithAlpha(colWaveTrace, AlphaSubtle)
	colSynthEnvFill = WithAlpha(colAccent, AlphaSubtle)
)

// synthPreviewWidth is the horizontal slice reserved for the right-
// half preview pane on desktop layouts. Returns 0 on mobile (no
// horizontal split — the preview disappears in favour of vertical
// knob stacks).
func synthPreviewWidth(contentR image.Rectangle, mobile bool) int {
	if mobile {
		return 0
	}
	target := Profile().DensityValues().SynthPreviewTargetW
	avail := contentR.Dx() / 3
	if avail > target {
		return target
	}
	if avail < Profile().DensityValues().SynthPreviewMinW {
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
	drawRoundedRect(dst, rect, TokenSurface1(), RadiusSM, true)
	drawRoundedRect(dst, rect, TokenBorderSubtle(), RadiusSM, false)

	pad := SpaceSM
	captionScale := FontSizeCaption / FontSizeBody
	captionH := int(float64(TextHeight()) * captionScale)

	// Reserve the bottom OUT strip (Delay/Reverb/Vol mini-meters) so the plots
	// never bleed into it. drawSynthOutRow renders it when paneR.Dy() >= 80;
	// match that threshold + its rowH + insets so the curves stop above it.
	const outRowH = 20
	plotAreaBottom := rect.Max.Y - pad
	if rect.Dy() >= 80 {
		plotAreaBottom = rect.Max.Y - outRowH - 2*SpaceSM
	}

	// Each plot gets a LABEL ROW above it (so the section label never paints on
	// the graph) plus a graph band. minGraphH is the floor below which a plot
	// is illegible — when the pane is too short we DROP the FILTER plot rather
	// than squeeze all three into 2px slivers.
	const minGraphH = 18
	availH := plotAreaBottom - (rect.Min.Y + pad)

	// Try three plots; if that starves the graphs, fall back to two (drop
	// FILTER — the least load-bearing seed cue).
	nPlots := 3
	cellH := availH / nPlots // label row + graph + gap, per plot
	if cellH-captionH-pad < minGraphH {
		nPlots = 2
		cellH = availH / nPlots
	}
	graphH := cellH - captionH - pad
	if graphH < minGraphH {
		// Even two won't fit a legible graph: clamp graphH to whatever's left
		// so we still draw something rather than returning blank.
		graphH = availH/nPlots - captionH - pad
		if graphH < 1 {
			return
		}
	}

	left := rect.Min.X + pad
	right := rect.Max.X - pad
	y := rect.Min.Y + pad

	drawPlot := func(label string, draw func(r image.Rectangle), enableParam string) {
		DrawTextColorAtScale(dst, label, left, y, TokenTextSecondary(), captionScale)
		// The graph band sits BELOW the label row and is bounded by graphH, so
		// each curve (which is computed relative to its rect) is clipped to its
		// own band — it can never paint on the label or bleed into the OUT
		// strip below, the two bugs this layout fixes.
		gr := image.Rect(left, y+captionH, right, y+captionH+graphH)
		draw(gr)
		dimPlotIfBypassed(dst, gr, instID, enableParam)
		y = gr.Max.Y + pad
	}

	drawPlot("OSC", func(r image.Rectangle) { drawSynthOscPlot(dst, r, instID) }, "osc_enabled")
	drawPlot("ADSR", func(r image.Rectangle) { drawSynthADSRPlot(dst, r, instID, decayMul) }, "env_enabled")
	if nPlots >= 3 {
		drawPlot("FILTER", func(r image.Rectangle) { drawSynthFilterPlot(dst, r, instID) }, "filter_enabled")
	}
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
		// Synthwave "outrun" fill: one soft wash rect per column from the
		// trace down to the centerline (the "energy over time" read). One
		// rect/column keeps the per-column alloc cost (drawRect boxes its
		// color) at +1 — the fixed fill color is served from pixelCache.
		// Clipped to this graph's rect.
		px := rect.Min.X + c
		if y0 < midY {
			drawRect(dst, image.Rect(px, y0, px+1, midY), colSynthOscFill, true)
		} else if y1 > midY {
			drawRect(dst, image.Rect(px, midY, px+1, y1), colSynthOscFill, true)
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
	// amp_curve (0=linear ramps, 1=exponential approach) bends the attack/decay/
	// release ramps so the curve flag visibly changes the picture, mirroring the
	// engine's linear-vs-exponential ADSR (src/c/adsr.h). The modular voice reads
	// it directly; non-modular recipes (no amp_curve knob) default to linear.
	curve := 0.0
	if synthVoiceExposesParam(audio.RecipeForInstrument(instID), "amp_curve") {
		curve = conceptMergedParams(instID)["amp_curve"]
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
	// Synthwave "outrun" fill: each envelope segment fills from its line down
	// to the baseline (yBot) so the curve reads as "energy over time". Two
	// fixed bands (subtle near the curve, faint near the base) keep it
	// alloc-cheap; the fill is clipped to this graph's rect by construction.
	// The three RAMPS (attack/decay/release) bend with amp_curve; the sustain
	// plateau is flat.
	drawSynthADSRRamp(dst, rect.Min.X, yBot, xA, yTop, colA, yBot, curve)
	drawSynthADSRRamp(dst, xA, yTop, xD, ySustain, colA, yBot, curve)
	// Sustain plateau: fill the held level down to the baseline.
	if xS > xD {
		drawSynthADSRFillColumn(dst, xD, xS, ySustain, yBot)
	}
	drawRect(dst, image.Rect(xD, ySustain, xS, ySustain+1), colA, true)
	drawSynthADSRRamp(dst, xS, ySustain, rect.Max.X, yBot, colA, yBot, curve)
	drawRect(dst, image.Rect(rect.Min.X, yBot, rect.Max.X, yBot+1), WithAlpha(genColorBorder, AlphaSubtle), true)
}

// adsrCurveShape maps a normalized segment progress t∈[0,1] through the
// linear↔exponential blend selected by curve∈[0,1]. At curve=0 it is the
// identity (a straight ramp); at curve=1 it is the asymptotic exponential
// approach v(t) = (1-exp(-k·t)) / (1-exp(-k)) that mirrors the engine's
// exponential ADSR (value += rate·(target-value)) — fast start, slow finish.
// Intermediate curve values blend linearly between the two so the picture
// morphs continuously as the knob is dragged.
func adsrCurveShape(t, curve float64) float64 {
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	if curve <= 0 {
		return t
	}
	const k = 4.0 // approach sharpness; matches a clearly-concave exponential ramp.
	expShape := (1 - math.Exp(-k*t)) / (1 - math.Exp(-k))
	if curve >= 1 {
		return expShape
	}
	return t + curve*(expShape-t)
}

// drawSynthADSRRamp draws one ADSR ramp from (x0,y0) to (x1,y1), bending the
// vertical interpolation by amp_curve via adsrCurveShape. At curve=0 it is
// identical to drawSynthADSRSegment (a straight line). Each column also gets the
// synthwave fill from the curve down to fillBaseline.
func drawSynthADSRRamp(dst *ebiten.Image, x0, y0, x1, y1 int, col interface {
	RGBA() (r, g, b, a uint32)
}, fillBaseline int, curve float64) {
	if curve <= 0 {
		drawSynthADSRSegment(dst, x0, y0, x1, y1, col, fillBaseline)
		return
	}
	if x1 < x0 {
		x0, x1, y0, y1 = x1, x0, y1, y0
	}
	dx := x1 - x0
	dy := y1 - y0
	if dx == 0 {
		return
	}
	for x := 0; x <= dx; x++ {
		t := float64(x) / float64(dx)
		y := y0 + int(math.Round(adsrCurveShape(t, curve)*float64(dy)))
		if fillBaseline >= 0 && y < fillBaseline {
			drawSynthADSRFillColumn(dst, x0+x, x0+x+1, y, fillBaseline)
		}
		drawRect(dst, image.Rect(x0+x, y, x0+x+1, y+1), col, true)
	}
}

// drawSynthADSRFillColumn paints the soft synthwave wash from yTop down to
// yBot across [x0, x1). One rect (fixed package color, served from pixelCache)
// keeps the per-column alloc cost at +1 in the per-pixel segment path.
func drawSynthADSRFillColumn(dst *ebiten.Image, x0, x1, yTop, yBot int) {
	if yTop >= yBot || x1 <= x0 {
		return
	}
	drawRect(dst, image.Rect(x0, yTop, x1, yBot), colSynthEnvFill, true)
}

// drawSynthADSRSegment draws a straight 1px line between two points
// inside the ADSR plot. Implemented as a series of 1×1 rects rather
// than calling out to ebiten/vector because the existing render
// pipeline uses drawRect exclusively. When fillBaseline >= 0 each
// column also gets the two-band synthwave fill from the line down to
// fillBaseline (drawSynthADSRFillColumn).
func drawSynthADSRSegment(dst *ebiten.Image, x0, y0, x1, y1 int, col interface {
	RGBA() (r, g, b, a uint32)
}, fillBaseline int) {
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
		if fillBaseline >= 0 && y < fillBaseline {
			drawSynthADSRFillColumn(dst, x0+x, x0+x+1, y, fillBaseline)
		}
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
