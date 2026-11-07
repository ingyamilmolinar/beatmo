package audio

import (
	"slices"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Except noise_seed, which no caller ever sets.
func TestEveryABIFieldIsDeclared(t *testing.T) {
	reg := BuiltinModuleRegistry()
	named := map[string]bool{"noise_seed": true}
	for _, ty := range reg.Types() {
		slots := []int{-1}
		if ty.Kind == instrument.KindLayer {
			slots = slots[:0]
			for s := 0; s < ty.Capacity; s++ {
				slots = append(slots, s)
			}
		}
		for _, s := range slots {
			n := func(f string) string {
				if s < 0 {
					return f
				}
				return instrument.LayerField(s, f)
			}
			for _, v := range ty.Values {
				named[n(v.Name)] = true
			}
			for _, x := range ty.Set {
				named[n(x.Field)] = true
			}
			if ty.Enable != nil {
				named[n(ty.Enable.Field)] = true
			}
			for _, g := range ty.Global {
				named[g.Field] = true
			}
		}
		if td := ty.Table; td != nil { // a table's cells and its row count
			named[td.CountField] = true
			for _, c := range td.Columns {
				for i := 0; i < td.Capacity; i++ {
					named[instrument.TableCell(c.Field, i)] = true
				}
			}
		}
	}
	for _, f := range ModularParamSchema() {
		if !named[f] {
			t.Errorf("ABI field %s belongs to no module type", f)
		}
	}
}

// A layer type's capacity is the ABI's slot count for its family block.
func TestLayerCapacityMatchesTheABI(t *testing.T) {
	reg := BuiltinModuleRegistry()
	for _, ty := range reg.Types() {
		if ty.Kind != instrument.KindLayer {
			continue
		}
		for s := 0; s < reg.Slots(); s++ {
			inABI := true
			for _, v := range ty.Values {
				if _, ok := reg.FieldIndex(instrument.LayerField(s, v.Name)); !ok {
					inABI = false
				}
			}
			if inABI != (s < ty.Capacity) {
				t.Errorf("%s: slot %d in ABI=%v but capacity=%d", ty.Type, s+1, inABI, ty.Capacity)
			}
		}
	}
}

// Every value has a ParamDef (its Init and, later, its UI metadata).
func TestEveryModuleValueHasAParamDef(t *testing.T) {
	defs := map[string]bool{}
	for _, d := range ModularSynthParamDefs() {
		defs[d.Name] = true
	}
	for _, ty := range BuiltinModuleRegistry().Types() {
		for _, v := range ty.Values {
			name := v.Name
			if ty.Kind == instrument.KindLayer {
				name = instrument.LayerField(0, v.Name)
			}
			if ty.Kind != instrument.KindFX && !defs[name] {
				t.Errorf("%s.%s has no ParamDef", ty.Type, v.Name)
			}
		}
	}
}

// Both platforms apply a document's voice FX through the C host, so the registry declares exactly those fx.* types.
func TestDocumentsCarryOnlyVoiceFXBothPlatformsRun(t *testing.T) {
	reg := BuiltinModuleRegistry()
	for _, et := range EffectTypes {
		_, declared := reg.Type("fx." + string(et))
		runs := slices.Contains(voiceFXTypes(), et)
		if declared != runs {
			t.Errorf("fx.%s declared=%v, applied by both platforms=%v", et, declared, runs)
		}
	}
	if !slices.Contains(voiceFXTypes(), EffectFilter) {
		t.Fatal("voice FX list incomplete")
	}
}
