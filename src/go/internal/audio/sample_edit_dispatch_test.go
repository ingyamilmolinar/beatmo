//go:build !test && !js

package audio

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// A synth with a saved edit KEEPS its binding, so synth changes still take effect.

// The done-sample is not part of the buffer; the 4 s cap matches RenderInstrumentOneShot.
func renderDispatchBuffer(t *testing.T, id instrument.ID) []float32 {
	t.Helper()
	v := newRecipeAwareVoice(id, 44100)
	if v == nil {
		t.Fatalf("dispatcher returned nil for %q", id)
	}
	out := make([]float32, 0, 44100)
	for len(out) < 44100*4 {
		s, done := v.Sample()
		if done {
			break
		}
		out = append(out, float32(s))
	}
	return out
}

func TestSampleEditDispatch_DescriptorAloneTakesRecipePath(t *testing.T) {
	resetSampleEditsForTest(t)
	t.Cleanup(func() {
		ClearSampleEdit(roster.Snare)
		ResetInstrumentParams(roster.Snare)
	})
	ResetInstrumentParams(roster.Snare)
	// Shipped values, but a descriptor exists, so the recipe path must be taken.
	full := renderDispatchBuffer(t, roster.Snare)
	SetSampleEdit(roster.Snare, SampleEdit{StartFrac: 0, EndFrac: 0.5})
	if _, ok := tryRecipeVoice(roster.Snare, 44100); !ok {
		t.Fatal("descriptor with empty overlay must take the recipe path")
	}
	half := renderDispatchBuffer(t, roster.Snare)
	if len(half) >= len(full) {
		t.Fatalf("trimmed render len=%d not shorter than full len=%d", len(half), len(full))
	}
}

func TestSampleEditDispatch_TransformMatchesBakeSample(t *testing.T) {
	resetSampleEditsForTest(t)
	t.Cleanup(func() {
		ClearSampleEdit(roster.Snare)
		ResetInstrumentParams(roster.Snare)
	})
	ResetInstrumentParams(roster.Snare)

	plain := renderDispatchBuffer(t, roster.Snare)
	edit := SampleEdit{StartFrac: 0.25, EndFrac: 0.75, GainDB: -6, Reverse: true}
	SetSampleEdit(roster.Snare, edit)
	edited := renderDispatchBuffer(t, roster.Snare)

	want := BakeSample(append([]float32(nil), plain...), 44100, edit)
	if len(edited) != len(want) {
		t.Fatalf("edited render len=%d, want %d (BakeSample of the plain render)", len(edited), len(want))
	}
	for i := range want {
		if edited[i] != want[i] {
			t.Fatalf("sample %d: edited=%v want=%v — dispatcher must apply the SAME transform as BakeSample", i, edited[i], want[i])
		}
	}
}

func TestSampleEditDispatch_NoDescriptorByteIdentical(t *testing.T) {
	resetSampleEditsForTest(t)
	t.Cleanup(func() {
		ClearSampleEdit(roster.Snare)
		ResetInstrumentParams(roster.Snare)
	})
	ResetInstrumentParams(roster.Snare)
	// No residue in the cache or the key.
	before := renderDispatchBuffer(t, roster.Snare)
	SetSampleEdit(roster.Snare, SampleEdit{StartFrac: 0, EndFrac: 0.5})
	renderDispatchBuffer(t, roster.Snare)
	ClearSampleEdit(roster.Snare)
	after := renderDispatchBuffer(t, roster.Snare)
	if len(before) != len(after) {
		t.Fatalf("len changed after set+clear: %d vs %d", len(before), len(after))
	}
	for i := range before {
		if before[i] != after[i] {
			t.Fatalf("sample %d changed after descriptor set+clear: %v vs %v", i, before[i], after[i])
		}
	}
}

func TestSampleEditDispatch_DescriptorChangeMissesCache(t *testing.T) {
	resetSampleEditsForTest(t)
	t.Cleanup(func() {
		ClearSampleEdit(roster.Snare)
		ResetInstrumentParams(roster.Snare)
	})
	ResetInstrumentParams(roster.Snare)
	SetSampleEdit(roster.Snare, SampleEdit{StartFrac: 0, EndFrac: 0.5})
	a := renderDispatchBuffer(t, roster.Snare)
	SetSampleEdit(roster.Snare, SampleEdit{StartFrac: 0, EndFrac: 0.25})
	b := renderDispatchBuffer(t, roster.Snare)
	if len(b) >= len(a) {
		t.Fatalf("descriptor change did not re-render: len(a)=%d len(b)=%d", len(a), len(b))
	}
}

// The edit's hash is in the key, so the stale entry simply goes unread; dropping every pitch's entry would make each
// following note cold-render on the dispatch goroutine.
func TestSampleEditDispatch_SetDoesNotBlanketInvalidate(t *testing.T) {
	resetSampleEditsForTest(t)
	t.Cleanup(func() {
		ClearSampleEdit(roster.Snare)
		ResetInstrumentParams(roster.Snare)
	})
	ResetInstrumentParams(roster.Snare)
	SetInstrumentParam(roster.Snare, "amp_decay", 0.7) // force the recipe path + cache fill
	renderDispatchBuffer(t, roster.Snare)
	before, _ := VoiceCacheStatsForTest()
	if before == 0 {
		t.Fatal("expected at least one cached entry after a recipe render — " +
			"without this the assertion below is vacuous")
	}
	SetSampleEdit(roster.Snare, SampleEdit{StartFrac: 0, EndFrac: 0.5})
	after, _ := VoiceCacheStatsForTest()
	if after < before {
		t.Fatalf("SetSampleEdit blanket-dropped cached voices (entries %d → %d). "+
			"The edit hash already rotates the key; dropping entries makes every "+
			"following note cold-render on the audio dispatch goroutine.", before, after)
	}
}

func TestRenderInstrumentOneShotRawIgnoresDescriptor(t *testing.T) {
	resetSampleEditsForTest(t)
	t.Cleanup(func() {
		ClearSampleEdit(roster.Snare)
		ResetInstrumentParams(roster.Snare)
	})
	ResetInstrumentParams(roster.Snare)
	SetSampleEdit(roster.Snare, SampleEdit{StartFrac: 0, EndFrac: 0.5})

	edited, _ := RenderInstrumentOneShot(roster.Snare)
	raw, _ := RenderInstrumentOneShotRaw(roster.Snare)
	if len(raw) == 0 {
		t.Fatal("raw capture returned no audio")
	}
	// The editor overlays the saved trim itself, so it must see the UN-edited source.
	if len(raw) <= len(edited) {
		t.Fatalf("raw len=%d not longer than edited len=%d — descriptor leaked into the raw capture", len(raw), len(edited))
	}
}
