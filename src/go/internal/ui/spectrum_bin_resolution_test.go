package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// The Spectrum tab aggregates FFT bins into display bands. A band that
// contains no bins is reported at the reference floor — indistinguishable
// from silence — so the analyser's bin width has to be finer than the
// NARROWEST display band, on every platform, at every sample rate the
// product runs at.
//
// It was not. The browser bridge delivered 64 linear bins spanning
// 0..Nyquist (375 Hz apart at 48 kHz) while the narrowest ISO band is 24 Hz
// wide, so the four lowest bands — 20-355 Hz, where a kick lives — held no
// bins whatsoever. The Spectrum tab rendered an empty plot for any
// per-instrument channel in the browser; desktop, at 43 Hz per bin, was
// correct throughout, so no Go test and no desktop screenshot could see it.
// The one visible clue was that the lowest thing ever drawn sat near
// 500 Hz: the 355-710 Hz band, the first one with a bin in it.
//
// Every fixture in the Go suite fed a spectrum that decays from bin 0, so
// the mid and high bands always carried data and the hole at the bottom was
// invisible. These tests feed a spectrum that is UNIFORMLY LOUD: any band
// that comes back at the floor did so purely because of resolution.

// shippedSpectrumShapes enumerates the (bin count, sample rate) pairs the
// renderer is actually handed in production.
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
			// Uniformly loud: an unmeasured band is a resolution artefact,
			// never a property of the signal.
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

// TestSpectrumBinWidthIsFinerThanTheNarrowestBand states the same invariant
// as arithmetic rather than as a sampled result, so a future change to the
// band table is checked too.
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
