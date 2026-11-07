//go:build test

package audio

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// The native Voice interface is empty under -tags test, so capture returns a deterministic synthetic buffer.

// A 220 Hz decaying sine.
func RenderInstrumentOneShot(id instrument.ID) ([]float32, int) {
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

// The stub never applies descriptors, so raw is the same buffer.
func RenderInstrumentOneShotRaw(id instrument.ID) ([]float32, int) {
	return RenderInstrumentOneShot(id)
}

// So tests can assert the live re-registration a Sampler edit performs.
var lastRegisteredSamplePCM = map[instrument.ID]SampleRecord{}

// RegisterSamplePCM registers an in-memory PCM buffer as a playable instrument.
func RegisterSamplePCM(id instrument.ID, pcm []float32, sr int) {
	registerStub(id)
	lastRegisteredSamplePCM[id] = SampleRecord{PCM: append([]float32(nil), pcm...), SampleRate: sr}
}

// Test build only.
func LastRegisteredSamplePCMForTest(id instrument.ID) (SampleRecord, bool) {
	rec, ok := lastRegisteredSamplePCM[id]
	return rec, ok
}

var sampleUnregisterCalls []instrument.ID

// So the built-in synth render is heard again. The test build only records the call.
func UnregisterSamplePCM(id instrument.ID) {
	sampleUnregisterCalls = append(sampleUnregisterCalls, id)
}

// sampleUnregisterCallsForTest returns the recorded UnregisterSamplePCM ids.
func sampleUnregisterCallsForTest() []instrument.ID {
	return append([]instrument.ID(nil), sampleUnregisterCalls...)
}

// resetSampleUnregisterCallsForTest clears the recorder between tests.
func resetSampleUnregisterCallsForTest() { sampleUnregisterCalls = nil }
