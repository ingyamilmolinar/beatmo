package ui

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

func TestDesktopPerfCountersCollect(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	// Freeze input to avoid incidental UI work.
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	// Build 3 tiny rectangles (1 grid-unit sides) to stress subdivision rate.
	build := func(row, x int) {
		g.pendingStartRow = row
		n0 := g.tryAddNode(x+0, 0, model.NodeTypeRegular)
		n1 := g.tryAddNode(x+1, 0, model.NodeTypeRegular)
		n2 := g.tryAddNode(x+1, 1, model.NodeTypeRegular)
		n3 := g.tryAddNode(x+0, 1, model.NodeTypeRegular)
		g.addEdge(n0, n1)
		g.addEdge(n1, n2)
		g.addEdge(n2, n3)
		g.addEdge(n3, n0)
		g.pendingStartRow = -1
	}
	for len(g.drum.Rows) < 3 {
		g.drum.AddRow()
	}
	build(0, 0)
	build(1, 3)
	build(2, 6)
	g.updateBeatInfos()

	g.drum.SetBPM(180)

	// Let BPM apply and then start.
	waitForUpdateCond(t, g, 10000, func() bool { return g.AppliedBPM() == 180 })
	pressPlay(t, g.drum)
	_ = g.Update()

	// Run a deterministic number of frames to accumulate metrics.
	advancePlaybackByAbs(g, g.grid.MaxDiv()*8)
	waitForUpdateCond(t, g, 10000, func() bool { return g.PerfSnapshot().AudioDeq > 0 })

	s := g.PerfSnapshot()
	t.Logf("perf desktop: fps=%.1f upd_avg=%.3fms upd_max=%.3fms enq=%d deq=%d qlat_avg=%.3fms call_avg=%.3fms",
		s.FPSAvg, s.UpdateAvgMS, s.UpdateMaxMS, s.AudioEnq, s.AudioDeq, s.AudioQLatAvg, s.AudioCallAvg)
	if s.Frames == 0 {
		t.Fatalf("no frames recorded")
	}
	if s.AudioDeq == 0 {
		t.Fatalf("no audio dispatch recorded")
	}
}

// Without zeroing the draw accumulators, DrawAvgMS divides cumulative draw time by per-reset frames and grows
// across every reset, corrupting every perf test that resets between phases.
func TestPerfResetClearsDrawStats(t *testing.T) {
	var p perfCounters
	// Get past the draw warmup so onDraw actually accumulates.
	for i := 0; i < perfDrawWarmupFrames+5; i++ {
		atomic.AddInt64(&p.frames, 1)
		p.onDraw(20 * time.Millisecond)
	}
	if got := atomic.LoadInt64(&p.drawSumNS); got == 0 {
		t.Fatalf("precondition: drawSumNS should be non-zero before reset, got %d", got)
	}
	p.reset()
	if got := atomic.LoadInt64(&p.drawSumNS); got != 0 {
		t.Errorf("reset() left drawSumNS=%d; want 0", got)
	}
	if got := atomic.LoadInt64(&p.drawMaxNS); got != 0 {
		t.Errorf("reset() left drawMaxNS=%d; want 0", got)
	}
	if got := atomic.LoadInt64(&p.frames); got != 0 {
		t.Errorf("reset() left frames=%d; want 0", got)
	}
}
