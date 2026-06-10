//go:build test

package ui

import "testing"

// TestSoakHeapBounded_Idle is the negative control: no playback, just
// Update+Draw for a long run. Today it should pass; if it fails we caught
// an idle-frame allocator regression independent of the playback leak.
func TestSoakHeapBounded_Idle(t *testing.T) {
	runSoakHeapBound(t, soakScenario{name: "idle"}, 8400, 300, 4, 1.5, 1.3)
}

// TestSoakHeapBounded_PlayingModerate is the primary repro: 6 rows × 8 nodes
// playing for 10× the production OOM duration. Fails today because parity
// recording (paritySeqDecisions) and timeline immutables grow without bound.
func TestSoakHeapBounded_PlayingModerate(t *testing.T) {
	runSoakHeapBound(t, soakScenario{name: "playing-moderate", playing: true}, 8400, 300, 4, 1.5, 1.3)
}

// TestSoakHeapBounded_PlayingWithEdits mirrors the production crash log
// (`row 0 solo on` mid-play) by toggling row solo + mute every 600 frames.
// Keeps coverage if a future fix only prunes idle decisions — the edits
// keep parity gen and the recording path active.
func TestSoakHeapBounded_PlayingWithEdits(t *testing.T) {
	runSoakHeapBound(t, soakScenario{name: "playing-with-edits", playing: true, editEvery: 600}, 8400, 300, 4, 1.5, 1.3)
}

// TestSoakHeapBounded_DrawHeavy draws every frame (sc.drawEvery = 1) over
// a shorter window. The production OOM stack landed in
// RowRackZone.drawRowControlsToCache → vector.Path tessellation; the moderate
// scenarios above sample Draw at 1/30th the cadence to keep wall time low,
// which under-samples Draw-path allocation. This variant catches a leak
// localized to the per-Draw allocator path that the moderate scenarios
// could miss (e.g. a Draw-path closure or temporary that only the real
// vector.Path allocator on WASM would surface).
func TestSoakHeapBounded_DrawHeavy(t *testing.T) {
	runSoakHeapBound(t, soakScenario{
		name:      "draw-heavy",
		playing:   true,
		drawEvery: 1,
	}, 2400, 100, 4, 1.5, 1.3)
}

// TestSoakHeapBounded_PlayingWithFXChurn mirrors the production OOM repro
// most directly: the user's WASM crash log shows
//
//	22:59:42.054 INFO [audio] insert FX removed kick-1 slot=0
//	23:08:43.134 WARN [refresh] row=4 total slow elapsed=47.39968ms
//	runtime: out of memory: cannot allocate 4194304-byte block (2130444288 in use)
//
// — i.e. the user added/removed an insert effect mid-play and ~9 minutes
// later the WASM linear memory hit the 2 GB ceiling. This scenario adds and
// removes a distortion slot every 200 frames during sustained playback, so
// any leak in the effect_chain → channel rebuild → platformInsertEffectsChanged
// path will surface as a slope failure here while the moderate scenario stays
// flat. Bounds match the moderate test — FX churn is rare enough that it
// should not visibly raise steady-state allocation.
func TestSoakHeapBounded_PlayingWithFXChurn(t *testing.T) {
	runSoakHeapBound(t, soakScenario{
		name:         "playing-with-fx-churn",
		playing:      true,
		fxChurnEvery: 200,
	}, 8400, 300, 4, 1.5, 1.3)
}

// TestSoakHeapBounded_PlayingWithFXParamChurn simulates a user dragging a
// slider on a live FX param (e.g. distortion drive) during sustained
// playback. Production WASM marshals the entire slot list to JSON on every
// SetInsertEffectParam call (insert_effects_wasm.go) — slider drags happen
// at frame rate (~60 Hz), so a per-call retain (e.g. an unforgotten
// js.FuncOf or a goroutine spawn per param change) compounds quickly.
//
// We tick the param at every frame ÷ 5 (~12 Hz) for the soak window; that's
// enough samples to catch even a small per-call leak while keeping wall time
// tractable. Bounds match the moderate test.
func TestSoakHeapBounded_PlayingWithFXParamChurn(t *testing.T) {
	runSoakHeapBound(t, soakScenario{
		name:              "playing-with-fx-param-churn",
		playing:           true,
		fxParamChurnEvery: 5,
	}, 8400, 300, 4, 1.5, 1.3)
}

// TestSoakHeapBounded_PlayingWithGraphEdits adds and deletes a node + edge
// on row 1 during sustained playback. Each mutation flips predDirty and
// bumps parityGen, exercising the predictor rebuild + parity prune paths
// that pure playback skips entirely. Real users edit while playing, so any
// leak in the graph-mutation → updateBeatInfos → parity-prune path needs
// soak-time coverage.
func TestSoakHeapBounded_PlayingWithGraphEdits(t *testing.T) {
	runSoakHeapBound(t, soakScenario{
		name:           "playing-with-graph-edits",
		playing:        true,
		graphEditEvery: 600,
	}, 8400, 300, 4, 1.5, 1.3)
}

// TestSoakHeapBounded_PlayingProductionLike combines all production-shaped
// load: edits + FX churn + FX param churn + graph mutation. This is the
// closest stub-build approximation of the user's reported workload. Any
// leak that this catches but the individual scenarios miss is in the
// interaction between mutation paths (e.g. graph edit during FX churn
// leaving parity prune racing the rebuild).
//
// Frame counts: 8 400 in -short mode (the historical baseline, ~10× the
// production OOM observation), 50 000 in the long mode (~6× longer; drives
// playing abs to ~800 k, well past the windowCap=4096 plateau and into
// archive territory). The long mode catches regressions in the sliding
// window + cold archive interaction that the short window misses.
func TestSoakHeapBounded_PlayingProductionLike(t *testing.T) {
	frames := 50_000
	editEvery := 3000
	fxChurnEvery := 1000
	fxParamChurnEvery := 25
	graphEditEvery := 3000
	if testing.Short() {
		frames = 8400
		editEvery = 600
		fxChurnEvery = 200
		fxParamChurnEvery = 5
		graphEditEvery = 600
	}
	runSoakHeapBound(t, soakScenario{
		name:              "playing-production-like",
		playing:           true,
		editEvery:         editEvery,
		fxChurnEvery:      fxChurnEvery,
		fxParamChurnEvery: fxParamChurnEvery,
		graphEditEvery:    graphEditEvery,
	}, frames, 1500, 4, 1.5, 1.3)
}
