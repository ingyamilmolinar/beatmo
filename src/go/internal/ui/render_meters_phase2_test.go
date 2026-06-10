//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
)

// TestLevelsPhase2_PeakHoldRises — feeding the latch a new peak should
// raise PeakHoldDB to that value, regardless of prior value.
func TestLevelsPhase2_PeakHoldRises(t *testing.T) {
	var l LevelsLatch
	l.PeakHoldDB = -40
	l.Update(0, -10, -16)
	if l.PeakHoldDB != -10 {
		t.Fatalf("PeakHoldDB after Update(-10): got %v, want -10", l.PeakHoldDB)
	}
}

// TestLevelsPhase2_PeakHoldDecays — feeding the latch a quieter peak
// should not lower PeakHoldDB this frame (sticks at hold), but after
// enough frames it should decay toward the live peak.
func TestLevelsPhase2_PeakHoldDecays(t *testing.T) {
	var l LevelsLatch
	l.Update(0, -3, -9)
	if l.PeakHoldDB != -3 {
		t.Fatalf("baseline PeakHoldDB: got %v, want -3", l.PeakHoldDB)
	}
	// Hold should not drop below -3 immediately when the live peak quiets.
	l.Update(0, -40, -50)
	if l.PeakHoldDB < -3.6 {
		t.Fatalf("PeakHoldDB decayed too fast in one frame: got %v, expected ≥ -3.6", l.PeakHoldDB)
	}
	// Many frames of low peak: hold should approach the live peak.
	for i := 0; i < 600; i++ {
		l.Update(0, -40, -50)
	}
	if l.PeakHoldDB > -10 {
		t.Fatalf("PeakHoldDB never decayed after long quiet period: got %v", l.PeakHoldDB)
	}
}

// TestLevelsPhase2_PeakHoldLineRendered — a non-zero PeakHoldDB should
// produce a thin vertical marker (colTextPrimary) on the Peak bar in
// the Levels panel.
func TestLevelsPhase2_PeakHoldLineRendered(t *testing.T) {
	rect := image.Rect(0, 0, 400, 80)
	dst := ebiten.NewImage(rect.Dx(), rect.Dy())
	ch := &analyzer.ChannelMetrics{PeakDB: -6, RMSDB: -18, Active: true}
	latch := &LevelsLatch{PeakHoldDB: -3}

	rects := collectFilledRects(t, func() {
		drawLevelsDetail(dst, rect, ch, latch)
	})

	// Peak bar lives in the top 60% of content area (below the 14px
	// header strip, above the 18px footer). Marker should be in that
	// vertical band.
	peakBandTop := rect.Min.Y + 14
	peakBandBot := peakBandTop + (rect.Dy()-14-18)*60/100
	peakBand := image.Rect(rect.Min.X+28, peakBandTop, rect.Max.X-4, peakBandBot)

	got := rectsWithColorInside(rects, peakBand, colTextPrimary)
	if got == 0 {
		t.Fatalf("expected ≥1 colTextPrimary rect (peak-hold marker) in Peak bar band %v, got 0", peakBand)
	}
}

// TestLevelsPhase2_ClipLEDLightsWhenLatched — when the latch is in
// latched state (recent clip event), the header strip should paint
// a small meterClip-colored LED.
func TestLevelsPhase2_ClipLEDLightsWhenLatched(t *testing.T) {
	rect := image.Rect(0, 0, 400, 80)
	dst := ebiten.NewImage(rect.Dx(), rect.Dy())
	ch := &analyzer.ChannelMetrics{PeakDB: -2, RMSDB: -10, ClipCount: 5, Active: true}
	latch := &LevelsLatch{}
	// First Update bumps LastClipCount from 0 → 5 and latches.
	latch.Update(ch.ClipCount, ch.PeakDB, ch.RMSDB)
	if !latch.Latched() {
		t.Fatal("latch should be Latched() after seeing clip event")
	}

	rects := collectFilledRects(t, func() {
		drawLevelsDetail(dst, rect, ch, latch)
	})

	// Header strip occupies the top 14px.
	header := image.Rect(rect.Min.X, rect.Min.Y, rect.Max.X, rect.Min.Y+14)
	if got := rectsWithColorInside(rects, header, meterClip); got == 0 {
		t.Fatalf("expected meterClip LED rect in header strip %v when latched, got 0", header)
	}
}

// TestLevelsPhase2_ClipLEDDarkWhenNoClip — header should NOT show
// the clip LED when there's no recent clip event.
func TestLevelsPhase2_ClipLEDDarkWhenNoClip(t *testing.T) {
	rect := image.Rect(0, 0, 400, 80)
	dst := ebiten.NewImage(rect.Dx(), rect.Dy())
	ch := &analyzer.ChannelMetrics{PeakDB: -12, RMSDB: -22, ClipCount: 0, Active: true}
	latch := &LevelsLatch{}
	latch.Update(ch.ClipCount, ch.PeakDB, ch.RMSDB)

	rects := collectFilledRects(t, func() {
		drawLevelsDetail(dst, rect, ch, latch)
	})

	header := image.Rect(rect.Min.X, rect.Min.Y, rect.Max.X, rect.Min.Y+14)
	if got := rectsWithColorInside(rects, header, meterClip); got > 0 {
		t.Fatalf("expected 0 meterClip rects in header when not latched, got %d in %v", got, header)
	}
}

var _ color.Color = colTextPrimary // ensure the token import is satisfied even if removed in renderer
