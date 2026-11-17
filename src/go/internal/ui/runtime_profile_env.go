package ui

import "github.com/ingyamilmolinar/beatmo/internal/env"

// applyEnvProfileOverride applies BEATMO_* environment overrides on top of a
// builder-produced profile. Phase F bench sweeps use these to flip individual
// knobs without recompiling.
//
// This file carries no build tags: env vars now work on WASM too (the JS
// bootstrap populates go.env from window.__beatmoEnv), so the browser and
// desktop profiles share one override path. The bespoke
// window.__beatmoProfileOverride reader that runtime_profile_js.go used to
// carry is retired in favor of __beatmoEnv.
//
// Each assignment is guarded on IsSet so an unset variable leaves the
// builder's value untouched — matching the (value, ok) helpers this
// replaced.
//
// Fourteen fields are covered, not thirteen: MinDispatchLeadSec was
// reachable ONLY through window.__beatmoProfileOverride's
// "minDispatchLeadSec" key before this task — no env var ever set it. It is
// included here so retiring that reader doesn't silently drop the ability to
// sweep it (see env.MinDispatchLeadSec's doc comment).
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
	// BEATMO_FORCE_GC_INTERVAL_MS in milliseconds; 0 disables the cooperative
	// forced GC. Desktop default is 0 (off); the GC-tuning sweep uses this to
	// probe whether forced GC helps desktop too.
	if env.ForceGCInterval.IsSet() {
		p.ForceGCInterval = env.ForceGCInterval.Get()
	}
}
