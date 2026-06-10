//go:build js && wasm && !test

package ui

import (
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	scope "github.com/ingyamilmolinar/beatmo/internal/scope"
)

// BuildAnalyzerStateFromSnapshots synthesizes an analyzer.State on WASM by
// calling the JS-backed ChannelAnalyzerSnapshot for master plus every drum row,
// and promoting the active row (if distinct from master) to the detail slot.
//
// The loop body lives in buildAnalyzerStateFromSnapshotsImpl (no build tags)
// so fast Go tests can exercise the same code with a stubbed fetch.
//
// Wrapped in a cross-Draw TTL cache (analyzer_state_cache.go): successive
// calls within stateCacheTTL with the same (activeID, rowsFingerprint,
// sampleRate) return the same *analyzer.State pointer. Saves the
// SnapshotToChannelMetrics path's per-channel FFT/Freq slice allocation +
// the State struct + InstrumentMetrics slice — the dominant per-Draw
// allocator after the snapshot-level cache landed.
func BuildAnalyzerStateFromSnapshots(activeID string, rows []*DrumRow, sampleRate int) *analyzer.State {
	fp := rowsFingerprint(rows)
	return cachedAnalyzerState(activeID, fp, sampleRate, func() *analyzer.State {
		return buildAnalyzerStateFromSnapshotsImpl(activeID, rows, sampleRate, audio.ChannelAnalyzerSnapshot)
	})
}

// BuildAnalyzerMetricsOnly is the scalar-only fast path for the Meters
// tab. Skips Spectrum/Waveform allocation entirely and routes through
// audio.ChannelAnalyzerMetrics so the WASM bridge never executes the
// per-element js.Value loop. Returned state has nil slice fields and
// nil Detail; consumers that need slice data must use
// BuildAnalyzerStateFromSnapshots instead. Ported under the OOM
// prevention plan; see render_meters.go (the only consumer).
//
// Cached the same way as BuildAnalyzerStateFromSnapshots — the Meters
// renderer reads it every Draw and the underlying ChannelAnalyzer-
// Metrics call still hits the JS bridge each invocation.
func BuildAnalyzerMetricsOnly(rows []*DrumRow) *analyzer.State {
	fp := rowsFingerprint(rows)
	return cachedAnalyzerMetrics(fp, func() *analyzer.State {
		return buildAnalyzerMetricsOnlyImpl(rows, audio.ChannelAnalyzerMetrics)
	})
}

// BuildScopeStateFromSnapshots synthesizes a scope.State on WASM by reading
// the per-stage JS analyser snapshots. Every scope.Stage now has a real WASM
// tap (or, for StageAntiPop, the ingress signal — see ScopeStageSnapshots).
//
// Cached cross-Draw with TTL gating on (instID, tapA, tapB). The
// Synthesize path no longer copies the tap Waveform on every call (the
// snapshot share now flows through to scope.TapData.Samples directly);
// the cache pin removes the rest of the per-Draw work (State struct +
// TapData struct construction) and the underlying snapshot-level JS
// bridge calls.
func BuildScopeStateFromSnapshots(instID string, tapA, tapB scope.Stage) *scope.State {
	id := instID
	if id == "" {
		id = "main"
	}
	return cachedScopeState(id, tapA, tapB, func() *scope.State {
		snaps := ScopeStageSnapshots{
			Synth:  audio.SynthAnalyzerSnapshot(id),
			PreEQ:  audio.PreEQAnalyzerSnapshot(id),
			PostEQ: audio.ChannelAnalyzerSnapshot(id),
			Sends:  audio.SendBusAnalyzerSnapshot(),
			Master: audio.ChannelAnalyzerSnapshot("main"),
		}
		return SynthesizeScopeState(id, tapA, tapB, snaps)
	})
}
