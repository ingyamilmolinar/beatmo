//go:build test

package ui

import (
	"os"
	"runtime"
	"testing"
	"time"
)

// TestLaneFullReconcileCost (LANE_COST=1): the cost of one FULL reconcile,
// forced every tick by a global-key change — what a node-param drag (graph
// version / predictor epoch bumped every frame) or a held BPM repeat costs —
// on the browser profile (H = 1 s), lane circuit. Measurement, not a gate.
func TestLaneFullReconcileCost(t *testing.T) {
	if os.Getenv("LANE_COST") == "" {
		t.Skip("measurement: LANE_COST=1")
	}
	g, sink := newLaneGame(t)
	laneRun(t, g, 0.05, 0.004)
	const n = 2000
	var ms0, ms1 runtime.MemStats
	runtime.ReadMemStats(&ms0)
	start := time.Now()
	for i := 0; i < n; i++ {
		g.laneGraphVersion.Add(1)
		g.laneSrc.refresh()
		g.audioLane.Tick()
	}
	el := time.Since(start)
	runtime.ReadMemStats(&ms1)
	st := g.audioLane.Stats()
	sched0, canc0 := st.Scheduled, st.Cancelled
	start = time.Now()
	const m = 200
	for i := 0; i < m; i++ { // a held BPM ± repeat: every future note moves
		prev := g.state.AppliedBPM()
		b := 120 + (i%2)*2
		g.drum.SetBPM(b)
		g.state.AnchorForBPMChange(prev, time.Now(), *laneTestNow(g))
		g.state.SetAppliedBPM(b)
		g.laneSrc.refresh()
		g.audioLane.Tick()
	}
	el2 := time.Since(start)
	st2 := g.audioLane.Stats()
	t.Logf("BPM change: %.1f µs/tick, %.1f re-commits/tick", float64(el2.Microseconds())/m, float64(st2.Scheduled-sched0)/m)
	_ = canc0
	t.Logf("full reconcile: %.1f µs/tick, %.0f B/tick, %d committed notes in H, scheduled %d cancelled %d",
		float64(el.Microseconds())/n, float64(ms1.TotalAlloc-ms0.TotalAlloc)/n, len(sink.scheduled()), st.Scheduled, st.Cancelled)
}
