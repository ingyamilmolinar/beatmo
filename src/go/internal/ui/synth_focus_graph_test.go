package ui

import (
	"testing"
)

func TestSynthFocusGraphTokenPresent(t *testing.T) {
	for _, d := range []Density{DensityCompact, DensityComfortable, DensitySpacious} {
		restore := SetDensityForTest(d)
		got := Profile().DensityValues().SynthFocusGraphH
		restore()
		if got < 24 {
			t.Fatalf("density %v: SynthFocusGraphH=%d, want >=24 (legible focus band)", d, got)
		}
	}
}
