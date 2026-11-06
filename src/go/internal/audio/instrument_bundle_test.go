//go:build test

package audio

import (
	"encoding/json"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/schema"
)

func TestInstrumentBundleRoundTrip(t *testing.T) {
	in := InstrumentBundle{
		Envelope: schema.Current(),
		Format:   InstrumentBundleFormat,
		Effects:  []EffectDef{{ID: "fx1", Name: "Crunch", Type: EffectType("bitcrusher"), Params: map[string]float64{"bits": 6}, Enabled: true}},
		Instruments: []BundleInstrument{{
			Name: "Warm 808", Kind: BundleSynth, Color: "#C87850FF",
			Synth: &instrument.Doc{ID: "user.kick.abc", Name: "Warm 808", Origin: "user.kick.abc",
				Stages: map[string]instrument.Stage{"core": {V: 1, On: true, Values: instrument.Values{"core_g": 0.4}}}},
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
	if out.Instruments[0].Synth == nil || out.Instruments[0].Synth.Stages["core"].Values["core_g"] != 0.4 {
		t.Errorf("synth document lost: %+v", out.Instruments[0].Synth)
	}
	if out.Instruments[0].EQ == nil || len(out.Instruments[0].EQ.GainsDB) != 3 {
		t.Errorf("eq lost: %+v", out.Instruments[0].EQ)
	}
}

func TestValidateBundleHeaderRejects(t *testing.T) {
	if ValidateBundleHeader(&InstrumentBundle{Envelope: schema.Current(), Format: "nope"}) == nil {
		t.Error("foreign format accepted")
	}
	if ValidateBundleHeader(&InstrumentBundle{Envelope: schema.Envelope{SchemaVersion: "99.0.0"}, Format: InstrumentBundleFormat}) == nil {
		t.Error("future version accepted")
	}
	if ValidateBundleHeader(&InstrumentBundle{Envelope: schema.Current(), Format: InstrumentBundleFormat}) != nil {
		t.Error("valid header rejected")
	}
	// A bundle without schema_version is not a bundle we can read.
	if ValidateBundleHeader(&InstrumentBundle{Format: InstrumentBundleFormat}) == nil {
		t.Error("unversioned bundle accepted")
	}
}
