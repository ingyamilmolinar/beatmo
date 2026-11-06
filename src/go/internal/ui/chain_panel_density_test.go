//go:build test

package ui

import (
	"testing"
)

// TestChainMiniMeterVisibleAtAllDensities pins the Phase 3 fix: the
// per-stage mini-meter rail must be at least density-table-width
// pixels wide. Pre-Phase-3 it was a fixed 4 px at every screen size,
// sub-perceptible on a 360-px mobile portrait.
func TestChainMiniMeterVisibleAtAllDensities(t *testing.T) {
	cases := []struct {
		density Density
		wantW   int
	}{
		{DensityCompact, 3},
		{DensityComfortable, 6},
		{DensitySpacious, 10},
	}
	for _, c := range cases {
		restore := SetDensityForTest(c.density)
		dv := Profile().DensityValues()
		if dv.ChainMiniMeterW < c.wantW {
			t.Errorf("density=%v: ChainMiniMeterW=%d, want ≥%d",
				c.density, dv.ChainMiniMeterW, c.wantW)
		}
		restore()
	}
}

// TestChainStageColWidensWithDensity — the stage column width
// expands from 48 (Compact) to 72 (Spacious) so touch interactions
// have more room at higher densities. Compact lets desktop power
// users see more chain at once.
func TestChainStageColWidensWithDensity(t *testing.T) {
	cases := []struct {
		density  Density
		wantColW int
		wantRowH int
	}{
		{DensityCompact, 48, 20},
		{DensityComfortable, 64, 26},
		{DensitySpacious, 72, 36},
	}
	for _, c := range cases {
		restore := SetDensityForTest(c.density)
		dv := Profile().DensityValues()
		if dv.ChainStageColW != c.wantColW {
			t.Errorf("density=%v: ChainStageColW=%d, want %d",
				c.density, dv.ChainStageColW, c.wantColW)
		}
		if dv.ChainStageRowH != c.wantRowH {
			t.Errorf("density=%v: ChainStageRowH=%d, want %d",
				c.density, dv.ChainStageRowH, c.wantRowH)
		}
		restore()
	}
}

// TestChainTextScalesGrowWithDensity — Chain text scales are authored as
// permille-of-Body and MUST grow monotonically from Compact → Spacious so
// labels/readouts/pills/badges enlarge on mobile (Spacious) instead of
// being frozen at the old hardcoded FontSizeCaption/FontSizeBody (~714).
func TestChainTextScalesGrowWithDensity(t *testing.T) {
	get := func(d Density) densityValues {
		restore := SetDensityForTest(d)
		defer restore()
		return Profile().DensityValues()
	}
	compact, comfortable, spacious := get(DensityCompact), get(DensityComfortable), get(DensitySpacious)

	scales := []struct {
		name    string
		c, m, s int
	}{
		{"ChainLabelScale", compact.ChainLabelScale, comfortable.ChainLabelScale, spacious.ChainLabelScale},
		{"ChainReadoutScale", compact.ChainReadoutScale, comfortable.ChainReadoutScale, spacious.ChainReadoutScale},
		{"ChainPillScale", compact.ChainPillScale, comfortable.ChainPillScale, spacious.ChainPillScale},
		{"ChainBadgeScale", compact.ChainBadgeScale, comfortable.ChainBadgeScale, spacious.ChainBadgeScale},
	}
	// NOTE (2026-07-16 typography migration): Chain text now renders through
	// the styled text-role API (RoleMicro/RoleCaption, fixed true-pixel
	// sizes) — these permille tokens no longer drive any text draw. The pins
	// below only keep the AUTHORED token values monotonic while they remain
	// in DESIGN.md; delete them together with the densities entries.
	for _, sc := range scales {
		if !(sc.c <= sc.m && sc.m <= sc.s) {
			t.Errorf("%s not monotonic across density: compact=%d comfortable=%d spacious=%d",
				sc.name, sc.c, sc.m, sc.s)
		}
	}

	// Layout margins + legend strip + mini-wave height also grow with density.
	margins := []struct {
		name    string
		c, m, s int
	}{
		{"ChainScopeLeftMargin", compact.ChainScopeLeftMargin, comfortable.ChainScopeLeftMargin, spacious.ChainScopeLeftMargin},
		{"ChainScopeBottomMargin", compact.ChainScopeBottomMargin, comfortable.ChainScopeBottomMargin, spacious.ChainScopeBottomMargin},
		{"ChainLegendStripH", compact.ChainLegendStripH, comfortable.ChainLegendStripH, spacious.ChainLegendStripH},
		{"ChainMiniWaveH", compact.ChainMiniWaveH, comfortable.ChainMiniWaveH, spacious.ChainMiniWaveH},
	}
	for _, mg := range margins {
		if !(mg.c <= mg.m && mg.m <= mg.s) {
			t.Errorf("%s not monotonic across density: compact=%d comfortable=%d spacious=%d",
				mg.name, mg.c, mg.m, mg.s)
		}
	}
}
