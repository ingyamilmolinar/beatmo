package instrument

import "testing"

func blockMap(r *Registry, b []float64) map[string]float64 {
	out := map[string]float64{}
	for i, f := range r.Fields() {
		if b[i] != 0 {
			out[f] = b[i]
		}
	}
	return out
}

func TestFlattenWritesOnlyPresentOnModules(t *testing.T) {
	r := testRegistry(t)
	d := minimalDoc()
	d.Stages["a"] = Stage{V: 1, On: false, Values: Values{"a_x": 3}} // off: writes nothing
	d.Source = &Module{Type: "osc.fm", V: 1, On: true, Values: Values{"fm_r": 2}}
	d.Layers = []*Module{kickLayer("kick.b")}
	got := blockMap(r, Flatten(d, r))
	want := map[string]float64{
		"core_g": 1, "fin_y": 0.5,
		"osc_enabled": 1, "osc_type": 4, "fm_r": 2,
		"gen1_source": 5, "gen1_kv": 1, "gen1_kc": 0.3, "kick_on": 1,
	}
	if len(got) != len(want) {
		t.Fatalf("Flatten = %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("Flatten[%s] = %v, want %v (all: %v)", k, got[k], v, got)
		}
	}
}

func TestOwnedFieldsReportsOffModulesWithEnableZero(t *testing.T) {
	r := testRegistry(t)
	d := minimalDoc()
	d.Stages["a"] = Stage{V: 1, On: false, Values: Values{"a_x": 3}}
	d.Layers = []*Module{{Type: "kick.a", V: 1, On: false, Values: Values{"kc": 0.9}}}
	o := OwnedFields(d, r)
	if o["a_enabled"] != 0 || o["a_x"] != 3 {
		t.Fatalf("off stage: %v", o)
	}
	if v, ok := o["gen1_source"]; !ok || v != 0 {
		t.Fatalf("off layer must own its switch at 0: %v", o)
	}
	if o["gen1_kc"] != 0.9 || o["gen1_kv"] != 0 || o["kick_on"] != 1 {
		t.Fatalf("off layer keeps values + type constants: %v", o)
	}
}

func TestFieldValuesFillsInitThenOverlays(t *testing.T) {
	r := testRegistry(t)
	d := minimalDoc()
	d.Stages["core"].Values["core_g"] = 9
	fv := FieldValues(d, r)
	if fv["core_g"] != 9 || fv["a_x"] != 8 || fv["a_enabled"] != 0 || fv["gen2_gain"] != 1 || fv["noise_seed"] != 0 {
		t.Fatalf("FieldValues = %v", fv)
	}
	if len(fv) != len(testFields) {
		t.Fatalf("FieldValues must cover every ABI field: %d vs %d", len(fv), len(testFields))
	}
}
