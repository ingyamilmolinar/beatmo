package audio

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// TestInsertEffectParamEditNeverRebuildsTheChain pins the contract a live FX
// slider relies on: a param edit updates ONE live effect in place. On WASM a
// whole-chain push (platformInsertEffectsChanged) builds fresh WebAudio nodes
// for every effect on the channel — a new AudioWorkletNode with an empty
// delay line for a pitch shifter — and rewires the channel. Pushed once per
// drag frame, that cut the kick on every frame and created ~1,000
// AudioWorkletNodes per second of dragging (2026-10-25, startup demo + a
// second Pitch Shifter on the kick, real Chrome).
func TestInsertEffectParamEditNeverRebuildsTheChain(t *testing.T) {
	resetChains(t)
	inst := instrument.ID("param-push")
	ch := channel.Of(inst)
	_ = InstrumentChannel(ch)
	AddInsertEffect(inst, EffectTape, nil)
	AddInsertEffect(inst, EffectPitchShift, nil)

	rebuilt := 0
	var chains [][]InsertEffectInstance
	oldRebuild := platformInsertEffectsChanged
	platformInsertEffectsChanged = func(id channel.ID, chain []InsertEffectInstance) {
		if id == ch {
			rebuilt++
			chains = append(chains, chain)
		}
	}
	t.Cleanup(func() { platformInsertEffectsChanged = oldRebuild })

	var got []paramPush
	oldParam := platformInsertEffectParamChanged
	platformInsertEffectParamChanged = func(id channel.ID, uid uint64, param string, value float64) {
		if id == ch {
			got = append(got, paramPush{uid, param, value})
		}
	}
	t.Cleanup(func() { platformInsertEffectParamChanged = oldParam })
	tape, shifter := InsertChainForPlatform(inst)[0].UID, InsertChainForPlatform(inst)[1].UID

	s0, p0 := InsertFXPushCounts()
	SetInsertEffectParam(inst, 1, "pitch", -7)
	SetInsertEffectParam(inst, 1, "pitch", 5)
	s1, p1 := InsertFXPushCounts()

	if rebuilt != 0 || s1 != s0 {
		t.Errorf("2 param edits rebuilt the insert chain (platform pushes %d, counted %d); a param edit must update the live effect in place", rebuilt, s1-s0)
	}
	if p1-p0 != 2 {
		t.Errorf("param pushes = %d, want 2 (one per edit)", p1-p0)
	}
	want := []paramPush{{shifter, "pitch", -7}, {shifter, "pitch", 5}}
	if len(got) != len(want) {
		t.Fatalf("in-place pushes = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("push %d = %+v, want %+v (the live instance, the param, its new value)", i, got[i], want[i])
		}
	}

	// Structural edits still rebuild — exactly once each.
	s2, _ := InsertFXPushCounts()
	ToggleInsertEffect(inst, 0, false)
	MoveInsertEffect(inst, 0, 1)
	RemoveInsertEffect(inst, 0)
	if s3, _ := InsertFXPushCounts(); s3-s2 != 3 || rebuilt != 3 {
		t.Errorf("toggle+move+remove: structural pushes %d (platform %d), want 3", s3-s2, rebuilt)
	}
	// The tape (toggled off, moved behind the shifter) survived the removal
	// of the shifter as the SAME instance: the browser keeps its C state by
	// this uid, as desktop keeps the Go processor.
	if last := chains[len(chains)-1]; len(last) != 1 || last[0].UID != tape || last[0].Type != EffectTape || last[0].Enabled {
		t.Errorf("after toggle+move+remove the chain is %+v, want the same disabled tape instance (uid %d)", last, tape)
	}
	// Every pushed slot carries its complete parameter set.
	if n := len(chains[0][0].Params); n != len(DefaultParams(EffectTape)) {
		t.Errorf("tape pushed %d params, the effect has %d: the browser must never fill a default itself", n, len(DefaultParams(EffectTape)))
	}
}

type paramPush struct {
	uid   uint64
	param string
	value float64
}
