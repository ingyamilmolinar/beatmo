//go:build !test && !js

package audio

import (
	"bufio"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

const proofSR = 44100

func renderBlockForTest(b []float64, n int) []float32 {
	buf := make([]float32, n)
	renderModularBlock(buf, proofSR, n, modularBlock(b))
	return buf
}

// Or is listed with a reason in testdata/dead_module_values.txt; a listed entry that is live fails (ratchet).
func TestDeclaredModuleValuesAreLive(t *testing.T) {
	reg := BuiltinModuleRegistry()
	defs := map[string]ParamDef{}
	for _, d := range ModularSynthParamDefs() {
		defs[d.Name] = d
	}
	known := loadDeadModuleValues(t)
	n := proofSR / 4
	live := map[string]bool{}
	seen := map[string]bool{}
	for _, doc := range loadShippedDocsForTest(t) {
		base := renderBlockForTest(instrument.Flatten(doc, reg), n)
		visit := func(ref instrument.Ref, m instrument.Module, slot int) {
			ty, _ := reg.Type(m.Type)
			seen[ty.Type] = true
			if !m.On || ty.FreeForm {
				return
			}
			for _, vd := range ty.Values {
				key := ty.Type + "." + vd.Name
				if live[key] || len(vd.Allowed) > 0 {
					continue
				}
				f := vd.Name
				if slot >= 0 {
					f = instrument.LayerField(slot, vd.Name)
				}
				v, ok := perturbFor(defs[f], m.Values[vd.Name])
				if !ok {
					continue
				}
				d, err := instrument.SetValue(doc, reg, ref, vd.Name, v)
				if err != nil {
					t.Fatal(err)
				}
				if !slices.Equal(base, renderBlockForTest(instrument.Flatten(d, reg), n)) {
					live[key] = true
				}
			}
		}
		for k, st := range doc.Stages {
			visit(instrument.StageRef(k), instrument.Module{Type: k, V: st.V, On: st.On, Values: st.Values}, -1)
		}
		if doc.Source != nil {
			visit(instrument.SourceRef(), *doc.Source, -1)
		}
		for i, m := range doc.Layers {
			if m != nil {
				visit(instrument.LayerRef(i), *m, i)
			}
		}
	}
	for _, ty := range reg.Types() {
		if ty.Kind == instrument.KindFX {
			continue
		}
		if !seen[ty.Type] {
			if _, ok := known[ty.Type+".*"]; !ok {
				t.Errorf("UNUSED %s.* — no shipped document uses this type; list it with a reason", ty.Type)
			}
			continue
		}
		for _, vd := range ty.Values {
			key := ty.Type + "." + vd.Name
			_, listed := known[key]
			switch {
			case live[key] && listed:
				t.Errorf("%s is live now — remove it from dead_module_values.txt", key)
			case !live[key] && !listed && len(vd.Allowed) == 0:
				t.Errorf("DEAD %s — no shipped document's render moves with it; fix the declaration or list it with a reason", key)
			}
		}
	}
}

func loadDeadModuleValues(t *testing.T) map[string]string {
	f, err := os.Open("testdata/dead_module_values.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	out := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, reason, ok := strings.Cut(line, " ")
		if !ok || strings.TrimSpace(reason) == "" {
			t.Fatalf("dead_module_values.txt: %q needs a reason", line)
		}
		out[key] = strings.TrimSpace(reason)
	}
	return out
}

// Within the param's own declared range, so the probe never leaves the schema.
func perturbFor(d ParamDef, cur float64) (float64, bool) {
	const eps = 1e-9
	if d.Max-d.Min < eps {
		return 0, false // a fixed param cannot be perturbed
	}
	// The far end, so a subtle-but-real effect still shows.
	lo, hi := d.Min, d.Max
	if cur-lo > hi-cur {
		return lo, true
	}
	return hi, true
}
