//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Real playback populates the predictor window and the timeline history a seek must rewind.
func buildSeekBenchGame(b *testing.B, loopNodes, playBeats int) *Game {
	b.Helper()
	g := New(testLogger)
	g.SetPlayFunc(func(instrument.ID, float64, ...float64) {})
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

	// "Seek after the song has been playing for a while."
	g.drum.playPressed = true
	_ = g.Update()
	advancePlaybackByAbs(g, playBeats*g.grid.MaxDiv())
	return g
}

// Alternating makes every iteration a real rewind, not a no-op re-seek; near vs far exposes RebaseAt's O(idx)
// per-row re-walk.
func benchSeekAlternating(b *testing.B, beat int) {
	g := buildSeekBenchGame(b, 4, 64)
	defer g.CloseForTest()
	div := g.grid.MaxDiv()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i&1 == 0 {
			g.SeekSteps(beat*div, hooks.SeekRibbon) // forward
		} else {
			g.SeekSteps(0, hooks.SeekRibbon) // backward to origin
		}
	}
}

// Near seek: short re-walk (beat 8). Representative of a small FF/RW jump.
func BenchmarkSeekNear_Beat8(b *testing.B) { benchSeekAlternating(b, 8) }

// Mid seek: beat 64.
func BenchmarkSeekMid_Beat64(b *testing.B) { benchSeekAlternating(b, 64) }

func BenchmarkSeekFar_Beat256(b *testing.B) { benchSeekAlternating(b, 256) }

// The fast-forward jump-through-a-song pattern.
func BenchmarkSeekForwardWalk(b *testing.B) {
	g := buildSeekBenchGame(b, 4, 64)
	defer g.CloseForTest()
	div := g.grid.MaxDiv()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		g.SeekSteps(((i%128)*4)*div, hooks.SeekTransport)
	}
}

// The rewind jump-back pattern.
func BenchmarkSeekBackwardWalk(b *testing.B) {
	g := buildSeekBenchGame(b, 4, 64)
	defer g.CloseForTest()
	div := g.grid.MaxDiv()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		g.SeekSteps(((127-(i%128))*4)*div, hooks.SeekTransport)
	}
}

// The other hot path while playing.
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
