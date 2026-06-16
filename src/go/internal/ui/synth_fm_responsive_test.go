//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// synth_fm_responsive_test.go — proves conceptFM (the FM-group concept picture)
// responds to EVERY FM knob the recipe exposes, not just per-operator DEPTH.
// The old conceptFM drew only depth bars (fmDepths), so ratio / level /
// algorithm turned nothing. The enhanced renderer draws the actual FM OUTPUT
// WAVEFORM with the FM stage forced audible, so each knob's effect is visible.

// findParamDef returns a pointer to the ParamDef named name within reg, or nil
// when the recipe does not expose that knob.
func findParamDef(reg *audio.RecipeRegistration, name string) *audio.ParamDef {
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

// fmConceptFingerprint renders conceptFM for instID with knob `name` set to v
// and returns a deterministic fingerprint of the drawn ink (rect count folded
// with rect coordinates). Two different fingerprints ⇒ the picture changed.
func fmConceptFingerprint(t *testing.T, instID, name string, v float64) int {
	t.Helper()
	audio.ResetInstrumentParams(instID)
	audio.SetInstrumentParam(instID, name, v)
	rect := image.Rect(0, 0, 260, 110)
	rects := collectFilledRects(t, func() {
		conceptFM(ebiten.NewImage(260, 110), rect, instID,
			audio.ParamDef{Name: name, Group: "fm"}, nil)
	})
	h := len(rects)
	for _, r := range rects {
		h = h*31 + r.Rect.Min.Y + r.Rect.Max.X*7 + r.Rect.Min.X*3 + r.Rect.Max.Y
	}
	return h
}

// assertKnobChangesPicture asserts conceptFM differs at the knob's Min vs Max
// for every named knob the recipe actually has (genuinely-absent knobs skip).
func assertKnobChangesPicture(t *testing.T, instID string, names []string) {
	t.Helper()
	reg := audio.RecipeRegistrations()[audio.RecipeForInstrument(instID)]
	if reg == nil {
		t.Fatalf("no recipe registration for %s", instID)
	}
	for _, name := range names {
		d := findParamDef(reg, name)
		if d == nil {
			t.Logf("recipe %s lacks %s — skip (genuinely absent)", reg.ID, name)
			continue
		}
		lo := fmConceptFingerprint(t, instID, name, d.Min)
		hi := fmConceptFingerprint(t, instID, name, d.Max)
		if lo == hi {
			t.Errorf("conceptFM identical at Min(%v)/Max(%v) for %s — knob not reflected in picture",
				d.Min, d.Max, name)
		}
	}
}

// TestConceptFM_RespondsToRatioLevelAlgorithm — the modular FM voice exposes
// the full operator surface; conceptFM must reflect ratio, level, algorithm and
// depth.
func TestConceptFM_RespondsToRatioLevelAlgorithm(t *testing.T) {
	inst := "fm-resp-modular"
	bindModularConceptInst(t, inst)
	assertKnobChangesPicture(t, inst, []string{
		"fm_op2_ratio", "fm_op2_level", "fm_algorithm", "fm_op2_depth",
	})
}

// TestConceptFM_RespondsToBespokeFMKnobs — a bespoke FM recipe (fm-bell) lacks
// algorithm/level but exposes op ratio + depth; those must still drive the
// picture. fm-lead carries op3 ratio/depth too.
func TestConceptFM_RespondsToBespokeFMKnobs(t *testing.T) {
	bell := "fm-resp-bell"
	audio.BindInstrumentToRecipe(bell, "fm-bell")
	t.Cleanup(func() { audio.ResetInstrumentParams(bell) })
	assertKnobChangesPicture(t, bell, []string{
		"fm_op2_ratio", "fm_op2_depth", "fm_algorithm",
	})

	lead := "fm-resp-lead"
	audio.BindInstrumentToRecipe(lead, "fm-lead")
	t.Cleanup(func() { audio.ResetInstrumentParams(lead) })
	assertKnobChangesPicture(t, lead, []string{
		"fm_op3_ratio", "fm_op3_depth",
	})
}
