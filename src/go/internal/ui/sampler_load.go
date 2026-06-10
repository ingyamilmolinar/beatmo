package ui

// samplerWAVLoadFn picks and decodes a WAV file, returning mono PCM at the
// engine sample rate and ok=false on cancel / error / no loader wired. The
// default is set in init() by the platform-specific variant
// (sampler_load_notjs.go desktop, sampler_load_wasm.go browser); the test build
// leaves it nil (Load WAV is a no-op) and tests inject a fake via
// SwapSamplerWAVLoadFnForTest.
var samplerWAVLoadFn func() (pcm []float32, sr int, ok bool)

// samplerLoadWAVImpl runs the wired WAV loader and installs the result into the
// sampler's working buffer.
func (dv *DrumView) samplerLoadWAVImpl() {
	if samplerWAVLoadFn == nil {
		return
	}
	if pcm, sr, ok := samplerWAVLoadFn(); ok && len(pcm) > 0 {
		dv.sampler.loadPCM(pcm, sr)
	}
}

// SwapSamplerWAVLoadFnForTest installs a fake WAV loader and returns the
// previous value so tests can exercise the load→edit path deterministically.
func SwapSamplerWAVLoadFnForTest(fn func() ([]float32, int, bool)) func() ([]float32, int, bool) {
	prev := samplerWAVLoadFn
	samplerWAVLoadFn = fn
	return prev
}
