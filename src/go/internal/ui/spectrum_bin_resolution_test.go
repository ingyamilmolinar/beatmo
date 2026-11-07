package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// A band holding no FFT bin reads as the floor, the same as silence, so the bin width must be finer than the
// narrowest display band on every platform at every rate. The spectrum here is UNIFORMLY loud, so a floored band
// can only be resolution.

// The shapes production actually hands the renderer.
func shippedSpectrumShapes() []struct {
	name string
	bins int
	sr   int
} {
	desktop := analyzer.DefaultConfig().FFTSize/2 + 1
	return []struct {
		name string
		bins int
		sr   int
	}{
		{"browser bridge @48kHz", audio.BridgeSpectrumBins, 48000},
		{"browser bridge @44.1kHz", audio.BridgeSpectrumBins, 44100},
		{"desktop analyzer service @44.1kHz", desktop, 44100},
		{"desktop analyzer service @48kHz", desktop, 48000},
	}
}

func TestSpectrumBinsCoverEveryDisplayBand(t *testing.T) {
	for _, shape := range shippedSpectrumShapes() {
		t.Run(shape.name, func(t *testing.T) {
			// An unmeasured band is then a resolution artefact, never the signal.
			spec := make([]float64, shape.bins)
			for i := range spec {
				spec[i] = 0.5
			}
			fft, freq := SpectrumLinearToDBBins(spec, shape.sr)
			_, measured := spectrumBandDB(fft, freq, isoBands)
			for b, ok := range measured {
				if !ok {
					t.Errorf("band %d (%.0f-%.0f Hz) holds NO FFT bin at %d bins / %d Hz "+
						"(%.1f Hz per bin) — it reports the reference floor whatever the "+
						"signal does, so anything living there renders as an empty plot",
						b, isoBands[b][0], isoBands[b][1], shape.bins, shape.sr, freq[1])
				}
			}
		})
	}
}

// As arithmetic, so a change to the band table is checked too.
func TestSpectrumBinWidthIsFinerThanTheNarrowestBand(t *testing.T) {
	narrowest := isoBands[0][1] - isoBands[0][0]
	for _, b := range isoBands {
		if w := b[1] - b[0]; w < narrowest {
			narrowest = w
		}
	}
	for _, shape := range shippedSpectrumShapes() {
		binHz := (float64(shape.sr) / 2) / float64(shape.bins)
		if binHz >= narrowest {
			t.Errorf("%s: %.1f Hz per bin does not resolve the narrowest display band (%.0f Hz wide)",
				shape.name, binHz, narrowest)
		}
	}
}
