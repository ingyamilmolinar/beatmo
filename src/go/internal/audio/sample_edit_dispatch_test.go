//go:build !test && !js

package audio

import "testing"

// Native-dispatch coverage for the non-destructive sample-edit descriptor:
// a synth instrument with a saved Sampler edit KEEPS its recipe binding; the
// edit is applied to the freshly-rendered recipe buffer at trigger time, so
// synth changes always take effect (next trigger) and the instrument never
// silently becomes a dead PCM blob.

// renderDispatchBuffer triggers id once through the dispatcher and drains the
// voice exactly (cVoice signals done only AFTER the last real sample, so the
// done-sample is not part of the buffer; 4s cap mirrors RenderInstrumentOneShot).
func renderDispatchBuffer(t *testing.T, id string) []float32 {
	t.Helper()
	v := newRecipeAwareVoice(id, 120, 44100)
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
		ClearSampleEdit("snare")
		ResetInstrumentParams("snare")
	})
	ResetInstrumentParams("snare")
	// No overlay params, defaults as shipped — but a descriptor exists, so the
	// recipe path must be taken (the gate change). A half-trim makes the
	// rendered buffer measurably shorter than the unedited render.
	full := renderDispatchBuffer(t, "snare")
	SetSampleEdit("snare", SampleEdit{StartFrac: 0, EndFrac: 0.5})
	if _, ok := tryRecipeVoice("snare", 120, 44100); !ok {
		t.Fatal("descriptor with empty overlay must take the recipe path")
	}
	half := renderDispatchBuffer(t, "snare")
	if len(half) >= len(full) {
		t.Fatalf("trimmed render len=%d not shorter than full len=%d", len(half), len(full))
	}
}

func TestSampleEditDispatch_TransformMatchesBakeSample(t *testing.T) {
	resetSampleEditsForTest(t)
	t.Cleanup(func() {
		ClearSampleEdit("snare")
		ResetInstrumentParams("snare")
	})
	ResetInstrumentParams("snare")

	plain := renderDispatchBuffer(t, "snare")
	edit := SampleEdit{StartFrac: 0.25, EndFrac: 0.75, GainDB: -6, Reverse: true}
	SetSampleEdit("snare", edit)
	edited := renderDispatchBuffer(t, "snare")

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
		ClearSampleEdit("snare")
		ResetInstrumentParams("snare")
	})
	ResetInstrumentParams("snare")
	// Parity-golden safety: set then clear a descriptor; the render must come
	// back byte-identical to the never-edited output (no residue in the cache
	// or the key).
	before := renderDispatchBuffer(t, "snare")
	SetSampleEdit("snare", SampleEdit{StartFrac: 0, EndFrac: 0.5})
	renderDispatchBuffer(t, "snare")
	ClearSampleEdit("snare")
	after := renderDispatchBuffer(t, "snare")
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
		ClearSampleEdit("snare")
		ResetInstrumentParams("snare")
	})
	ResetInstrumentParams("snare")
	SetSampleEdit("snare", SampleEdit{StartFrac: 0, EndFrac: 0.5})
	a := renderDispatchBuffer(t, "snare")
	SetSampleEdit("snare", SampleEdit{StartFrac: 0, EndFrac: 0.25})
	b := renderDispatchBuffer(t, "snare")
	if len(b) >= len(a) {
		t.Fatalf("descriptor change did not re-render: len(a)=%d len(b)=%d", len(a), len(b))
	}
}

func TestSampleEditDispatch_SetInvalidatesCachedVoices(t *testing.T) {
	resetSampleEditsForTest(t)
	t.Cleanup(func() {
		ClearSampleEdit("snare")
		ResetInstrumentParams("snare")
	})
	ResetInstrumentParams("snare")
	SetInstrumentParam("snare", "decay", 0.7) // force the recipe path + cache fill
	renderDispatchBuffer(t, "snare")
	before, _ := VoiceCacheStatsForTest()
	if before == 0 {
		t.Fatal("expected at least one cached entry after a recipe render")
	}
	SetSampleEdit("snare", SampleEdit{StartFrac: 0, EndFrac: 0.5})
	after, _ := VoiceCacheStatsForTest()
	if after >= before {
		t.Fatalf("SetSampleEdit must invalidate the instrument's cached voices: entries %d → %d", before, after)
	}
}

func TestRenderInstrumentOneShotRawIgnoresDescriptor(t *testing.T) {
	resetSampleEditsForTest(t)
	t.Cleanup(func() {
		ClearSampleEdit("snare")
		ResetInstrumentParams("snare")
	})
	ResetInstrumentParams("snare")
	SetSampleEdit("snare", SampleEdit{StartFrac: 0, EndFrac: 0.5})

	edited, _ := RenderInstrumentOneShot("snare")
	raw, _ := RenderInstrumentOneShotRaw("snare")
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
