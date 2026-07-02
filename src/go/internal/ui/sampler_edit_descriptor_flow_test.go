//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// sampler_edit_descriptor_flow_test.go — the Sampler-tab Save on a SYNTH
// source is non-destructive: it stores the edit as a per-instrument
// descriptor (audio.SetSampleEdit) and KEEPS the recipe binding, so the synth
// stays the source of truth and later synth changes always take effect.
// Only WAV sources bake PCM.

// sampleEditApproxEqual compares two SampleEdits with a tolerance on the float
// fields (trim fractions, pitch, gain, fades) and exact on the bool flags. Used
// where the reverse trim-mirror introduces sub-ulp float drift on round-trip.
func sampleEditApproxEqual(a, b audio.SampleEdit, eps float64) bool {
	close := func(x, y float64) bool {
		d := x - y
		if d < 0 {
			d = -d
		}
		return d <= eps
	}
	return close(a.StartFrac, b.StartFrac) && close(a.EndFrac, b.EndFrac) &&
		close(a.TransposeSemis, b.TransposeSemis) && close(a.DetuneCents, b.DetuneCents) &&
		close(float64(a.GainDB), float64(b.GainDB)) &&
		close(a.FadeInMs, b.FadeInMs) && close(a.FadeOutMs, b.FadeOutMs) &&
		a.Reverse == b.Reverse && a.Normalize == b.Normalize
}

// cleanupSamplerEditState snapshots and restores the global audio state the
// sampler Save flow mutates (TEST GOTCHA: save() writes GLOBAL maps).
func cleanupSamplerEditState(t *testing.T, instID string) {
	t.Helper()
	origBinding := audio.RecipeForInstrument(instID)
	origEdit, hadEdit := audio.SampleEditFor(instID)
	t.Cleanup(func() {
		audio.BindInstrumentToRecipe(instID, origBinding)
		if hadEdit {
			audio.SetSampleEdit(instID, origEdit)
		} else {
			audio.ClearSampleEdit(instID)
		}
	})
}

func TestSamplerSaveOnSynthKeepsBindingAndStoresDescriptor(t *testing.T) {
	assertDefaultParityState(t)
	cleanupSamplerEditState(t, "kick")
	audio.BindInstrumentToRecipe("kick", "drum-kick")
	audio.ClearSampleEdit("kick")

	s := &samplerState{}
	s.reset()
	s.captureFromSynth("kick")
	s.startFrac, s.endFrac = 0.25, 0.75
	s.gainDB = -6
	s.reverseBuffer()
	s.save()

	if got := audio.RecipeForInstrument("kick"); got != "drum-kick" {
		t.Errorf("after Save, RecipeForInstrument(kick)=%q, want drum-kick — the synth MUST stay the source of truth", got)
	}
	e, ok := audio.SampleEditFor("kick")
	if !ok {
		t.Fatal("after Save, no sample-edit descriptor stored")
	}
	if want := s.editDescriptor(); e != want {
		t.Errorf("descriptor = %+v, want %+v", e, want)
	}
	if !e.Reverse {
		t.Error("descriptor must carry the declarative reverse flag")
	}
	if _, isSample := audio.UserSamplePCM("kick"); isSample {
		t.Error("Save on a synth source must NOT register baked PCM (that's the drift bug)")
	}
}

// TestSamplerSaveOnWAVStoresDescriptorNonDestructively: a WAV / user-sample
// Save is now NON-DESTRUCTIVE and real-time, mirroring the synth path. It stores
// the edit as a descriptor (already live via commitSamplerEdit), KEEPS the
// pristine source PCM, and re-registers the BAKED buffer for playback — it does
// NOT replace the stored PCM with a baked one. Replaces the old bake-only test.
func TestSamplerSaveOnWAVStoresDescriptorNonDestructively(t *testing.T) {
	assertDefaultParityState(t)
	const id = "user.sample.wavsave"
	cleanupSamplerEditState(t, id)

	pcm := make([]float32, 800)
	for i := range pcm {
		pcm[i] = 0.5
	}
	// ensureSamplerLoaded seeds the pristine PCM into the store; mirror that.
	audio.PutUserSample(id, pcm, 48000)

	s := &samplerState{}
	s.reset()
	s.loadFromInstrument(id, append([]float32(nil), pcm...), 48000, samplerSourceWAV)
	s.startFrac = 0.25
	s.save()

	if !audio.HasSampleEdit(id) {
		t.Error("WAV-source Save must store the sample-edit descriptor (real-time, non-destructive)")
	}
	rec, ok := audio.UserSamplePCM(id)
	if !ok || len(rec.PCM) != 800 {
		t.Errorf("WAV-source Save must KEEP the pristine PCM (len=%d, want 800), not bake over it", len(rec.PCM))
	}
	reg, ok := audio.LastRegisteredSamplePCMForTest(id)
	if !ok || len(reg.PCM) != 600 {
		t.Errorf("WAV-source Save must re-register the baked playable buffer (len=%d, want 600 — quarter-trimmed 800)", len(reg.PCM))
	}
}

func TestSamplerEnsureLoadedRestoresDescriptorIntoEditor(t *testing.T) {
	g := newSamplerTabGame(t)
	const id = "descload.synth.test"
	cleanupSamplerEditState(t, id)
	audio.BindInstrumentToRecipe(id, "drum-kick")
	want := audio.SampleEdit{StartFrac: 0.2, EndFrac: 0.8, TransposeSemis: 3, DetuneCents: -10, GainDB: -4.5, Normalize: true, Reverse: true, FadeInMs: samplerFadeMs, FadeOutMs: samplerFadeMs}
	audio.SetSampleEdit(id, want)

	g.drum.ensureSamplerLoaded(id)
	s := &g.drum.sampler
	if !s.hasBuffer() {
		t.Fatal("no buffer after descriptor-bearing synth load")
	}
	if s.source != samplerSourceSynth {
		t.Fatalf("source=%v, want synth (descriptor keeps the synth source of truth)", s.source)
	}
	// Tolerance compare: a reversed descriptor's trim is mirrored to the display
	// frame on load and back to the source frame on save (editDescriptor), and the
	// 1-x mirror is not exactly self-inverse in float64 (1-(1-0.2) ≈ 0.2 to ~1e-16).
	// The round-trip is faithful well below a single sample, so compare with eps.
	if got := s.editDescriptor(); !sampleEditApproxEqual(got, want, 1e-9) {
		t.Errorf("editor state after load = %+v, want the saved descriptor %+v", got, want)
	}
	if !s.fadeOn || !s.normalize {
		t.Errorf("fadeOn=%v normalize=%v, want both true", s.fadeOn, s.normalize)
	}
}

func TestSamplerResetClearsDescriptorKeepsBinding(t *testing.T) {
	g := newSamplerTabGame(t)
	const id = "descreset.synth.test"
	cleanupSamplerEditState(t, id)
	audio.BindInstrumentToRecipe(id, "drum-kick")
	audio.SetSampleEdit(id, audio.SampleEdit{StartFrac: 0.3, EndFrac: 1})

	g.drum.sampler.captureID = id
	g.drum.samplerReset()

	if audio.HasSampleEdit(id) {
		t.Error("Reset must clear the sample-edit descriptor")
	}
	if got := audio.RecipeForInstrument(id); got != "drum-kick" {
		t.Errorf("Reset must keep the recipe binding, got %q", got)
	}
}

// stubSampleEditSink records SaveSampleEdit / DeleteSampleEdit calls.
type stubSampleEditSink struct {
	saved   map[string]map[string]float64
	deleted []string
}

func (s *stubSampleEditSink) SaveSampleEdit(instID string, fields map[string]float64) error {
	if s.saved == nil {
		s.saved = map[string]map[string]float64{}
	}
	s.saved[instID] = fields
	return nil
}

func (s *stubSampleEditSink) DeleteSampleEdit(instID string) error {
	s.deleted = append(s.deleted, instID)
	return nil
}

func TestSamplerSavePersistsDescriptorViaSink(t *testing.T) {
	assertDefaultParityState(t)
	cleanupSamplerEditState(t, "kick")
	audio.BindInstrumentToRecipe("kick", "drum-kick")
	audio.ClearSampleEdit("kick")

	sink := &stubSampleEditSink{}
	SetSampleEditSink(sink)
	t.Cleanup(func() { SetSampleEditSink(nil) })

	s := &samplerState{}
	s.reset()
	s.captureFromSynth("kick")
	s.startFrac = 0.25
	s.save()

	fields, ok := sink.saved["kick"]
	if !ok {
		t.Fatal("Save did not persist the descriptor via the sink")
	}
	if got := audio.SampleEditFromFields(fields); got != s.editDescriptor() {
		t.Errorf("persisted fields decode to %+v, want %+v", got, s.editDescriptor())
	}
}

func TestSamplerResetDeletesPersistedDescriptor(t *testing.T) {
	g := newSamplerTabGame(t)
	const id = "descsinkreset.test"
	cleanupSamplerEditState(t, id)
	audio.BindInstrumentToRecipe(id, "drum-kick")
	audio.SetSampleEdit(id, audio.SampleEdit{StartFrac: 0.3, EndFrac: 1})

	sink := &stubSampleEditSink{}
	SetSampleEditSink(sink)
	t.Cleanup(func() { SetSampleEditSink(nil) })

	g.drum.sampler.captureID = id
	g.drum.samplerReset()

	if len(sink.deleted) != 1 || sink.deleted[0] != id {
		t.Errorf("Reset must delete the persisted descriptor, got deletes %v", sink.deleted)
	}
}

// TestSamplerSaveAsDoesNotLeakDescriptor pins the Save vs Save As split:
// Save As bakes the edit INTO the new instrument's PCM (a genuinely new
// sample), so the descriptor must NOT be copied onto the new id — and the
// original synth instrument keeps both its descriptor and recipe binding.
func TestSamplerSaveAsDoesNotLeakDescriptor(t *testing.T) {
	assertDefaultParityState(t)
	cleanupSamplerEditState(t, "kick")
	audio.BindInstrumentToRecipe("kick", "drum-kick")
	audio.ClearSampleEdit("kick")

	s := &samplerState{}
	s.reset()
	s.captureFromSynth("kick")
	s.startFrac = 0.25
	s.save() // non-destructive: stores the descriptor on kick

	newID := s.saveAs("Leak Check")
	if newID == "" {
		t.Fatal("saveAs returned empty id")
	}
	if audio.HasSampleEdit(newID) {
		t.Errorf("Save As must not leak the descriptor onto the new instrument %q (the edit is baked into its PCM)", newID)
	}
	if _, ok := audio.UserSamplePCM(newID); !ok {
		t.Errorf("Save As must register baked PCM for the new instrument %q", newID)
	}
	if !audio.HasSampleEdit("kick") {
		t.Error("Save As must leave the ORIGINAL instrument's descriptor in place")
	}
	if got := audio.RecipeForInstrument("kick"); got != "drum-kick" {
		t.Errorf("Save As must leave the original recipe binding intact, got %q", got)
	}
}

func TestSamplerSourceSignatureFoldsDescriptor(t *testing.T) {
	assertDefaultParityState(t)
	const id = "descsig.synth.test"
	cleanupSamplerEditState(t, id)
	audio.ClearSampleEdit(id)

	before := samplerSourceSignatureFn(id)
	audio.SetSampleEdit(id, audio.SampleEdit{StartFrac: 0.1, EndFrac: 1})
	after := samplerSourceSignatureFn(id)
	if before == after {
		t.Error("source signature must change when the sample-edit descriptor changes")
	}
}
