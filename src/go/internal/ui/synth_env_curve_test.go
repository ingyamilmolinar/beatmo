//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// envCurveFingerprint renders conceptEnvelope for instID into a fresh image and
// returns a hash of the painted rects, so a changed ADSR picture changes the hash.
func envCurveFingerprint(t *testing.T, instID string, def audio.ParamDef) int {
	t.Helper()
	rect := image.Rect(0, 0, 260, 110)
	rects := collectFilledRects(t, func() {
		conceptEnvelope(ebiten.NewImage(260, 110), rect, instID, def, nil)
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

// fingerprintForKnob sets one env knob to a value and fingerprints conceptEnvelope.
func fingerprintForKnob(t *testing.T, instID, knob string, val float64) int {
	t.Helper()
	audio.ResetInstrumentParams(instID)
	audio.SetInstrumentParam(instID, knob, val)
	def := envParamDef(t, knob)
	return envCurveFingerprint(t, instID, def)
}

func envParamDef(t *testing.T, knob string) audio.ParamDef {
	t.Helper()
	reg := audio.RecipeRegistrations()["synth-modular"]
	if reg == nil {
		t.Fatal("synth-modular recipe missing")
	}
	for _, d := range reg.Params {
		if d.Name == knob {
			return d
		}
	}
	t.Fatalf("env knob %s not found in synth-modular", knob)
	return audio.ParamDef{}
}

// TestConceptEnvelope_AmpCurveResponsive proves conceptEnvelope reflects the
// amp_curve knob: the ADSR ramps bend between linear (Min) and exponential (Max),
// so the rendered picture differs.
func TestConceptEnvelope_AmpCurveResponsive(t *testing.T) {
	id := "env-curve-test"
	audio.BindInstrumentToRecipe(id, "synth-modular")
	t.Cleanup(func() { audio.ResetInstrumentParams(id) })

	def := envParamDef(t, "amp_curve")
	// Use audible attack/decay/release so the ramps have visible slopes to bend.
	set := func(curve float64) int {
		audio.ResetInstrumentParams(id)
		audio.SetInstrumentParam(id, "amp_attack", 0.5)
		audio.SetInstrumentParam(id, "amp_decay", 1.0)
		audio.SetInstrumentParam(id, "amp_sustain", 0.4)
		audio.SetInstrumentParam(id, "amp_release", 1.0)
		audio.SetInstrumentParam(id, "amp_curve", curve)
		return envCurveFingerprint(t, id, def)
	}
	lin := set(def.Min)
	exp := set(def.Max)
	if lin == exp {
		t.Errorf("conceptEnvelope identical at amp_curve Min(%g, linear) and Max(%g, exponential) — "+
			"the curve flag does not bend the ADSR ramps", def.Min, def.Max)
	}
}

// TestConceptEnvelope_TimingKnobsStillResponsive guards against regressing the
// attack/decay/sustain/release responsiveness while adding amp_curve shaping.
func TestConceptEnvelope_TimingKnobsStillResponsive(t *testing.T) {
	id := "env-timing-test"
	audio.BindInstrumentToRecipe(id, "synth-modular")
	t.Cleanup(func() { audio.ResetInstrumentParams(id) })

	for _, knob := range []string{"amp_attack", "amp_decay", "amp_sustain", "amp_release"} {
		def := envParamDef(t, knob)
		lo := fingerprintForKnob(t, id, knob, def.Min)
		hi := fingerprintForKnob(t, id, knob, def.Max)
		if lo == hi {
			t.Errorf("conceptEnvelope identical at %s Min(%g) and Max(%g) — regression", knob, def.Min, def.Max)
		}
	}
}
