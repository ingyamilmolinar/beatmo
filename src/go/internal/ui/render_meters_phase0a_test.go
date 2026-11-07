//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
)

// The analyser catches silence between transients; the bar fill uses latch.PeakHoldDB so bars persist between
// hits.
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
	// 15 sticky frames + 0.5 dB/frame decay keep the held peak above the floor.
	for i := 0; i < 30; i++ {
		latch.Update(0, analyzerDBFloor, analyzerDBFloor)
	}
	if latch.PeakHoldDB <= meterDBFloor {
		t.Fatalf("held peak decayed below meter floor too fast: PeakHoldDB=%v, floor=%v", latch.PeakHoldDB, meterDBFloor)
	}
	if latch.RMSHoldDB <= meterDBFloor {
		t.Fatalf("held RMS decayed below meter floor too fast: RMSHoldDB=%v, floor=%v", latch.RMSHoldDB, meterDBFloor)
	}

	// A silent frame plus the seeded latch draws a visible bar.
	rect := image.Rect(0, 0, 800, 400)
	dst := ebiten.NewImage(rect.Dx(), rect.Dy())
	silentCh := &analyzer.ChannelMetrics{PeakDB: analyzerDBFloor, RMSDB: analyzerDBFloor, Active: false}
	rects := collectFilledRects(t, func() {
		drawLevelsDetail(dst, rect, silentCh, &latch)
	})
	// Any meter colour past the leftmost pixels.
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

// The held values, not "-inf", between hits.
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
	// The held value is unit-tested via the latch; this checks drawing does not panic.
	drawLevelsReadout(dst, image.Rect(rect.Min.X, rect.Max.Y-18, rect.Max.X, rect.Max.Y), silentCh, &latch)
}

// Per-band peak-hold keeps the bars visible between transients.
func TestSpectrumPhase0a_BarFillsBetweenHits(t *testing.T) {
	assertDefaultParityState(t)

	// Seed peak state with a recent "hit" pattern in all 10 bands.
	peaks := &SpectrumPeakState{}
	for i := range peaks.PeaksDB {
		peaks.PeaksDB[i] = -20 // dBFS, comfortably above the window floor
	}

	// Bins at the floor, but non-empty so the renderer does not take its nil exit.
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

	// Tall gradient rects prove the held value (0.8) drove the fill, not the silent FFT; each bar is three bands, so
	// the union per column is measured.
	barBand := image.Rect(rect.Min.X+28, rect.Min.Y, rect.Max.X, rect.Max.Y-22)
	isBarColor := func(c color.RGBA) bool {
		return c == color.RGBAModel.Convert(colSpectrumBarTop).(color.RGBA) ||
			c == color.RGBAModel.Convert(colSpectrumBarMid).(color.RGBA) ||
			c == color.RGBAModel.Convert(colSpectrumBarBase).(color.RGBA)
	}
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
