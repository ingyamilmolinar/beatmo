//go:build test

package ui

import (
	"image"
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestSynthPhase4_RealCachedSampleRenderedWhenAvailable — when the
// voice-cache override yields a non-empty buffer, the synth header
// waveform must paint via drawWaveTrace (colored TokenAccent()), not
// the placeholder exponential.
func TestSynthPhase4_RealCachedSampleRenderedWhenAvailable(t *testing.T) {
	prev := testSynthHeaderSampleOverride
	t.Cleanup(func() { testSynthHeaderSampleOverride = prev })

	// Inject a tiny sine wave as the "cached sample" for any instrument.
	wave := make([]float32, 256)
	for i := range wave {
		wave[i] = float32(0.7 * math.Sin(2*math.Pi*float64(i)/32))
	}
	testSynthHeaderSampleOverride = func(string) []float32 { return wave }

	r := image.Rect(0, 0, 200, 60)
	dst := ebiten.NewImage(200, 60)
	rects := collectFilledRects(t, func() {
		drawSynthHeaderWaveform(dst, r, "kick-1")
	})

	// drawWaveTrace paints colWaveTrace columns. ≥10 indicates real
	// trace rendering vs the smooth-curve placeholder (which only
	// uses TokenAccentDim()).
	got := rectsWithColorInside(rects, r, colWaveTrace)
	if got < 10 {
		t.Fatalf("expected ≥10 colWaveTrace rects for real sample render, got %d", got)
	}
}

// TestSynthPhase4_PlaceholderWhenNoSample — when the override returns
// nil (or there is no cached sample), the renderer must fall back to
// the placeholder so the panel never shows a blank rectangle.
func TestSynthPhase4_PlaceholderWhenNoSample(t *testing.T) {
	prev := testSynthHeaderSampleOverride
	t.Cleanup(func() { testSynthHeaderSampleOverride = prev })
	testSynthHeaderSampleOverride = func(string) []float32 { return nil }

	r := image.Rect(0, 0, 200, 60)
	dst := ebiten.NewImage(200, 60)
	rects := collectFilledRects(t, func() {
		drawSynthHeaderWaveform(dst, r, "kick-1")
	})

	// Placeholder uses TokenAccentDim() — count those.
	got := rectsWithColorInside(rects, r, TokenAccentDim())
	if got < 10 {
		t.Fatalf("expected ≥10 TokenAccentDim() rects for placeholder, got %d", got)
	}
	// And NO real trace.
	if got := rectsWithColorInside(rects, r, colWaveTrace); got > 0 {
		t.Fatalf("expected no colWaveTrace rects in placeholder mode, got %d", got)
	}
}
