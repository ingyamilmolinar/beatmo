package ui

import (
	"image"
	"math"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// synth_concept_viz.go — the concept-renderer registry.
//
// Each synth param GROUP maps to one renderer that draws an abstract picture
// of "what this knob does": the OSC group shows the seed wave, FILTER shows the
// magnitude response, ENV shows the ADSR, FM shows harmonic bars, the motion
// stages (pitch / lfo / burst) show a modulation curve, and POST shows the
// waveshaper transfer. All renderers are PURE MATH (no audio render), so they
// update live while a knob is dragged.
//
// instID resolves live params via audio.MergeRecipeDefaults. ghost is the
// pre-drag param snapshot (nil = no ghost): renderers that support it draw a
// faint "before" overlay so the user sees the delta their drag is making.

// conceptRenderer draws the abstract picture for a param group into rect. def is
// the ParamDef of the specific knob the picture sits under (group renderers
// ignore it; the per-knob value-bar uses it to read that knob's own value).
type conceptRenderer func(dst *ebiten.Image, rect image.Rectangle, instID string, def audio.ParamDef, ghost map[string]float64)

// conceptRenderers maps every param GROUP that the recipe registry actually uses
// to its concept renderer. It is the SINGLE source of truth; the discipline test
// TestEveryRecipeGroupHasExplicitConceptRenderer enumerates every live
// ParamDef.Group and asserts each appears here, so a recipe that introduces a new
// group can't silently inherit the wrong picture via the fallback below.
//
// The grab-bag groups (core / generic / voice) hold heterogeneous knobs
// (volume, pan, tune, …) with no single coherent shape, so they draw a per-knob
// value bar rather than a misleading oscillator wave.
var conceptRenderers = map[string]conceptRenderer{
	// Timbre knobs render the ACTUAL resulting wave (conceptKnobWave) so the
	// picture literally shows "this is the signal, and this is how the knob
	// bends it". Time-domain stages (envelope, modulation) keep their dedicated
	// time-curve pictures — those ARE their wave (amplitude / pitch over time).
	"osc":      conceptKnobWave,
	"filter":   conceptKnobWave,
	"env":      conceptEnvelope,
	"fm":       conceptKnobWave,
	"post":     conceptKnobWave,
	"pitch":    conceptMotion,
	"pitchenv": conceptMotion,
	"lfo":      conceptMotion,
	"burst":    conceptMotion,
	"core":     conceptKnobWave,
	"generic":  conceptKnobWave,
	"voice":    conceptKnobWave,
	"":         conceptKnobWave,
}

// conceptStageEnableParam returns the *_enabled param that must be ON for a
// group's effect to show in the rendered wave, so a knob's picture demonstrates
// its effect even when the stage is currently bypassed (the picture teaches what
// the knob DOES, independent of the live on/off state).
func conceptStageEnableParam(group string) (string, bool) {
	switch group {
	case "filter":
		return "filter_enabled", true
	case "fm":
		return "fm_enabled", true
	case "post":
		return "drive_enabled", true
	}
	return "", false
}

// conceptKnobWave renders the instrument's actual output wave (a few clean
// cycles) so each knob's picture shows the resulting signal. A faint ghost draws
// the wave with THIS knob pulled to a contrasting reference (its min, or sine
// for an oscillator-type enum), so the morph from ghost→solid reads as "this is
// how this knob bends the wave". The stage gate is forced on for both renders so
// the effect is visible even when the stage is currently bypassed.
func conceptKnobWave(dst *ebiten.Image, rect image.Rectangle, instID string, def audio.ParamDef, _ map[string]float64) {
	if rect.Dx() < 8 || rect.Dy() < 6 {
		return
	}
	conceptBgFill(dst, rect)
	const cycles, pts = 3, 64

	midY := (rect.Min.Y + rect.Max.Y) / 2
	half := float64(rect.Dy()-2) / 2
	yAt := func(v float64) int {
		if v > 1 {
			v = 1
		} else if v < -1 {
			v = -1
		}
		return int(math.Round(float64(midY) - v*half))
	}

	ref := def.Min
	if len(def.Enum) > 0 {
		ref = 0 // contrast an oscillator/type enum against its first shape
	}
	solid := map[string]float64{}
	ghost := map[string]float64{def.Name: ref}
	if en, ok := conceptStageEnableParam(def.Group); ok {
		solid[en] = 1
		ghost[en] = 1
	}
	cw := audio.RenderInstrumentPreviewWave(instID, solid, cycles, pts)
	gw := audio.RenderInstrumentPreviewWave(instID, ghost, cycles, pts)
	// Auto-zoom to keep the shape legible: a busy wave shows fewer cycles. Both
	// curves use the SAME window (computed from the live wave) so they align.
	zoomLen := len(autoZoomWindow(cw, cycles))
	if zoomLen > 0 && zoomLen <= len(cw) {
		cw = cw[:zoomLen]
	}
	if zoomLen > 0 && zoomLen <= len(gw) {
		gw = gw[:zoomLen]
	}
	if len(gw) > 0 && !sameWave(gw, cw) {
		conceptDrawFilledCurve(dst, rect, gw, yAt, midY, nil, colConceptGhost)
	}
	conceptDrawFilledCurve(dst, rect, cw, yAt, midY, colConceptFill, colConceptStroke)
}

func sameWave(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// conceptRendererForGroup is TOTAL: every group resolves to a non-nil renderer.
// Known groups come from conceptRenderers; any unexpected group falls back to the
// honest per-knob value bar (NOT the oscillator wave, which would mislead for a
// non-oscillator knob). The discipline test keeps the fallback from silently
// absorbing a real new group.
func conceptRendererForGroup(group string) conceptRenderer {
	if r, ok := conceptRenderers[group]; ok {
		return r
	}
	return conceptValueBar
}

// conceptMergedParams resolves the instrument's effective params (shipped
// defaults ⊕ user edits). Never nil.
func conceptMergedParams(instID string) audio.RecipeParams {
	return audio.MergeRecipeDefaults(audio.RecipeForInstrument(instID), audio.GetInstrumentParams(instID))
}

// conceptBgFill paints the standard scope-bg wash used by every concept plot,
// matching the convention in drawSynthOscPlot.
func conceptBgFill(dst *ebiten.Image, rect image.Rectangle) {
	drawRect(dst, rect, WithAlpha(genColorVizScopeBg, AlphaOverlay), true)
}

// concept fill/stroke colors. The fill is a soft synthwave wash beneath the
// curve (matching colSynthOscFill convention); the stroke is the bright curve
// itself drawn 2px tall so the shape reads in a short band. Fixed package vars
// so drawRect serves them from the pixel cache without per-column allocation.
var (
	colConceptFill   = WithAlpha(colWaveTrace, AlphaSubtle)
	colConceptStroke = WithAlpha(colWaveTrace, AlphaStrong)
	// colConceptGhost is the faint "before you grabbed it" overlay stroke used by
	// every ghost-aware renderer.
	colConceptGhost = WithAlpha(genColorBorder, AlphaSubtle)
)

// conceptStrokeThickness is the curve stroke height in pixels — thick enough to
// read in a short concept band.
const conceptStrokeThickness = 2

// conceptValueBarH is the height of the per-knob value-bar fill band.
const conceptValueBarH = 6

// conceptDrawFilledCurve plots samples[] (already mapped to a y-pixel via yAt)
// across rect as a per-column FILL down to fillBaseline plus a 2px-tall stroke
// at the curve. One fill rect + one stroke rect per column — allocation-cheap.
// strokeCol may be nil to draw fill only (used for the faint ghost overlay).
func conceptDrawFilledCurve(dst *ebiten.Image, rect image.Rectangle, samples []float64, yAt func(float64) int, fillBaseline int, fillCol, strokeCol interface {
	RGBA() (r, g, b, a uint32)
}) {
	n := len(samples)
	if n < 2 {
		return
	}
	w := rect.Dx()
	for px := 0; px < w; px++ {
		x := rect.Min.X + px
		// Map column to a sample index.
		idx := px * (n - 1) / (w - 1)
		if idx >= n {
			idx = n - 1
		}
		y := yAt(samples[idx])
		// Soft fill from the curve to the baseline.
		if fillCol != nil {
			yTop, yBot := y, fillBaseline
			if yTop > yBot {
				yTop, yBot = yBot, yTop
			}
			if yBot > yTop {
				drawRect(dst, image.Rect(x, yTop, x+1, yBot), fillCol, true)
			}
		}
		// 2px-tall stroke at the curve.
		if strokeCol != nil {
			sy := y
			if sy < rect.Min.Y {
				sy = rect.Min.Y
			}
			if sy+conceptStrokeThickness > rect.Max.Y {
				sy = rect.Max.Y - conceptStrokeThickness
			}
			drawRect(dst, image.Rect(x, sy, x+1, sy+conceptStrokeThickness), strokeCol, true)
		}
	}
}

// ── delegating renderers (reuse the existing preview plots) ────────────────

func conceptOsc(dst *ebiten.Image, r image.Rectangle, instID string, _ audio.ParamDef, ghost map[string]float64) {
	drawSynthOscPlot(dst, r, instID)
	// Ghost overlay: faint "before" wave when the pre-drag osc params differ.
	gt, gfr, gfd, ok := oscParamsFromMap(ghost)
	if !ok {
		return
	}
	lt, lfr, lfd := synthOscParamsFor(instID)
	if gt == lt && gfr == lfr && gfd == lfd {
		return
	}
	w := r.Dx()
	midY := float64(r.Min.Y) + float64(r.Dy())/2
	amp := float64(r.Dy()) / 2 * 0.85
	yAt := func(v float64) int { return int(midY - v*amp) }
	samples := make([]float64, w)
	for c := range samples {
		samples[c] = synthOscSample(gt, float64(c)/float64(w), gfr, gfd)
	}
	conceptDrawFilledCurve(dst, r, samples, yAt, int(midY), nil, colConceptGhost)
}

func conceptEnvelope(dst *ebiten.Image, r image.Rectangle, instID string, _ audio.ParamDef, ghost map[string]float64) {
	drawSynthADSRPlot(dst, r, instID, 1.0)
	// Ghost overlay: faint "before" ADSR when the pre-drag env params differ.
	ga, gd, gs, grel, ok := adsrParamsFromMap(ghost)
	if !ok {
		return
	}
	la, ld, ls, lrel, lok := synthADSRParamsFor(instID)
	if lok && ga == la && gd == ld && gs == ls && grel == lrel {
		return
	}
	w := r.Dx()
	yTop := r.Min.Y + 2
	yBot := r.Max.Y - 2
	yAt := func(v float64) int { return yBot - int(float64(yBot-yTop)*v) }
	conceptDrawFilledCurve(dst, r, adsrGhostSamples(ga, gd, gs, grel, w), yAt, yBot, nil, colConceptGhost)
}

// oscParamsFromMap reads the oscillator-shape params from a snapshot map (ok is
// false when the map has no osc_type, i.e. not a modular voice snapshot).
func oscParamsFromMap(m map[string]float64) (oscType int, fmRatio, fmDepth float64, ok bool) {
	if m == nil {
		return 0, 1, 0, false
	}
	tv, has := m["osc_type"]
	if !has {
		return 0, 1, 0, false
	}
	fmRatio = m["fm_op2_ratio"]
	if fmRatio <= 0 {
		fmRatio = 1
	}
	return int(tv + 0.5), fmRatio, m["fm_op2_depth"], true
}

// adsrParamsFromMap reads the amp-envelope params from a snapshot map (ok is
// false when the map has no amp_attack, matching synthADSRParamsFor's gate).
func adsrParamsFromMap(m map[string]float64) (a, d, s, rel float64, ok bool) {
	if m == nil {
		return 0, 0, 0, 0, false
	}
	if _, has := m["amp_attack"]; !has {
		return 0, 0, 0, 0, false
	}
	return m["amp_attack"], m["amp_decay"], m["amp_sustain"], m["amp_release"], true
}

// adsrGhostSamples returns w samples of the ADSR level (0..1) over the same
// relative time layout drawSynthADSRPlot uses (attack/decay/0.25 sustain/release).
func adsrGhostSamples(a, d, s, rel float64, w int) []float64 {
	if w < 2 {
		w = 2
	}
	const sustainLen = 0.25
	total := a + d + sustainLen + rel
	if total <= 0 {
		total = 1
	}
	if s < 0 {
		s = 0
	} else if s > 1 {
		s = 1
	}
	out := make([]float64, w)
	for c := 0; c < w; c++ {
		tt := float64(c) / float64(w-1) * total
		var v float64
		switch {
		case tt < a && a > 0:
			v = tt / a
		case tt < a+d && d > 0:
			v = 1 - (1-s)*(tt-a)/d
		case tt < a+d+sustainLen:
			v = s
		case rel > 0:
			v = s * (1 - (tt-(a+d+sustainLen))/rel)
		}
		if v < 0 {
			v = 0
		}
		out[c] = v
	}
	return out
}

// conceptValueBar is implemented in the value-bar section below (Round 2). It is
// the renderer for the heterogeneous core / generic / voice groups.

// conceptFilter draws the filter magnitude response as a FILLED area down to
// the band baseline plus a 2px stroke, so the curve reads clearly in a short
// concept band (the shared drawSynthFilterPlot used in the preview pane is a
// 1px line and is left untouched for that larger surface).
func conceptFilter(dst *ebiten.Image, rect image.Rectangle, instID string, _ audio.ParamDef, ghost map[string]float64) {
	if rect.Dx() < 8 || rect.Dy() < 6 {
		return
	}
	conceptBgFill(dst, rect)

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
	if len(resp) == 0 {
		return
	}

	const (
		dbSpan = 24.0
		minHz  = 20.0
		maxHz  = 22000.0
	)
	logMin := math.Log10(minHz)
	logMax := math.Log10(maxHz)
	midY := float64(rect.Min.Y+rect.Max.Y) / 2
	half := float64(rect.Dy()-2) / 2
	yAt := func(db float64) int {
		if db > dbSpan {
			db = dbSpan
		} else if db < -dbSpan {
			db = -dbSpan
		}
		return int(math.Round(midY - (db/dbSpan)*half))
	}

	// Resample the log-spaced response onto one value per pixel column so the
	// shared per-column fill helper can draw a clean shape.
	w := rect.Dx()
	resample := func(rp []audio.FreqResponsePoint) []float64 {
		s := make([]float64, w)
		for px := 0; px < w; px++ {
			frac := float64(px) / float64(w-1)
			hz := math.Pow(10, logMin+frac*(logMax-logMin))
			// Nearest response point.
			best := rp[0].GainDB
			bestD := math.Inf(1)
			for _, p := range rp {
				d := math.Abs(p.FreqHz - hz)
				if d < bestD {
					bestD, best = d, p.GainDB
				}
			}
			s[px] = best
		}
		return s
	}
	conceptDrawFilledCurve(dst, rect, resample(resp), yAt, rect.Max.Y-1, colConceptFill, colConceptStroke)

	// Ghost overlay: faint "before" response when the pre-drag filter params
	// differ (only meaningful for the modular voice, which exposes filter_*).
	if gft, gc, gq, ok := filterParamsFromMap(ghost); ok {
		lft, lc, lq, lok := synthFilterParamsFor(instID)
		if !lok || gft != lft || gc != lc || gq != lq {
			if gresp := audio.ModularFilterResponse(gft, gc, gq, 48000, 96, 20, 22000); len(gresp) > 0 {
				conceptDrawFilledCurve(dst, rect, resample(gresp), yAt, rect.Max.Y-1, nil, colConceptGhost)
			}
		}
	}
}

// filterParamsFromMap reads the modular filter params from a snapshot map (ok is
// false when the map has no filter_cutoff).
func filterParamsFromMap(m map[string]float64) (ft int, cutoff, q float64, ok bool) {
	if m == nil {
		return 0, 0, 0, false
	}
	if _, has := m["filter_cutoff"]; !has {
		return 0, 0, 0, false
	}
	return int(m["filter_type"] + 0.5), m["filter_cutoff"], m["filter_resonance"], true
}

// ── conceptFM — FM output waveform with operators forced audible ────────────

// fmForceActiveOverride builds an override that forces the FM stage + its
// operators into an audible state, so the selected FM knob's effect is visible
// in the rendered output even from a default/inert voice (where every op depth
// is 0 ⇒ a plain carrier). It turns fm_enabled on and sets every FM DEPTH param
// the recipe actually exposes to ~0.4·Max (a moderate, clearly-audible
// modulation index). `exclude` is the SELECTED knob — it is left OUT of the
// override so it renders at its LIVE value and its own effect isn't masked by
// the forcing.
func fmForceActiveOverride(recipeID, exclude string) map[string]float64 {
	ov := map[string]float64{}
	reg := audio.RecipeRegistrations()[recipeID]
	if reg == nil {
		// No registration (e.g. unbound id): still force the toggle + a single
		// modulator so the picture isn't a flat carrier.
		if exclude != "fm_enabled" {
			ov["fm_enabled"] = 1
		}
		if exclude != "fm_op2_depth" {
			ov["fm_op2_depth"] = 3
		}
		return ov
	}
	if exclude != "fm_enabled" {
		// Only force the toggle when the recipe has it; bespoke recipes gate on
		// depth instead and adding a stray key is harmless (preview ignores it).
		ov["fm_enabled"] = 1
	}
	for _, d := range reg.Params {
		if d.Group != "fm" || d.Name == exclude {
			continue
		}
		if !isFMDepthParam(d.Name) {
			continue
		}
		force := 0.4 * d.Max
		if force <= 0 {
			force = 3 // defensive: a depth knob with a non-positive Max
		}
		ov[d.Name] = force
	}
	return ov
}

// isFMDepthParam reports whether name is a per-operator FM DEPTH (modulation
// index) param across both the modular (fm_opN_depth) and bespoke recipe
// schemes. Depth is what makes the FM timbre audible, so these are the knobs
// the force-active override drives up.
func isFMDepthParam(name string) bool {
	switch name {
	case "fm_op1_depth", "fm_op2_depth", "fm_op3_depth", "fm_op4_depth":
		return true
	}
	return false
}

// conceptFM renders the instrument's actual FM OUTPUT WAVEFORM with the FM stage
// + every operator forced into an audible state (minus the selected knob), so
// turning ANY FM knob — ratio, level, algorithm, depth — visibly bends the
// picture. A faint depth-bar underlay keeps a per-operator "how much FM" cue,
// and a ghost waveform shows the pre-drag value of the selected knob.
func conceptFM(dst *ebiten.Image, rect image.Rectangle, instID string, def audio.ParamDef, ghost map[string]float64) {
	if rect.Dx() < 8 || rect.Dy() < 6 {
		return
	}
	conceptBgFill(dst, rect)

	recipeID := audio.RecipeForInstrument(instID)

	// Faint depth-bar underlay (the original per-operator "how rich" cue), drawn
	// first so the waveform reads on top.
	conceptFMDepthUnderlay(dst, rect, conceptMergedParams(instID))

	const cycles, pts = 3, 64
	midY := (rect.Min.Y + rect.Max.Y) / 2
	half := float64(rect.Dy()-2) / 2
	yAt := func(v float64) int {
		if v > 1 {
			v = 1
		} else if v < -1 {
			v = -1
		}
		return int(math.Round(float64(midY) - v*half))
	}

	// Force the FM stage audible, leaving the SELECTED knob at its live value so
	// its own effect is the thing that moves.
	base := fmForceActiveOverride(recipeID, def.Name)
	cw := audio.RenderInstrumentPreviewWave(instID, base, cycles, pts)

	// Ghost: same forced context but with the selected knob pinned to its
	// pre-drag value, so the morph ghost→solid reads as "this is what this knob
	// does". Override map (not live mutation): RenderInstrumentPreviewWave takes
	// overrides directly.
	var gw []float64
	if def.Name != "" && ghost != nil {
		if gv, ok := ghost[def.Name]; ok {
			gov := make(map[string]float64, len(base)+1)
			for k, v := range base {
				gov[k] = v
			}
			gov[def.Name] = gv
			gw = audio.RenderInstrumentPreviewWave(instID, gov, cycles, pts)
		}
	}

	// Auto-zoom both curves to the SAME window (from the live wave) so they align.
	zoomLen := len(autoZoomWindow(cw, cycles))
	if zoomLen > 0 && zoomLen <= len(cw) {
		cw = cw[:zoomLen]
	}
	if zoomLen > 0 && zoomLen <= len(gw) {
		gw = gw[:zoomLen]
	}

	if len(gw) > 0 && !sameWave(gw, cw) {
		conceptDrawFilledCurve(dst, rect, gw, yAt, midY, nil, colConceptGhost)
	}
	conceptDrawFilledCurve(dst, rect, cw, yAt, midY, colConceptFill, colConceptStroke)
}

// conceptFMDepthUnderlay paints the faint per-operator depth bars beneath the
// waveform, preserving the original "how much FM per operator" glance cue.
func conceptFMDepthUnderlay(dst *ebiten.Image, rect image.Rectangle, merged map[string]float64) {
	depths := fmDepths(merged)
	// modular fm depth Max is 8 (modular_recipe.go); normalize against it.
	const depthMax = 8.0
	n := len(depths)
	if n == 0 {
		return
	}
	gap := 2
	barW := (rect.Dx() - (n+1)*gap) / n
	if barW < 1 {
		barW = 1
	}
	baseY := rect.Max.Y - 1
	usableH := rect.Dy() - 2
	if usableH < 1 {
		usableH = 1
	}
	col := WithAlpha(colWaveTrace, AlphaFaint)
	for i, d := range depths {
		frac := d / depthMax
		if frac < 0 {
			frac = 0
		}
		if frac > 1 {
			frac = 1
		}
		h := int(math.Round(frac * float64(usableH)))
		if h < 1 {
			h = 1 // baseline stub: every operator slot stays visible even at 0 depth.
		}
		x0 := rect.Min.X + gap + i*(barW+gap)
		x1 := x0 + barW
		drawRect(dst, image.Rect(x0, baseY-h, x1, baseY), col, true)
	}
}

// fmDepths returns the per-operator FM depths from a params map, falling back to
// the single fm_op2_depth modulator (the only one bespoke FM recipes expose)
// when no indexed op has any depth.
func fmDepths(m map[string]float64) []float64 {
	d := []float64{m["fm_op1_depth"], m["fm_op2_depth"], m["fm_op3_depth"], m["fm_op4_depth"]}
	allZero := true
	for _, v := range d {
		if v != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		if v, ok := m["fm_op2_depth"]; ok {
			return []float64{v}
		}
	}
	return d
}

// ── conceptMotion — modulation curve (pitch drop / LFO wobble / burst) ──────

// motionCurveSamples returns n samples of the modulation curve for instID over
// the visualization window (normalized roughly to [-1, 1]). The curve is the
// sum of:
//   - a pitch-envelope decay (pitchenv_amt scaled by an exponential whose rate
//     comes from pitchenv_decay), and
//   - an LFO wobble (sine at lfo_rate, amplitude lfo_depth).
//
// Burst hits add small markers via the offsets, but the dominant, testable
// signal is the LFO wiggle count vs lfo_rate. Pure math — no audio render.
func motionCurveSamples(instID string, n int) []float64 {
	return motionCurveFromParams(conceptMergedParams(instID), n)
}

// motionCurveFromParams is motionCurveSamples over an explicit params map, so the
// ghost overlay can render the pre-drag snapshot with the same math.
func motionCurveFromParams(merged map[string]float64, n int) []float64 {
	if n < 2 {
		n = 2
	}

	// Pitch-env: amount in semitones (Min -24..24), decay seconds (0.01..2).
	peAmt := merged["pitchenv_amt"] / 24.0 // normalize to ~[-1,1]
	peDecay := merged["pitchenv_decay"]
	if peDecay <= 0 {
		peDecay = 0.08
	}

	// LFO: rate Hz (0.1..40), depth (0..1).
	lfoRate := merged["lfo_rate"]
	lfoDepth := merged["lfo_depth"]

	// The visualization window is a fixed slice of time so a higher lfo_rate
	// packs more cycles into it ⇒ more wiggles.
	const windowSec = 1.0

	out := make([]float64, n)
	for i := 0; i < n; i++ {
		t := float64(i) / float64(n-1) // 0..1
		ts := t * windowSec

		// Pitch-env decay: starts at peAmt, exponentially relaxes to 0.
		pitch := peAmt * math.Exp(-ts/peDecay)

		// LFO wobble.
		lfo := lfoDepth * math.Sin(2*math.Pi*lfoRate*ts)

		out[i] = pitch + lfo
	}
	return out
}

// motionParamsDiffer reports whether the ghost snapshot's motion params differ
// from the live ones (the only inputs to motionCurveFromParams).
func motionParamsDiffer(ghost, live map[string]float64) bool {
	if ghost == nil {
		return false
	}
	for _, k := range []string{"lfo_rate", "lfo_depth", "pitchenv_amt", "pitchenv_decay"} {
		if ghost[k] != live[k] {
			return true
		}
	}
	return false
}

// conceptMotion draws the modulation curve from motionCurveSamples as connected
// segments, plus burst markers (drawn regardless of the live burst_enabled gate
// so the burst sub-params teach what they do — see drawBurstMarkers).
func conceptMotion(dst *ebiten.Image, rect image.Rectangle, instID string, _ audio.ParamDef, ghost map[string]float64) {
	if rect.Dx() < 8 || rect.Dy() < 6 {
		return
	}
	conceptBgFill(dst, rect)

	n := rect.Dx()
	if n < 2 {
		n = 2
	}
	samples := motionCurveSamples(instID, n)

	midYf := float64(rect.Min.Y+rect.Max.Y) / 2
	half := float64(rect.Dy()-2) / 2
	yAt := func(v float64) int {
		if v > 1 {
			v = 1
		}
		if v < -1 {
			v = -1
		}
		return int(math.Round(midYf - v*half))
	}

	// Bipolar curve: fill to the centerline so positive and negative excursions
	// both read as area, plus a 2px stroke at the curve. Use the accent family
	// to match the motion stages' identity.
	midBaseline := int(math.Round(midYf))
	conceptDrawFilledCurve(dst, rect, samples, yAt, midBaseline,
		WithAlpha(colAccent, AlphaSubtle), WithAlpha(colAccent, AlphaStrong))

	merged := conceptMergedParams(instID)

	// Ghost overlay: faint "before" modulation curve when the pre-drag motion
	// params differ.
	if motionParamsDiffer(ghost, merged) {
		conceptDrawFilledCurve(dst, rect, motionCurveFromParams(ghost, n), yAt, midBaseline,
			nil, colConceptGhost)
	}

	// Burst markers: vertical ticks at each hit offset. Drawn REGARDLESS of the
	// live burst_enabled gate (off by default) so the burst sub-params teach what
	// they do even from an inert voice — consistent with conceptKnobWave/conceptFM
	// forcing their stage on for the picture. Each marker's:
	//   - x position comes from burstN_off  (when the hit fires),
	//   - height comes from burstN_amp       (how loud the hit is), and
	//   - width/height come from burst_sharp (sharper ⇒ thinner + taller tick).
	drawBurstMarkers(dst, rect, merged)
}

// drawBurstMarkers paints the per-hit burst ticks for the motion picture. The
// burst stage is force-treated as ON (the live burst_enabled gate is ignored) so
// burstN_off / burstN_amp / burst_sharp all visibly move the picture even though
// the stage is off by default.
func drawBurstMarkers(dst *ebiten.Image, rect image.Rectangle, merged map[string]float64) {
	tick := WithAlpha(genColorBorder, AlphaSubtle)
	const maxOff = 0.25 // burst*_off Max
	// burst_sharp is 1..200; map to a [0,1] sharpness where higher ⇒ thinner +
	// taller tick. A non-positive/absent value reads as the recipe default 40.
	sharp := merged["burst_sharp"]
	if sharp <= 0 {
		sharp = 40
	}
	const sharpMin, sharpMax = 1.0, 200.0
	sf := (sharp - sharpMin) / (sharpMax - sharpMin)
	if sf < 0 {
		sf = 0
	}
	if sf > 1 {
		sf = 1
	}
	// Tick width: 3px at the dullest, 1px at the sharpest.
	tickW := 3 - int(math.Round(sf*2))
	if tickW < 1 {
		tickW = 1
	}
	usableH := rect.Dy() - 2
	if usableH < 1 {
		usableH = 1
	}
	baseY := rect.Max.Y - 1
	for _, name := range []string{"burst1_off", "burst2_off", "burst3_off", "burst4_off"} {
		amp := merged[burstAmpFor(name)]
		if amp <= 0 {
			continue
		}
		if amp > 1 {
			amp = 1
		}
		off := merged[name]
		frac := off / maxOff
		if frac < 0 {
			frac = 0
		}
		if frac > 1 {
			frac = 1
		}
		x := rect.Min.X + int(math.Round(frac*float64(rect.Dx()-1)))
		// Height = amplitude, boosted by sharpness (sharper hits read as taller
		// transients). Clamp into the band.
		hFrac := amp * (0.6 + 0.4*sf)
		if hFrac > 1 {
			hFrac = 1
		}
		h := int(math.Round(hFrac * float64(usableH)))
		if h < 1 {
			h = 1
		}
		x1 := x + tickW
		if x1 > rect.Max.X {
			x1 = rect.Max.X
			x = x1 - tickW
			if x < rect.Min.X {
				x = rect.Min.X
			}
		}
		drawRect(dst, image.Rect(x, baseY-h, x1, baseY), tick, true)
	}
}

// burstAmpFor maps a burstN_off param name to its companion burstN_amp.
func burstAmpFor(offName string) string {
	switch offName {
	case "burst1_off":
		return "burst1_amp"
	case "burst2_off":
		return "burst2_amp"
	case "burst3_off":
		return "burst3_amp"
	case "burst4_off":
		return "burst4_amp"
	}
	return ""
}

// ── conceptPost — waveshaper transfer (drive + gain), with ghost ───────────

// postCurveSamples returns n samples of tanh(drive*x)*gain over x in [-1, 1].
// More drive squashes the curve into a harder S; gain scales its amplitude.
func postCurveSamples(drive, gain float64, n int) []float64 {
	if n < 2 {
		n = 2
	}
	// Map the normalized drive knob (0..1) to a useful tanh pre-gain range.
	k := 1.0 + drive*9.0 // 1..10
	out := make([]float64, n)
	for i := 0; i < n; i++ {
		x := -1.0 + 2.0*float64(i)/float64(n-1) // -1..1
		out[i] = math.Tanh(k*x) * gain
	}
	return out
}

// conceptPost draws the waveshaper transfer curve. When ghost carries a
// DIFFERENT drive/gain, a faint "before" curve is drawn first so the live curve
// reads as the delta.
func conceptPost(dst *ebiten.Image, rect image.Rectangle, instID string, _ audio.ParamDef, ghost map[string]float64) {
	if rect.Dx() < 8 || rect.Dy() < 6 {
		return
	}
	conceptBgFill(dst, rect)

	merged := conceptMergedParams(instID)
	drive := merged["drive"]
	gain := merged["gain"]
	if gain == 0 {
		gain = 1
	}

	midYf := float64(rect.Min.Y+rect.Max.Y) / 2
	half := float64(rect.Dy()-2) / 2
	yAt := func(v float64) int {
		// gain Max is 1.5; normalize against it so the curve fits the band.
		v = v / 1.5
		if v > 1 {
			v = 1
		}
		if v < -1 {
			v = -1
		}
		return int(math.Round(midYf - v*half))
	}

	n := rect.Dx()
	midBaseline := int(math.Round(midYf))

	// Ghost first (faint, stroke-only), only when it differs from the live
	// values — keeps the "before" reference legible without burying the live
	// curve under a second fill.
	if ghost != nil {
		gDrive, hasD := ghost["drive"]
		gGain, hasG := ghost["gain"]
		if !hasD {
			gDrive = drive
		}
		if !hasG {
			gGain = gain
		}
		if gGain == 0 {
			gGain = 1
		}
		if gDrive != drive || gGain != gain {
			conceptDrawFilledCurve(dst, rect, postCurveSamples(gDrive, gGain, n), yAt, midBaseline,
				nil, WithAlpha(genColorBorder, AlphaSubtle))
		}
	}

	// Live curve on top: soft fill to the centerline + 2px stroke.
	conceptDrawFilledCurve(dst, rect, postCurveSamples(drive, gain, n), yAt, midBaseline,
		colConceptFill, colConceptStroke)
}

// ── conceptValueBar — per-knob value meter for grab-bag groups ──────────────

// conceptValueBar draws a horizontal fill bar showing the knob's CURRENT value
// as a fraction of its [Min,Max] range. Used for the heterogeneous core /
// generic / voice groups (volume, pan, tune, …) where no single coherent shape
// fits — an honest per-knob "this is where the knob sits" picture, NOT the
// misleading oscillator wave the OSC fallback used to paint here. When a ghost
// snapshot carries a DIFFERENT value for this knob, a faint tick marks the
// pre-drag position so the before/after delta reads.
func conceptValueBar(dst *ebiten.Image, rect image.Rectangle, instID string, def audio.ParamDef, ghost map[string]float64) {
	if rect.Dx() < 8 || rect.Dy() < 6 {
		return
	}
	conceptBgFill(dst, rect)
	if def.Name == "" || def.Max <= def.Min {
		return
	}

	frac := func(v float64) float64 {
		f := (v - def.Min) / (def.Max - def.Min)
		if f < 0 {
			f = 0
		}
		if f > 1 {
			f = 1
		}
		return f
	}

	merged := conceptMergedParams(instID)
	v := merged[def.Name]

	// Track + fill band, vertically centered.
	pad := 2
	barTop := rect.Min.Y + (rect.Dy()-conceptValueBarH)/2
	if barTop < rect.Min.Y+pad {
		barTop = rect.Min.Y + pad
	}
	barBot := barTop + conceptValueBarH
	if barBot > rect.Max.Y-pad {
		barBot = rect.Max.Y - pad
	}
	x0 := rect.Min.X + pad
	x1 := rect.Max.X - pad
	if x1 <= x0 || barBot <= barTop {
		return
	}
	// Track.
	drawRect(dst, image.Rect(x0, barTop, x1, barBot), WithAlpha(genColorBorder, AlphaSubtle), true)
	// Fill from the left to the value fraction.
	fillW := int(math.Round(frac(v) * float64(x1-x0)))
	if fillW > 0 {
		drawRect(dst, image.Rect(x0, barTop, x0+fillW, barBot), colConceptStroke, true)
	}

	// Ghost tick at the pre-drag value when it differs.
	if ghost != nil {
		if gv, ok := ghost[def.Name]; ok && gv != v {
			gx := x0 + int(math.Round(frac(gv)*float64(x1-x0)))
			if gx < x0 {
				gx = x0
			}
			if gx >= x1 {
				gx = x1 - 1
			}
			drawRect(dst, image.Rect(gx, rect.Min.Y+1, gx+1, rect.Max.Y-1), WithAlpha(colAccent, AlphaStrong), true)
		}
	}
}

// ── conceptFMEnvelope — schematic time-curves for time-domain FM params ──────
//
// The steady, cycle-normalized conceptFM wave cannot show params whose effect
// evolves over the note: per-operator amplitude decay and the FM pitch sweep.
// conceptFMEnvelope draws a PURE-MATH schematic of the param's meaning over a
// normalized time window — an exp decay curve for an operator's decay knob, a
// pitch-vs-time sweep for the FM pitch-env — exactly as conceptMotion/conceptEnvelope
// draw schematic time pictures. No audio render; live during drag.

// isFMPitchEnvKnob / isFMDecayKnob / isFMTimeDomainKnob classify the FM knobs
// whose effect is time-domain (shown by conceptFMEnvelope). Handles both the
// modular (fm_opN_decay / fm_pitch_env_*) and bespoke (fm_decN / fm_pe_*) naming.
func isFMPitchEnvKnob(name string) bool {
	switch name {
	case "fm_pitch_env_amount", "fm_pitch_env_decay", "fm_pe_amt", "fm_pe_decay":
		return true
	}
	return false
}

func isFMDecayKnob(name string) bool {
	if strings.HasPrefix(name, "fm_op") && strings.HasSuffix(name, "_decay") {
		return true
	}
	return strings.HasPrefix(name, "fm_dec")
}

func isFMTimeDomainKnob(name string) bool {
	return isFMPitchEnvKnob(name) || isFMDecayKnob(name)
}

// fmDecaySamples returns n samples of an exponential amplitude fade exp(-t/tau)
// over [0, windowSec]. Larger tau ⇒ slower fall ⇒ flatter curve.
func fmDecaySamples(tau, windowSec float64, n int) []float64 {
	if n < 2 {
		n = 2
	}
	if tau <= 0.02 {
		tau = 0.02
	}
	out := make([]float64, n)
	for i := 0; i < n; i++ {
		t := float64(i) / float64(n-1) * windowSec
		out[i] = math.Exp(-t / tau)
	}
	return out
}

// fmPitchEnvSamples returns n samples of a pitch-vs-time sweep
// sign(amt)·sqrt(|amt|/maxAmt)·exp(-t/dec) over [0, windowSec], normalized to
// ~[-1,1]. The sqrt amplitude compression keeps modest sweeps legible while
// remaining monotonic in amount.
func fmPitchEnvSamples(amt, dec, maxAmt, windowSec float64, n int) []float64 {
	if n < 2 {
		n = 2
	}
	if dec <= 0 {
		dec = 0.06
	}
	if maxAmt <= 0 {
		maxAmt = 24
	}
	// Compress the amplitude so a modest sweep (e.g. 3 of 24) still shows a clear
	// shape, while staying monotonic in amount: peak = sign(amt)·sqrt(|amt|/maxAmt).
	frac := amt / maxAmt
	if frac > 1 {
		frac = 1
	} else if frac < -1 {
		frac = -1
	}
	mag := math.Sqrt(math.Abs(frac))
	if frac < 0 {
		mag = -mag
	}
	out := make([]float64, n)
	for i := 0; i < n; i++ {
		t := float64(i) / float64(n-1) * windowSec
		out[i] = mag * math.Exp(-t/dec)
	}
	return out
}

// fmPitchEnvParams reads the FM pitch-env amount + decay from a params map,
// handling both the modular (fm_pitch_env_*) and bespoke (fm_pe_*) naming.
func fmPitchEnvParams(m map[string]float64) (amt, dec float64) {
	amt = m["fm_pitch_env_amount"]
	if amt == 0 {
		amt = m["fm_pe_amt"]
	}
	dec = m["fm_pitch_env_decay"]
	if dec == 0 {
		dec = m["fm_pe_decay"]
	}
	if dec <= 0 {
		dec = 0.06
	}
	return amt, dec
}

// conceptFMEnvelope draws the schematic time-curve for a time-domain FM knob.
func conceptFMEnvelope(dst *ebiten.Image, rect image.Rectangle, instID string, def audio.ParamDef, ghost map[string]float64) {
	if rect.Dx() < 8 || rect.Dy() < 6 {
		return
	}
	conceptBgFill(dst, rect)
	merged := conceptMergedParams(instID)
	n := rect.Dx()
	if n < 2 {
		n = 2
	}

	if isFMPitchEnvKnob(def.Name) {
		// Bipolar pitch sweep around the centerline.
		const windowSec, maxAmt = 0.4, 24.0
		midYf := float64(rect.Min.Y+rect.Max.Y) / 2
		half := float64(rect.Dy()-2) / 2
		yAt := func(v float64) int {
			if v > 1 {
				v = 1
			} else if v < -1 {
				v = -1
			}
			return int(math.Round(midYf - v*half))
		}
		mid := int(math.Round(midYf))
		amt, dec := fmPitchEnvParams(merged)
		if ghost != nil {
			gAmt, gDec := fmPitchEnvParams(ghost)
			if gAmt != amt || gDec != dec {
				conceptDrawFilledCurve(dst, rect, fmPitchEnvSamples(gAmt, gDec, maxAmt, windowSec, n), yAt, mid, nil, colConceptGhost)
			}
		}
		conceptDrawFilledCurve(dst, rect, fmPitchEnvSamples(amt, dec, maxAmt, windowSec, n), yAt, mid,
			WithAlpha(colAccent, AlphaSubtle), WithAlpha(colAccent, AlphaStrong))
		return
	}

	// Decay mode: amplitude fade from the SELECTED operator's decay value.
	const windowSec = 1.0
	baseY := rect.Max.Y - 1
	topY := rect.Min.Y + 1
	yAt := func(v float64) int {
		if v < 0 {
			v = 0
		} else if v > 1 {
			v = 1
		}
		return baseY - int(v*float64(baseY-topY))
	}
	tau := merged[def.Name]
	if ghost != nil {
		if gv, ok := ghost[def.Name]; ok && gv != tau {
			conceptDrawFilledCurve(dst, rect, fmDecaySamples(gv, windowSec, n), yAt, baseY, nil, colConceptGhost)
		}
	}
	conceptDrawFilledCurve(dst, rect, fmDecaySamples(tau, windowSec, n), yAt, baseY, colConceptFill, colConceptStroke)
}
