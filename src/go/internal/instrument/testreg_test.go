package instrument

import "testing"

// testFields is a miniature ABI: two required stages, one switchable stage,
// two source types, a 2-slot generator bank with a 1-slot kick family.
var testFields = []string{
	"core_g", "fin_y", "a_enabled", "a_x", "osc_enabled", "osc_type", "fm_r",
	"gen1_source", "gen1_gain", "gen1_kv", "gen1_kc", "kick_on",
	"gen2_source", "gen2_gain", "noise_seed",
}

func testDecls() []ModuleTypeDecl {
	return []ModuleTypeDecl{
		{Type: "core", Version: 1, Kind: KindStage, Required: true, Values: []ValueDecl{{Name: "core_g", Init: 1}}},
		{Type: "fin", Version: 1, Kind: KindStage, Required: true, Values: []ValueDecl{{Name: "fin_y", Init: 0.5}}},
		{Type: "a", Version: 1, Kind: KindStage, Enable: &FieldSet{"a_enabled", 1}, Values: []ValueDecl{{Name: "a_x", Init: 8}}},
		{Type: "osc.wave", Version: 1, Kind: KindSource, Enable: &FieldSet{"osc_enabled", 1},
			Values: []ValueDecl{{Name: "osc_type", Init: 0, Allowed: []float64{0, 1}}}},
		{Type: "osc.fm", Version: 1, Kind: KindSource, Enable: &FieldSet{"osc_enabled", 1},
			Set: []FieldSet{{"osc_type", 4}}, Values: []ValueDecl{{Name: "fm_r", Init: 1}}},
		{Type: "tone", Version: 1, Kind: KindLayer, Enable: &FieldSet{"source", 1}, Values: []ValueDecl{{Name: "gain", Init: 1}}},
		{Type: "kick.a", Version: 1, Kind: KindLayer, Enable: &FieldSet{"source", 5}, Set: []FieldSet{{"kv", 0}},
			Global: []FieldSet{{"kick_on", 1}}, Capacity: 1, Values: []ValueDecl{{Name: "kc", Init: 0.3}}},
		{Type: "kick.b", Version: 1, Kind: KindLayer, Enable: &FieldSet{"source", 5}, Set: []FieldSet{{"kv", 1}},
			Global: []FieldSet{{"kick_on", 1}}, Capacity: 1, Values: []ValueDecl{{Name: "kc", Init: 0.3}}},
		{Type: "fx.filter", Version: 1, Kind: KindFX, FreeForm: true},
	}
}

func testRegistry(t *testing.T) *Registry {
	t.Helper()
	r, err := NewRegistry(testDecls(), testFields, 2)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// minimalDoc is the smallest valid document of the test registry.
func minimalDoc() Doc {
	return Doc{
		ID: "t", Name: "T", Category: "test",
		Render: Render{Seconds: 0.5, Amplitude: 0.8},
		Stages: map[string]Stage{
			"core": {V: 1, On: true, Values: Values{"core_g": 1}},
			"fin":  {V: 1, On: true, Values: Values{"fin_y": 0.5}},
		},
	}
}
