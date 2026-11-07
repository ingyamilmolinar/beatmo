//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// The toggles drive both what is heard and what is drawn: the waveform reflects every toggle as the baked audio
// does, including Reverse after a forward live re-capture. Full range, no pitch, so the comparison is
// sample for sample.

func TestSamplerNormalizeRenderedSignalMatchesAudio(t *testing.T) {
	assertDefaultParityState(t)
	// Peak 0.3: Normalize brings the audio to 0 dBFS, and the drawing must rise too.
	s := &samplerState{raw: []float32{0.1, -0.2, 0.3, -0.05}, rawSampleRate: 48000}
	s.reset()
	s.normalize = true // the Normalize button latches this

	baked, _ := s.bake()    // what you hear
	wave := s.waveFloat64() // what you see
	if len(wave) != len(baked) {
		t.Fatalf("displayed len %d != baked len %d", len(wave), len(baked))
	}
	for i := range baked {
		if !fEq32(wave[i], float64(baked[i])) {
			t.Fatalf("Normalize ON: displayed signal does not match audio at %d: shown=%v heard=%v\n  shown=%v\n  heard=%v",
				i, wave[i], baked[i], wave, baked)
		}
	}
}

func TestSamplerFadeRenderedSignalMatchesAudio(t *testing.T) {
	assertDefaultParityState(t)
	// Fade ramps the edges to ~0, and the drawing must show it.
	s := &samplerState{raw: []float32{1, 1, 1, 1, 1, 1, 1, 1}, rawSampleRate: 48000}
	s.reset()
	s.fadeOn = true // the Fade button latches this

	baked, _ := s.bake()
	wave := s.waveFloat64()
	if len(wave) != len(baked) {
		t.Fatalf("displayed len %d != baked len %d", len(wave), len(baked))
	}
	for i := range baked {
		if !fEq32(wave[i], float64(baked[i])) {
			t.Fatalf("Fade ON: displayed signal does not match audio at %d: shown=%v heard=%v\n  shown=%v\n  heard=%v",
				i, wave[i], baked[i], wave, baked)
		}
	}
}

func TestSamplerReverseRenderedSignalMatchesAudioAfterRecapture(t *testing.T) {
	assertDefaultParityState(t)
	// The live re-render is forward while Reverse stays on, so the drawing must follow the button.
	forward := []float32{10, 20, 30, 40} // the fresh synth render (always forward)
	s := &samplerState{raw: []float32{1, 2, 3, 4}, rawSampleRate: 48000}
	s.reset()
	s.reverseBuffer()                                         // Reverse button ON
	s.recaptureRaw(append([]float32(nil), forward...), 48000) // live synth edit

	if !s.reverse {
		t.Fatal("precondition: Reverse button must still be latched ON after recapture")
	}
	// Synth path audio truth: editDescriptor() applied to the FORWARD recipe render.
	want := audio.BakeSample(append([]float32(nil), forward...), 48000, s.editDescriptor())
	wave := s.waveFloat64()
	if len(wave) != len(want) {
		t.Fatalf("displayed len %d != audio len %d", len(wave), len(want))
	}
	for i := range want {
		if !fEq32(wave[i], float64(want[i])) {
			t.Fatalf("Reverse ON after recapture: displayed signal does not match audio at %d: shown=%v heard=%v\n  shown=%v\n  heard=%v",
				i, wave[i], want[i], wave, want)
		}
	}
}
