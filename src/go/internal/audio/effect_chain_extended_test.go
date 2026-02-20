package audio

import (
	"sync"
	"testing"
)

func TestEffectChainConcurrentAddRemove(t *testing.T) {
	resetChains(t)
	id := "conc-add-rm"
	_ = InstrumentChannel(id)

	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				AddInsertEffect(id, EffectDistortion, nil)
				slots := GetInsertEffects(id)
				if len(slots) > 0 {
					RemoveInsertEffect(id, 0)
				}
			}
		}()
	}
	wg.Wait()
	// If we got here without panic/race, the test passes.
}

func TestEffectChainConcurrentReadWrite(t *testing.T) {
	resetChains(t)
	id := "conc-rw"
	_ = InstrumentChannel(id)
	AddInsertEffect(id, EffectDelay, nil)

	var wg sync.WaitGroup
	// 1 writer
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			SetInsertEffectParam(id, 0, "time", float64(100+i))
		}
	}()
	// 4 readers
	for r := 0; r < 4; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				_ = GetInsertEffects(id)
			}
		}()
	}
	wg.Wait()
}

func TestEffectChainDisabledBypassInProcessorChain(t *testing.T) {
	resetChains(t)
	id := "disabled-bypass"
	_ = InstrumentChannel(id)

	// Add distortion with high drive, then disable it.
	AddInsertEffect(id, EffectDistortion, map[string]float64{
		"drive": 20, "tone": 4000, "mix": 1,
	})
	ToggleInsertEffect(id, 0, false)

	// Process signal through channel: should pass unchanged (volume=1).
	ch := chanMgr.ensureChannel(id)
	ch.SetVolume(1.0)
	out := ch.ProcessSampleLocal(0.5)
	if out < 0.49 || out > 0.51 {
		t.Errorf("disabled effect: ProcessSampleLocal(0.5) = %f, want ≈ 0.5", out)
	}
}

func TestEffectChainImportExportRoundTrip(t *testing.T) {
	resetChains(t)
	id := "roundtrip"
	_ = InstrumentChannel(id)

	chain := []EffectSlot{
		{Type: EffectDistortion, Enabled: true, Params: map[string]float64{"drive": 5, "tone": 2000, "mix": 0.8}},
		{Type: EffectChorus, Enabled: false, Params: map[string]float64{"rate": 3, "depth": 10, "mix": 0.3}},
		{Type: EffectFilter, Enabled: true, Params: map[string]float64{"mode": 1, "cutoff": 500, "q": 2, "mix": 1}},
	}
	SetInsertEffects(id, chain)
	got := GetInsertEffects(id)

	if len(got) != len(chain) {
		t.Fatalf("round-trip: got %d slots, want %d", len(got), len(chain))
	}
	for i, s := range got {
		if s.Type != chain[i].Type {
			t.Errorf("slot[%d].Type = %s, want %s", i, s.Type, chain[i].Type)
		}
		if s.Enabled != chain[i].Enabled {
			t.Errorf("slot[%d].Enabled = %v, want %v", i, s.Enabled, chain[i].Enabled)
		}
		for k, v := range chain[i].Params {
			if got[i].Params[k] != v {
				t.Errorf("slot[%d].Params[%s] = %f, want %f", i, k, got[i].Params[k], v)
			}
		}
	}
}

func TestEffectChainLargeChain(t *testing.T) {
	resetChains(t)
	id := "large-chain"
	_ = InstrumentChannel(id)

	for i := 0; i < 20; i++ {
		AddInsertEffect(id, EffectDistortion, nil)
	}
	slots := GetInsertEffects(id)
	if len(slots) != 20 {
		t.Errorf("expected 20 effects, got %d", len(slots))
	}
}

func TestEffectChainRemoveOutOfBounds(t *testing.T) {
	resetChains(t)
	id := "rm-oob"
	_ = InstrumentChannel(id)
	AddInsertEffect(id, EffectDelay, nil)

	// These should be safe no-ops.
	RemoveInsertEffect(id, -1)
	RemoveInsertEffect(id, 100)

	slots := GetInsertEffects(id)
	if len(slots) != 1 {
		t.Errorf("expected 1 effect after out-of-bounds removes, got %d", len(slots))
	}
}

func TestEffectChainMoveOutOfBounds(t *testing.T) {
	resetChains(t)
	id := "mv-oob"
	_ = InstrumentChannel(id)
	AddInsertEffect(id, EffectDistortion, nil)
	AddInsertEffect(id, EffectDelay, nil)

	// Out-of-bounds moves should be safe no-ops.
	MoveInsertEffect(id, -1, 0)
	MoveInsertEffect(id, 0, 100)
	MoveInsertEffect(id, 100, 0)

	slots := GetInsertEffects(id)
	if len(slots) != 2 || slots[0].Type != EffectDistortion || slots[1].Type != EffectDelay {
		t.Errorf("chain should be unchanged after oob moves: %v", slots)
	}
}

func TestEffectChainSetParamOutOfBounds(t *testing.T) {
	resetChains(t)
	id := "sp-oob"
	_ = InstrumentChannel(id)
	AddInsertEffect(id, EffectDistortion, nil)

	// Should be safe no-ops.
	SetInsertEffectParam(id, -1, "drive", 5)
	SetInsertEffectParam(id, 100, "drive", 5)

	// Original param should be unchanged.
	slots := GetInsertEffects(id)
	if slots[0].Params["drive"] != 2 { // default drive
		t.Errorf("drive param changed by oob SetInsertEffectParam: %f", slots[0].Params["drive"])
	}
}

func TestEffectChainPlatformCallbackFired(t *testing.T) {
	resetChains(t)
	id := "callback-fire"
	_ = InstrumentChannel(id)

	called := false
	oldCb := platformInsertEffectsChanged
	platformInsertEffectsChanged = func(instID string, slots []EffectSlot) {
		if instID == id {
			called = true
		}
	}
	t.Cleanup(func() { platformInsertEffectsChanged = oldCb })

	AddInsertEffect(id, EffectReverb, nil)
	if !called {
		t.Error("platformInsertEffectsChanged callback not fired after AddInsertEffect")
	}
}

func TestEffectChainRebuildPreservesEQ(t *testing.T) {
	resetChains(t)
	id := "preserve-eq"
	_ = InstrumentChannel(id)

	// Set EQ on the channel.
	SetChannelEQ(id, 44100, EQBand{Kind: EQPeaking, Freq: 1000, Q: 1, GainDB: 3})

	// Add an insert effect — EQ should be preserved at the end.
	AddInsertEffect(id, EffectDistortion, nil)

	ch := chanMgr.ensureChannel(id)
	ch.mu.RLock()
	numProcs := len(ch.processors)
	ch.mu.RUnlock()

	// Should have at least 2: insert + EQ
	if numProcs < 2 {
		t.Errorf("expected ≥2 processors (insert + EQ), got %d", numProcs)
	}
}
