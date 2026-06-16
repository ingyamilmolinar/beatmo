package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// synth_ghost_test.go — Stage 5 per-knob ghost capture/fade state machine.

// ghostTestInst returns a row instrument that resolves to a synth recipe (so
// MergeRecipeDefaults yields a non-empty param map) and clears any overlay on
// cleanup so the process-global params manager doesn't leak between tests.
func ghostTestInst(t *testing.T, g *Game) string {
	t.Helper()
	inst := g.drum.Rows[0].Instrument
	if audio.RecipeForInstrument(inst) == "" {
		t.Fatalf("row0 instrument %q has no recipe binding", inst)
	}
	audio.ResetInstrumentParams(inst)
	t.Cleanup(func() { audio.ResetInstrumentParams(inst) })
	return inst
}

func TestSynthGhostCaptureReturnsSnapshot(t *testing.T) {
	withDefaultAudio(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	dv := g.drum
	inst := ghostTestInst(t, g)

	if dv.synthConceptGhost(3) != nil {
		t.Fatalf("ghost should be nil before any capture")
	}
	dv.captureSynthGhost(3, inst)
	got := dv.synthConceptGhost(3)
	if got == nil {
		t.Fatalf("synthConceptGhost(3) = nil after capture, want snapshot")
	}
	want := audio.MergeRecipeDefaults(audio.RecipeForInstrument(inst), audio.GetInstrumentParams(inst))
	if len(got) != len(want) || len(got) == 0 {
		t.Fatalf("ghost snapshot size = %d, want %d (non-empty)", len(got), len(want))
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("ghost[%q] = %v, want %v", k, got[k], v)
		}
	}
	// A different knob index has no ghost.
	if dv.synthConceptGhost(7) != nil {
		t.Fatalf("ghost for un-captured knob 7 should be nil")
	}
}

func TestSynthGhostFadesToNil(t *testing.T) {
	withDefaultAudio(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	dv := g.drum
	inst := ghostTestInst(t, g)

	dv.captureSynthGhost(1, inst)
	// While held, advancing frames must NOT clear the ghost.
	for i := 0; i < synthGhostFadeFrames*3; i++ {
		dv.advanceSynthGhost()
	}
	if dv.synthConceptGhost(1) == nil {
		t.Fatalf("held ghost must not fade while drag is in progress")
	}

	// Release: now it fades over synthGhostFadeFrames frames.
	dv.fadeSynthGhost(1)
	for i := 0; i < synthGhostFadeFrames-1; i++ {
		dv.advanceSynthGhost()
	}
	if dv.synthConceptGhost(1) == nil {
		t.Fatalf("ghost cleared too early (before %d frames)", synthGhostFadeFrames)
	}
	dv.advanceSynthGhost() // final tick reaches 0
	if dv.synthConceptGhost(1) != nil {
		t.Fatalf("ghost should be nil after %d fade frames", synthGhostFadeFrames)
	}
}

func TestSynthGhostSnapshotIsIndependentCopy(t *testing.T) {
	withDefaultAudio(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	dv := g.drum
	inst := ghostTestInst(t, g)

	// Pick a param present in the merged set to mutate after capture.
	merged := audio.MergeRecipeDefaults(audio.RecipeForInstrument(inst), audio.GetInstrumentParams(inst))
	var probe string
	var before float64
	for k, v := range merged {
		probe, before = k, v
		break
	}
	if probe == "" {
		t.Fatalf("instrument %q has no merged params to probe", inst)
	}

	dv.captureSynthGhost(2, inst)
	snap := dv.synthConceptGhost(2)
	if snap == nil {
		t.Fatalf("no ghost after capture")
	}

	// Mutate the LIVE params after capture; the ghost snapshot must not change.
	audio.SetInstrumentParam(inst, probe, before+0.123)
	if got := dv.synthConceptGhost(2)[probe]; got != before {
		t.Fatalf("ghost[%q] = %v after live mutation, want frozen %v (snapshot not an independent copy)", probe, got, before)
	}
}
