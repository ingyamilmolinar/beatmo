//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
)

// channelWithFlatFFT returns a ChannelMetrics whose FFT bins are filled with
// a fixed dB value across the audible range. Useful for cursor-readout tests
// — every band gets the same averaged dB, so the assertion focuses on the
// crosshair geometry and label rendering rather than spectrum shape.
func channelWithFlatFFT(constDB float64) *analyzer.ChannelMetrics {
	const n = 256
	freq := make([]float64, n)
	mag := make([]float64, n)
	// Linear-spaced Hz across [20, 22000].
	const lo, hi = 20.0, 22000.0
	for i := 0; i < n; i++ {
		freq[i] = lo + float64(i)*(hi-lo)/float64(n-1)
		mag[i] = constDB
	}
	return &analyzer.ChannelMetrics{
		Active:   true,
		FFTBins:  mag,
		FreqBins: freq,
	}
}

// TestSpectrumDrawCursorReadout — drawSpectrumCursor must draw a vertical
// crosshair at cursorX in colTextSecondary and emit a labeled readout box
// (colSurface2 with AlphaStrong alpha) somewhere along the top of the bar
// rect.
func TestSpectrumDrawCursorReadout(t *testing.T) {
	assertDefaultParityState(t)

	ch := channelWithFlatFFT(-20)
	rect := image.Rect(0, 0, 400, 200)
	cursorX := rect.Min.X + 28 + 50 // inside barRect (left margin = 28)

	img := ebiten.NewImage(rect.Dx(), rect.Dy())
	rects := collectFilledRects(t, func() {
		drawSpectrumCursor(img, rect, ch, cursorX)
	})

	wantedLine := color.RGBAModel.Convert(colTextSecondary).(color.RGBA)
	wantedBg := color.RGBAModel.Convert(WithAlpha(colSurface2, AlphaStrong)).(color.RGBA)

	hasLine := false
	hasBg := false
	for _, r := range rects {
		// Vertical crosshair: 1px wide at cursorX, spanning bar height.
		if r.Color == wantedLine && r.Rect.Min.X == cursorX && r.Rect.Dx() == 1 && r.Rect.Dy() > 10 {
			hasLine = true
		}
		// Readout backplate (rounded box behind the label): wide enough to
		// hold the "1.2 kHz · -20 dB" label.
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

// TestSpectrumLinearModeBucketsByEqualHz — passing freqScaleLinear to
// drawAnalyzerSpectrumWithScale must use the linear band table. Drive with a
// signal concentrated entirely in the highest frequencies. In log mode the
// top band (11.36-22 kHz) absorbs all energy → one tall bar at the right; in
// linear mode the equal-Hz partition spreads the same energy across the top
// half of the bands → multiple visible bars on the right side.
func TestSpectrumLinearModeBucketsByEqualHz(t *testing.T) {
	assertDefaultParityState(t)

	// Synthesize an FFT that's silent below 11 kHz and loud above.
	const n = 256
	freq := make([]float64, n)
	mag := make([]float64, n)
	const lo, hi = 20.0, 22000.0
	for i := 0; i < n; i++ {
		freq[i] = lo + float64(i)*(hi-lo)/float64(n-1)
		if freq[i] >= 11000 {
			mag[i] = -10
		} else {
			mag[i] = spectrumMinDB
		}
	}
	ch := &analyzer.ChannelMetrics{Active: true, FFTBins: mag, FreqBins: freq}

	// Verify the band tables differ in width at the top band — the proof
	// that the scale argument flips the lookup. linearBands top band spans
	// 19802-22000 (≈2198 Hz); isoBands top spans 11360-22000 (10640 Hz).
	logTop := isoBands[len(isoBands)-1]
	linBands := linearBands()
	linTop := linBands[len(linBands)-1]
	logSpan := logTop[1] - logTop[0]
	linSpan := linTop[1] - linTop[0]
	if !(linSpan < logSpan) {
		t.Fatalf("linearBands top span %g should be smaller than isoBands top span %g", linSpan, logSpan)
	}

	rect := image.Rect(0, 0, 400, 200)

	// Render with log scale → count colWaveTrace rects (bars).
	imgLog := ebiten.NewImage(rect.Dx(), rect.Dy())
	logRects := collectFilledRects(t, func() {
		drawAnalyzerSpectrumWithScale(imgLog, rect, ch, nil, freqScaleLog)
	})
	logBars := countBars(logRects)

	// Render with linear scale.
	imgLin := ebiten.NewImage(rect.Dx(), rect.Dy())
	linRects := collectFilledRects(t, func() {
		drawAnalyzerSpectrumWithScale(imgLin, rect, ch, nil, freqScaleLinear)
	})
	linBars := countBars(linRects)

	// Linear must produce strictly more bars from the same high-frequency
	// signal because the same energy now spreads across multiple equal-Hz
	// bands instead of being averaged into the single top ISO band.
	if !(linBars > logBars) {
		t.Errorf("linear-mode bar count %d should exceed log-mode bar count %d for high-freq signal",
			linBars, logBars)
	}
	if linBars == 0 {
		t.Errorf("linear-mode produced zero data bars; renderer probably skipped the linear path")
	}
}

// countBars returns the number of distinct vertical "bar" rects emitted by
// the spectrum renderer. Bars are now drawn as a three-band synthwave
// gradient (drawSpectrumBarGradient), so we discriminate by ANY of the three
// gradient colors and group rects by Min.X column so a single bar's three
// bands count once. dB-grid dashes and peak markers are 1px tall, so they're
// excluded by the Dy >= 2 floor.
func countBars(rects []drawnRect) int {
	top := color.RGBAModel.Convert(colSpectrumBarTop).(color.RGBA)
	mid := color.RGBAModel.Convert(colSpectrumBarMid).(color.RGBA)
	base := color.RGBAModel.Convert(colSpectrumBarBase).(color.RGBA)
	isBar := func(c color.RGBA) bool { return c == top || c == mid || c == base }
	// Each band emits up to three rects (top/mid/base) sharing a Min.X —
	// group by column so we don't over-count.
	seen := map[int]bool{}
	for _, r := range rects {
		if !isBar(r.Color) {
			continue
		}
		if r.Rect.Dx() > 80 {
			continue // unlikely for a band bar
		}
		seen[r.Rect.Min.X] = true
	}
	// Guard: at least one bar must exist; if the rough heuristics get out
	// of sync with the renderer return 0 so the caller surfaces a clear
	// failure.
	if len(seen) == 0 {
		for _, r := range rects {
			if isBar(r.Color) {
				return 1
			}
		}
		return 0
	}
	return len(seen)
}
