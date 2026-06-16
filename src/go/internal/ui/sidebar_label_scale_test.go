//go:build test

package ui

import "testing"

// TestSidebarLabelScaleIsDensityToken pins sidebar label scaling to a density
// token (was a hardcoded sidebarTextScale=1.2 const at 30+ sites). The helper
// resolves the per-mille token to a float multiplier; Comfortable preserves
// today's 1.2.
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
