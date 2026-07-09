//go:build !test && !js

package audio

import "testing"

// TestNewVoiceCacheHitDoesNotRender pins the removal of the synchronous
// round-robin variant top-up (2026-07-13): NewVoice on a cache HIT ran
// renderAndCache inline until the entry held RoundRobinVariants buffers, so
// triggers #2 and #3 of every legacy-path instrument paid a ~13-20ms C render
// on the audioLoop dispatch goroutine — the stall class the timing-parity
// gate panics on — for buffers that are byte-identical anyway (ma_noise is
// re-seeded with 0 per call; see legacy_render_determinism_test.go). A hit
// must not render; only the first (miss) trigger renders.
func TestNewVoiceCacheHitDoesNotRender(t *testing.T) {
	renders := 0
	inst := CVariantInstrument{
		Name:  "test-no-topup-voice",
		Beats: 0.25,
		Render: func(buf []float32, sampleRate, samples int) {
			renders++
		},
	}
	for i := range 5 {
		if v := inst.NewVoice(120, 44100); v == nil {
			t.Fatalf("NewVoice returned nil on call %d", i)
		}
	}
	if renders != 1 {
		t.Fatalf("want exactly 1 render (first miss), got %d — a cache hit re-rendered on the dispatch path", renders)
	}
}

// TestNewVoiceWithParamsCacheHitDoesNotRender is the same guard for the
// parameterized path.
func TestNewVoiceWithParamsCacheHitDoesNotRender(t *testing.T) {
	renders := 0
	inst := CVariantInstrument{
		Name:  "test-no-topup-voice-params",
		Beats: 0.25,
		Render: func(buf []float32, sampleRate, samples int) {
			renders++
		},
		RenderParam: func(buf []float32, sampleRate, samples int, params SynthParams) {
			renders++
		},
	}
	params := SynthParams{Decay: 0.7}
	if params.IsDefault() {
		t.Fatal("test params unexpectedly default; pick a non-default field")
	}
	for i := range 5 {
		if v := inst.NewVoiceWithParams(120, 44100, params); v == nil {
			t.Fatalf("NewVoiceWithParams returned nil on call %d", i)
		}
	}
	if renders != 1 {
		t.Fatalf("want exactly 1 render (first miss), got %d — a cache hit re-rendered on the dispatch path", renders)
	}
}
