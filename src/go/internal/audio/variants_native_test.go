//go:build !test && !js

package audio

import (
	"math"
	"testing"
)

// CVariantInstrument is the live instrument table in engine_instruments.go
// (bass-guitar, snare-1, kick-1, ...) yet none of its methods were executed
// by native tests — NewVoice, NewVoiceWithParams, renderAndCache,
// renderParamAndCache and gateTail all sat at 0%. These tests drive the full
// render→post→normalize→cache→round-robin pipeline directly.

func variantSamples(v Voice, n int) []float32 {
	out := make([]float32, 0, n)
	for range n {
		s, done := v.Sample()
		if done {
			break
		}
		out = append(out, float32(s))
	}
	return out
}

func TestCVariantInstrumentNewVoice(t *testing.T) {
	defer globalVoiceCache.Clear()
	inst := CVariantInstrument{
		Name: "covtest-fm-bass-1",
		// Every legacy family migrated to the modular engine (FM in Phase-7, the
		// LAST). renderFMBassVoice is the migrated no-edit modular FM fast path; it
		// stands in as a generic Render to exercise the CVariantInstrument machinery.
		Render: renderFMBassVoice,
		Beats:  0.5,
		Post:   func(buf []float32, _ int) { gateTail(buf, 0.5) },
	}

	const bpm, sr = 120, 44100
	v := inst.NewVoice(bpm, sr)
	if v == nil {
		t.Fatal("NewVoice returned nil")
	}
	samples := variantSamples(v, sr)
	if len(samples) == 0 {
		t.Fatal("voice produced no samples")
	}
	var energy float64
	for i, s := range samples {
		if math.IsNaN(float64(s)) || math.IsInf(float64(s), 0) {
			t.Fatalf("sample[%d] not finite", i)
		}
		energy += float64(s) * float64(s)
	}
	if energy == 0 {
		t.Fatal("voice rendered silence")
	}

	// gateTail Post: the second half of the buffer must be hard-gated.
	half := len(samples) / 2
	for i := half + 1; i < len(samples); i++ {
		if samples[i] != 0 {
			t.Fatalf("sample[%d] = %v, want 0 after gateTail(0.5)", i, samples[i])
		}
	}

	// Second call hits the cache (and may kick a round-robin background
	// render); must still produce a voice.
	if v2 := inst.NewVoice(bpm, sr); v2 == nil {
		t.Fatal("cached NewVoice returned nil")
	}

	// LatestVoiceSample exposes the most recent cached render for the
	// Synth-tab preview.
	if got := LatestVoiceSample("covtest-fm-bass-1"); len(got) == 0 {
		t.Fatal("LatestVoiceSample returned nothing after a render")
	}
	if got := LatestVoiceSample("never-rendered-instrument"); got != nil {
		t.Fatalf("LatestVoiceSample(unknown) = %d samples, want nil", len(got))
	}
}

func TestCVariantInstrumentNewVoiceWithParams(t *testing.T) {
	defer globalVoiceCache.Clear()
	// Uses the migrated FM modular fast path as a generic Render/RenderParam pair
	// to exercise the CVariantInstrument NewVoiceWithParams machinery; every legacy
	// render_X / render_X_p was deleted by the modular migration (FM last, Phase-7).
	// renderFMBassVoice is the no-edit voice; the RenderParam wrapper ignores the
	// (unused) SynthParams surface and renders the same baked voice.
	// The migrated families no longer expose a SynthParams-driven _p renderer
	// (their knobs travel in the wide modular block). To exercise the
	// NewVoiceWithParams machinery (param cache-keying + RenderParam dispatch), use
	// the base modular voice as the unparameterized Render and a RenderParam that
	// maps the generic SynthParams onto modular fields (pitch + drive) so a
	// non-default param set produces audibly different output.
	inst := CVariantInstrument{
		Name:   "covtest-fm-bass-1",
		Render: renderModular,
		RenderParam: func(buf []float32, sr, n int, sp SynthParams) {
			mp := recipeParamsToModular(RecipeParams{})
			mp.Pitch = sp.Pitch
			mp.Drive = sp.Drive
			renderModularP(buf, sr, n, mp)
		},
		Beats: 0.5,
	}

	const bpm, sr = 120, 44100

	// Default params fall back to the unparameterized path.
	vDefault := inst.NewVoiceWithParams(bpm, sr, SynthParams{})
	defSamples := variantSamples(vDefault, sr)

	// Non-default params take the parameterized render path with its own
	// cache key.
	params := SynthParams{Pitch: 7, Decay: 0.5, Drive: 0.8}
	vParam := inst.NewVoiceWithParams(bpm, sr, params)
	paramSamples := variantSamples(vParam, sr)

	if len(defSamples) == 0 || len(paramSamples) == 0 {
		t.Fatal("voices produced no samples")
	}
	if buffersEqualF32(defSamples, paramSamples) {
		t.Fatal("param render identical to default — params not applied")
	}

	// Same params again: cache hit on the param key.
	if v := inst.NewVoiceWithParams(bpm, sr, params); v == nil {
		t.Fatal("cached param voice is nil")
	}

	// No RenderParam: params are ignored, default path is used.
	noParam := CVariantInstrument{Name: "covtest-tom-1", Render: renderTomVoice, Beats: 0.5}
	if v := noParam.NewVoiceWithParams(bpm, sr, params); v == nil {
		t.Fatal("fallback voice is nil")
	}
}

func TestGateTailBounds(t *testing.T) {
	buf := []float32{1, 1, 1, 1}
	gateTail(buf, 0)   // no-op: cutoff <= 0
	gateTail(buf, 1)   // no-op: cutoff >= 1
	gateTail(buf, 1.5) // no-op
	for i, v := range buf {
		if v != 1 {
			t.Fatalf("buf[%d] = %v after no-op gates, want 1", i, v)
		}
	}
	gateTail(buf, 0.5)
	if buf[0] != 1 || buf[1] != 1 || buf[2] != 0 || buf[3] != 0 {
		t.Fatalf("gateTail(0.5) = %v, want [1 1 0 0]", buf)
	}
}
