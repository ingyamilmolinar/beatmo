package ui

import (
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/env"
)

func TestApplyEnvProfileOverrideAppliesEveryField(t *testing.T) {
	restores := []func(){
		env.EdgeCachePad.SetForTest(11),
		env.GridCachePad.SetForTest(12),
		env.DisableNodeGlow.SetForTest(true),
		env.DisableEdgeArrows.SetForTest(true),
		env.SimpleDrawDefault.SetForTest(true),
		env.TimelineInfoThrottleMS.SetForTest(13),
		env.AudioLookaheadSec.SetForTest(0.25),
		env.MinDispatchLeadSec.SetForTest(0.017),
		env.AudioBatchMax.SetForTest(14),
		env.SequencerTickMS.SetForTest(15),
		env.AdaptivePanPad.SetForTest(true),
		env.FastPanDetect.SetForTest(true),
		env.PredictorBackoff.SetForTest(true),
		env.ForceGCInterval.SetForTest(16 * time.Millisecond),
	}
	for _, r := range restores {
		defer r()
	}

	p := &RuntimeProfile{}
	applyEnvProfileOverride(p)

	if p.EdgeCachePad != 11 {
		t.Errorf("EdgeCachePad=%d, want 11", p.EdgeCachePad)
	}
	if p.GridCachePad != 12 {
		t.Errorf("GridCachePad=%d, want 12", p.GridCachePad)
	}
	if !p.DisableNodeGlow {
		t.Errorf("DisableNodeGlow=false, want true")
	}
	if !p.DisableEdgeArrows {
		t.Errorf("DisableEdgeArrows=false, want true")
	}
	if !p.SimpleDrawDefault {
		t.Errorf("SimpleDrawDefault=false, want true")
	}
	if p.TimelineInfoThrottleMS != 13 {
		t.Errorf("TimelineInfoThrottleMS=%d, want 13", p.TimelineInfoThrottleMS)
	}
	if p.AudioLookaheadSec != 0.25 {
		t.Errorf("AudioLookaheadSec=%v, want 0.25", p.AudioLookaheadSec)
	}
	if p.MinDispatchLeadSec != 0.017 {
		t.Errorf("MinDispatchLeadSec=%v, want 0.017", p.MinDispatchLeadSec)
	}
	if p.AudioBatchMax != 14 {
		t.Errorf("AudioBatchMax=%d, want 14", p.AudioBatchMax)
	}
	if p.SequencerTickMS != 15 {
		t.Errorf("SequencerTickMS=%d, want 15", p.SequencerTickMS)
	}
	if !p.AdaptivePanPad {
		t.Errorf("AdaptivePanPad=false, want true")
	}
	if !p.FastPanDetect {
		t.Errorf("FastPanDetect=false, want true")
	}
	if !p.PredictorBackoffOnSlowDraw {
		t.Errorf("PredictorBackoffOnSlowDraw=false, want true")
	}
	if p.ForceGCInterval != 16*time.Millisecond {
		t.Errorf("ForceGCInterval=%v, want 16ms", p.ForceGCInterval)
	}
}

// A var left unset must not clobber the builder's value.
func TestApplyEnvProfileOverrideLeavesUnsetFieldsAlone(t *testing.T) {
	p := &RuntimeProfile{EdgeCachePad: 99, DisableNodeGlow: true, MinDispatchLeadSec: 0.5}
	applyEnvProfileOverride(p)
	if p.EdgeCachePad != 99 {
		t.Errorf("EdgeCachePad=%d, want 99 — an unset env var must not overwrite", p.EdgeCachePad)
	}
	if !p.DisableNodeGlow {
		t.Errorf("DisableNodeGlow=false, want true — an unset env var must not overwrite")
	}
	if p.MinDispatchLeadSec != 0.5 {
		t.Errorf("MinDispatchLeadSec=%v, want 0.5 — an unset env var must not overwrite", p.MinDispatchLeadSec)
	}
}
