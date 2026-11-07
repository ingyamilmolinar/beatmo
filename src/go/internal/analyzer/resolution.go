package analyzer

// The Spectrum tab's FFT length on BOTH platforms (the desktop observer and BridgeAnalyserFFTSize). A display
// invariant: a band holding no bin reports the floor, indistinguishable from silence, so SampleRate/FFTSize must stay
// under the narrowest band (24 Hz, 20-44 Hz) at every rate; 1024 does not. Larger costs per-tick FFT for nothing shown.
const SpectrumFFTSize = 2048
