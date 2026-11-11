//go:build test

package audio

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// TestResetSampleToFactory_RestoresPlaybackRegistration — reverting a built-in
// must un-register the user sample from the playback layer (desktop instrument
// table / WASM render cache) so the synth is actually heard again, not just
// returned to its library entry. Asserts the UnregisterSamplePCM seam is invoked.
// (The recorder seam is test-build-only, hence this file's build tag.)
func TestResetSampleToFactory_RestoresPlaybackRegistration(t *testing.T) {
	resetUserSamplesForTest()
	sink := &fakeSampleSink{}
	restore := SetSampleSink(sink)
	defer restore()
	resetSampleUnregisterCallsForTest()

	const instID = roster.KickSnap
	resetLibraryForTest(t)
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
