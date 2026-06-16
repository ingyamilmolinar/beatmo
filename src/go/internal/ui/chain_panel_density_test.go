//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
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

// TestChainTriggerMarkerVisibleAtAllDensities — the trigger marker
// must paint at least density-table-width pixels of trigger color.
func TestChainTriggerMarkerVisibleAtAllDensities(t *testing.T) {
	cases := []struct {
		density Density
		wantW   int
	}{
		{DensityCompact, 2},
		{DensityComfortable, 3},
		{DensitySpacious, 4},
	}
	dst := ebiten.NewImage(200, 60)
	for _, c := range cases {
		restore := SetDensityForTest(c.density)
		rects := collectFilledRects(t, func() {
			drawChainTriggerMarker(dst, image.Rect(0, 0, 200, 60))
		})
		// The vertical-line rect should be at least wantW pixels wide.
		var maxLineW int
		for _, r := range rects {
			w := r.Rect.Dx()
			h := r.Rect.Dy()
			if h > 40 && w > maxLineW {
				maxLineW = w
			}
		}
		if maxLineW < c.wantW {
			t.Errorf("density=%v: max trigger-line width=%d, want ≥%d",
				c.density, maxLineW, c.wantW)
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
	for _, sc := range scales {
		if !(sc.c <= sc.m && sc.m <= sc.s) {
			t.Errorf("%s not monotonic across density: compact=%d comfortable=%d spacious=%d",
				sc.name, sc.c, sc.m, sc.s)
		}
		// Spacious must beat the old hardcoded ~714 permille so mobile text is bigger.
		if sc.s <= 714 {
			t.Errorf("%s spacious=%d must exceed the old hardcoded 714 permille", sc.name, sc.s)
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

// TestChainScaleHelperConvertsPermille — chainScale divides permille by 1000.
func TestChainScaleHelperConvertsPermille(t *testing.T) {
	if got := chainScale(850); got != 0.85 {
		t.Errorf("chainScale(850)=%v, want 0.85", got)
	}
	if got := chainScale(1100); got != 1.1 {
		t.Errorf("chainScale(1100)=%v, want 1.1", got)
	}
}
