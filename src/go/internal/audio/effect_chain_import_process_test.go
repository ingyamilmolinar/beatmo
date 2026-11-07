package audio

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// SetInsertEffects bulk-replaces the chain on import; the imported chain must process audio and a disabled slot be
// bypassed, not merely restored as inert state.

func TestSetInsertEffectsChainProcessesAudio(t *testing.T) {
	resetChains(t)
	id := "import-process"
	ch := InstrumentChannel(channel.Of(instrument.ID(id)))

	const sr = 44100
	input := sineSamples(440, sr, sr/4)
	dry := processThrough(ch, input)

	SetInsertEffects(instrument.ID(id), []EffectSlot{
		{Type: EffectDistortion, Enabled: true, Params: map[string]float64{"drive": 18, "mix": 1}},
	})
	wet := processThrough(ch, input)

	var diff float64
	for i := range dry {
		diff += math.Abs(wet[i] - dry[i])
	}
	diff /= float64(len(dry))
	if diff < 1e-4 {
		t.Errorf("imported distortion chain did not process audio (mean |wet-dry| = %g)", diff)
	}
	for i, s := range wet {
		if math.IsNaN(s) || math.IsInf(s, 0) {
			t.Fatalf("wet[%d] non-finite: %v", i, s)
		}
	}
}

func TestSetInsertEffectsDisabledSlotIsBypassed(t *testing.T) {
	resetChains(t)
	id := "import-bypass"
	ch := InstrumentChannel(channel.Of(instrument.ID(id)))

	const sr = 44100
	input := sineSamples(440, sr, sr/4)
	dry := processThrough(ch, input)

	SetInsertEffects(instrument.ID(id), []EffectSlot{
		{Type: EffectDistortion, Enabled: false, Params: map[string]float64{"drive": 18, "mix": 1}},
	})
	bypassed := processThrough(ch, input)

	for i := range dry {
		if math.Abs(bypassed[i]-dry[i]) > 1e-9 {
			t.Fatalf("disabled imported slot processed audio at sample %d: dry=%v got=%v", i, dry[i], bypassed[i])
		}
	}
}
