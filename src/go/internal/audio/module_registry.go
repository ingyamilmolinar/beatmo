package audio

import (
	"slices"
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

var (
	builtinRegOnce sync.Once
	builtinReg     *instrument.Registry
)

// Panics on an invalid table: a build invariant.
func BuiltinModuleRegistry() *instrument.Registry {
	builtinRegOnce.Do(func() {
		defaults := map[string]float64{}
		for _, d := range ModularSynthParamDefs() {
			defaults[d.Name] = d.Default
		}
		decls := builtinModuleDecls()
		for i := range decls {
			for j := range decls[i].Values {
				v := &decls[i].Values[j]
				name := v.Name
				if decls[i].Kind == instrument.KindLayer {
					name = instrument.LayerField(0, name)
				}
				v.Init = defaults[name]
				if len(v.Allowed) > 0 && !slices.Contains(v.Allowed, v.Init) {
					v.Init = v.Allowed[0]
				}
			}
		}
		r, err := instrument.NewRegistry(decls, ModularParamSchema(), modularGenSlots)
		if err != nil {
			panic(err)
		}
		builtinReg = r
	})
	return builtinReg
}
