//go:build js && wasm && !test

package ui

import (
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	scope "github.com/ingyamilmolinar/beatmo/internal/scope"
)

// The loop lives in the untagged Impl so fast tests run it with a stubbed fetch. TTL-cached on (activeID, rows,
// rate), returning the same pointer: the per-channel slice allocations were the dominant per-Draw allocator.
func BuildAnalyzerStateFromSnapshots(activeID channel.ID, rows []*DrumRow, sampleRate int) *analyzer.State {
	fp := rowsFingerprint(rows)
	return cachedAnalyzerState(activeID, fp, sampleRate, func() *analyzer.State {
		return buildAnalyzerStateFromSnapshotsImpl(activeID, rows, sampleRate, audio.ChannelAnalyzerSnapshot, audio.ChannelAnalyzerMetrics)
	})
}

// Scalar-only for the Levels tab: no slices and no per-element js.Value loop. Cached like the full state, since
// every call still crosses the bridge.
func BuildAnalyzerMetricsOnly(rows []*DrumRow) *analyzer.State {
	fp := rowsFingerprint(rows)
	return cachedAnalyzerMetrics(fp, func() *analyzer.State {
		return buildAnalyzerMetricsOnlyImpl(rows, audio.ChannelAnalyzerMetrics)
	})
}

// TTL-cached on (instID, tapA, tapB), which removes the per-Draw state construction and the bridge calls.
func BuildScopeStateFromSnapshots(instID channel.ID, tapA, tapB scope.Stage) *scope.State {
	id := instID
	return cachedScopeState(id, tapA, tapB, func() *scope.State {
		snaps := ScopeStageSnapshots{
			Synth:  audio.SynthAnalyzerSnapshot(id),
			PreEQ:  audio.PreEQAnalyzerSnapshot(id),
			PostEQ: audio.ChannelAnalyzerSnapshot(id),
			Master: audio.ChannelAnalyzerSnapshot(channel.Master),
		}
		return SynthesizeScopeState(id, tapA, tapB, snaps)
	})
}

// A no-op: the browser reads real snapshots; kept so scene_catalog.go compiles on both platforms.
func SetSceneScopeState(*scope.State) {}
