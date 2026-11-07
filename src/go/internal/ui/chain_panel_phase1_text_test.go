//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/scope"
)

// Sized to the styled caption metrics, which are true pixels, so the badge is the same at every density.
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
			// State is nil, so no trace or legend uses this colour.
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
