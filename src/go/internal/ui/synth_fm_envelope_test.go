//go:build test

package ui

import (
	"image"
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// fmEnvFingerprint renders conceptFMEnvelope for inst with knob `name`=v and
// returns a hash of the painted rects (changes when the picture changes).
func fmEnvFingerprint(t *testing.T, inst, name string, v float64) int {
	t.Helper()
	audio.ResetInstrumentParams(inst)
	audio.SetInstrumentParam(inst, name, v)
	def := audio.ParamDef{Name: name, Group: "fm"}
	rects := collectFilledRects(t, func() {
		conceptFMEnvelope(ebiten.NewImage(260, 110), image.Rect(0, 0, 260, 110), inst, def, nil)
	})
	h := len(rects)
	for _, r := range rects {
		h = h*31 + r.Rect.Min.Y + r.Rect.Max.X*7
	}
	return h
}

// fmFindParam returns the matching ParamDef pointer from a recipe registration.
func fmFindParam(reg *audio.RecipeRegistration, name string) *audio.ParamDef {
	if reg == nil {
		return nil
	}
	for i := range reg.Params {
		if reg.Params[i].Name == name {
			return &reg.Params[i]
		}
	}
	return nil
}

// assertFMEnvResponds binds inst to recipeID and, for every knob the recipe
// actually exposes, asserts conceptFMEnvelope paints a DIFFERENT picture at the
// knob's Min vs Max. It returns the count of knobs it actually exercised so the
// caller can assert at least one decay knob and one pitch-env knob were hit.
func assertFMEnvResponds(t *testing.T, inst, recipeID string, names []string) int {
	t.Helper()
	audio.BindInstrumentToRecipe(inst, recipeID)
	t.Cleanup(func() { audio.ResetInstrumentParams(inst) })
	reg := audio.RecipeRegistrations()[audio.RecipeForInstrument(inst)]
	exercised := 0
	for _, name := range names {
		d := fmFindParam(reg, name)
		if d == nil {
			t.Logf("recipe %s lacks %s, skipping", recipeID, name)
			continue
		}
		if fmEnvFingerprint(t, inst, name, d.Min) == fmEnvFingerprint(t, inst, name, d.Max) {
			t.Errorf("conceptFMEnvelope identical at Min(%g)/Max(%g) for %s on %s — not reflected",
				d.Min, d.Max, name, recipeID)
		}
		exercised++
	}
	return exercised
}

func TestConceptFMEnvelope_RespondsToDecayAndPitchEnv(t *testing.T) {
	// Modular recipe exposes the bespoke naming (fm_dec2 / fm_pe_amt / fm_pe_decay).
	modDecay := assertFMEnvResponds(t, "fm-env-modular", "synth-modular", []string{"fm_dec2"})
	modPenv := assertFMEnvResponds(t, "fm-env-modular-pe", "synth-modular", []string{"fm_pe_amt", "fm_pe_decay"})

	// fm-bass (a bespoke wired FM recipe) exposes the canonical naming
	// (fm_op2_decay / fm_pitch_env_amount / fm_pitch_env_decay).
	bassDecay := assertFMEnvResponds(t, "fm-env-bass-d", "fm-bass", []string{"fm_op2_decay"})
	bassPenv := assertFMEnvResponds(t, "fm-env-bass-p", "fm-bass",
		[]string{"fm_pitch_env_amount", "fm_pitch_env_decay"})

	if modDecay+bassDecay == 0 {
		t.Fatal("no DECAY knob exercised on any recipe — test would be vacuous")
	}
	if modPenv+bassPenv == 0 {
		t.Fatal("no PITCH-ENV knob exercised on any recipe — test would be vacuous")
	}
}

func TestSynthFocusRendererForKnob_FMRouting(t *testing.T) {
	cases := []struct {
		name string
		want conceptRenderer
	}{
		{"fm_op2_decay", conceptFMEnvelope},
		{"fm_pitch_env_amount", conceptFMEnvelope},
		{"fm_pitch_env_decay", conceptFMEnvelope},
		{"fm_dec2", conceptFMEnvelope},
		{"fm_pe_amt", conceptFMEnvelope},
		{"fm_base_freq", conceptPitchWave},
		{"fm_base", conceptPitchWave},
		{"fm_op2_ratio", conceptFM}, // unchanged: timbre → group routing
	}
	for _, c := range cases {
		got := synthFocusRendererForKnob(audio.ParamDef{Name: c.name, Group: "fm"})
		if rendererName(got) != rendererName(c.want) {
			t.Errorf("%s routed to %s, want %s", c.name, rendererName(got), rendererName(c.want))
		}
	}
}

func TestFMPitchEnvSamples_ModestAmountIsLegibleAndMonotonic(t *testing.T) {
	const maxAmt, win, n = 24.0, 0.4, 128
	peak := func(amt float64) float64 {
		s := fmPitchEnvSamples(amt, 0.06, maxAmt, win, n)
		p := 0.0
		for _, v := range s {
			if av := math.Abs(v); av > p {
				p = av
			}
		}
		return p
	}
	// A modest amount (3 of 24) must produce a clearly-visible peak (>= 0.30 of
	// the band), not the raw-linear 0.125.
	if got := peak(3); got < 0.30 {
		t.Fatalf("modest pitch-env peak = %.3f, want >= 0.30 (legible)", got)
	}
	// Still monotonic: a larger amount is taller; full scale reaches ~1.
	if peak(12) <= peak(3) {
		t.Fatalf("pitch-env not monotonic in amount: peak(12)=%.3f peak(3)=%.3f", peak(12), peak(3))
	}
	if peak(24) < 0.95 {
		t.Fatalf("full-scale pitch-env peak = %.3f, want ~1.0", peak(24))
	}
}
