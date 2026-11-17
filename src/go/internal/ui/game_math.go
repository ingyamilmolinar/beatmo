package ui

import (
	"math"
)

/* ─────────────── math helpers ─────────────────────────────────────────── */

func hypot(a, b float64) float64 { return math.Hypot(a, b) }
func abs(i int) int {
	if i < 0 {
		return -i
	}
	return i
}

// PerfSnapshot returns a copy of the current perf stats for tests and JS.
func (g *Game) PerfSnapshot() PerfStats {
	s := g.perf.snapshot()
	s.SchedMetrics = g.schedMetrics.Snapshot()
	gc := g.gcMetrics.snapshot()
	s.GCPauses = gc.Pauses
	s.GCCycles = gc.Cycles
	s.GCForced = gc.Forced
	s.GCPauseP50MS = gc.PauseP50MS
	s.GCPauseP99MS = gc.PauseP99MS
	s.GCPauseMaxMS = gc.PauseMaxMS
	s.GCHeapHighKB = gc.HeapHighKB
	return s
}
