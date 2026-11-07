//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
)

// Every band gets the same dB, so the test is about the crosshair and label.
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

// A colTextSecondary crosshair at cursorX and a readout box (surface-2 at AlphaStrong) along the top.
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
		// Wide enough for "1.2 kHz · -20 dB".
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

// Energy only at the top: log puts it in one tall right bar, linear's equal-Hz bands spread it over several.
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

	// The top band differs: linear spans 19802-22000, ISO 11360-22000.
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

	// Strictly more bars in linear.
	if !(linBars > logBars) {
		t.Errorf("linear-mode bar count %d should exceed log-mode bar count %d for high-freq signal",
			linBars, logBars)
	}
	if linBars == 0 {
		t.Errorf("linear-mode produced zero data bars; renderer probably skipped the linear path")
	}
}

// Any of the three gradient colours, grouped by column; 1 px grid dashes and peak marks fall under the Dy ≥ 2
// floor.
func countBars(rects []drawnRect) int {
	top := color.RGBAModel.Convert(colSpectrumBarTop).(color.RGBA)
	mid := color.RGBAModel.Convert(colSpectrumBarMid).(color.RGBA)
	base := color.RGBAModel.Convert(colSpectrumBarBase).(color.RGBA)
	isBar := func(c color.RGBA) bool { return c == top || c == mid || c == base }
	// A bar's three bands share a Min.X.
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
	// 0 when the heuristics drift, so the caller fails clearly.
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
