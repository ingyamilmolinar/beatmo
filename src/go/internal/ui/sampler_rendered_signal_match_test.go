//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// sampler_rendered_signal_match_test.go — the Sampler toggle BUTTONS (Reverse,
// Normalize, Fade) are the single source of truth for BOTH the audio the user
// hears AND the waveform the user sees. The displayed/rendered signal
// (s.waveFloat64(), drawn by drawSamplerWaveform) must therefore reflect every
// toggle exactly as the baked audio does — WYSIWYG.
//
// BUG (reported): toggling Normalize or Fade changes the audio (they are applied
// in edit() → bake()) but NOT the rendered waveform (drawn straight from the raw
// working buffer), so the picture lies about what you hear. Reverse is even
// sneakier: it is baked into the buffer in place, so it usually shows — but a
// live synth re-capture (recaptureRaw) re-renders the source FORWARD while the
// Reverse button stays latched ON, leaving the rendered signal forward while the
// audio (editDescriptor().Reverse) plays reversed.
//
// These tests assert the invariant for the full-range / no-pitch case, where the
// baked buffer is the same length as the displayed one, so the comparison is
// sample-for-sample (mirrors TestSamplerBakeAfterReverseMatchesDisplayedBuffer).

func TestSamplerNormalizeRenderedSignalMatchesAudio(t *testing.T) {
	assertDefaultParityState(t)
	// A quiet buffer (peak 0.3). With Normalize ON the audio is brought to 0 dBFS,
	// so the rendered waveform must rise to match — not stay quiet.
	s := &samplerState{raw: []float32{0.1, -0.2, 0.3, -0.05}, rawSampleRate: 48000}
	s.reset()
	s.normalize = true // the Normalize button latches this

	baked, _ := s.bake()  // what you hear
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
	// A flat buffer. With Fade ON the audio ramps in/out (edges go to ~0), so the
	// rendered waveform must show the ramp — not stay flat at full amplitude.
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
	// Synth source: reverse the buffer (button ON), then a live synth edit arrives
	// and re-renders the source FORWARD via recaptureRaw. The Reverse button stays
	// ON, so the audio (editDescriptor().Reverse) plays the fresh render reversed —
	// the displayed waveform must follow the button and show it reversed too.
	forward := []float32{10, 20, 30, 40} // the fresh synth render (always forward)
	s := &samplerState{raw: []float32{1, 2, 3, 4}, rawSampleRate: 48000}
	s.reset()
	s.reverseBuffer()                                       // Reverse button ON
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
