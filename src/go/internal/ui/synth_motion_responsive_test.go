//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// motionFingerprint renders conceptMotion for instID under def into a fresh
// image and returns a hash of the painted rects (count + summed geometry) that
// changes whenever the picture changes.
func motionFingerprint(t *testing.T, instID string, def audio.ParamDef) int {
	t.Helper()
	rects := collectFilledRects(t, func() {
		conceptMotion(ebiten.NewImage(120, 60), conceptVizRect(), instID, def, nil)
	})
	h := 0
	for _, r := range rects {
		h = h*31 + r.Rect.Min.X
		h = h*31 + r.Rect.Min.Y
		h = h*31 + r.Rect.Max.X
		h = h*31 + r.Rect.Max.Y
	}
	return h*31 + len(rects)
}

// TestConceptMotion_RespondsToEveryMotionKnob proves the motion picture changes
// at Min vs Max for every motion knob the modular recipe exposes — including the
// burst sub-params (burst*_off / burst*_amp / burst_sharp), which previously only
// drew markers when the (off-by-default) burst stage was live. conceptMotion now
// force-treats the burst/lfo/pitchenv stages as ON for the picture, so the knobs
// teach what they do regardless of the live bypass gate.
func TestConceptMotion_RespondsToEveryMotionKnob(t *testing.T) {
	const inst = "motion-resp-test"
	bindModularConceptInst(t, inst)

	reg := audio.RecipeRegistrations()["synth-modular"]
	if reg == nil {
		t.Fatal("synth-modular recipe not registered")
	}
	// Index the recipe's ParamDefs by name so we render each knob with its OWN def.
	defByName := map[string]audio.ParamDef{}
	for _, d := range reg.Params {
		defByName[d.Name] = d
	}

	want := []string{
		"burst1_off", "burst1_amp", "burst_sharp",
		"lfo_rate", "lfo_depth", "pitchenv_amt", "pitchenv_decay",
	}
	for _, name := range want {
		def, ok := defByName[name]
		if !ok {
			t.Logf("SKIP %s — not present in synth-modular ParamDefs", name)
			continue
		}
		if def.Max <= def.Min {
			t.Logf("SKIP %s — degenerate range [%g,%g]", name, def.Min, def.Max)
			continue
		}

		audio.ResetInstrumentParams(inst)
		audio.SetInstrumentParam(inst, name, def.Min)
		lo := motionFingerprint(t, inst, def)

		audio.ResetInstrumentParams(inst)
		audio.SetInstrumentParam(inst, name, def.Max)
		hi := motionFingerprint(t, inst, def)

		audio.ResetInstrumentParams(inst)

		if lo == hi {
			t.Errorf("conceptMotion identical at Min(%g) and Max(%g) for %s (group %s) — picture ignores this knob",
				def.Min, def.Max, name, def.Group)
		}
	}
}
