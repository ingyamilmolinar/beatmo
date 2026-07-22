//go:build test

package audio

import "testing"

func TestNormalizeDedupsAndPreservesOrder(t *testing.T) {
	chain := []EffectSlot{
		{Type: "bitcrusher", Enabled: true, Params: map[string]float64{"bits": 6}},
		{Type: "reverb", Enabled: true, Params: map[string]float64{"mix": 0.3}},
		{Type: "bitcrusher", Enabled: true, Params: map[string]float64{"bits": 6}}, // dup of #0
	}
	defs, ids := NormalizeEffects(chain)
	if len(defs) != 2 {
		t.Fatalf("want 2 deduped defs, got %d (%+v)", len(defs), defs)
	}
	if len(ids) != 3 || ids[0] != ids[2] || ids[0] == ids[1] {
		t.Errorf("order/repeat not preserved: %v", ids)
	}
	back := DenormalizeEffects(defs, ids)
	if len(back) != 3 || back[0].Type != "bitcrusher" || back[1].Type != "reverb" || back[2].Type != "bitcrusher" {
		t.Errorf("round-trip mismatch: %+v", back)
	}
	if back[0].Params["bits"] != 6 || back[1].Params["mix"] != 0.3 {
		t.Errorf("params lost: %+v", back)
	}
	// Deep-copy: mutating the returned slot must not touch a def.
	back[0].Params["bits"] = 99
	if defs2, _ := NormalizeEffects(chain); defs2[0].Params["bits"] != 6 {
		t.Error("params not deep-copied on denormalize")
	}
}

func TestNormalizeDistinctEnabledOrParams(t *testing.T) {
	chain := []EffectSlot{
		{Type: "reverb", Enabled: true, Params: map[string]float64{"mix": 0.3}},
		{Type: "reverb", Enabled: false, Params: map[string]float64{"mix": 0.3}}, // differs by Enabled
		{Type: "reverb", Enabled: true, Params: map[string]float64{"mix": 0.5}},  // differs by param
	}
	defs, ids := NormalizeEffects(chain)
	if len(defs) != 3 {
		t.Fatalf("distinct slots collapsed: %d defs", len(defs))
	}
	if ids[0] == ids[1] || ids[0] == ids[2] || ids[1] == ids[2] {
		t.Errorf("distinct slots share ids: %v", ids)
	}
}

func TestDenormalizeSkipsUnknownID(t *testing.T) {
	back := DenormalizeEffects([]EffectDef{{ID: "fx1", Type: "reverb"}}, []string{"fx1", "ghost"})
	if len(back) != 1 {
		t.Errorf("unknown id not skipped: %+v", back)
	}
}

func TestNormalizeEmpty(t *testing.T) {
	defs, ids := NormalizeEffects(nil)
	if len(defs) != 0 || len(ids) != 0 {
		t.Errorf("empty chain produced %d defs / %d ids", len(defs), len(ids))
	}
}
