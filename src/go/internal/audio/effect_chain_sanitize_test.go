package audio

import (
	"math"
	"testing"
)

// SetInsertEffects is the project-import boundary for insert chains: a
// hand-edited or corrupted beatmo.json must not poison the processors (C
// effects on native/worklet receive these values raw). Mirrors the synth
// param boundary (sanitizeParamValue in synth_recipe.go): out-of-range
// values clamp to the registry's [Min,Max]; non-finite values fall back to
// the registry default; unknown effect types pass through untouched
// (forward-compat — they already render as passthrough processors).
func TestSetInsertEffectsSanitizesParams(t *testing.T) {
	t.Cleanup(ClearAllInsertEffects)

	regs := EffectRegistrations()
	dist, ok := regs["distortion"]
	if !ok || len(dist.Params) < 3 {
		t.Fatal("distortion not registered with >=3 params")
	}
	p0, p1, p2 := dist.Params[0], dist.Params[1], dist.Params[2]

	SetInsertEffects("snare", []EffectSlot{
		{Type: "distortion", Enabled: true, Params: map[string]float64{
			p0.Name: p0.Max + 1e9, // far above max → clamp to Max
			p1.Name: p1.Min - 50,  // below min → clamp to Min
			p2.Name: math.NaN(),   // non-finite → registry default
			"x-new": 0.5,          // unknown key on known type → kept as-is
		}},
		{Type: "totally-unknown-fx", Enabled: false, Params: map[string]float64{"weird": 123}},
	})

	got := GetInsertEffects("snare")
	if len(got) != 2 {
		t.Fatalf("chain length: got %d want 2 (unknown types must be kept for forward-compat)", len(got))
	}
	if v := got[0].Params[p0.Name]; v != p0.Max {
		t.Errorf("%s: got %v want clamped to Max %v", p0.Name, v, p0.Max)
	}
	if v := got[0].Params[p1.Name]; v != p1.Min {
		t.Errorf("%s: got %v want clamped to Min %v", p1.Name, v, p1.Min)
	}
	if v := got[0].Params[p2.Name]; math.IsNaN(v) || math.IsInf(v, 0) {
		t.Errorf("%s: non-finite value survived sanitization: %v", p2.Name, v)
	} else if v != p2.Default {
		t.Errorf("%s: got %v want registry default %v for non-finite input", p2.Name, v, p2.Default)
	}
	if v := got[0].Params["x-new"]; v != 0.5 {
		t.Errorf("unknown finite key x-new: got %v want 0.5 (kept for forward-compat)", v)
	}
	if got[1].Type != "totally-unknown-fx" || got[1].Enabled || got[1].Params["weird"] != 123 {
		t.Errorf("unknown effect type slot mangled: %+v", got[1])
	}
}
