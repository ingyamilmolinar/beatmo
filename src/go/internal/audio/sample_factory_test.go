package audio

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// TestResetSampleToFactory_BuiltinRevertsToSynth — a built-in instrument that
// was converted to a sample (Sampler Save; or a persisted sample reloaded at
// startup) reverts all the way to its synth: the project copy returns to its
// library entry, the user sample is discarded and its persisted copy deleted
// (the "reverse kick-snap won't reset after restart" bug).
func TestResetSampleToFactory_BuiltinRevertsToSynth(t *testing.T) {
	resetLibraryForTest(t)
	resetUserSamplesForTest()
	sink := &fakeSampleSink{}
	restore := SetSampleSink(sink)
	defer restore()

	const instID = roster.KickSnap
	SetInstrumentParam(instID, "post_decay", 0.3)
	PutUserSample(instID, []float32{0.7, -0.7, 0.7}, 44100)

	prior, factory := ResetSampleToFactory(instID)
	if !factory || prior != instID {
		t.Fatalf("ResetSampleToFactory = (%q, %v), want (%q, true)", prior, factory, instID)
	}
	if InstrumentDocModified(instID) {
		t.Errorf("after reset %q's document is still edited, want its library entry", instID)
	}
	if IsUserSample(instID) {
		t.Errorf("after reset %q is still a user sample, want discarded", instID)
	}
	found := false
	for _, d := range sink.deleted {
		if d == instID {
			found = true
		}
	}
	if !found {
		t.Errorf("sink.DeleteSample(%q) not called; deleted=%v", instID, sink.deleted)
	}
}

// TestResetSampleToFactory_UserWAVRestoresOriginal — a user sample with no
// origin recipe (loaded WAV / Save-As) reverts to its first-loaded buffer.
func TestResetSampleToFactory_UserWAVRestoresOriginal(t *testing.T) {
	resetUserSamplesForTest()
	sink := &fakeSampleSink{}
	restore := SetSampleSink(sink)
	defer restore()

	const id = "user.sample.wav"
	SaveUserSample(id, []float32{0.1, 0.2, 0.3}, 48000) // first load = original
	SaveUserSample(id, []float32{0.9, 0.8}, 48000)      // edited re-save

	prior, factory := ResetSampleToFactory(id)
	if factory || prior != "" {
		t.Fatalf("ResetSampleToFactory = (%q, %v), want (\"\", false)", prior, factory)
	}
	cur, ok := UserSamplePCM(id)
	if !ok || len(cur.PCM) != 3 || cur.PCM[0] != 0.1 {
		t.Errorf("after reset buffer = %+v, want pristine 3-sample original", cur)
	}
	saved, ok := sink.saved[id]
	if !ok || len(saved.PCM) != 3 || saved.PCM[0] != 0.1 {
		t.Errorf("reset did not re-persist original via sink; saved=%+v", saved)
	}
}

// TestResetSampleToFactory_NoOpWhenUnknown — unknown id is a safe no-op.
func TestResetSampleToFactory_NoOpWhenUnknown(t *testing.T) {
	resetUserSamplesForTest()
	sink := &fakeSampleSink{}
	restore := SetSampleSink(sink)
	defer restore()

	prior, factory := ResetSampleToFactory("does.not.exist")
	if factory || prior != "" {
		t.Errorf("ResetSampleToFactory(unknown) = (%q, %v), want (\"\", false)", prior, factory)
	}
	if len(sink.deleted) != 0 || len(sink.saved) != 0 {
		t.Errorf("unknown reset touched sink: deleted=%v saved=%v", sink.deleted, sink.saved)
	}
}
