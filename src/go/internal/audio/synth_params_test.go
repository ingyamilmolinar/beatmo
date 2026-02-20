//go:build !test

package audio

import (
	"math"
	"testing"
)

func TestSynthParamsIsDefaultAllZero(t *testing.T) {
	p := SynthParams{}
	if !p.IsDefault() {
		t.Error("zero-value SynthParams should be default")
	}
}

func TestSynthParamsIsDefaultNonZero(t *testing.T) {
	fields := []SynthParams{
		{Pitch: 1},
		{Decay: 0.5},
		{Tone: -0.3},
		{Attack: 0.1},
		{Drive: 0.8},
		{Body: 0.2},
		{Color: -0.5},
		{Brightness: 0.7},
	}
	for i, p := range fields {
		if p.IsDefault() {
			t.Errorf("field[%d]: non-zero SynthParams reported as default: %+v", i, p)
		}
	}
}

func TestSynthParamsToCParamsNilForDefault(t *testing.T) {
	p := SynthParams{}
	cp := p.toCParams()
	if cp != nil {
		t.Error("toCParams should return nil for default params")
	}
}

func TestSynthParamsToCParamsNonNil(t *testing.T) {
	p := SynthParams{Pitch: 2.5, Drive: 0.8, Brightness: 0.3}
	cp := p.toCParams()
	if cp == nil {
		t.Fatal("toCParams should return non-nil for non-default params")
	}
	// Use tolerance for float32→float64 conversion precision loss.
	if math.Abs(float64(cp.pitch)-2.5) > 0.001 {
		t.Errorf("C pitch = %f, want 2.5", float64(cp.pitch))
	}
	if math.Abs(float64(cp.drive)-0.8) > 0.001 {
		t.Errorf("C drive = %f, want 0.8", float64(cp.drive))
	}
	if math.Abs(float64(cp.brightness)-0.3) > 0.001 {
		t.Errorf("C brightness = %f, want 0.3", float64(cp.brightness))
	}
}

func TestSynthParamsVoiceCacheKeyDifferentiation(t *testing.T) {
	vc := newTestCache()
	k1 := voiceCacheKey{instrumentID: "snare", sampleRate: 44100, synthParams: SynthParams{}}
	k2 := voiceCacheKey{instrumentID: "snare", sampleRate: 44100, synthParams: SynthParams{Pitch: 3}}

	vc.Put(k1, []float32{100})
	vc.Put(k2, []float32{200})

	g1, _ := vc.Get(k1)
	g2, _ := vc.Get(k2)
	if g1[0] == g2[0] {
		t.Error("keys with different SynthParams should produce separate entries")
	}
}

func TestParamRenderFuncsMapCompleteness(t *testing.T) {
	expected := []string{"snare", "kick", "hihat", "clap", "tom", "cowbell"}
	for _, name := range expected {
		if _, ok := ParamRenderFuncs[name]; !ok {
			t.Errorf("ParamRenderFuncs missing entry for %q", name)
		}
	}
}

func TestParamRenderFuncSafeForZeroSamples(t *testing.T) {
	buf := make([]float32, 1024)
	for name, fn := range ParamRenderFuncs {
		fn(buf, 44100, 0, SynthParams{})
		// Should return safely without panic.
		_ = name
	}
}

func TestParamRenderFuncSafeForOversizedSamples(t *testing.T) {
	buf := make([]float32, 64)
	for name, fn := range ParamRenderFuncs {
		fn(buf, 44100, 128, SynthParams{}) // samples > len(buf)
		// The guard `if samples > len(buf)` should prevent this from crashing.
		_ = name
	}
}
