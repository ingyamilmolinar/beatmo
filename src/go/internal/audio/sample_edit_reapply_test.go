//go:build test

package audio

import "testing"

// sample_edit_reapply_test.go — SetSampleEdit / ClearSampleEdit re-derive a user
// sample's playable PCM from its pristine source so a Sampler edit on a WAV /
// user-sample instrument is audible in real time (no Save), and clearing it
// reverts. Synth instruments (no stored pristine) are untouched — they re-render
// through the recipe path instead.

func TestSetSampleEditReregistersUserSampleBaked(t *testing.T) {
	const id = "user.sample.reapply"
	pcm := make([]float32, 800)
	for i := range pcm {
		pcm[i] = 0.5
	}
	PutUserSample(id, pcm, 48000)
	t.Cleanup(func() { ClearSampleEdit(id); forgetUserSample(id) })

	SetSampleEdit(id, SampleEdit{StartFrac: 0.25, EndFrac: 0.75})
	reg, ok := LastRegisteredSamplePCMForTest(id)
	if !ok {
		t.Fatal("SetSampleEdit must re-register the baked PCM for a user sample")
	}
	if len(reg.PCM) != 400 {
		t.Errorf("baked len=%d, want 400 (0.25..0.75 of 800)", len(reg.PCM))
	}
}

func TestClearSampleEditRestoresUserSamplePristine(t *testing.T) {
	const id = "user.sample.reapply.clear"
	pcm := make([]float32, 600)
	for i := range pcm {
		pcm[i] = 0.3
	}
	PutUserSample(id, pcm, 48000)
	t.Cleanup(func() { ClearSampleEdit(id); forgetUserSample(id) })

	SetSampleEdit(id, SampleEdit{StartFrac: 0.5, EndFrac: 1})
	ClearSampleEdit(id)

	reg, ok := LastRegisteredSamplePCMForTest(id)
	if !ok {
		t.Fatal("ClearSampleEdit must re-register the pristine PCM for a user sample")
	}
	if len(reg.PCM) != 600 {
		t.Errorf("restored len=%d, want 600 (full pristine length)", len(reg.PCM))
	}
}

func TestSetSampleEditDoesNotReregisterSynth(t *testing.T) {
	const id = "synth.reapply.noop"
	// No PutUserSample: a synth has no stored pristine PCM.
	BindInstrumentToRecipe(id, "drum-kick")
	t.Cleanup(func() { ClearSampleEdit(id); BindInstrumentToRecipe(id, "") })

	SetSampleEdit(id, SampleEdit{StartFrac: 0.25, EndFrac: 0.75})
	if _, ok := LastRegisteredSamplePCMForTest(id); ok {
		t.Error("SetSampleEdit must NOT re-register PCM for a synth (no stored pristine) — it re-renders via the recipe path")
	}
}
