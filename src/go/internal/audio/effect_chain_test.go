package audio

import (
	"testing"
)

func resetChains(t *testing.T) {
	t.Helper()
	ClearAllInsertEffects()
	InitInsertChains(44100)
	withDefaultAudio(t)
}

func TestEffectChainAddRemove(t *testing.T) {
	resetChains(t)
	id := "test-inst"
	_ = InstrumentChannel(id)

	// Initially empty.
	slots := GetInsertEffects(id)
	if len(slots) != 0 {
		t.Fatalf("expected 0 effects, got %d", len(slots))
	}

	// Add distortion.
	idx := AddInsertEffect(id, EffectDistortion, nil)
	if idx != 0 {
		t.Errorf("expected slot 0, got %d", idx)
	}
	slots = GetInsertEffects(id)
	if len(slots) != 1 || slots[0].Type != EffectDistortion {
		t.Fatalf("expected 1 distortion, got %v", slots)
	}

	// Add delay.
	idx = AddInsertEffect(id, EffectDelay, nil)
	if idx != 1 {
		t.Errorf("expected slot 1, got %d", idx)
	}
	slots = GetInsertEffects(id)
	if len(slots) != 2 {
		t.Fatalf("expected 2 effects, got %d", len(slots))
	}

	// Remove first (distortion).
	RemoveInsertEffect(id, 0)
	slots = GetInsertEffects(id)
	if len(slots) != 1 || slots[0].Type != EffectDelay {
		t.Errorf("after remove: expected [delay], got %v", slots)
	}
}

func TestEffectChainReorder(t *testing.T) {
	resetChains(t)
	id := "test-inst"
	_ = InstrumentChannel(id)

	AddInsertEffect(id, EffectDistortion, nil)
	AddInsertEffect(id, EffectDelay, nil)
	AddInsertEffect(id, EffectReverb, nil)

	// Move reverb from slot 2 to slot 0.
	MoveInsertEffect(id, 2, 0)
	slots := GetInsertEffects(id)
	if len(slots) != 3 {
		t.Fatalf("expected 3 effects, got %d", len(slots))
	}
	if slots[0].Type != EffectReverb || slots[1].Type != EffectDistortion || slots[2].Type != EffectDelay {
		t.Errorf("wrong order: %v %v %v", slots[0].Type, slots[1].Type, slots[2].Type)
	}
}

func TestEffectChainToggle(t *testing.T) {
	resetChains(t)
	id := "test-inst"
	_ = InstrumentChannel(id)

	AddInsertEffect(id, EffectDistortion, nil)
	slots := GetInsertEffects(id)
	if !slots[0].Enabled {
		t.Fatal("new effect should be enabled")
	}

	// Disable.
	ToggleInsertEffect(id, 0, false)
	slots = GetInsertEffects(id)
	if slots[0].Enabled {
		t.Error("effect should be disabled")
	}

	// Re-enable.
	ToggleInsertEffect(id, 0, true)
	slots = GetInsertEffects(id)
	if !slots[0].Enabled {
		t.Error("effect should be re-enabled")
	}
}

func TestEffectChainSetParam(t *testing.T) {
	resetChains(t)
	id := "test-inst"
	_ = InstrumentChannel(id)

	AddInsertEffect(id, EffectDistortion, nil)
	SetInsertEffectParam(id, 0, "drive", 15)
	slots := GetInsertEffects(id)
	if slots[0].Params["drive"] != 15 {
		t.Errorf("expected drive=15, got %.1f", slots[0].Params["drive"])
	}
}

func TestEffectChainSetInsertEffects(t *testing.T) {
	resetChains(t)
	id := "test-inst"
	_ = InstrumentChannel(id)

	// Set a full chain at once (simulates import).
	chain := []EffectSlot{
		{Type: EffectChorus, Enabled: true, Params: DefaultParams(EffectChorus)},
		{Type: EffectFilter, Enabled: false, Params: DefaultParams(EffectFilter)},
	}
	SetInsertEffects(id, chain)
	slots := GetInsertEffects(id)
	if len(slots) != 2 {
		t.Fatalf("expected 2, got %d", len(slots))
	}
	if slots[0].Type != EffectChorus || !slots[0].Enabled {
		t.Errorf("slot 0: %v", slots[0])
	}
	if slots[1].Type != EffectFilter || slots[1].Enabled {
		t.Errorf("slot 1: %v", slots[1])
	}
}

func TestEffectChainClearAll(t *testing.T) {
	resetChains(t)
	_ = InstrumentChannel("a")
	_ = InstrumentChannel("b")
	AddInsertEffect("a", EffectDistortion, nil)
	AddInsertEffect("b", EffectDelay, nil)

	ClearAllInsertEffects()
	if len(GetInsertEffects("a")) != 0 {
		t.Error("expected empty after clear")
	}
	if len(GetInsertEffects("b")) != 0 {
		t.Error("expected empty after clear")
	}
}

func TestEffectChainResetOnStop(t *testing.T) {
	resetChains(t)
	id := "test-inst"
	_ = InstrumentChannel(id)

	AddInsertEffect(id, EffectDelay, map[string]float64{
		"time": 100, "feedback": 0.9, "mix": 1,
	})

	// Feed signal through the processor directly.
	m := insertChainMgr
	m.mu.RLock()
	e := m.chains[id]
	if e == nil || len(e.processors) == 0 {
		m.mu.RUnlock()
		t.Fatal("no processors")
	}
	proc := e.processors[0]
	m.mu.RUnlock()

	for i := 0; i < 5000; i++ {
		proc.ProcessSample(0.8)
	}

	// Reset should clear delay buffer.
	ResetAllInsertEffects()
	out := proc.ProcessSample(0)
	if out > 0.01 {
		t.Errorf("after ResetAll: expected silence, got %.4f", out)
	}
}
