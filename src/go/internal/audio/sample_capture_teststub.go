//go:build test

package audio

import "math"

// Test-build variants of the Sampler capture/registration seams. The native
// Voice interface is empty under -tags test, so capture cannot drain a real
// voice; it returns a deterministic synthetic buffer instead, and PCM
// registration just makes the id available like the other stub registrations.

// RenderInstrumentOneShot returns a deterministic synthetic one-shot buffer
// (a 220 Hz sine with exponential decay) so the editing pipeline and UI flows
// are testable without a real C renderer.
func RenderInstrumentOneShot(id string) ([]float32, int) {
	sr := SampleRate()
	n := sr / 2
	out := make([]float32, n)
	const freq = 220.0
	for i := range out {
		t := float64(i) / float64(sr)
		env := math.Exp(-3 * t)
		out[i] = float32(math.Sin(2*math.Pi*freq*t) * env)
	}
	return out, sr
}

// RenderInstrumentOneShotRaw mirrors the native raw-capture seam (render
// ignoring any sample-edit descriptor). The stub render never applies
// descriptors anyway, so it is the same synthetic buffer.
func RenderInstrumentOneShotRaw(id string) ([]float32, int) {
	return RenderInstrumentOneShot(id)
}

// lastRegisteredSamplePCM records the most recent RegisterSamplePCM payload per
// id so tests can assert the live re-registration the Sampler's real-time edit
// flow performs (SetSampleEdit → reapplyUserSampleEdit → RegisterSamplePCM).
var lastRegisteredSamplePCM = map[string]SampleRecord{}

// RegisterSamplePCM registers an in-memory PCM buffer as a playable instrument.
func RegisterSamplePCM(id string, pcm []float32, sr int) {
	registerStub(id)
	lastRegisteredSamplePCM[id] = SampleRecord{PCM: append([]float32(nil), pcm...), SampleRate: sr}
}

// LastRegisteredSamplePCMForTest returns the PCM most recently handed to
// RegisterSamplePCM for id (test build only). Lets the Sampler real-time tests
// verify that an edit re-registered the BAKED buffer for playback.
func LastRegisteredSamplePCMForTest(id string) (SampleRecord, bool) {
	rec, ok := lastRegisteredSamplePCM[id]
	return rec, ok
}

// sampleUnregisterCalls records UnregisterSamplePCM ids so tests can assert the
// factory-reset path drops the user sample from the playback layer.
var sampleUnregisterCalls []string

// UnregisterSamplePCM removes a user sample's playback registration so the
// instrument's built-in synth render is heard again. In the test build it just
// records the call (there is no real playback layer to restore).
func UnregisterSamplePCM(id string) {
	sampleUnregisterCalls = append(sampleUnregisterCalls, id)
}

// sampleUnregisterCallsForTest returns the recorded UnregisterSamplePCM ids.
func sampleUnregisterCallsForTest() []string { return append([]string(nil), sampleUnregisterCalls...) }

// resetSampleUnregisterCallsForTest clears the recorder between tests.
func resetSampleUnregisterCallsForTest() { sampleUnregisterCalls = nil }
