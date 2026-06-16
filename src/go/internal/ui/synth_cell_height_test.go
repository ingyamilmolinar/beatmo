package ui

import "testing"

func TestKnobCellHeight_ExcludesConceptVizBand(t *testing.T) {
	restore := SetDensityForTest(DensityComfortable)
	defer restore()
	d := Profile().DensityValues()
	want := d.SynthKnobIdeal + d.SynthKnobCaptionH + SpaceXS
	got := synthKnobCellHeight()
	if got != want {
		t.Fatalf("cell height = %d, want %d (no viz band)", got, want)
	}
}
