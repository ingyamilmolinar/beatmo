//go:build test

package ui

import (
	"image"
	"maps"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

func fpConcept(t *testing.T, r conceptRenderer, inst string, def audio.ParamDef) int {
	t.Helper()
	rect := image.Rect(0, 0, 260, 110)
	return fingerprintFocus(t, func(img *ebiten.Image) { r(img, rect, inst, def, nil) })
}

func lfoDefFor(t *testing.T, inst, name string) audio.ParamDef {
	t.Helper()
	reg := audio.RecipeRegistrations()[audio.RecipeForInstrument(inst)]
	for _, d := range reg.Params {
		if d.Name == name {
			return d
		}
	}
	t.Fatalf("param %s not found", name)
	return audio.ParamDef{}
}

func TestConceptLFO_RespondsToRateDepthDelay(t *testing.T) {
	const inst = "lfo-viz-test"
	audio.BindInstrumentToRecipe(inst, "synth-modular")
	t.Cleanup(func() { audio.ResetInstrumentParams(inst) })
	for _, name := range []string{"lfo_rate", "lfo_depth", "lfo_delay"} {
		def := lfoDefFor(t, inst, name)
		audio.ResetInstrumentParams(inst)
		audio.SetInstrumentParam(inst, "lfo_depth", 0.6) // wobble visible for rate/delay sweeps
		audio.SetInstrumentParam(inst, name, def.Min)
		lo := fpConcept(t, conceptLFO, inst, def)
		audio.SetInstrumentParam(inst, name, def.Max)
		hi := fpConcept(t, conceptLFO, inst, def)
		if lo == hi {
			t.Errorf("conceptLFO unresponsive to %s", name)
		}
	}
}

// TestConceptLFO_RateRespondsAtZeroDepth — lfo_rate must stay responsive even
// when lfo_depth is explicitly 0 (the synth-modular-sax default is a near-zero
// seed, not exactly 0 — see instrument_seeds.go — but 0 is the true floor of
// the knob's own [Min,Max] range and the sharpest case). lfoCurveFromParams
// affine-maps the real depth onto a [0.35, 1] DISPLAY range so the picture
// still teaches what lfo_rate/lfo_delay do at depth=0; this test pins that
// behavior directly against the depth=0 boundary.
func TestConceptLFO_RateRespondsAtZeroDepth(t *testing.T) {
	const inst = "lfo-viz-zero-depth-test"
	audio.BindInstrumentToRecipe(inst, "synth-modular")
	t.Cleanup(func() { audio.ResetInstrumentParams(inst) })

	rateDef := lfoDefFor(t, inst, "lfo_rate")
	audio.ResetInstrumentParams(inst)
	audio.SetInstrumentParam(inst, "lfo_depth", 0)
	audio.SetInstrumentParam(inst, "lfo_rate", rateDef.Min)
	lo := fpConcept(t, conceptLFO, inst, rateDef)
	audio.SetInstrumentParam(inst, "lfo_rate", rateDef.Max) // lfo_depth stays 0 from above
	hi := fpConcept(t, conceptLFO, inst, rateDef)
	if lo == hi {
		t.Errorf("conceptLFO unresponsive to lfo_rate at lfo_depth=0 — the display-depth mapping in lfoCurveFromParams regressed")
	}
}

// TestConceptLFO_NoDepthDeadZone — the display-depth mapping must be an affine
// RAMP (0.35 + 0.65·depth), not a hard floor: with a hard `max(depth, 0.35)`
// every depth position in 0..0.35 would paint an identical wobble, recreating
// the "knob does nothing on screen" problem this feature exists to fix. Guard:
// depth 0.1 vs 0.3 (both under the old floor) must fingerprint differently.
func TestConceptLFO_NoDepthDeadZone(t *testing.T) {
	const inst = "lfo-viz-dead-zone-test"
	audio.BindInstrumentToRecipe(inst, "synth-modular")
	t.Cleanup(func() { audio.ResetInstrumentParams(inst) })

	depthDef := lfoDefFor(t, inst, "lfo_depth")
	audio.ResetInstrumentParams(inst)
	audio.SetInstrumentParam(inst, "lfo_depth", 0.1)
	lo := fpConcept(t, conceptLFO, inst, depthDef)
	audio.SetInstrumentParam(inst, "lfo_depth", 0.3)
	hi := fpConcept(t, conceptLFO, inst, depthDef)
	if lo == hi {
		t.Errorf("conceptLFO paints identically at lfo_depth=0.1 and 0.3 — display-depth dead zone (hard floor) regressed")
	}
}

func TestConceptBurst_EveryBurstKnobResponds(t *testing.T) {
	const inst = "burst-viz-test"
	audio.BindInstrumentToRecipe(inst, "synth-modular")
	t.Cleanup(func() { audio.ResetInstrumentParams(inst) })
	names := []string{
		"burst1_off", "burst1_amp", "burst2_off", "burst2_amp",
		"burst3_off", "burst3_amp", "burst4_off", "burst4_amp", "burst_sharp",
	}
	for _, name := range names {
		def := lfoDefFor(t, inst, name)
		audio.ResetInstrumentParams(inst)
		audio.SetInstrumentParam(inst, name, def.Min)
		lo := fpConcept(t, conceptBurst, inst, def)
		audio.SetInstrumentParam(inst, name, def.Max)
		hi := fpConcept(t, conceptBurst, inst, def)
		if lo == hi {
			t.Errorf("conceptBurst unresponsive to %s (burst4_off must respond via the dim placeholder spike)", name)
		}
	}
}

// TestConceptBurst_GhostAddsInk — conceptBurst overlays a faint "before" spike
// for the selected hit when the pre-drag burstN_off/burstN_amp differ from the
// live values (the ghost branch: `ghost != nil && (selected == hit || selected
// == 0)`). Follows the sibling ghost-test pattern (TestConceptOsc_GhostAddsInk /
// TestConceptFM_GhostAddsInk): clone the shared rev-gated params map via
// maps.Clone before mutating (conceptMergedParams is READ-ONLY), then compare
// non-bg ink with vs without the ghost snapshot.
func TestConceptBurst_GhostAddsInk(t *testing.T) {
	const inst = "burst-ghost-test"
	audio.BindInstrumentToRecipe(inst, "synth-modular")
	t.Cleanup(func() { audio.ResetInstrumentParams(inst) })

	// def.Name selects hit 1 (burstHitIndexForKnob("burst1_off") == 1), so the
	// ghost branch's `selected == hit` arm fires for hit 1.
	def := lfoDefFor(t, inst, "burst1_off")
	audio.SetInstrumentParam(inst, "burst1_off", 0.05)
	audio.SetInstrumentParam(inst, "burst1_amp", 0.5)

	dst := ebiten.NewImage(260, 110)
	r := image.Rect(0, 0, 260, 110)
	no := countNonBgRects(collectFilledRects(t, func() { conceptBurst(dst, r, inst, def, nil) }))

	// conceptMergedParams returns the shared rev-gated cache map (READ-ONLY);
	// ghost-building must clone it before mutating, exactly like the production
	// ghost path (captureSynthGhost).
	ghost := maps.Clone(conceptMergedParams(inst))
	ghost["burst1_off"] = 0.2
	ghost["burst1_amp"] = 0.9
	with := countNonBgRects(collectFilledRects(t, func() { conceptBurst(dst, r, inst, def, ghost) }))

	if with <= no {
		t.Errorf("conceptBurst ghost did not add ink: noGhost=%d withGhost=%d", no, with)
	}
}

func TestConceptBurst_SelectedHitHighlighted(t *testing.T) {
	const inst = "burst-hl-test"
	audio.BindInstrumentToRecipe(inst, "synth-modular")
	t.Cleanup(func() { audio.ResetInstrumentParams(inst) })
	d1 := lfoDefFor(t, inst, "burst1_off")
	d2 := lfoDefFor(t, inst, "burst2_off")
	// Same params, different selected knob ⇒ different highlight ⇒ different pixels.
	if fpConcept(t, conceptBurst, inst, d1) == fpConcept(t, conceptBurst, inst, d2) {
		t.Fatalf("selected-hit highlight not visible: burst1 vs burst2 selection paint identically")
	}
}

func TestBurstHitIndexForKnob(t *testing.T) {
	for name, want := range map[string]int{
		"burst1_off": 1, "burst3_amp": 3, "burst_sharp": 0, "lfo_rate": 0,
	} {
		if got := burstHitIndexForKnob(name); got != want {
			t.Errorf("burstHitIndexForKnob(%s)=%d want %d", name, got, want)
		}
	}
}

func TestConceptPostWave_DriveMorphsWave(t *testing.T) {
	const inst = "post-viz-test"
	audio.BindInstrumentToRecipe(inst, "synth-modular")
	t.Cleanup(func() { audio.ResetInstrumentParams(inst) })
	def := lfoDefFor(t, inst, "drive")
	audio.SetInstrumentParam(inst, def.Name, def.Min)
	lo := fpConcept(t, conceptPostWave, inst, def)
	audio.SetInstrumentParam(inst, def.Name, def.Max)
	hi := fpConcept(t, conceptPostWave, inst, def)
	if lo == hi {
		t.Fatalf("conceptPostWave: max drive paints the same wave as no drive")
	}
}

func TestPostNeutralRef(t *testing.T) {
	if postNeutralRef(audio.ParamDef{Name: "gain", Min: 0}) != 1 {
		t.Fatal("gain neutral must be unity (Min=0 would be silence, hiding the knob's effect)")
	}
	if postNeutralRef(audio.ParamDef{Name: "drive", Min: 0}) != 0 {
		t.Fatal("drive neutral must be 0 (no drive)")
	}
}

func TestConceptPitchEnvSweep_Responds(t *testing.T) {
	const inst = "penv-viz-test"
	audio.BindInstrumentToRecipe(inst, "synth-modular")
	t.Cleanup(func() { audio.ResetInstrumentParams(inst) })
	for _, name := range []string{"pitchenv_amt", "pitchenv_decay"} {
		def := lfoDefFor(t, inst, name)
		audio.ResetInstrumentParams(inst)
		if name == "pitchenv_decay" {
			audio.SetInstrumentParam(inst, "pitchenv_amt", 12) // sweep visible for decay sweep
		}
		audio.SetInstrumentParam(inst, name, def.Min)
		lo := fpConcept(t, conceptPitchEnvSweep, inst, def)
		audio.SetInstrumentParam(inst, name, def.Max)
		hi := fpConcept(t, conceptPitchEnvSweep, inst, def)
		if lo == hi {
			t.Errorf("conceptPitchEnvSweep unresponsive to %s", name)
		}
	}
}
