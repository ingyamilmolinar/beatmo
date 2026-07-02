//go:build test

package ui

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// sampler_wav_instrument_test.go — a WAV instrument imported via the file
// picker is a raw-PCM Sample with NO recipe binding and NO entry in the
// user-sample store (registerInstrument → audio.RegisterWAV). It must behave in
// the Sampler tab exactly like a synth: edits apply in REAL TIME (per gesture,
// no Save) and are non-destructive.
//
// Mechanism: the Sampler seeds the WAV's pristine PCM into the user-sample
// store on load, then every edit gesture writes the sample-edit DESCRIPTOR
// (commitSamplerEdit). audio.SetSampleEdit re-derives the playable PCM from the
// pristine source through the descriptor and re-registers it
// (reapplyUserSampleEdit), so the chop is audible on the next trigger without
// clicking Save.

func TestSamplerWAVInstrumentClassifiedAsWAVSource(t *testing.T) {
	g := newSamplerTabGame(t)
	const id = "user.wav.rawsample"
	// A file-picker WAV: registered for playback but NOT recipe-bound and NOT in
	// the user-sample store (mirrors registerInstrument → audio.RegisterWAV).
	if err := audio.RegisterWAV(id, "dummy.wav"); err != nil {
		t.Fatalf("RegisterWAV: %v", err)
	}
	t.Cleanup(func() { audio.ClearSampleEdit(id) })
	if audio.RecipeForInstrument(id) != "" {
		t.Fatalf("precondition: %q must have NO recipe binding", id)
	}

	g.drum.ensureSamplerLoaded(id)
	s := &g.drum.sampler
	if !s.hasBuffer() {
		t.Fatal("no buffer after loading a WAV instrument")
	}
	if s.source != samplerSourceWAV {
		t.Fatalf("source=%v, want WAV — a non-recipe raw sample is a WAV source", s.source)
	}
}

// TestSamplerWAVEditAppliesLiveWithoutSave is the core regression for the
// reported bug: trim/reverse/etc on a WAV instrument must take effect in real
// time, NOT wait for the Save button.
func TestSamplerWAVEditAppliesLiveWithoutSave(t *testing.T) {
	g := newSamplerTabGame(t)
	const id = "user.wav.realtime"
	if err := audio.RegisterWAV(id, "dummy.wav"); err != nil {
		t.Fatalf("RegisterWAV: %v", err)
	}
	t.Cleanup(func() { audio.ClearSampleEdit(id) })

	g.drum.ensureSamplerLoaded(id)
	s := &g.drum.sampler
	if !s.hasBuffer() {
		t.Fatal("no buffer after loading a WAV instrument")
	}
	srcLen := len(s.raw)

	// A trim gesture, committed at release — but Save is NEVER called.
	s.startFrac, s.endFrac = 0.25, 0.75
	g.drum.commitSamplerEdit()

	if !audio.HasSampleEdit(id) {
		t.Fatal("WAV sampler edit must apply LIVE (descriptor set) WITHOUT clicking Save")
	}
	rec, ok := audio.LastRegisteredSamplePCMForTest(id)
	if !ok || len(rec.PCM) == 0 {
		t.Fatal("live WAV edit must re-register the baked PCM for immediate playback")
	}
	// Quarter-trimmed from both ends ⇒ ~half the source length.
	if got, want := len(rec.PCM), srcLen/2; got != want {
		t.Errorf("re-registered baked len=%d, want %d (0.25..0.75 of %d)", got, want, srcLen)
	}
}

// TestSamplerWAVReverseAppliesLiveWithoutSave guards the specific control the
// user called out (reverse) through the same live path.
func TestSamplerWAVReverseAppliesLiveWithoutSave(t *testing.T) {
	g := newSamplerTabGame(t)
	const id = "user.wav.reverse"
	if err := audio.RegisterWAV(id, "dummy.wav"); err != nil {
		t.Fatalf("RegisterWAV: %v", err)
	}
	t.Cleanup(func() { audio.ClearSampleEdit(id) })

	g.drum.ensureSamplerLoaded(id)
	s := &g.drum.sampler
	if !s.hasBuffer() {
		t.Fatal("no buffer after loading a WAV instrument")
	}
	first := s.raw[1] // a non-edge sample to compare after reversal

	s.reverseBuffer()
	g.drum.commitSamplerEdit()

	e, ok := audio.SampleEditFor(id)
	if !ok || !e.Reverse {
		t.Fatal("reverse on a WAV instrument must set the descriptor's Reverse flag live")
	}
	rec, ok := audio.LastRegisteredSamplePCMForTest(id)
	if !ok || len(rec.PCM) == 0 {
		t.Fatal("live reverse must re-register the baked PCM")
	}
	// The re-registered buffer is baked from the PRISTINE source + Reverse, so its
	// tail equals the source head — proving the reversal reached playback.
	if rec.PCM[len(rec.PCM)-2] != first {
		t.Errorf("re-registered buffer not reversed: tail=%v want source-head %v", rec.PCM[len(rec.PCM)-2], first)
	}
}

// TestSamplerWAVEditReloadKeepsPristineSource verifies the edit is
// non-destructive: re-loading the instrument shows the PRISTINE waveform with
// the saved edit overlaid on the knobs (not a re-baked, double-trimmed buffer).
func TestSamplerWAVEditReloadKeepsPristineSource(t *testing.T) {
	g := newSamplerTabGame(t)
	const id = "user.wav.reload"
	if err := audio.RegisterWAV(id, "dummy.wav"); err != nil {
		t.Fatalf("RegisterWAV: %v", err)
	}
	t.Cleanup(func() { audio.ClearSampleEdit(id) })

	g.drum.ensureSamplerLoaded(id)
	s := &g.drum.sampler
	pristineLen := len(s.raw)

	s.startFrac, s.endFrac = 0.25, 0.75
	g.drum.commitSamplerEdit()

	// Switch away and back: forces a fresh ensureSamplerLoaded.
	g.drum.ensureSamplerLoaded("kick")
	g.drum.ensureSamplerLoaded(id)

	if len(s.raw) != pristineLen {
		t.Errorf("reload corrupted the source buffer: raw len=%d want pristine %d", len(s.raw), pristineLen)
	}
	if s.startFrac != 0.25 || s.endFrac != 0.75 {
		t.Errorf("reload lost the saved edit knobs: start=%v end=%v want 0.25/0.75", s.startFrac, s.endFrac)
	}
}

// TestSamplerEditDescriptorFlipsTrimWhenReversed is the root-cause unit test for
// the "reverse + trim don't match the audio" bug. The trim handles are over the
// DISPLAYED (reversed) buffer, but the descriptor is applied to the FORWARD
// source and BakeSample trims BEFORE it reverses — so the source-frame trim must
// be the mirror of the display-frame trim: [s,e] over reverse(P) ==
// reverse(P[1-e : 1-s]).
func TestSamplerEditDescriptorFlipsTrimWhenReversed(t *testing.T) {
	s := &samplerState{}
	s.reset()
	s.raw = make([]float32, 800)
	s.rawSampleRate = 48000
	s.reverseBuffer() // s.reverse = true
	s.startFrac, s.endFrac = 0.0, 0.5

	e := s.editDescriptor()
	if !e.Reverse {
		t.Fatal("descriptor must carry Reverse")
	}
	if math.Abs(e.StartFrac-0.5) > 1e-9 || math.Abs(e.EndFrac-1.0) > 1e-9 {
		t.Errorf("reversed trim not flipped to source frame: got [%v,%v], want [0.5,1.0]", e.StartFrac, e.EndFrac)
	}
}

// TestSamplerWAVReverseTrimMatchesDisplay is the end-to-end audible check: with
// reverse on, the live (no-Save) playback buffer must equal the region the trim
// handles select over the DISPLAYED (reversed) waveform.
func TestSamplerWAVReverseTrimMatchesDisplay(t *testing.T) {
	g := newSamplerTabGame(t)
	const id = "user.wav.revtrim"
	const n = 800
	pcm := make([]float32, n)
	for i := range pcm {
		pcm[i] = float32(i) // ramp: each sample's value identifies its source position
	}
	audio.PutUserSample(id, pcm, 48000)
	t.Cleanup(func() { audio.ClearSampleEdit(id) })

	g.drum.ensureSamplerLoaded(id)
	s := &g.drum.sampler
	if !s.hasBuffer() {
		t.Fatal("no buffer after loading the WAV instrument")
	}

	// Displayed buffer after reverse = [799, 798, ..., 0]. Selecting the first
	// half ([0,0.5]) of THAT is [799 .. 400].
	s.reverseBuffer()
	s.startFrac, s.endFrac = 0.0, 0.5
	g.drum.commitSamplerEdit()

	reg, ok := audio.LastRegisteredSamplePCMForTest(id)
	if !ok {
		t.Fatal("no re-registration after edit")
	}
	if len(reg.PCM) != 400 {
		t.Fatalf("baked len=%d, want 400 (half of 800)", len(reg.PCM))
	}
	if reg.PCM[0] != 799 || reg.PCM[399] != 400 {
		t.Errorf("reversed+trim audio mismatches the displayed selection: got first=%v last=%v, want first=799 last=400",
			reg.PCM[0], reg.PCM[399])
	}
}

// TestSamplerReverseTrimDescriptorRoundTrips verifies the source-frame flip is
// inverted on load, so reopening a reversed+trimmed chop restores the same
// display-frame handles.
func TestSamplerReverseTrimDescriptorRoundTrips(t *testing.T) {
	s := &samplerState{}
	s.reset()
	s.raw = make([]float32, 800)
	s.rawSampleRate = 48000
	s.reverseBuffer()
	s.startFrac, s.endFrac = 0.1, 0.4
	e := s.editDescriptor()

	s2 := &samplerState{}
	s2.reset()
	s2.raw = make([]float32, 800)
	s2.loadEditDescriptor(e)

	if math.Abs(s2.startFrac-0.1) > 1e-9 || math.Abs(s2.endFrac-0.4) > 1e-9 {
		t.Errorf("round-trip lost display trim: got [%v,%v], want [0.1,0.4]", s2.startFrac, s2.endFrac)
	}
	if !s2.reverse {
		t.Error("round-trip lost reverse")
	}
}
