//go:build !test && !js

package audio

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// l1 returns the sum of absolute sample differences between two buffers.
// 0 means bit-identical content over the shared prefix.
func l1(a, b []float32) float64 {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	var d float64
	for i := 0; i < n; i++ {
		d += math.Abs(float64(a[i]) - float64(b[i]))
	}
	return d
}

// saveCopyToLibraryForTest is the audio-layer half of the Synth tab's Save:
// the project copy becomes its library entry.
func saveCopyToLibraryForTest(t *testing.T, id string) {
	t.Helper()
	d, ok := InstrumentDoc(instrument.ID(id))
	if !ok {
		t.Fatalf("no document for %q", id)
	}
	d.ID = instrument.ID(id)
	if err := SaveLibraryDoc(d); err != nil {
		t.Fatal(err)
	}
}

// TestSynthSavePersists_RenderUnchangedAfterSave is the faithful root-cause
// test for "after Save the sound reverts". It drives the NATIVE dispatcher
// (newRecipeAwareVoice — the -tags test stub Play is a no-op and cannot
// exercise this) and proves that after Save and a later Reset (the copy back
// on its library entry — also the state after a restart) the voice still
// renders the liked tone, not the bare baseline.
//
// Uses kick: a pitched-sine renderer with no noise, so renders are
// deterministic and byte-comparable across cache invalidations.
func TestSynthSavePersists_RenderUnchangedAfterSave(t *testing.T) {
	resetLibraryForTest(t)
	const id = roster.Kick

	base := drainVoiceToBuffer(newRecipeAwareVoice(id, 44100), 44100)

	// User transposes the voice up an octave — the "liked" sound.
	SetInstrumentParam(id, "post_pitch", 12)
	edited := drainVoiceToBuffer(newRecipeAwareVoice(id, 44100), 44100)
	if len(base) == 0 || len(edited) == 0 {
		t.Fatalf("empty buffers: base=%d edited=%d", len(base), len(edited))
	}
	if l1(base, edited) == 0 {
		t.Fatal("sanity: post_pitch=+12 produced an identical buffer to baseline; knob has no effect")
	}

	saveCopyToLibraryForTest(t, string(id))
	afterSave := drainVoiceToBuffer(newRecipeAwareVoice(id, 44100), 44100)
	ResetInstrumentParams(id) // back to the (saved) library entry
	afterReset := drainVoiceToBuffer(newRecipeAwareVoice(id, 44100), 44100)

	if d := l1(afterSave, edited); d != 0 {
		t.Errorf("post-save render differs from the liked sound (L1=%v); Save changed the sound", d)
	}
	if d := l1(afterReset, edited); d != 0 {
		t.Errorf("render after Save + Reset differs from the liked sound (L1=%v); the saved entry was ignored", d)
	}
}

// TestSynthReset_RenderReturnsToOriginal: after a Save over a shipped
// instrument, Restore shipped (drop the user entry) + Reset brings the
// original sound back, byte for byte.
func TestSynthReset_RenderReturnsToOriginal(t *testing.T) {
	resetLibraryForTest(t)
	const id = roster.Kick

	base := drainVoiceToBuffer(newRecipeAwareVoice(id, 44100), 44100)
	SetInstrumentParam(id, "post_pitch", 12)
	saveCopyToLibraryForTest(t, string(id))
	ResetInstrumentParams(id)
	saved := drainVoiceToBuffer(newRecipeAwareVoice(id, 44100), 44100)
	if l1(saved, base) == 0 {
		t.Fatal("sanity: saved render should differ from baseline")
	}

	if err := DeleteUserLibraryDoc(id); err != nil {
		t.Fatal(err)
	}
	ResetInstrumentParams(id)
	reset := drainVoiceToBuffer(newRecipeAwareVoice(id, 44100), 44100)
	if d := l1(reset, base); d != 0 {
		t.Errorf("after Restore shipped + Reset, render differs from the original baseline (L1=%v)", d)
	}
}
