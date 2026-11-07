//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// Before any data the panel draws its axis chrome (midline and labels), not a bare outline, as the browser does.
func TestWaveformIdleStateDrawsAxisChrome(t *testing.T) {
	dst := ebiten.NewImage(400, 200)
	rect := image.Rect(0, 0, 400, 200)

	var gotMidline bool
	restore := interceptDrawRect(func(_ *ebiten.Image, r image.Rectangle, _ color.Color, filled bool) {
		// A filled 1 px midline across the wave area, inside the panel.
		if filled && r.Dy() == 1 && r.Dx() > rect.Dx()/2 &&
			r.Min.Y > rect.Min.Y+10 && r.Max.Y < rect.Max.Y-10 {
			gotMidline = true
		}
	})
	defer restore()

	drawAnalyzerWaveform(dst, rect, nil, nil, nil, 1.0, false, false)

	if !gotMidline {
		t.Fatal("idle waveform panel drew no midline — dead-panel regression")
	}
}

// The idle Spectrum draws the baseline the bars rise from.
func TestSpectrumIdleStateDrawsBaseline(t *testing.T) {
	dst := ebiten.NewImage(400, 200)
	rect := image.Rect(0, 0, 400, 200)

	var gotBaseline bool
	restore := interceptDrawRect(func(_ *ebiten.Image, r image.Rectangle, _ color.Color, filled bool) {
		if filled && r.Dy() == 1 && r.Dx() > rect.Dx()/2 &&
			r.Min.Y > rect.Min.Y+10 && r.Max.Y < rect.Max.Y-4 {
			gotBaseline = true
		}
	})
	defer restore()

	drawAnalyzerSpectrumWithScale(dst, rect, nil, nil, freqScaleLog)

	if !gotBaseline {
		t.Fatal("idle spectrum panel drew no baseline — dead-panel regression")
	}
}
