//go:build test

package ui

import (
	"testing"
)

// A fixed 4 px rail was sub-perceptible on a phone.
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

// 48 at Compact to 72 at Spacious.
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

// Monotonic from Compact to Spacious.
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
	// Text now renders through the styled roles; these only keep the authored tokens monotonic while DESIGN.md
	// still lists them.
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
