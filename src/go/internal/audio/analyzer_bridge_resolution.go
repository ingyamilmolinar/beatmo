package audio

import "github.com/ingyamilmolinar/beatmo/internal/analyzer"

// Analyser resolution for the browser (WASM) bridge.
//
// Untagged on purpose. These numbers are browser-only — the desktop
// analyzer service runs its own FFT over the channel's waveform
// (analyzer.Config.FFTSize) and never reads them — but the renderer's
// band-coverage guard has to assert against the resolution the browser
// ACTUALLY ships, and that guard runs in the non-js fast test path. Behind
// a `//go:build js` tag the constants would be invisible to it and the
// guard would be asserting against nothing.
//
// See internal/ui/spectrum_bin_resolution_test.go.
const (
	// BridgeAnalyserFFTSize is the AnalyserNode fftSize the bridge asks for
	// on every channel analyser.
	//
	// It is a FREQUENCY-RESOLUTION choice, not a latency one, and it is the
	// SAME number the desktop analyzer service uses — see
	// analyzer.SpectrumFFTSize for the display invariant that fixes it. At
	// the previous 512 the bins were 93.8 Hz apart before the snapshot was
	// even downsampled.
	BridgeAnalyserFFTSize = analyzer.SpectrumFFTSize

	// BridgeSpectrumBins is how many spectrum bins the bridge asks JS for
	// per snapshot. Equal to the analyser's own bin count
	// (fftSize/2), i.e. NO downsampling.
	//
	// JS's channelAnalyzerSnapshot groups bins LINEARLY, so any downsample
	// costs resolution where it is scarcest — at the bottom, where the
	// display bands are an octave wide and the bins are not. Its default of
	// 64 threw away 15 of every 16 bins and left the bass bands empty.
	BridgeSpectrumBins = BridgeAnalyserFFTSize / 2

	// BridgeWaveformSamples caps the time-domain samples exported per
	// snapshot, INDEPENDENT of fftSize.
	//
	// One AnalyserNode governs both domains, so raising fftSize for the
	// spectrum would otherwise quadruple every waveform crossing the bridge
	// — the scope traces, the Wave tab and the per-stage taps all read
	// Waveform, and none of them wants a longer window. The cap keeps that
	// side byte-for-byte where it was; JS exports the most RECENT samples,
	// so the tap is unchanged.
	BridgeWaveformSamples = 512
)
