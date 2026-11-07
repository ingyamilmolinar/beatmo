package ui

import "testing"

// The card fills its cell, so the cell is exactly ParamCardH.
func TestKnobCellHeight_IsParamCardHeight(t *testing.T) {
	restore := SetDensityForTest(DensityComfortable)
	defer restore()
	want := Profile().DensityValues().ParamCardH
	got := synthControlCellHeight()
	if got != want {
		t.Fatalf("cell height = %d, want %d (ParamCardH)", got, want)
	}
}
