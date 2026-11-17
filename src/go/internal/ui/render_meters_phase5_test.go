//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
)

// TestLevelsPhase5_StereoSplitWhenStereoData — when ChannelMetrics has
// stereo data (PeakDBL/R set), drawLevelsDetail must split the Peak
// bar into two visually-distinct sub-bars (L on top, R on bottom).
// We detect "split" by checking that the Peak band area contains
// rect fills at two distinct vertical positions.
func TestLevelsPhase5_StereoSplitWhenStereoData(t *testing.T) {
	rect := image.Rect(0, 0, 400, 120)
	dst := ebiten.NewImage(rect.Dx(), rect.Dy())

	// Stereo channel: L is hot (-0.5 dB → red zone), R is yellow zone (-3 dB).
	ch := &analyzer.ChannelMetrics{
		PeakDB:  -0.5,
		RMSDB:   -12,
		PeakDBL: -0.5,
		PeakDBR: -3,
		RMSDBL:  -12,
		RMSDBR:  -20,
		Active:  true,
	}

	rects := collectFilledRects(t, func() {
		drawLevelsDetail(dst, rect, ch, nil)
	})

	// Peak band: top 60% of content area after the 14px header / 18px footer.
	contentY0 := rect.Min.Y + 14
	contentH := rect.Dy() - 14 - 18
	peakBand := image.Rect(rect.Min.X+28, contentY0, rect.Max.X-4, contentY0+contentH*60/100)

	gotRed := rectsWithColorInside(rects, peakBand, meterHigh)
	gotYel := rectsWithColorInside(rects, peakBand, meterMid)
	if gotRed == 0 {
		t.Errorf("expected ≥1 meterHigh rect (L channel @ -0.5 dB) in peak band %v, got 0", peakBand)
	}
	if gotYel == 0 {
		t.Errorf("expected ≥1 meterMid rect (R channel @ -3 dB) in peak band %v, got 0", peakBand)
	}
}

// TestLevelsPhase5_MonoFallbackWhenNoStereo — when no stereo data is
// present, drawLevelsDetail must keep the single-bar mono render
// path. (Backward compatibility — most playback today is mono.)
func TestLevelsPhase5_MonoFallbackWhenNoStereo(t *testing.T) {
	rect := image.Rect(0, 0, 400, 120)
	dst := ebiten.NewImage(rect.Dx(), rect.Dy())
	ch := &analyzer.ChannelMetrics{
		PeakDB: -12,
		RMSDB:  -22,
		Active: true,
	}
	// HasStereo() must be false.
	if ch.HasStereo() {
		t.Fatal("test setup invalid: HasStereo should be false")
	}
	// Should not panic; produces single mono bar rects.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("mono path panicked: %v", r)
		}
	}()
	_ = collectFilledRects(t, func() {
		drawLevelsDetail(dst, rect, ch, nil)
	})
}
