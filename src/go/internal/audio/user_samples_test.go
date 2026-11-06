package audio

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// resetUserSamplesForTest clears the registry between tests.
func resetUserSamplesForTest() {
	userSamplesMu.Lock()
	userSamples = map[instrument.ID]SampleRecord{}
	originalUserSamples = map[instrument.ID]SampleRecord{}
	userSamplesMu.Unlock()
}

type fakeSampleSink struct {
	saved   map[instrument.ID]SampleRecord
	deleted []instrument.ID
}

func (f *fakeSampleSink) SaveSample(id instrument.ID, pcm []float32, sr int) {
	if f.saved == nil {
		f.saved = map[instrument.ID]SampleRecord{}
	}
	f.saved[id] = SampleRecord{PCM: append([]float32(nil), pcm...), SampleRate: sr}
}
func (f *fakeSampleSink) DeleteSample(id instrument.ID) { f.deleted = append(f.deleted, id) }

type fakeSampleSource struct {
	recs map[instrument.ID]SampleRecord
}

func (f fakeSampleSource) LoadSamples() map[instrument.ID]SampleRecord { return f.recs }

func TestPutUserSampleStoresAndRegisters(t *testing.T) {
	resetUserSamplesForTest()
	id := "user.sample.put"
	PutUserSample(instrument.ID(id), []float32{0.1, 0.2, 0.3}, 44100)

	if !IsUserSample(instrument.ID(id)) {
		t.Fatalf("IsUserSample(%q) = false after PutUserSample", id)
	}
	rec, ok := UserSamplePCM(instrument.ID(id))
	if !ok {
		t.Fatalf("UserSamplePCM(%q) not found", id)
	}
	if len(rec.PCM) != 3 || rec.SampleRate != 44100 {
		t.Errorf("record = %+v, want 3 samples @ 44100", rec)
	}
	// PutUserSample must also register for playback.
	found := false
	for _, x := range Instruments() {
		if x == instrument.ID(id) {
			found = true
		}
	}
	if !found {
		t.Errorf("PutUserSample did not register %q for playback", id)
	}
}

func TestPutUserSampleCapturesOriginalOnce(t *testing.T) {
	resetUserSamplesForTest()
	id := "user.sample.orig"

	PutUserSample(instrument.ID(id), []float32{0.1, 0.2}, 44100)
	orig, ok := OriginalUserSamplePCM(instrument.ID(id))
	if !ok || len(orig.PCM) != 2 || orig.PCM[0] != 0.1 {
		t.Fatalf("first Put did not capture original: %+v ok=%v", orig, ok)
	}

	// A later Put (an edit re-save) must NOT move the captured original.
	PutUserSample(instrument.ID(id), []float32{0.9, 0.8, 0.7}, 44100)
	orig2, _ := OriginalUserSamplePCM(instrument.ID(id))
	if len(orig2.PCM) != 2 || orig2.PCM[0] != 0.1 {
		t.Errorf("original moved after second Put: %+v, want pristine 2-sample buffer", orig2)
	}
	cur, _ := UserSamplePCM(instrument.ID(id))
	if len(cur.PCM) != 3 || cur.PCM[0] != 0.9 {
		t.Errorf("current buffer = %+v, want the edited 3-sample buffer", cur)
	}
}

func TestSaveUserSamplePersistsViaSink(t *testing.T) {
	resetUserSamplesForTest()
	sink := &fakeSampleSink{}
	restore := SetSampleSink(sink)
	defer restore()

	SaveUserSample("user.sample.persist", []float32{0.5, -0.5}, 48000)
	rec, ok := sink.saved["user.sample.persist"]
	if !ok {
		t.Fatal("SaveUserSample did not call sink.SaveSample")
	}
	if len(rec.PCM) != 2 || rec.SampleRate != 48000 {
		t.Errorf("sink record = %+v, want 2 samples @ 48000", rec)
	}
}

func TestApplySavedSamplesReRegisters(t *testing.T) {
	resetUserSamplesForTest()
	restoreGate := SetUseSampleStore(true)
	defer restoreGate()

	src := fakeSampleSource{recs: map[instrument.ID]SampleRecord{
		"user.sample.a": {PCM: []float32{0.1}, SampleRate: 44100},
		"user.sample.b": {PCM: []float32{0.2, 0.3}, SampleRate: 44100},
	}}
	ApplySavedSamples(src)

	for _, id := range []string{"user.sample.a", "user.sample.b"} {
		if !IsUserSample(instrument.ID(id)) {
			t.Errorf("ApplySavedSamples did not load %q", id)
		}
	}
}

// TestApplySavedSamplesSkipsFactorySynths — "synths remain synths": a
// persisted sample whose id is a factory-recipe-bound builtin (e.g. "snare")
// is a legacy artifact of the old destructive Sampler-Save. Re-registering it
// at startup overrides the synth's playback registration — on WASM that
// deletes RENDER[id] and freezes the instrument so every Synth-tab edit is
// audibly a no-op (the user-reported "knobs/stages/Save do nothing in the
// browser" bug). Startup must skip those entries; pure user samples
// (user.sample.*, no factory binding) still load.
func TestApplySavedSamplesSkipsFactorySynths(t *testing.T) {
	resetUserSamplesForTest()
	restoreGate := SetUseSampleStore(true)
	defer restoreGate()

	src := fakeSampleSource{recs: map[instrument.ID]SampleRecord{
		roster.Snare:    {PCM: []float32{0.1, 0.2}, SampleRate: 48000}, // legacy artifact
		"user.sample.c": {PCM: []float32{0.3}, SampleRate: 48000},      // real user sample
	}}
	ApplySavedSamples(src)

	if IsUserSample(roster.Snare) {
		t.Error("ApplySavedSamples registered a persisted sample over the factory synth 'snare' — the builtin must remain a synth")
	}
	if !IsUserSample("user.sample.c") {
		t.Error("ApplySavedSamples skipped a pure user sample; only factory-bound ids must be skipped")
	}
}

func TestApplySavedSamplesGatedOff(t *testing.T) {
	resetUserSamplesForTest()
	restoreGate := SetUseSampleStore(false)
	defer restoreGate()

	src := fakeSampleSource{recs: map[instrument.ID]SampleRecord{
		"user.sample.gated": {PCM: []float32{0.1}, SampleRate: 44100},
	}}
	ApplySavedSamples(src)
	if IsUserSample("user.sample.gated") {
		t.Error("ApplySavedSamples loaded samples despite gate being off")
	}
}
