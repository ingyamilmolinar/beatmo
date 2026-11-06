package audio

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

func TestInstrumentParamDefsDefaultIsTheDocumentValue(t *testing.T) {
	resetLibraryForTest(t)
	defs := InstrumentParamDefs(roster.KickSnap)
	for _, d := range defs {
		if d.Name == "gen1_kick_click" && d.Default != 0.45 {
			t.Fatalf("Default = %v, want the document's 0.45", d.Default)
		}
		if d.Default < d.Min || d.Default > d.Max {
			t.Fatalf("%s: Default %v outside [%v,%v] — the range must widen", d.Name, d.Default, d.Min, d.Max)
		}
	}
	if len(defs) != len(ModularSynthParamDefs()) {
		t.Fatal("one ParamDef per ABI field, as the Synth tab's bindings expect")
	}
}

func TestDeclaresFieldHidesDeadKnobs(t *testing.T) {
	resetLibraryForTest(t)
	cases := []struct {
		id, field string
		want      bool
	}{
		{string(roster.KickSnap), "gen1_kick_click", true},
		{string(roster.KickSnap), "gen1_kick_h3", false}, // gen_kick_punchy never reads h3
		{string(roster.KickSnap), "gen1_kick_variant", true},
		{string(roster.KickSnap), "kick_enabled", false},
		{string(roster.KickSnap), "filter_cutoff", true}, // stages are always offered
		{string(roster.Violin), "osc_bow_pos", true},
		{string(roster.Violin), "fm_op1_ratio", false}, // FM operators belong to osc.fm
		{string(roster.Violin), "osc_type", true},
		{string(roster.Violin), "gen1_gain", true}, // the bow-hair noise layer
		{string(roster.Violin), "gen2_gain", false},
	}
	for _, c := range cases {
		if got := InstrumentDeclaresField(instrument.ID(c.id), c.field); got != c.want {
			t.Errorf("%s %s = %v, want %v", c.id, c.field, got, c.want)
		}
	}
}

func TestSetInstrumentParamEditsTheDocument(t *testing.T) {
	resetLibraryForTest(t)
	SetInstrumentParam(roster.KickSnap, "gen1_kick_click", 0.9)
	d, _ := InstrumentDoc(roster.KickSnap)
	if d.Layers[0].Values["kick_click"] != 0.9 || !InstrumentDocModified(roster.KickSnap) {
		t.Fatal("SetInstrumentParam must edit the project copy")
	}
	ResetInstrumentParams(roster.KickSnap)
	if InstrumentDocModified(roster.KickSnap) {
		t.Fatal("Reset returns the copy to its library entry")
	}
	SetInstrumentParam(roster.Violin, "fm_op1_ratio", 3) // inactive: ignored, no panic
	if v := MergedInstrumentParamsRO(roster.Violin)["fm_op1_ratio"]; v == 3 {
		t.Fatal("an inactive field must not be stored")
	}
}

// A knob range widens to the shipped value, so editing a neighbouring knob
// never clamps a tuned value (A10 decision: tuned values stand).
func TestShippedOutOfRangeValueSurvivesAnEdit(t *testing.T) {
	resetLibraryForTest(t)
	SetInstrumentParam(roster.KickAcoustic, "gen1_kick_click", 0.3)
	if v := MergedInstrumentParamsRO(roster.KickAcoustic)["gen1_kick_mode_decay"]; v != 45 {
		t.Fatalf("mode_decay = %v, want the tuned 45", v)
	}
	SetInstrumentParam(roster.KickAcoustic, "gen1_kick_mode_decay", 45)
	if v := MergedInstrumentParamsRO(roster.KickAcoustic)["gen1_kick_mode_decay"]; v != 45 {
		t.Fatalf("re-setting the tuned value clamped it to %v", v)
	}
}
