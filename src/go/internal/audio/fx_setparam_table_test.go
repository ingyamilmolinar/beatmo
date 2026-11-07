package audio

import (
	"math"
	"testing"
)

// Min, max and midpoint of every declared param plus an unknown name, one sample each for NaN/Inf; a new registered
// effect is covered automatically.
func TestEffectSetParamCatalogCoverage(t *testing.T) {
	catalog := InsertEffectCatalog()
	if len(catalog) == 0 {
		t.Fatalf("InsertEffectCatalog is empty")
	}

	for effectType, params := range catalog {
		t.Run(string(effectType), func(t *testing.T) {
			eff := NewEffectProcessor(EffectSlot{Type: effectType, Enabled: true}, 48000)
			if eff == nil {
				t.Fatalf("NewEffectProcessor returned nil for %q", effectType)
			}

			assertFiniteOutput := func(stage string) {
				t.Helper()
				out := eff.ProcessSample(0.1)
				if math.IsNaN(out) || math.IsInf(out, 0) {
					t.Errorf("%s: ProcessSample(0.1) returned non-finite %v", stage, out)
				}
			}

			for _, def := range params {
				eff.SetParam(def.Name, def.Min)
				assertFiniteOutput("SetParam(" + def.Name + ", Min)")
				eff.SetParam(def.Name, def.Max)
				assertFiniteOutput("SetParam(" + def.Name + ", Max)")
				mid := (def.Min + def.Max) / 2
				eff.SetParam(def.Name, mid)
				assertFiniteOutput("SetParam(" + def.Name + ", mid)")
			}

			eff.SetParam("__unknown_param_name__", 0)
			assertFiniteOutput("SetParam(unknown)")

			eff.Reset()
			assertFiniteOutput("after Reset")
		})
	}
}
