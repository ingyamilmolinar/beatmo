package ui

import "github.com/ingyamilmolinar/beatmo/internal/env"

// Shared by both builds: the JS bootstrap fills go.env from window.__beatmoEnv. Each field is guarded on IsSet so
// an unset variable leaves the builder's value.
func applyEnvProfileOverride(p *RuntimeProfile) {
	if env.EdgeCachePad.IsSet() {
		p.EdgeCachePad = env.EdgeCachePad.Get()
	}
	if env.GridCachePad.IsSet() {
		p.GridCachePad = env.GridCachePad.Get()
	}
	if env.DisableNodeGlow.IsSet() {
		p.DisableNodeGlow = env.DisableNodeGlow.Get()
	}
	if env.DisableEdgeArrows.IsSet() {
		p.DisableEdgeArrows = env.DisableEdgeArrows.Get()
	}
	if env.SimpleDrawDefault.IsSet() {
		p.SimpleDrawDefault = env.SimpleDrawDefault.Get()
	}
	if env.TimelineInfoThrottleMS.IsSet() {
		p.TimelineInfoThrottleMS = env.TimelineInfoThrottleMS.Get()
	}
	if env.AudioLookaheadSec.IsSet() {
		p.AudioLookaheadSec = env.AudioLookaheadSec.Get()
	}
	if env.MinDispatchLeadSec.IsSet() {
		p.MinDispatchLeadSec = env.MinDispatchLeadSec.Get()
	}
	if env.AudioLane.IsSet() {
		p.AudioLaneEnabled = env.AudioLane.Get()
	}
	if env.AudioHorizonSec.IsSet() {
		p.AudioHorizonSec = env.AudioHorizonSec.Get()
	}
	if env.AudioBatchMax.IsSet() {
		p.AudioBatchMax = env.AudioBatchMax.Get()
	}
	if env.SequencerTickMS.IsSet() {
		p.SequencerTickMS = env.SequencerTickMS.Get()
	}
	if env.AdaptivePanPad.IsSet() {
		p.AdaptivePanPad = env.AdaptivePanPad.Get()
	}
	if env.FastPanDetect.IsSet() {
		p.FastPanDetect = env.FastPanDetect.Get()
	}
	if env.PredictorBackoff.IsSet() {
		p.PredictorBackoffOnSlowDraw = env.PredictorBackoff.Get()
	}
	// Milliseconds; 0 disables the forced GC.
	if env.ForceGCInterval.IsSet() {
		p.ForceGCInterval = env.ForceGCInterval.Get()
	}
}
