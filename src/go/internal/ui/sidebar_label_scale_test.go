//go:build test

package ui

import "testing"

// A density token; Comfortable keeps 1.2.
func TestSidebarLabelScaleIsDensityToken(t *testing.T) {
	if got := Profile().DensityValues().SidebarLabelScale; got <= 0 {
		t.Fatalf("SidebarLabelScale density token unset (got %d per-mille)", got)
	}
	restore := SetDensityForTest(DensityComfortable)
	defer restore()
	if s := sidebarLabelScale(); s < 1.0 || s > 1.5 {
		t.Fatalf("sidebarLabelScale() = %v at Comfortable, want ~1.2", s)
	}
}
