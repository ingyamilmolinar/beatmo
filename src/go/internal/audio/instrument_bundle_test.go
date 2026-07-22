//go:build test

package audio

import (
	"encoding/json"
	"testing"
)

func TestInstrumentBundleRoundTrip(t *testing.T) {
	in := InstrumentBundle{
		Format:  InstrumentBundleFormat,
		Version: InstrumentBundleVersion,
		Effects: []EffectDef{{ID: "fx1", Name: "Crunch", Type: EffectType("bitcrusher"), Params: map[string]float64{"bits": 6}, Enabled: true}},
		Instruments: []InstrumentDoc{{
			Name: "Warm 808", Kind: "synth", Color: "#C87850FF",
			Recipe:      RecipeDoc{ID: "user.kick.abc", BaseRecipe: "drum-kick", Origin: OriginUser},
			SynthParams: map[string]float64{"tune": 0.4},
			EffectChain: []string{"fx1", "fx1"},
			EQ:          &InstrumentEQ{GainsDB: []float64{0, 0, 3}},
		}},
	}
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out InstrumentBundle
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if err := ValidateBundleHeader(&out); err != nil {
		t.Fatalf("valid header rejected: %v", err)
	}
	if len(out.Instruments[0].EffectChain) != 2 || out.Instruments[0].EffectChain[0] != "fx1" {
		t.Errorf("effect chain not preserved: %v", out.Instruments[0].EffectChain)
	}
	if out.Instruments[0].Recipe.BaseRecipe != "drum-kick" {
		t.Errorf("base recipe lost: %q", out.Instruments[0].Recipe.BaseRecipe)
	}
	if out.Instruments[0].EQ == nil || len(out.Instruments[0].EQ.GainsDB) != 3 {
		t.Errorf("eq lost: %+v", out.Instruments[0].EQ)
	}
}

func TestValidateBundleHeaderRejects(t *testing.T) {
	if ValidateBundleHeader(&InstrumentBundle{Format: "nope", Version: 1}) == nil {
		t.Error("foreign format accepted")
	}
	if ValidateBundleHeader(&InstrumentBundle{Format: InstrumentBundleFormat, Version: InstrumentBundleVersion + 1}) == nil {
		t.Error("future version accepted")
	}
	if ValidateBundleHeader(&InstrumentBundle{Format: InstrumentBundleFormat, Version: 1}) != nil {
		t.Error("valid header rejected")
	}
}
