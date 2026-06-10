//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/scope"
)

// TestChainPhase1_BadgeGrowsWithDensity — the A/B badge pill is sized to
// the density-aware badge text (chainBadgeScale). So the painted badge box
// (a drawRect we can intercept) must be wider at Spacious than at Compact.
// This is the rendering-level proof that Chain text actually enlarges on
// mobile, not just that the token grows.
func TestChainPhase1_BadgeGrowsWithDensity(t *testing.T) {
	badgeW := func(d Density) int {
		restore := SetDensityForTest(d)
		defer restore()
		cb := ChainCallbacks{ScopeState: func() *scope.State { return nil }}
		z := NewChainPanelZone(cb)
		z.SetTapA(scope.StageSynth) // A badge on the top (Synth) stage button
		z.Layout(image.Rect(0, 0, 600, 240))
		dst := ebiten.NewImage(600, 240)
		rects := collectFilledRects(t, func() { z.Draw(dst) })

		want := color.RGBAModel.Convert(colScopeA).(color.RGBA)
		maxW := 0
		for _, r := range rects {
			// The A badge fills colScopeA and lives at the top of the stage
			// column. State is nil so no trace/legend draws this color.
			if r.Color == want && r.Rect.Min.X < chainStageColW+48 && r.Rect.Min.Y < 56 {
				if r.Rect.Dx() > maxW {
					maxW = r.Rect.Dx()
				}
			}
		}
		return maxW
	}

	compact := badgeW(DensityCompact)
	spacious := badgeW(DensitySpacious)
	if compact == 0 || spacious == 0 {
		t.Fatalf("A badge box not found: compact=%d spacious=%d", compact, spacious)
	}
	if spacious <= compact {
		t.Errorf("A badge should widen with density (bigger text): compact=%d spacious=%d", compact, spacious)
	}
}
