package audio

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

const shippedDocsDir = "../assets/instruments"

func loadShippedDocsForTest(t *testing.T) map[string]instrument.Doc {
	t.Helper()
	paths, _ := filepath.Glob(filepath.Join(shippedDocsDir, "*.json"))
	out := map[string]instrument.Doc{}
	for _, p := range paths {
		if filepath.Base(p) == "index.json" {
			continue
		}
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		d, err := instrument.ParseFile(b)
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		out[string(d.ID)] = d
	}
	return out
}

func TestShippedDocumentsValidate(t *testing.T) {
	docs := loadShippedDocsForTest(t)
	for id, d := range docs {
		if err := instrument.Validate(d, BuiltinModuleRegistry()); err != nil {
			t.Errorf("%s: %v", id, err)
		}
	}
	b, err := os.ReadFile(filepath.Join(shippedDocsDir, "index.json"))
	if err != nil {
		t.Fatal(err)
	}
	var idx struct {
		Instruments []instrument.ID `json:"instruments"`
	}
	if err := json.Unmarshal(b, &idx); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(idx.Instruments, roster.All) || len(docs) != len(idx.Instruments) {
		t.Fatalf("index.json (%d) must list every document (%d) in roster.All order", len(idx.Instruments), len(docs))
	}
}

// Shipped values lie inside their knob ranges, or the derived range (Task B4)
// widens to contain them — this lists which, so a widening is a decision.
func TestShippedValuesInsideParamDefRanges(t *testing.T) {
	defs := map[string]ParamDef{}
	for _, d := range ModularSynthParamDefs() {
		defs[d.Name] = d
	}
	reg := BuiltinModuleRegistry()
	for id, d := range loadShippedDocsForTest(t) {
		for f, v := range instrument.OwnedFields(d, reg) {
			pd, ok := defs[f]
			if ok && (v < pd.Min || v > pd.Max) {
				t.Logf("WIDEN %s %s=%v outside [%v,%v]", id, f, v, pd.Min, pd.Max)
			}
		}
	}
}
