package instrument

import (
	"errors"
	"testing"
)

func TestSetValueIsImmutableAndChecked(t *testing.T) {
	r := testRegistry(t)
	a := minimalDoc()
	b, err := SetValue(a, r, StageRef("core"), "core_g", 3)
	if err != nil || b.Stages["core"].Values["core_g"] != 3 || a.Stages["core"].Values["core_g"] != 1 {
		t.Fatalf("SetValue: err=%v a=%v b=%v", err, a.Stages["core"], b.Stages["core"])
	}
	if _, err := SetValue(a, r, StageRef("core"), "nope", 1); !errors.Is(err, ErrNotDeclared) {
		t.Fatalf("want ErrNotDeclared, got %v", err)
	}
	if _, err := SetValue(a, r, StageRef("a"), "a_x", 1); !errors.Is(err, ErrNoModule) {
		t.Fatalf("want ErrNoModule for an absent stage, got %v", err)
	}
}

func TestSetOnNeedsASwitch(t *testing.T) {
	r := testRegistry(t)
	if _, err := SetOn(minimalDoc(), r, StageRef("core"), false); !errors.Is(err, ErrNoSwitch) {
		t.Fatalf("want ErrNoSwitch, got %v", err)
	}
}

// Review Focus 2.
func TestSetFieldMaterialisesAbsentStageOff(t *testing.T) {
	r := testRegistry(t)
	d, err := SetField(minimalDoc(), r, "a_x", 5)
	if err != nil {
		t.Fatal(err)
	}
	st, ok := d.Stages["a"]
	if !ok || st.On || st.Values["a_x"] != 5 {
		t.Fatalf("stage a = %+v (present %v)", st, ok)
	}
	if blockMap(r, Flatten(d, r))["a_x"] != 0 {
		t.Fatal("an OFF materialised stage must not reach the engine")
	}
	d, _ = SetField(d, r, "a_enabled", 1)
	if !d.Stages["a"].On {
		t.Fatal("a_enabled=1 must switch the stage on")
	}
}

func TestSetFieldSwitchesSourceType(t *testing.T) {
	r := testRegistry(t)
	d, err := SetField(minimalDoc(), r, "osc_enabled", 1) // absent → first source type, on
	if err != nil || d.Source == nil || d.Source.Type != "osc.wave" || !d.Source.On {
		t.Fatalf("osc_enabled: %+v %v", d.Source, err)
	}
	d, _ = SetField(d, r, "osc_type", 1)
	if d.Source.Type != "osc.wave" || d.Source.Values["osc_type"] != 1 {
		t.Fatalf("osc_type 1 stays a wave value: %+v", d.Source)
	}
	d, err = SetField(d, r, "osc_type", 4)
	if err != nil || d.Source.Type != "osc.fm" || d.Source.Values["fm_r"] != 1 || !d.Source.On {
		t.Fatalf("osc_type 4 must switch to osc.fm from Init, keeping on: %+v %v", d.Source, err)
	}
	if _, err := SetField(d, r, "osc_type", 9); !errors.Is(err, ErrNotAllowed) {
		t.Fatalf("want ErrNotAllowed, got %v", err)
	}
}

func TestSetFieldOnLayers(t *testing.T) {
	r := testRegistry(t)
	d, err := SetField(minimalDoc(), r, "gen1_source", 5) // kick family: first declared variant
	if err != nil || d.Layers[0].Type != "kick.a" || !d.Layers[0].On {
		t.Fatalf("gen1_source=5: %+v %v", d.Layers, err)
	}
	d, _ = SetField(d, r, "gen1_kc", 0.7)
	d, err = SetField(d, r, "gen1_kv", 1) // a variant selector switches type, carrying kc
	if err != nil || d.Layers[0].Type != "kick.b" || d.Layers[0].Values["kc"] != 0.7 {
		t.Fatalf("gen1_kv=1: %+v %v", d.Layers[0], err)
	}
	d, _ = SetField(d, r, "gen1_source", 0)
	if d.Layers[0].On {
		t.Fatal("gen1_source=0 switches the layer off")
	}
	if _, err := SetField(d, r, "gen2_gain", 1); !errors.Is(err, ErrInactive) {
		t.Fatalf("a value of an empty slot is inactive, got %v", err)
	}
	if _, err := SetField(d, r, "kick_on", 0); !errors.Is(err, ErrNotSettable) {
		t.Fatalf("a Global constant is not settable, got %v", err)
	}
	if _, err := SetField(d, r, "zzz", 0); !errors.Is(err, ErrUnknownField) {
		t.Fatalf("want ErrUnknownField, got %v", err)
	}
}

// Review Focus 1.
func TestSwitchTypeRespectsCapacity(t *testing.T) {
	r := testRegistry(t)
	d := minimalDoc()
	d.Layers = []*Module{{Type: "tone", V: 1, On: true, Values: Values{"gain": 1}}, {Type: "tone", V: 1, On: true, Values: Values{"gain": 1}}}
	if _, err := SwitchType(d, r, LayerRef(1), "kick.a"); !errors.Is(err, ErrCapacity) {
		t.Fatalf("want ErrCapacity, got %v", err)
	}
}

// Review Focus 1.
func TestCopyModulesRespectsCapacity(t *testing.T) {
	r := testRegistry(t)
	src := minimalDoc()
	src.Layers = []*Module{kickLayer("kick.a")}
	src.Stages["a"] = Stage{V: 1, On: true, Values: Values{"a_x": 2}}
	dst, err := CopyModules(minimalDoc(), src, r, []Ref{StageRef("a"), LayerRef(0)})
	if err != nil || dst.Stages["a"].Values["a_x"] != 2 || dst.Layers[0].Type != "kick.a" {
		t.Fatalf("copy: %+v %v", dst, err)
	}
	if err := Validate(dst, r); err != nil {
		t.Fatal(err)
	}
	src.Layers = []*Module{{Type: "tone", V: 1, On: true, Values: Values{"gain": 1}}, kickLayer("kick.a")}
	if _, err := CopyModules(minimalDoc(), src, r, []Ref{LayerRef(1)}); !errors.Is(err, ErrCapacity) {
		t.Fatalf("want ErrCapacity, got %v", err)
	}
}

func TestRemoveRequiredStageFails(t *testing.T) {
	if _, err := RemoveModule(minimalDoc(), testRegistry(t), StageRef("core")); err == nil {
		t.Fatal("removing a required stage must fail")
	}
}
