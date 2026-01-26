package ui

import (
	"math"
)

/* ─────────────── math helpers ─────────────────────────────────────────── */

func atan2(y, x float64) float64 { return math.Atan2(y, x) }
func hypot(a, b float64) float64 { return math.Hypot(a, b) }
func abs(i int) int {
	if i < 0 {
		return -i
	}
	return i
}

// PerfSnapshot returns a copy of the current perf stats for tests and JS.
func (g *Game) PerfSnapshot() PerfStats { return g.perf.snapshot() }
