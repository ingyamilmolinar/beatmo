//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

func TestSynthStageDownstreamOverride_DisablesDownstreamOnly(t *testing.T) {
	g := newModularSynthTabGame(t)
	layoutSynthTab(t, g)
	dv := g.drum
	if len(dv.instEditorChips) < 3 {
		t.Skip("modular chip strip expected")
	}
	ov := dv.synthStageDownstreamOverride(1)
	for i, c := range dv.instEditorChips {
		if c.enableParam == "" {
			continue
		}
		_, disabled := ov[c.enableParam]
		if i <= 1 && disabled {
			t.Errorf("chip %d (%s) is upstream/self but disabled", i, c.enableParam)
		}
		if i > 1 && !disabled {
			t.Errorf("chip %d (%s) is downstream but not disabled", i, c.enableParam)
		}
	}
}

func TestSynthStageThumbs_DifferAcrossStagesAndHashGate(t *testing.T) {
	g := newModularSynthTabGame(t)
	layoutSynthTab(t, g)
	dv := g.drum
	inst := dv.resolveSynthInstrument("")
	// Make at least one gated stage audible so its thumb differs from upstream.
	audio.SetInstrumentParam(inst, "filter_enabled", 1)
	audio.SetInstrumentParam(inst, "filter_cutoff", 300)
	// nil pool: the render job stays pending until drainForTest — deterministic.
	th := &synthStageThumbs{}
	th.ensure(dv, inst, "h1")
	th.drainForTest()
	first := th.waveFor(0)
	last := th.waveFor(len(dv.instEditorChips) - 1)
	if len(first) == 0 || len(last) == 0 {
		t.Fatalf("thumbs not rendered")
	}
	if sameWave(first, last) {
		t.Fatalf("first-stage and last-stage thumbs identical — pipeline not depicted")
	}
	// Hash gate: same hash must not re-render (mutate a param w/o changing hash).
	audio.SetInstrumentParam(inst, "filter_cutoff", 8000)
	th.ensure(dv, inst, "h1")
	th.drainForTest()
	if !sameWave(th.waveFor(len(dv.instEditorChips)-1), last) {
		t.Fatalf("same hash re-rendered — gate broken")
	}
}

// TestSynthStageThumbs_StaleWhileRevalidate pins the debounce contract: after
// a new-hash ensure the OLD waves stay visible (no empty flicker mid-drag)
// until the render job lands (drainForTest here; the shared 1-worker pool in
// production), and the NEW waves appear after.
func TestSynthStageThumbs_StaleWhileRevalidate(t *testing.T) {
	g := newModularSynthTabGame(t)
	layoutSynthTab(t, g)
	dv := g.drum
	inst := dv.resolveSynthInstrument("")
	th := &synthStageThumbs{}
	th.ensure(dv, inst, "h1")
	th.drainForTest()
	old := th.waveFor(0)
	if len(old) == 0 {
		t.Fatalf("initial thumbs not rendered")
	}
	// Real param change + new hash, NOT drained yet: old waves must survive.
	audio.SetInstrumentParam(inst, "osc_type", 1) // sine -> saw
	th.ensure(dv, inst, "h2")
	if !sameWave(th.waveFor(0), old) {
		t.Fatalf("stale waves replaced before the render job ran")
	}
	// Drain: the new render lands and differs from the old.
	th.drainForTest()
	if sameWave(th.waveFor(0), old) {
		t.Fatalf("thumbs unchanged after drain — revalidate did not land")
	}
}

// TestSynthStageThumbs_EnsureIsCheapWhenUnchanged pins the frame-budget fix:
// an ensure with an unchanged hash+chip count must not stash a render job
// (the pre-fix synchronous path re-rendered ~10 previews per drag frame).
func TestSynthStageThumbs_EnsureIsCheapWhenUnchanged(t *testing.T) {
	g := newModularSynthTabGame(t)
	layoutSynthTab(t, g)
	dv := g.drum
	inst := dv.resolveSynthInstrument("")
	th := &synthStageThumbs{}
	th.ensure(dv, inst, "h1")
	th.drainForTest()
	th.ensure(dv, inst, "h1")
	th.mu.Lock()
	pending := th.pending != nil
	th.mu.Unlock()
	if pending {
		t.Fatalf("unchanged ensure stashed a render job — per-frame gate broken")
	}
}
