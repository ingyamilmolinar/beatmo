package instrument

import (
	"strings"
	"testing"
)

func TestNewRegistryAccepts(t *testing.T) {
	r := testRegistry(t)
	if kt, ok := r.Type("kick.a"); !ok || kt.Capacity != 1 {
		t.Fatalf("kick.a capacity = %+v", kt)
	}
	if tt, _ := r.Type("tone"); tt.Capacity != 2 {
		t.Fatalf("Capacity 0 must normalise to the slot count, got %d", tt.Capacity)
	}
	if v, ok := r.Init("gen2_gain"); !ok || v != 1 {
		t.Fatalf("Init(gen2_gain) = %v %v", v, ok)
	}
	if v, ok := r.Init("gen1_kc"); !ok || v != 0.3 {
		t.Fatalf("Init(gen1_kc) = %v %v", v, ok)
	}
	if _, ok := r.Init("noise_seed"); ok {
		t.Fatal("noise_seed is owned by no value, so it has no Init")
	}
	if LayerField(0, "gain") != "gen1_gain" {
		t.Fatal(LayerField(0, "gain"))
	}
}

func TestNewRegistryRejects(t *testing.T) {
	cases := []struct {
		name   string
		mutate func([]ModuleTypeDecl) []ModuleTypeDecl
		want   string
	}{
		{"duplicate type", func(d []ModuleTypeDecl) []ModuleTypeDecl { return append(d, d[0]) }, "duplicate type"},
		{"unknown field", func(d []ModuleTypeDecl) []ModuleTypeDecl { d[2].Values[0].Name = "nope"; return d }, `"nope"`},
		{"layer past ABI", func(d []ModuleTypeDecl) []ModuleTypeDecl { d[6].Capacity = 2; return d }, "gen2_kc"},
		{"steps vs version", func(d []ModuleTypeDecl) []ModuleTypeDecl { d[0].Version = 2; return d }, "steps"},
		{"required with switch", func(d []ModuleTypeDecl) []ModuleTypeDecl { d[2].Required = true; return d }, "required"},
		{"init not allowed", func(d []ModuleTypeDecl) []ModuleTypeDecl { d[3].Values[0].Init = 3; return d }, "Allowed"},
		{"set overlaps value", func(d []ModuleTypeDecl) []ModuleTypeDecl {
			d[4].Values = append(d[4].Values, ValueDecl{Name: "osc_type"})
			return d
		}, "both"},
		{"layer without switch", func(d []ModuleTypeDecl) []ModuleTypeDecl { d[5].Enable = nil; return d }, "source field"},
		{"free-form non-fx", func(d []ModuleTypeDecl) []ModuleTypeDecl { d[2].FreeForm = true; return d }, "free-form"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := NewRegistry(c.mutate(testDecls()), testFields, 2)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want error containing %q, got %v", c.want, err)
			}
		})
	}
}
