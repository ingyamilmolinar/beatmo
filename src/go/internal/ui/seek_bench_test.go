//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// buildSeekBenchGame builds a game with a single row driven by an N-node loop,
// advances real playback so the predictor window and the timeline immutables
// sidecar are populated (the state a seek must rewind), and returns it playing.
// playBeats controls how much history is laid down before the benchmark loop.
func buildSeekBenchGame(b *testing.B, loopNodes, playBeats int) *Game {
	b.Helper()
	g := New(testLogger)
	g.SetPlayFunc(func(string, float64, ...float64) {})
	g.Layout(1024, 720)
	g.drum.SetFollow(true)
	g.drum.SetLength(16)
	g.drum.SetBPM(120)

	nodes := make([]*uiNode, loopNodes)
	for i := 0; i < loopNodes; i++ {
		nodes[i] = g.tryAddNode(i, 0, model.NodeTypeRegular)
		if i > 0 {
			g.addEdge(nodes[i-1], nodes[i])
		}
	}
	g.addEdge(nodes[loopNodes-1], nodes[0])
	g.start = nodes[0]
	g.graph.StartNodeID = nodes[0].ID
	g.drum.Rows[0].Origin = nodes[0].ID
	g.drum.Rows[0].Node = nodes[0]
	g.updateBeatInfos()
	g.refreshDrumRow()

	// Lay down real playback history so RewindTo has immutables to archive and
	// RebaseAt has a populated window to reset. This mirrors "seek after the
	// song has been playing for a while".
	g.drum.playPressed = true
	_ = g.Update()
	advancePlaybackByAbs(g, playBeats*g.grid.MaxDiv())
	return g
}

// benchSeekAlternating alternates seeking to `beat` and back to 0, so every
// iteration performs a real rewind (RewindTo per row + Predictor.RebaseAt +
// counter/pulse realloc) rather than a no-op re-seek to the same position.
// Comparing near vs far `beat` exposes RebaseAt's O(idx) per-row re-walk.
func benchSeekAlternating(b *testing.B, beat int) {
	g := buildSeekBenchGame(b, 4, 64)
	defer g.CloseForTest()
	div := g.grid.MaxDiv()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i&1 == 0 {
			g.SeekSteps(beat*div, seekOriginRibbon) // forward
		} else {
			g.SeekSteps(0, seekOriginRibbon) // backward to origin
		}
	}
}

// Near seek: short re-walk (beat 8). Representative of a small FF/RW jump.
func BenchmarkSeekNear_Beat8(b *testing.B) { benchSeekAlternating(b, 8) }

// Mid seek: beat 64.
func BenchmarkSeekMid_Beat64(b *testing.B) { benchSeekAlternating(b, 64) }

// Far seek: long re-walk (beat 256). Exposes the O(idx) cost of RebaseAt's
// re-walk from 0.
func BenchmarkSeekFar_Beat256(b *testing.B) { benchSeekAlternating(b, 256) }

// BenchmarkSeekForwardWalk seeks strictly forward across the song (0,4,8,…),
// wrapping — the FF-jump-through-a-song access pattern.
func BenchmarkSeekForwardWalk(b *testing.B) {
	g := buildSeekBenchGame(b, 4, 64)
	defer g.CloseForTest()
	div := g.grid.MaxDiv()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		g.SeekSteps(((i%128)*4)*div, seekOriginTransport)
	}
}

// BenchmarkSeekBackwardWalk seeks strictly backward across the song
// (508,504,…,0) — the RW-jump-back access pattern.
func BenchmarkSeekBackwardWalk(b *testing.B) {
	g := buildSeekBenchGame(b, 4, 64)
	defer g.CloseForTest()
	div := g.grid.MaxDiv()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		g.SeekSteps(((127-(i%128))*4)*div, seekOriginTransport)
	}
}

// BenchmarkTimelineEditDuringPlayback measures a structural timeline edit
// (add a node then delete it) while the circuit is playing — the other "used
// during playback" hot path.
func BenchmarkTimelineEditDuringPlayback(b *testing.B) {
	g := buildSeekBenchGame(b, 4, 64)
	defer g.CloseForTest()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		n := g.tryAddNode(6, 1, model.NodeTypeRegular)
		if n != nil {
			g.updateBeatInfos()
			g.deleteNode(n)
			g.updateBeatInfos()
		}
	}
}
