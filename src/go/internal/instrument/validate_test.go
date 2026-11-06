package instrument

import (
	"math"
	"strings"
	"testing"
)

func kickLayer(typ string) *Module {
	return &Module{Type: typ, V: 1, On: true, Values: Values{"kc": 0.3}}
}

func TestValidateAcceptsMinimalAndFull(t *testing.T) {
	r := testRegistry(t)
	if err := Validate(minimalDoc(), r); err != nil {
		t.Fatal(err)
	}
	d := minimalDoc()
	d.Stages["a"] = Stage{V: 1, On: false, Values: Values{"a_x": 3}}
	d.Source = &Module{Type: "osc.fm", V: 1, On: true, Values: Values{"fm_r": 2}}
	d.Layers = []*Module{kickLayer("kick.b"), {Type: "tone", V: 1, On: true, Values: Values{"gain": 1}}}
	d.FX = []Module{{Type: "fx.filter", V: 1, On: true, Values: Values{"anything": 1}}}
	if err := Validate(d, r); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRejects(t *testing.T) {
	r := testRegistry(t)
	cases := []struct {
		name string
		edit func(*Doc)
		want string
	}{
		{"missing required", func(d *Doc) { delete(d.Stages, "fin") }, "stages.fin: required"},
		{"required off", func(d *Doc) { s := d.Stages["core"]; s.On = false; d.Stages["core"] = s }, "cannot be off"},
		{"unknown stage", func(d *Doc) { d.Stages["zz"] = Stage{V: 1, On: true} }, "stages.zz: unknown"},
		{"missing value", func(d *Doc) { d.Stages["a"] = Stage{V: 1, On: true, Values: Values{}} }, "stages.a.values.a_x: missing"},
		{"extra value", func(d *Doc) { d.Stages["core"].Values["zz"] = 1 }, "zz: not a value of core"},
		{"non-finite", func(d *Doc) { d.Stages["core"].Values["core_g"] = math.NaN() }, "core_g: not finite"},
		{"not allowed", func(d *Doc) {
			d.Source = &Module{Type: "osc.wave", V: 1, On: true, Values: Values{"osc_type": 3}}
		}, "osc_type: 3 not in"},
		{"wrong version", func(d *Doc) { s := d.Stages["core"]; s.V = 2; d.Stages["core"] = s }, "version 2"},
		{"layer as source", func(d *Doc) { d.Source = kickLayer("kick.a") }, "unknown source type"},
		{"trailing empty slot", func(d *Doc) { d.Layers = []*Module{kickLayer("kick.a"), nil} }, "trailing empty"},
		{"zero seconds", func(d *Doc) { d.Render.Seconds = 0 }, "render.seconds"},
		{"gate out of range", func(d *Doc) { d.Render.GateTail = 1 }, "render.gate_tail"},
		{"empty id", func(d *Doc) { d.ID = "" }, "id: empty"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := minimalDoc()
			c.edit(&d)
			err := Validate(d, r)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want error containing %q, got %v", c.want, err)
			}
		})
	}
}

// Review Focus 1: a kick in slot 2 renders silence in the C engine.
func TestValidateRejectsLayerPastCapacity(t *testing.T) {
	d := minimalDoc()
	d.Layers = []*Module{{Type: "tone", V: 1, On: true, Values: Values{"gain": 1}}, kickLayer("kick.a")}
	err := Validate(d, testRegistry(t))
	if err == nil || !strings.Contains(err.Error(), "only in slots 1..1") {
		t.Fatalf("want a slot-limit error, got %v", err)
	}
}
