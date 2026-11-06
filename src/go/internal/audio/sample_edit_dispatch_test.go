//go:build !test && !js

package audio

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Native-dispatch coverage for the non-destructive sample-edit descriptor:
// a synth instrument with a saved Sampler edit KEEPS its recipe binding; the
// edit is applied to the freshly-rendered recipe buffer at trigger time, so
// synth changes always take effect (next trigger) and the instrument never
// silently becomes a dead PCM blob.

// renderDispatchBuffer triggers id once through the dispatcher and drains the
// voice exactly (cVoice signals done only AFTER the last real sample, so the
// done-sample is not part of the buffer; 4s cap mirrors RenderInstrumentOneShot).
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
	// No overlay params, defaults as shipped — but a descriptor exists, so the
	// recipe path must be taken (the gate change). A half-trim makes the
	// rendered buffer measurably shorter than the unedited render.
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
	// Parity-golden safety: set then clear a descriptor; the render must come
	// back byte-identical to the never-edited output (no residue in the cache
	// or the key).
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

// A sample-edit must ROTATE the binding, not blanket-drop the instrument's other
// cached voices. hashSampleEdit is already XORed into the voice-cache key
// (synth_recipe_dispatch.go:138/:192/:264), so a new edit maps to a DIFFERENT key:
// dispatch renders the new sound there and the stale entry simply goes unread.
//
// This test used to assert the OPPOSITE (that SetSampleEdit invalidates). That
// blanket clear dropped every pitch's entry, so each following note cold-rendered
// ~24ms+ ON the audioLoop dispatch goroutine — measured at 30-44 dispatch renders
// per sampler edit, ~8x a synth-control drag. It is the same storm SetInstrumentParam
// was fixed to avoid; see the NOTE at synth_recipe.go:256-267 and its retargeted
// test TestSetInstrumentParam_DoesNotBlanketInvalidate.
//
// The BEHAVIOUR this file used to conflate with that mechanism — "an edit actually
// takes effect on the next render" — is covered by
// TestSampleEditDispatch_DescriptorChangeMissesCache above. Keep both.
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
	// The raw capture feeds the Sampler editor, which overlays the saved trim
	// itself — it must see the UN-edited source, i.e. strictly more audio than
	// the half-trimmed dispatch render.
	if len(raw) <= len(edited) {
		t.Fatalf("raw len=%d not longer than edited len=%d — descriptor leaked into the raw capture", len(raw), len(edited))
	}
}
