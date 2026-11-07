//go:build test

package ui

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
)

// So drawWaveformCursor has data under the cursor.
func channelWithSineWave() *analyzer.ChannelMetrics {
	const n = 512
	w := make([]float64, n)
	for i := range w {
		w[i] = 0.6 * math.Sin(2*math.Pi*float64(i)/64.0)
	}
	return &analyzer.ChannelMetrics{Active: true, Waveform: w}
}

// A colTextSecondary crosshair at cursorX and a readout box (surface-2 at AlphaStrong) along the top.
func TestWaveformDrawCursorReadout(t *testing.T) {
	assertDefaultParityState(t)

	ch := channelWithSineWave()
	rect := image.Rect(0, 0, 400, 200)
	cursorX := rect.Min.X + 28 + 50 // inside waveRect (left margin = 28)

	img := ebiten.NewImage(rect.Dx(), rect.Dy())
	rects := collectFilledRects(t, func() {
		drawWaveformCursor(img, rect, ch, nil, cursorX)
	})

	wantedLine := color.RGBAModel.Convert(colTextSecondary).(color.RGBA)
	wantedBg := color.RGBAModel.Convert(WithAlpha(colSurface2, AlphaStrong)).(color.RGBA)

	hasLine := false
	hasBg := false
	for _, r := range rects {
		if r.Color == wantedLine && r.Rect.Min.X == cursorX && r.Rect.Dx() == 1 && r.Rect.Dy() > 10 {
			hasLine = true
		}
		if r.Color == wantedBg && r.Rect.Dx() > 10 && r.Rect.Dy() > 4 {
			hasBg = true
		}
	}
	if !hasLine {
		t.Errorf("no colTextSecondary vertical crosshair at x=%d found", cursorX)
	}
	if !hasBg {
		t.Errorf("no colSurface2/AlphaStrong readout backplate emitted")
	}
}

// Nothing to render.
func TestWaveformCursorClampsOutsideRect(t *testing.T) {
	assertDefaultParityState(t)

	ch := channelWithSineWave()
	rect := image.Rect(0, 0, 400, 200)

	img := ebiten.NewImage(rect.Dx(), rect.Dy())
	rects := collectFilledRects(t, func() {
		drawWaveformCursor(img, rect, ch, nil, -50)
	})
	if len(rects) != 0 {
		t.Errorf("expected no draws when cursor is left of rect; got %d rects", len(rects))
	}

	rects = collectFilledRects(t, func() {
		drawWaveformCursor(img, rect, ch, nil, rect.Max.X+50)
	})
	if len(rects) != 0 {
		t.Errorf("expected no draws when cursor is right of rect; got %d rects", len(rects))
	}
}
