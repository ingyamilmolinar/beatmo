package instrument

import (
	"errors"
	"fmt"
)

// Applies each type's Steps in order; unknown types are left for Validate, a version newer than this build names its path.
func Migrate(d Doc, reg *Registry) (Doc, error) {
	out := d.Clone()
	var errs []error
	step := func(path, typ string, v *int, vals *Values) {
		t, ok := reg.byType[typ]
		if !ok {
			return
		}
		switch {
		case *v > t.Version:
			errs = append(errs, fmt.Errorf("%s: version %d is newer than this build (%d)", path, *v, t.Version))
			return
		case *v < 1:
			errs = append(errs, fmt.Errorf("%s: version %d < 1", path, *v))
			return
		}
		for *v < t.Version {
			*vals = t.Steps[*v-1](vals.clone())
			*v++
		}
	}
	for _, k := range sortedKeys(out.Stages) {
		st := out.Stages[k]
		step("stages."+k, k, &st.V, &st.Values)
		out.Stages[k] = st
	}
	if out.Source != nil {
		step("source", out.Source.Type, &out.Source.V, &out.Source.Values)
	}
	for i, m := range out.Layers {
		if m != nil {
			step(fmt.Sprintf("layers[%d]", i), m.Type, &m.V, &m.Values)
		}
	}
	for i := range out.FX {
		step(fmt.Sprintf("fx[%d]", i), out.FX[i].Type, &out.FX[i].V, &out.FX[i].Values)
	}
	return out, errors.Join(errs...)
}
