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
// selectable knob the focus graph explains). Pitch knobs (osc_octave/osc_detune/
// pitch/tune/...) route to conceptPitchWave (per-knob routing) and ARE responsive
// — they are NOT exempt.
//
// What remains is a tiny FM residue. Task 1/2 closed the time-domain FM knobs:
// operator decay (fm_op*_decay) and the FM pitch-envelope sweep
// (fm_pitch_env_*) now route through conceptFMEnvelope, a schematic time-curve,
// and ARE responsive — they are no longer exempt. One FM knob stays exempt
// because no faithful picture exists: fm_op1_level (carrier emitted without
// level scaling across all FM algorithms). fm_base_freq is now responsive —
// RenderInstrumentPreview honors it as the FM carrier fundamental (Task 5), so
// conceptPitchWave's fixed-window wave changes at Min vs Max. The remaining
// exemption FAILS in every recipe it appears in, so a name-level exemption
// loses no passing assertion.
var focusSweepExemptByName = map[string]string{
	// ── fm group. Most FM knobs are PROVEN responsive: ratio/depth/level/algorithm
	//    drive conceptFM (the FM output waveform), and the time-domain knobs
	//    (operator decay, FM pitch-env sweep) drive conceptFMEnvelope (Task 1/2).
	//    What stays exempt is what NO faithful picture can show:
	"fm_op1_level": "op1 level is not a reliable output-amplitude control (carrier emitted without level scaling across all FM algorithms — preview_render.go FM matrix), so no faithful picture exists.",

	// ── burst group → conceptMotion. The burst markers (Task 17) are drawn
	//    regardless of the live burst_enabled gate, so burstN_off / burstN_amp /
	//    burst_sharp move the picture — EXCEPT hit-4, which is silent by default.
	"burst4_off": "the hit-4 marker is gated by burst4_amp, which defaults to 0 (a SILENT hit) across every recipe; with no audible hit-4, its off-TIME has nothing to position. (burst1/2/3_off respond — their amps default >0.)",

	// ── lfo group → conceptMotion, which draws the LFO's steady-state wobble from
	//    lfo_rate/lfo_depth. Two LFO knobs have no curve under that renderer:
	"lfo_target": "lfo_target is an enum ROUTING choice (Amp/Pitch/Cutoff) — it changes WHAT the LFO modulates, not the wobble's own rate/depth shape conceptMotion draws, so Min vs Max share the same curve. (lfo_rate/lfo_depth — the wobble's shape — respond.)",
	"lfo_delay":  "lfo_delay is the vibrato ONSET delay; conceptMotion paints a fixed steady-state wobble window and does not model the onset ramp, so Min vs Max paint the same steady wobble. (lfo_rate/lfo_depth respond.)",
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
// rects (count + summed geometry) that changes when the picture changes.
func fingerprintFocus(t *testing.T, draw func(*ebiten.Image)) int {
	t.Helper()
	rects := collectFilledRects(t, func() { draw(ebiten.NewImage(260, 110)) })
	h := 0
	for _, r := range rects {
		h = h*31 + r.Rect.Min.X
		h = h*31 + r.Rect.Min.Y
		h = h*31 + r.Rect.Max.X
		h = h*31 + r.Rect.Max.Y
	}
	return h*31 + len(rects)
}
