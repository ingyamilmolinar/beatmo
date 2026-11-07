package instrument

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
)

// ValidationError lists every problem found, each prefixed by its path.
type ValidationError struct{ Problems []string }

func (e *ValidationError) Error() string { return "instrument: " + strings.Join(e.Problems, "; ") }

func finite(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Structure, completeness, finiteness, Allowed sets, versions and slot limits; never knob ranges (UI hints).
func Validate(d Doc, reg *Registry) error {
	var p []string
	bad := func(format string, a ...any) { p = append(p, fmt.Sprintf(format, a...)) }
	if d.ID == "" {
		bad("id: empty")
	}
	r := d.Render
	if !finite(r.Seconds) || r.Seconds <= 0 {
		bad("render.seconds: must be a number > 0")
	}
	if !finite(r.Amplitude) || r.Amplitude < 0 {
		bad("render.amplitude: must be a number >= 0")
	}
	if !finite(r.GateTail) || r.GateTail < 0 || r.GateTail >= 1 {
		bad("render.gate_tail: must be in [0, 1)")
	}
	for _, t := range reg.decls {
		if t.Required {
			if _, ok := d.Stages[t.Type]; !ok {
				bad("stages.%s: required", t.Type)
			}
		}
	}
	for _, key := range sortedKeys(d.Stages) {
		st := d.Stages[key]
		path := "stages." + key
		t, ok := reg.byType[key]
		if !ok || t.Kind != KindStage {
			bad("%s: unknown stage type", path)
			continue
		}
		checkModule(bad, path, t, st.V, st.On, st.Values)
		switch {
		case t.Table != nil:
			checkTable(bad, path, t.Table, st.Table)
		case st.Table != nil:
			bad("%s.table: %s has no table", path, key)
		}
	}
	if m := d.Source; m != nil {
		path := "source(" + m.Type + ")"
		if t, ok := reg.byType[m.Type]; !ok || t.Kind != KindSource {
			bad("%s: unknown source type", path)
		} else {
			checkModule(bad, path, t, m.V, m.On, m.Values)
		}
	}
	for i, m := range d.Layers {
		if m == nil {
			if i == len(d.Layers)-1 {
				bad("layers[%d]: trailing empty slot", i)
			}
			continue
		}
		path := fmt.Sprintf("layers[%d](%s)", i, m.Type)
		t, ok := reg.byType[m.Type]
		if !ok || t.Kind != KindLayer {
			bad("%s: unknown layer type", path)
			continue
		}
		if i >= t.Capacity {
			bad("%s: the engine plays this voice only in slots 1..%d", path, t.Capacity)
		}
		checkModule(bad, path, t, m.V, m.On, m.Values)
	}
	for i, m := range d.FX {
		path := fmt.Sprintf("fx[%d](%s)", i, m.Type)
		if t, ok := reg.byType[m.Type]; !ok || t.Kind != KindFX {
			bad("%s: unknown fx type", path)
		} else {
			checkModule(bad, path, t, m.V, m.On, m.Values)
		}
	}
	if len(p) == 0 {
		return nil
	}
	return &ValidationError{Problems: p}
}

func checkModule(bad func(string, ...any), path string, t *ModuleTypeDecl, v int, on bool, vals Values) {
	if v != t.Version {
		bad("%s: version %d, this build reads version %d", path, v, t.Version)
	}
	if !on && t.Enable == nil && t.Kind != KindFX {
		bad("%s: has no on/off switch, so it cannot be off (remove it instead)", path)
	}
	if t.FreeForm {
		for _, k := range sortedKeys(vals) {
			if !finite(vals[k]) {
				bad("%s.values.%s: not finite", path, k)
			}
		}
		return
	}
	for _, d := range t.Values {
		x, ok := vals[d.Name]
		switch {
		case !ok:
			bad("%s.values.%s: missing", path, d.Name)
		case !finite(x):
			bad("%s.values.%s: not finite", path, d.Name)
		case len(d.Allowed) > 0 && !slices.Contains(d.Allowed, x):
			bad("%s.values.%s: %v not in %v", path, d.Name, x, d.Allowed)
		}
	}
	for _, k := range sortedKeys(vals) {
		if _, ok := t.value(k); !ok {
			bad("%s.values.%s: not a value of %s", path, k, t.Type)
		}
	}
}
