//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// An idle analyzer panel (no channel data yet — e.g. before the first play)
// must render its axis chrome (midline + amplitude labels), not a bare
// outline. Regression: the native-desktop Wave/Spectrum tabs drew ~200px of
// pure black at boot while the browser drew a full axis — a dead-panel UX
// and an accidental platform divergence (2026-07-04 screenshot critique,
// A5).
func TestWaveformIdleStateDrawsAxisChrome(t *testing.T) {
	dst := ebiten.NewImage(400, 200)
	rect := image.Rect(0, 0, 400, 200)

	var gotMidline bool
	restore := interceptDrawRect(func(_ *ebiten.Image, r image.Rectangle, _ color.Color, filled bool) {
		// The zero/midline: a filled 1-px-tall horizontal line spanning the
		// wave area, vertically inside the panel (not a border row).
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

// Same contract for the Spectrum tab: an idle panel draws a baseline rule
// (where the bars will rise from) rather than a bare outline.
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
