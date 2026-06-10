//go:build test

package audio

import "testing"

// TestResetSampleToFactory_RestoresPlaybackRegistration — reverting a built-in
// must un-register the user sample from the playback layer (desktop instrument
// table / WASM render cache) so the synth is actually heard again, not just
// re-bound in the registry. Asserts the UnregisterSamplePCM seam is invoked.
// (The recorder seam is test-build-only, hence this file's build tag.)
func TestResetSampleToFactory_RestoresPlaybackRegistration(t *testing.T) {
	resetUserSamplesForTest()
	sink := &fakeSampleSink{}
	restore := SetSampleSink(sink)
	defer restore()
	resetSampleUnregisterCallsForTest()

	const instID = "kick-1"
	bindBuiltinInstrumentRecipes()
	t.Cleanup(bindBuiltinInstrumentRecipes)
	PutUserSample(instID, []float32{0.7, -0.7}, 44100)

	ResetSampleToFactory(instID)

	found := false
	for _, id := range sampleUnregisterCallsForTest() {
		if id == instID {
			found = true
		}
	}
	if !found {
		t.Errorf("UnregisterSamplePCM(%q) not called; reversed sample would keep playing. calls=%v", instID, sampleUnregisterCallsForTest())
	}
}
