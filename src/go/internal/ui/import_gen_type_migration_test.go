package ui

import (
	"encoding/json"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// TestImport_MigratesGenTypeEraProjects locks the gen_type → Modular
// migration at the import seam: a project saved before the native-engine
// deprecation that re-voiced an instrument through the Generator selector
// (gen_type >= 1 in synth_params) must load as the Modular recipe with the
// matching osc_type — same sound, no dead key. A Native/absent gen_type is
// dropped without touching the recipe binding.
func TestImport_MigratesGenTypeEraProjects(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	t.Cleanup(func() {
		audio.BindInstrumentToRecipe("snare", "drum-snare")
		audio.ResetInstrumentParams("snare")
		audio.BindInstrumentToRecipe("kick", "drum-kick")
		audio.ResetInstrumentParams("kick")
	})

	type tInstP struct {
		Name        string             `json:"name"`
		ID          string             `json:"id"`
		Kind        string             `json:"kind"`
		Volume      float64            `json:"volume"`
		Origin      int                `json:"origin"`
		Color       string             `json:"color"`
		Recipe      string             `json:"recipe,omitempty"`
		SynthParams map[string]float64 `json:"synth_params,omitempty"`
	}
	f := struct {
		Version     int      `json:"version"`
		Subdiv      int      `json:"subdiv"`
		BPM         int      `json:"bpm"`
		Instruments []tInstP `json:"instruments"`
		Nodes       []tNode  `json:"nodes"`
	}{
		Version: 1,
		Subdiv:  32,
		BPM:     120,
		Instruments: []tInstP{
			{
				Name: "Snare", ID: "snare", Kind: "builtin", Volume: 1, Origin: 1, Color: "#C87850FF",
				Recipe:      "drum-snare",
				SynthParams: map[string]float64{"gen_type": 2, "pitch": 3}, // re-voiced Saw
			},
			{
				Name: "Kick", ID: "kick", Kind: "builtin", Volume: 1, Origin: 2, Color: "#C87850FF",
				Recipe:      "drum-kick",
				SynthParams: map[string]float64{"gen_type": 0, "drive": 0.5}, // Native
			},
		},
		Nodes: []tNode{
			{ID: 1, I: 0, J: 0, Type: "regular"},
			{ID: 2, I: 1, J: 0, Type: "regular"},
		},
	}
	b, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.Import(b); err != nil {
		t.Fatalf("import: %v", err)
	}

	// Re-voiced instrument: rebound to Modular with osc_type=gen_type-1.
	if got := audio.RecipeForInstrument("snare"); got != "synth-modular" {
		t.Errorf("snare recipe = %q, want synth-modular (gen_type 2 migrates to the Modular recipe)", got)
	}
	sp := audio.GetInstrumentParams("snare")
	if sp["osc_type"] != 1 {
		t.Errorf("snare osc_type = %v, want 1 (Saw)", sp["osc_type"])
	}
	if sp["pitch"] != 3 {
		t.Errorf("snare pitch = %v, want 3 (generic knobs carry over)", sp["pitch"])
	}
	if _, ok := sp["gen_type"]; ok {
		t.Error("snare params still carry the dead gen_type key")
	}

	// Native instrument: binding untouched, key dropped, knobs kept.
	if got := audio.RecipeForInstrument("kick"); got != "drum-kick" {
		t.Errorf("kick recipe = %q, want drum-kick (Native gen_type is a no-op)", got)
	}
	kp := audio.GetInstrumentParams("kick")
	if kp["drive"] != 0.5 {
		t.Errorf("kick drive = %v, want 0.5", kp["drive"])
	}
	if _, ok := kp["gen_type"]; ok {
		t.Error("kick params still carry the dead gen_type key")
	}
}
