package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

func TestScaleFromParamDefMarksDiscrete(t *testing.T) {
	enum := audio.ParamDef{Name: "osc_type", Min: 0, Max: 3, Enum: []string{"Sine", "Saw", "Square", "Tri"}}
	sc, discrete, endless := scaleFromParamDef(enum)
	if !discrete || endless {
		t.Fatalf("enum must be discrete, not endless (d=%v e=%v)", discrete, endless)
	}
	if len(sc.Enum) != 4 {
		t.Fatalf("scale enum len=%d", len(sc.Enum))
	}
	cont := audio.ParamDef{Name: "filter_cutoff", Min: 20, Max: 20000, Unit: "Hz"}
	sc2, d2, e2 := scaleFromParamDef(cont)
	if d2 || !e2 {
		t.Fatalf("continuous must be endless, not discrete (d=%v e=%v)", d2, e2)
	}
	if sc2.Unit != "Hz" || sc2.Max != 20000 || sc2.Min != 20 {
		t.Fatalf("scale not carried: %+v", sc2)
	}
	oct := audio.ParamDef{Name: "osc_octave", Min: -2, Max: 2, Step: 1}
	scO, d3, e3 := scaleFromParamDef(oct)
	if !d3 || e3 {
		t.Fatalf("Step>0 must be discrete (d=%v e=%v)", d3, e3)
	}
	if scO.Step != 1 {
		t.Fatalf("step not carried: %+v", scO)
	}
}
