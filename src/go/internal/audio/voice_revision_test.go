package audio

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// VoiceRevision is part of every audio-lane note's identity: anything that
// changes the PCM an instrument's next note renders must move it, so the lane
// replaces that instrument's not-yet-started notes (real-time synth/sampler
// edits under a long horizon).
func TestVoiceRevisionMovesOnSampleEdit(t *testing.T) {
	const inst = roster.Kick
	t.Cleanup(func() { ClearSampleEdit(inst) })
	r0 := VoiceRevision(inst)
	ApplySampleEditLive(inst, SampleEdit{StartFrac: 0, EndFrac: 1, GainDB: -6})
	r1 := VoiceRevision(inst)
	if r1 == r0 {
		t.Fatal("a live sample edit must change the voice revision")
	}
	ClearSampleEdit(inst)
	if VoiceRevision(inst) == r1 {
		t.Fatal("clearing the sample edit must change the voice revision")
	}
}

func TestVoiceRevisionMovesOnSynthParam(t *testing.T) {
	const inst = roster.Kick
	r0 := VoiceRevision(inst)
	SetInstrumentParam(inst, "gain", 0.7)
	t.Cleanup(func() { ResetInstrumentParams(inst) })
	if VoiceRevision(inst) == r0 {
		t.Fatal("a synth param edit must change the voice revision")
	}
}
