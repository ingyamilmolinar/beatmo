//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// bindFocusSweepInst points a throwaway instrument id at an arbitrary recipe so
// the sweep can SetInstrumentParam on it and feed it to a focus renderer. The id
// is unique per recipe and reset on cleanup so recipes don't bleed into each
// other across the sweep.
func bindFocusSweepInst(t *testing.T, recipeID string) string {
	t.Helper()
	id := "focus-sweep-" + recipeID
	audio.BindInstrumentToRecipe(id, recipeID)
	t.Cleanup(func() { audio.ResetInstrumentParams(id) })
	return id
}

// focusSweepExemptByName lists SELECTABLE-grid-knob NAMES whose focus renderer
// genuinely cannot depict the knob's Min-vs-Max difference IN EVERY recipe the
// knob appears in. Each entry names the mechanism and the reason; the sweep LOGS
// every applied exemption (it is never silently skipped) AND fails if an exempt
// knob is actually responsive (so a stale exemption can't hide a regression),
// per the repo's no-silent-caps rule.
//
// SCOPE: per-stage enable toggles (*_enabled) and engine-internal hidden params
// are NOT keys here — they are filtered out before the sweep loop by
// synthParamIsGridKnob (they render as section pills / have no UI, never as a
// selectable knob the focus graph explains).
//
// ROUTING (synthFocusRendererForKnob, synth_focus_graph.go): each swept knob
// picks its renderer in this order —
//  1. Pitch knobs (isSynthPitchKnob: osc_octave/osc_detune/pitch/tune/fm_base/
//     fm_base_freq/fundamental/base_freq) → conceptPitchWave, a real-note
//     fixed-time-window render whose cycle count tracks pitch.
//  2. FM time-domain knobs (isFMTimeDomainKnob: operator decay, FM pitch-env
//     sweep) → conceptFMEnvelope, a schematic time-curve.
//  3. Everything else routes BY GROUP (synthFocusRendererForGroup):
//     osc → conceptOsc (cycle-normalized timbre curve); filter → conceptFilter;
//     env → conceptEnvelope; fm → conceptFM (the FM output waveform, operators
//     forced audible); post → conceptPostWave (before/after waveshaper-transfer
//     curve); pitch → conceptPitchWave; pitchenv → conceptPitchEnvSweep (bipolar
//     pitch-vs-time sweep over pitchenv_amt/pitchenv_decay); lfo → conceptLFO
//     (the LFO's own wobble: rate/depth plus the delay onset ramp, with real
//     lfo_depth affine-mapped onto a teachable [0.35,1] display range so even a
//     near-zero seed depth still visibly changes); burst → conceptBurst (a
//     per-hit spike timeline; a silent hit, e.g. burst4_amp defaulting to 0,
//     still draws a dim placeholder spike so its *_off timing knob stays
//     responsive). Groups with no dedicated domain curve yet (filtenv, unison,
//     kick, core, and any unlisted group) fall through to
//     conceptFocusLevelWave, which scales the displayed output wave's amplitude
//     by the knob's Min..Max fraction.
//
// What remains exempt is a tiny, genuinely-unshowable residue:
//   - fm_op1_level (fm group): op1's level is not a reliable output-amplitude
//     control in any FM algorithm (see fmOp1IsCarrier's evidence in
//     synth_focus_graph.go — the carrier path never multiplies by levels[0]),
//     so no faithful picture exists; it falls through to conceptFM but Min vs
//     Max share the same curve in every recipe.
//   - lfo_target (lfo group): an enum ROUTING choice (Amp/Pitch/Cutoff) that
//     changes WHAT the LFO modulates, not the wobble's own rate/depth/onset
//     shape conceptLFO draws — lfo_rate/lfo_depth/lfo_delay (the shape and
//     onset) all respond and are NOT exempt.
//   - the fm-epiano/fm_op3_ratio PAIR (focusSweepExemptPair below): op3 ships a
//     ratio but zero depth in that one recipe, so the operator is inert there
//     specifically (op3_ratio DOES respond in fm-lead and the modular recipes).
//
// Each of the two by-name exemptions FAILS in every recipe it appears in, so a
// name-level exemption loses no passing assertion.
//
// Known pre-existing baseline (NOT exempted here): every "group osc"
// physical-model knob (osc_sax_*/osc_bow_*/osc_type) on recipes carrying a
// physical-model oscillator (e.g. synth-modular-oboe-full,
// synth-modular-piano-felt) currently fails this gate — conceptOsc's
// cycle-normalized timbre curve does not yet depict those knobs' effect. That
// gap is owned by a concurrent, in-flight physical-model work stream and is
// deliberately left UNEXEMPTED: adding exemptions now would flip to
// "EXEMPT but now RESPONSIVE" the moment that work lands and silently mask
// the very knobs it's fixing. The failure count fluctuates as that work
// progresses (observed in the 140..497 range) — do not chase it here.
var focusSweepExemptByName = map[string]string{
	// ── fm group. Most FM knobs are PROVEN responsive: ratio/depth/level/algorithm
	//    drive conceptFM (the FM output waveform), and the time-domain knobs
	//    (operator decay, FM pitch-env sweep) drive conceptFMEnvelope (Task 1/2).
	//    What stays exempt is what NO faithful picture can show:
	"fm_op1_level": "op1 level is not a reliable output-amplitude control (carrier emitted without level scaling across all FM algorithms — preview_render.go FM matrix), so no faithful picture exists.",

	// ── burst group → conceptBurst (Task 7). Every burstN_off / burstN_amp /
	//    burst_sharp knob is now responsive, including hit-4: a silent hit
	//    (burst4_amp defaults to 0 across every recipe) draws a dim placeholder
	//    spike at 15% height instead of nothing, so burst4_off's timing still
	//    moves the picture. No burst-group exemptions remain.

	// ── lfo group → conceptLFO, which draws the LFO's own wobble (rate/depth) plus
	//    the vibrato onset ramp (lfo_delay: silent, then a 0.25s ramp to full
	//    depth). One LFO knob has no shape under that renderer:
	"lfo_target": "lfo_target is an enum ROUTING choice (Amp/Pitch/Cutoff) — it changes WHAT the LFO modulates, not the wobble's own rate/depth/onset shape conceptLFO draws, so Min vs Max share the same curve. (lfo_rate/lfo_depth/lfo_delay — the wobble's shape and onset — respond.)",
}

// focusSweepExemptPair lists per-(recipe, knob) exemptions for knobs that respond
// in MOST recipes but not a specific one, so they cannot be exempted by name
// without losing the passing assertions. Key is recipeID + "\x00" + paramName.
var focusSweepExemptPair = map[string]string{
	// fm-epiano ships op3 with a fm_op3_ratio knob but NO fm_op3_depth (op3 has
	// zero modulation index), so the operator contributes nothing to the steady
	// FM wave and its ratio leaves the picture flat. (op3 ratio DOES respond in
	// fm-lead and the modular recipes, which give op3 a depth.)
	"fm-epiano\x00fm_op3_ratio": "fm-epiano gives op3 a ratio but no depth (zero modulation index); with op3 inert, its ratio does not bend the steady FM wave.",
}

// TestFocusGraph_RespondsToEveryKnob is the OBJECTIVE GATE over the Synth-tab
// focus graph: for every SELECTABLE GRID KNOB (synthParamIsGridKnob — excludes
// hidden params and per-stage enable toggles) of every registered recipe, the
// per-knob focus renderer (synthFocusRendererForKnob — pitch knobs → pitch wave,
// else by group) must paint a DIFFERENT picture at the knob's Min vs its Max,
// proving the focus graph reflects the knob.
//
// The gate is BIDIRECTIONAL:
//   - a non-exempt selectable knob that is UNRESPONSIVE fails (the core gate);
//   - an EXEMPT knob that is RESPONSIVE fails (a stale/over-broad exemption that
//     would otherwise silently hide a regression).
//
// The exemption set is PINNED and MINIMAL (focusSweepExemptByName +
// focusSweepExemptPair above), each entry carries an inline reason, and every
// applied exemption is LOGGED — never silently ignored — per the no-silent-caps
// rule.
func TestFocusGraph_RespondsToEveryKnob(t *testing.T) {
	rect := image.Rect(0, 0, 260, 110)
	swept := 0
	exempted := 0
	for recipeID, reg := range audio.RecipeRegistrations() {
		if reg == nil {
			continue
		}
		inst := bindFocusSweepInst(t, recipeID)
		for _, d := range reg.Params {
			// Only SELECTABLE grid knobs are explained by the focus graph.
			// Hidden params and per-stage enable toggles render as pills, not
			// knobs, so the focus graph never depicts them — skip outright.
			if !synthParamIsGridKnob(d) || d.Max <= d.Min {
				continue
			}
			pairKey := recipeID + "\x00" + d.Name
			exemptReason, pairExempt := focusSweepExemptPair[pairKey]
			if !pairExempt {
				exemptReason, _ = focusSweepExemptByName[d.Name]
			}

			// Per-knob routing: pitch knobs route through the pitch-wave
			// renderer, everything else by group. Using the per-knob entry
			// point exercises the same routing the live Synth tab uses.
			render := synthFocusRendererForKnob(d)

			audio.ResetInstrumentParams(inst)
			audio.SetInstrumentParam(inst, d.Name, d.Min)
			lo := fingerprintFocus(t, func(img *ebiten.Image) { render(img, rect, inst, d, nil) })

			audio.ResetInstrumentParams(inst)
			audio.SetInstrumentParam(inst, d.Name, d.Max)
			hi := fingerprintFocus(t, func(img *ebiten.Image) { render(img, rect, inst, d, nil) })

			responsive := lo != hi

			if exemptReason != "" {
				// Exempt knob: it must be GENUINELY unresponsive. If it now
				// responds, the exemption is stale/over-broad and silently
				// hides a regression — fail so it gets removed.
				exempted++
				t.Logf("EXEMPT: recipe %s knob %s (group %s) — %s", recipeID, d.Name, d.Group, exemptReason)
				if responsive {
					t.Errorf("recipe %s knob %s (group %s): EXEMPT but now RESPONSIVE at Min(%g) vs Max(%g) — "+
						"the focus renderer now depicts this knob; remove it from the exemption map",
						recipeID, d.Name, d.Group, d.Min, d.Max)
				}
				continue
			}

			swept++
			if !responsive {
				t.Errorf("recipe %s knob %s (group %s): focus graph identical at Min(%g) and Max(%g) — "+
					"either the focus renderer ignores this selectable grid knob (extend the renderer or add a "+
					"reasoned exemption) or the exemption maps are stale", recipeID, d.Name, d.Group, d.Min, d.Max)
			}
		}
	}
	if swept == 0 {
		t.Fatal("no selectable grid knobs swept — registry empty?")
	}
	t.Logf("focus-graph sweep: %d selectable knobs responsive, %d exempted (pinned)", swept, exempted)
}

// fingerprintFocus renders into a fresh image and returns a hash of the painted
// rects (count + summed geometry + color) that changes when the picture
// changes. Color is included (not just geometry) so a renderer that repaints
// the SAME rect in a different color — e.g. conceptBurst's selected-hit
// highlight, which swaps a spike between AlphaStrong/AlphaMedium without
// moving it — still fingerprints as a different picture.
func fingerprintFocus(t *testing.T, draw func(*ebiten.Image)) int {
	t.Helper()
	rects := collectFilledRects(t, func() { draw(ebiten.NewImage(260, 110)) })
	h := 0
	for _, r := range rects {
		h = h*31 + r.Rect.Min.X
		h = h*31 + r.Rect.Min.Y
		h = h*31 + r.Rect.Max.X
		h = h*31 + r.Rect.Max.Y
		h = h*31 + int(r.Color.R)
		h = h*31 + int(r.Color.G)
		h = h*31 + int(r.Color.B)
		h = h*31 + int(r.Color.A)
	}
	return h*31 + len(rects)
}
