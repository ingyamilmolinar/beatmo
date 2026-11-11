//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/scope"
)

// TestChainBadgeSizedToStyledCaption — the A/B badge pill is sized to the
// styled caption-role text metrics (chainBadgeSize → StyledTextWidth/Height
// at RoleCaption), replacing the old density-scaled bitmap text which
// pixelated at every factor ≠ 1. The painted badge box (a drawRect we can
// intercept) must therefore match chainBadgeSize at EVERY density: styled
// roles render at a fixed true pixel size, so the badge is density-invariant
// by design (2026-07-16 audio-tab typography migration).
func TestChainBadgeSizedToStyledCaption(t *testing.T) {
	badgeBox := func(d Density) (w, h int) {
		restore := SetDensityForTest(d)
		defer restore()
		cb := ChainCallbacks{ScopeState: func() *scope.State { return nil }}
		z := NewChainPanelZone(cb)
		z.SetTapA(scope.StageSynth) // A badge on the top (Synth) stage button
		z.Layout(image.Rect(0, 0, 600, 240))
		dst := ebiten.NewImage(600, 240)
		rects := collectFilledRects(t, func() { z.Draw(dst) })

		want := color.RGBAModel.Convert(colScopeA).(color.RGBA)
		for _, r := range rects {
			// The A badge fills colScopeA and lives at the top of the stage
			// column. State is nil so no trace/legend draws this color.
			if r.Color == want && r.Rect.Min.X < chainStageColW+48 && r.Rect.Min.Y < 56 {
				if r.Rect.Dx() > w {
					w, h = r.Rect.Dx(), r.Rect.Dy()
				}
			}
		}
		return w, h
	}

	wantW, wantH := chainBadgeSize("A")
	for _, d := range []Density{DensityCompact, DensitySpacious} {
		gotW, gotH := badgeBox(d)
		if gotW == 0 {
			t.Fatalf("A badge box not found at density %v", d)
		}
		if gotW != wantW || gotH != wantH {
			t.Errorf("density %v: badge box %dx%d, want styled-caption size %dx%d",
				d, gotW, gotH, wantW, wantH)
		}
	}
}
