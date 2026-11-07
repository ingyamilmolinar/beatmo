package audio

import "github.com/ingyamilmolinar/beatmo/internal/analyzer"

// Untagged on purpose: these are browser-only, but the renderer's band-coverage guard runs in the non-js test path
// and must assert against what the browser actually ships.
const (
	// A FREQUENCY-RESOLUTION choice, the same number desktop uses (analyzer.SpectrumFFTSize fixes it).
	BridgeAnalyserFFTSize = analyzer.SpectrumFFTSize

	// The analyser's own bin count, no downsampling: JS groups bins LINEARLY, so a downsample costs resolution where the
	// octave-wide bass bands have the fewest bins.
	BridgeSpectrumBins = BridgeAnalyserFFTSize / 2

	// Independent of fftSize: one AnalyserNode serves both domains, and raising fftSize for the spectrum must not
	// lengthen every waveform the scope, Wave tab and stage taps read. JS exports the most RECENT samples.
	BridgeWaveformSamples = 512
)
