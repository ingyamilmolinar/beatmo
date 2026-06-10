//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
)

// TestSpectrumPhase2_PersistentMaxMarker — once a band crosses a higher
// dB value, the persistent MAX marker should stay at that level even
// after the live value drops. Distinct from the existing peak-hold
// (which decays in ~500 ms): MAX never decays until Reset is called.
func TestSpectrumPhase2_PersistentMaxMarker(t *testing.T) {
	var st SpectrumPeakState

	// Feed a loud burst on band 4 (Mids).
	loud := [10]float64{}
	loud[4] = 0.85
	st.UpdateMax(loud)
	if st.MaxPeaks[4] < 0.85 {
		t.Fatalf("MAX should rise to live peak: got %v, want ≥0.85", st.MaxPeaks[4])
	}

	// Feed many quiet frames.
	quiet := [10]float64{}
	for i := 0; i < 500; i++ {
		st.UpdateMax(quiet)
	}
	if st.MaxPeaks[4] < 0.85 {
		t.Fatalf("MAX should persist across 500 quiet frames, got %v", st.MaxPeaks[4])
	}
}

// TestSpectrumPhase2_ResetMaxClearsAllBands — calling ResetMax on the
// state should zero every band's MAX.
func TestSpectrumPhase2_ResetMaxClearsAllBands(t *testing.T) {
	var st SpectrumPeakState
	st.MaxPeaks = [10]float64{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0}
	st.ResetMax()
	for i, v := range st.MaxPeaks {
		if v != 0 {
			t.Errorf("MaxPeaks[%d] after reset: got %v, want 0", i, v)
		}
	}
}

// TestSpectrumPhase2_MaxMarkerRendered — when SpectrumPeakState.MaxPeaks
// has non-zero entries, the spectrum renderer should paint
// genColorVizSpectrumPeakMarker rects at the MAX positions on top of the
// existing decaying peak-hold markers.
func TestSpectrumPhase2_MaxMarkerRendered(t *testing.T) {
	rect := image.Rect(0, 0, 600, 120)
	dst := ebiten.NewImage(rect.Dx(), rect.Dy())

	// Build a minimal ChannelMetrics with non-empty FFT/Freq pairs so the
	// renderer's "active" check passes.
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
	st.MaxPeaks[1] = 0.9
	st.MaxPeaks[4] = 0.7
	st.MaxPeaks[8] = 0.95

	rects := collectFilledRects(t, func() {
		drawAnalyzerSpectrumWithScale(dst, rect, ch, st, freqScaleLog)
	})

	// MAX markers are rendered in genColorVizSpectrumPeakMarker (same hue
	// as the decay peak), but at a 2px-tall mark distinguishable by
	// surviving in many positions. Count their occurrences.
	got := rectsWithColorInside(rects, rect, genColorVizSpectrumPeakMarker)
	if got < 3 {
		t.Fatalf("expected ≥3 spectrum peak-marker rects (MAX + decay), got %d in %v", got, rect)
	}
}
