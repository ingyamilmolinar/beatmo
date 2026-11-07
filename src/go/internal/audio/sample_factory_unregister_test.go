//go:build test

package audio

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Returning to the library entry is not enough: the playback layer must drop the user sample so the synth is heard.
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
