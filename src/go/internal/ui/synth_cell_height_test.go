package ui

import "testing"

// Task 3: each synth grid cell is now a full ParamCard slot (the card fills the
// whole cell), so the cell height is exactly the density's ParamCardH — no dial
// diameter, caption band, or concept-viz band arithmetic.
func TestKnobCellHeight_IsParamCardHeight(t *testing.T) {
	restore := SetDensityForTest(DensityComfortable)
	defer restore()
	want := Profile().DensityValues().ParamCardH
	got := synthKnobCellHeight()
	if got != want {
		t.Fatalf("cell height = %d, want %d (ParamCardH)", got, want)
	}
}
