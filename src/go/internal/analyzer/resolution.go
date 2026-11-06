package analyzer

// SpectrumFFTSize is the analysis FFT length behind the Spectrum tab, on
// BOTH platforms: the desktop service configures its own observer with it
// (audio/engine.go) and the browser bridge asks the AnalyserNode for it
// (audio.BridgeAnalyserFFTSize).
//
// It is fixed by a display invariant, not by taste. The tab aggregates bins
// into fixed frequency bands and a band holding no bins reports the
// reference floor — indistinguishable from silence. The narrowest band is
// 24 Hz wide (20-44 Hz), so the bin width, Nyquist/(FFTSize/2) =
// SampleRate/FFTSize, has to stay under that at every sample rate the
// product runs at:
//
//	FFTSize   44.1 kHz   48 kHz
//	    1024    43.1 Hz   46.9 Hz   <- both too coarse: the 31 Hz band is empty
//	    2048    21.5 Hz   23.4 Hz   <- fits, with little to spare
//
// Two defects came out of the old 1024. In the browser the bridge
// compounded it with a 64-bin downsample (375 Hz per bin) and the four
// lowest bands — everything a kick is made of — rendered as an empty plot;
// desktop at 43 Hz per bin looked correct, so no screenshot of it could
// show the defect. On desktop the same 1024 left the 31 Hz band empty
// whenever the engine ran at 48 kHz.
//
// Raising it further is a real cost (the observer's FFT is per tick, per
// observed channel) for resolution the 10-band display cannot show, so the
// invariant, not the headroom, is the thing to preserve. Guards:
// internal/ui/spectrum_bin_resolution_test.go.
const SpectrumFFTSize = 2048
