package audio

import (
	"math"
	"testing"
)

// A document can never carry a waveform: tables are bounded in size and every column in value.
func TestTablesAreModelParametersOnly(t *testing.T) {
	reg := BuiltinModuleRegistry()
	cells := 0
	tabled := 0
	for _, decl := range reg.Types() {
		if decl.Table == nil {
			continue
		}
		tabled++
		cells += decl.Table.Capacity * len(decl.Table.Columns)
		for _, c := range decl.Table.Columns {
			if math.IsInf(c.Min, 0) || math.IsInf(c.Max, 0) || !(c.Min < c.Max) {
				t.Errorf("%s.%s: bounds [%v, %v] are not physical", decl.Type, c.Name, c.Min, c.Max)
			}
		}
	}
	if tabled != 2 {
		t.Fatalf("%d tabled stage types, want modal + noiseband", tabled)
	}
	if cells > 1024 {
		t.Fatalf("tables hold %d values per document; the data rule caps them at 1024", cells)
	}
}
