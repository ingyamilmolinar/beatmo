//go:build !test && !js

package audio

import (
	"math"
	"testing"
)

// Joint insert-FX × synth-recipe coverage: synth_recipe_* tests prove the
// recipe render path and effect_chain_* tests prove the chain in isolation,
// but nothing verified the production composition — a recipe-rendered voice
// flowing through its instrument's insert chain at the channel layer.
func TestRecipeVoiceThroughInsertChain(t *testing.T) {
	const inst = "snare"
	const bpm, sr = 120, 44100

	ClearAllInsertEffects()
	InitInsertChains(sr)
	t.Cleanup(func() {
		ClearAllInsertEffects()
		ResetInstrumentParams(inst)
		globalVoiceCache.Clear()
	})

	// Engage the recipe path with a live user param.
	ResetInstrumentParams(inst)
	SetInstrumentParam(inst, "drive", 0.4)
	v := newRecipeAwareVoice(inst, bpm, sr)
	if v == nil {
		t.Fatal("recipe voice is nil")
	}
	voice := variantSamples(v, sr)
	if len(voice) == 0 {
		t.Fatal("recipe voice rendered no samples")
	}

	ch := InstrumentChannel(inst)
	process := func() []float64 {
		in := make([]float64, len(voice))
		for i, s := range voice {
			in[i] = float64(s)
		}
		out := make([]float64, len(voice))
		ch.ProcessBlockLocal(in, out)
		return out
	}

	dry := process()

	// Distortion + delay on the instrument's channel.
	AddInsertEffect(inst, EffectDistortion, map[string]float64{"drive": 0.8, "mix": 1})
	AddInsertEffect(inst, EffectDelay, map[string]float64{"time": 120, "feedback": 0.4, "mix": 0.5})
	wet := process()

	var dryE, wetE, diff float64
	for i := range wet {
		if math.IsNaN(wet[i]) || math.IsInf(wet[i], 0) {
			t.Fatalf("wet[%d] not finite", i)
		}
		dryE += dry[i] * dry[i]
		wetE += wet[i] * wet[i]
		d := wet[i] - dry[i]
		diff += d * d
	}
	if dryE == 0 || wetE == 0 {
		t.Fatalf("zero energy: dry=%v wet=%v", dryE, wetE)
	}
	if diff == 0 {
		t.Fatal("insert chain had no effect on the recipe voice")
	}

	// Disabling the slots restores (near-)dry behavior on a fresh process.
	ToggleInsertEffect(inst, 0, false)
	ToggleInsertEffect(inst, 1, false)
	bypassed := process()
	var bypassDiff float64
	for i := range bypassed {
		d := bypassed[i] - dry[i]
		bypassDiff += d * d
	}
	if bypassDiff != 0 {
		t.Fatalf("disabled chain still alters the voice (diff energy %v)", bypassDiff)
	}
}
