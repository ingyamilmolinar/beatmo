//go:build !test && !js

package audio

import (
	"strings"
	"testing"
)

// Essential is the default (Tier ""); only the listed fine-tuning knobs collapse under the stage's More.
func TestP4_TierClassification(t *testing.T) {
	defs := ModularSynthParamDefs()
	byName := map[string]ParamDef{}
	for _, d := range defs {
		byName[d.Name] = d
	}

	wantAdvanced := []string{
		"fm_op1_depth", "fm_op4_level", "burst2_off", "burst4_amp",
		"gen1_kick_mode_gain", "gen1_kick_reverb",
	}
	for _, n := range wantAdvanced {
		if byName[n].Tier != TierAdvanced {
			t.Errorf("%q should be Advanced (Tier=%q), got %q", n, TierAdvanced, byName[n].Tier)
		}
	}

	// Character knobs must stay Essential (always visible).
	wantEssential := []string{
		"osc_type", "amp_attack", "amp_decay", "filter_cutoff", "filter_resonance",
		"fm_algorithm", "fm_op1_ratio", "burst_sharp", "burst1_off",
	}
	for _, n := range wantEssential {
		if d, ok := byName[n]; ok && d.Tier != TierEssential {
			t.Errorf("%q should be Essential (Tier=%q), got %q", n, TierEssential, d.Tier)
		}
	}

	// The Advanced set stays small. Contextual knobs (physical-model oscillators, the modal plate, table-gated stages) are
	// excluded from the cap: hidden until their context is selected, they never crowd the default view.
	adv := 0
	for _, d := range defs {
		if d.Tier != TierAdvanced {
			continue
		}
		if strings.HasPrefix(d.Name, "osc_sax_") || strings.HasPrefix(d.Name, "osc_bow_") {
			continue
		}
		if f, ok := strings.CutPrefix(d.Name, "gen1_"); ok && IsCymbalModalField(f) {
			continue
		}
		// The WAV-clone stages are table-gated: their knobs exist only on a
		// document with mode rows (tableGatedStages).
		if _, gated := tableGatedFields()[d.Name]; gated {
			continue
		}
		adv++
	}
	if adv == 0 {
		t.Error("no Advanced params — tiering classification did not apply")
	}
	if adv > 40 {
		t.Errorf("Advanced set too large (%d) — tiering should hide only fine-tuning, not the bulk", adv)
	}
}
