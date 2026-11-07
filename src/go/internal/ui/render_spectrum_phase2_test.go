//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
)

// The MAX marker stays at the loudest level seen; unlike the ~500 ms peak-hold it never decays until Reset.
func TestSpectrumPhase2_PersistentMaxMarker(t *testing.T) {
	var st SpectrumPeakState

	// Levels are dBFS, not normalised heights.
	loud := [10]float64{}
	for i := range loud {
		loud[i] = spectrumMinDB
	}
	loud[4] = -12
	st.UpdateMax(loud)
	if st.maxDB(4) > -12 {
		t.Fatalf("MAX should rise to the live peak: got %v, want <= -12 dB", st.maxDB(4))
	}

	// Feed many quiet frames.
	quiet := [10]float64{}
	for i := range quiet {
		quiet[i] = spectrumMinDB
	}
	for i := 0; i < 500; i++ {
		st.UpdateMax(quiet)
	}
	if st.maxDB(4) != -12 {
		t.Fatalf("MAX should persist across 500 quiet frames, got %v", st.maxDB(4))
	}
}

func TestSpectrumPhase2_ResetMaxClearsAllBands(t *testing.T) {
	var st SpectrumPeakState
	st.MaxPeaksDB = [10]float64{-1, -2, -3, -4, -5, -6, -7, -8, -9, -10}
	st.ResetMax()
	for i, v := range st.MaxPeaksDB {
		if v != spectrumNoPeakDB {
			t.Errorf("MaxPeaks[%d] after reset: got %v, want 0", i, v)
		}
	}
}

// Painted at the MAX positions on top of the decaying peak markers.
func TestSpectrumPhase2_MaxMarkerRendered(t *testing.T) {
	rect := image.Rect(0, 0, 600, 120)
	dst := ebiten.NewImage(rect.Dx(), rect.Dy())

	// Non-empty FFT/freq pairs, so the renderer's active check passes.
	const n = 64
	freq := make([]float64, n)
	fft := make([]float64, n)
	for i := 0; i < n; i++ {
		freq[i] = 20 + float64(i)*200
		fft[i] = -30
	}
	ch := &analyzer.ChannelMetrics{Active: true, FFTBins: fft, FreqBins: freq}

	// Pre-set MAX markers near the top of bands 1, 4, 8 so we can find them.
	st := &SpectrumPeakState{}
	st.MaxPeaksDB[1] = -10
	st.MaxPeaksDB[4] = -24
	st.MaxPeaksDB[8] = -6

	// The renderer still takes the detail flag.
	rects := collectFilledRects(t, func() {
		drawAnalyzerSpectrumImpl(dst, rect, ch, st, freqScaleLog, spectrumChromeOpts{detail: true})
	})

	// The same hue as the decay peak, counted across positions.
	got := rectsWithColorInside(rects, rect, genColorVizSpectrumPeakMarker)
	if got < 3 {
		t.Fatalf("expected ≥3 spectrum peak-marker rects (MAX + decay), got %d in %v", got, rect)
	}
}

// A faint wash under the curve and a three-band gradient on each bar, both drawn when there is energy.
func TestSpectrumSynthwaveFill(t *testing.T) {
	rect := image.Rect(0, 0, 600, 160)
	dst := ebiten.NewImage(rect.Dx(), rect.Dy())

	// Decaying spectrum so multiple bands carry energy (varied dB, not floor).
	const n = 64
	freq := make([]float64, n)
	fft := make([]float64, n)
	for i := 0; i < n; i++ {
		freq[i] = 20 + float64(i)*340
		fft[i] = -10 - float64(i) // -10 dB sloping down
	}
	ch := &analyzer.ChannelMetrics{Active: true, FFTBins: fft, FreqBins: freq}

	rects := collectFilledRects(t, func() {
		drawAnalyzerSpectrumWithScale(dst, rect, ch, nil, freqScaleLog)
	})

	// Curve area wash beneath the FFT underlay.
	if got := rectsWithColorInside(rects, rect, colSpectrumCurveFill); got == 0 {
		t.Errorf("expected colSpectrumCurveFill wash beneath the FFT curve, got 0 in %v", rect)
	}
	// Bar gradient: the bright peak band must appear for at least a few bars.
	if got := rectsWithColorInside(rects, rect, colSpectrumBarTop); got < 3 {
		t.Errorf("expected >=3 colSpectrumBarTop gradient rects (one per drawn bar), got %d in %v", got, rect)
	}
}
