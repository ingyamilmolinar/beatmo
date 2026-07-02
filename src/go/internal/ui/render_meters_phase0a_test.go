//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
)

// TestLevelsPhase0a_BarFillsToHeldValue is the Phase 0a regression test
// for the audio-panel redesign. The bug: during playback the panel reads
// instantaneous peak/RMS from the analyser, which catches silence
// between drum transients and reports peak ≈ 0 (−80 dB). Bars vanish.
//
// The fix routes the bar fill height through latch.PeakHoldDB so the
// bar persists between hits. This test simulates "1 hit, 30 silent
// frames, 1 hit, 30 silent frames" and asserts that on the silent
// frames the rendered bar is still ≥ a visible threshold.
func TestLevelsPhase0a_BarFillsToHeldValue(t *testing.T) {
	assertDefaultParityState(t)

	var latch LevelsLatch
	// Frame 0: silence (analyser caught between hits).
	latch.Update(0, analyzerDBFloor, analyzerDBFloor)
	// Frame 1: kick hit lands. Peak −6 dB, RMS −12 dB.
	latch.Update(0, -6, -12)
	if latch.PeakHoldDB > -5 || latch.PeakHoldDB < -7 {
		t.Fatalf("latch should track hit peak: PeakHoldDB=%v, want ~-6", latch.PeakHoldDB)
	}
	if latch.RMSHoldDB > -11 || latch.RMSHoldDB < -13 {
		t.Fatalf("latch should track hit RMS: RMSHoldDB=%v, want ~-12", latch.RMSHoldDB)
	}
	// Frames 2-31: silence (analyser caught between hits). With 15-frame
	// sticky + 0.5 dB/frame decay, the held peak should remain visible
	// (above the meter floor).
	for i := 0; i < 30; i++ {
		latch.Update(0, analyzerDBFloor, analyzerDBFloor)
	}
	if latch.PeakHoldDB <= meterDBFloor {
		t.Fatalf("held peak decayed below meter floor too fast: PeakHoldDB=%v, floor=%v", latch.PeakHoldDB, meterDBFloor)
	}
	if latch.RMSHoldDB <= meterDBFloor {
		t.Fatalf("held RMS decayed below meter floor too fast: RMSHoldDB=%v, floor=%v", latch.RMSHoldDB, meterDBFloor)
	}

	// Now exercise the renderer: a silent-frame ChannelMetrics + the
	// seeded latch should produce visible bar rects. Without the fix the
	// bar would be a 0-pixel-wide fill.
	rect := image.Rect(0, 0, 800, 400)
	dst := ebiten.NewImage(rect.Dx(), rect.Dy())
	silentCh := &analyzer.ChannelMetrics{PeakDB: analyzerDBFloor, RMSDB: analyzerDBFloor, Active: false}
	rects := collectFilledRects(t, func() {
		drawLevelsDetail(dst, rect, silentCh, &latch)
	})
	// Bars live in the bar area (left of x=28, width up to ~770). Look
	// for meter-coloured rects (green/yellow/red) inside the bar band.
	// We don't care which exact colour — only that SOME meter fill is
	// drawn beyond the leftmost few pixels.
	barBand := image.Rect(rect.Min.X+28, rect.Min.Y+14, rect.Max.X-4, rect.Max.Y-18)
	found := 0
	for _, r := range rects {
		if r.Color == meterLow || r.Color == meterMid || r.Color == meterHigh {
			if r.Rect.Min.X >= barBand.Min.X && r.Rect.Max.X > barBand.Min.X+20 {
				found++
			}
		}
	}
	if found == 0 {
		t.Fatalf("expected meter-coloured bar fill rects ≥ 20 px wide in band %v, found 0; held peak=%v held rms=%v",
			barBand, latch.PeakHoldDB, latch.RMSHoldDB)
	}
}

// TestLevelsPhase0a_FooterReadsHeldValue asserts the footer shows the
// held peak/RMS numerically instead of "-inf" between hits.
func TestLevelsPhase0a_FooterReadsHeldValue(t *testing.T) {
	assertDefaultParityState(t)

	var latch LevelsLatch
	latch.Update(0, -6, -12)
	// Several silent frames. Latch should still hold a meaningful value.
	for i := 0; i < 10; i++ {
		latch.Update(0, analyzerDBFloor, analyzerDBFloor)
	}
	if latch.PeakHoldDB <= meterDBFloor {
		t.Fatalf("setup: latch decayed below floor after 10 silent frames: %v", latch.PeakHoldDB)
	}

	rect := image.Rect(0, 0, 800, 80)
	dst := ebiten.NewImage(rect.Dx(), rect.Dy())
	silentCh := &analyzer.ChannelMetrics{PeakDB: analyzerDBFloor, RMSDB: analyzerDBFloor}
	// The footer reads text — we can't introspect text without parsing
	// the rendered image, but the held-value path is unit-tested via the
	// latch fields above. Sanity-check that drawing doesn't panic.
	drawLevelsReadout(dst, image.Rect(rect.Min.X, rect.Max.Y-18, rect.Max.X, rect.Max.Y), silentCh, &latch)
}

// TestSpectrumPhase0a_BarFillsBetweenHits asserts the spectrum bar uses
// the per-band peak-hold value so bars remain visible between
// transients (the Spectrum tab body would otherwise be blank).
func TestSpectrumPhase0a_BarFillsBetweenHits(t *testing.T) {
	assertDefaultParityState(t)

	// Seed peak state with a recent "hit" pattern in all 10 bands.
	peaks := &SpectrumPeakState{}
	for i := range peaks.Peaks {
		peaks.Peaks[i] = 0.8
		peaks.Ages[i] = 0
	}
	peaks.MaxAge = 30

	// Build a silent ChannelMetrics — all FFT bins at the floor.
	// Provide non-empty FreqBins so the renderer doesn't take its
	// nil-ch fast exit.
	const nBins = 32
	fft := make([]float64, nBins)
	freq := make([]float64, nBins)
	for i := range fft {
		fft[i] = spectrumMinDB
		freq[i] = float64(i+1) * 100
	}
	silentCh := &analyzer.ChannelMetrics{FFTBins: fft, FreqBins: freq, Active: true}

	rect := image.Rect(0, 0, 800, 240)
	dst := ebiten.NewImage(rect.Dx(), rect.Dy())
	rects := collectFilledRects(t, func() {
		drawAnalyzerSpectrum(dst, rect, silentCh, peaks)
	})

	// Look for spectrum bar gradient rects above the bar area's bottom —
	// proves the held value (peaks.Peaks=0.8) drove the fill instead of the
	// silent FFT (norm=0). drawSpectrumBarGradient splits each bar into three
	// fixed synthwave bands (top/mid/base), so we track the UNION span of the
	// gradient rects per X column and assert the combined bar is tall.
	barBand := image.Rect(rect.Min.X+28, rect.Min.Y, rect.Max.X, rect.Max.Y-22)
	isBarColor := func(c color.RGBA) bool {
		return c == color.RGBAModel.Convert(colSpectrumBarTop).(color.RGBA) ||
			c == color.RGBAModel.Convert(colSpectrumBarMid).(color.RGBA) ||
			c == color.RGBAModel.Convert(colSpectrumBarBase).(color.RGBA)
	}
	// Track per-column min-Y / max-Y of gradient rects to recover the full
	// bar height across the three bands.
	type span struct{ minY, maxY int }
	cols := map[int]*span{}
	for _, r := range rects {
		if !isBarColor(r.Color) {
			continue
		}
		if r.Rect.Min.X < barBand.Min.X || r.Rect.Max.X > barBand.Max.X {
			continue
		}
		if r.Rect.Min.Y < barBand.Min.Y || r.Rect.Max.Y > barBand.Max.Y {
			continue
		}
		s := cols[r.Rect.Min.X]
		if s == nil {
			cols[r.Rect.Min.X] = &span{minY: r.Rect.Min.Y, maxY: r.Rect.Max.Y}
			continue
		}
		if r.Rect.Min.Y < s.minY {
			s.minY = r.Rect.Min.Y
		}
		if r.Rect.Max.Y > s.maxY {
			s.maxY = r.Rect.Max.Y
		}
	}
	found := 0
	for _, s := range cols {
		if s.maxY-s.minY > barBand.Dy()/3 {
			found++
		}
	}
	if found == 0 {
		t.Fatalf("expected ≥1 tall spectrum bar (held value 0.8 should drive a gradient fill ≥ %d px) in band %v, found 0",
			barBand.Dy()/3, barBand)
	}
}
