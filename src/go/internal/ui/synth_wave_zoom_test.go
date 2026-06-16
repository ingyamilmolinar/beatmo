//go:build test

package ui

import (
	"math"
	"testing"
)

func makeWave(cycles, ptsPerCycle int, harmonic float64) []float64 {
	n := cycles * ptsPerCycle
	w := make([]float64, n)
	for i := range w {
		w[i] = math.Sin(2 * math.Pi * harmonic * float64(i) / float64(ptsPerCycle))
	}
	return w
}

// TestAutoZoomCycles_BusyZoomsIn — a busy, harmonic-rich wave must DISPLAY fewer
// cycles (zoom in) than a simple sine, so its shape stays legible instead of
// smearing into a dense band.
func TestAutoZoomCycles_BusyZoomsIn(t *testing.T) {
	sine := makeWave(6, 96, 1)  // 2 crossings/cycle
	busy := makeWave(6, 96, 6)  // ~12 crossings/cycle
	zSine := autoZoomCycles(sine, 6)
	zBusy := autoZoomCycles(busy, 6)
	if zBusy >= zSine {
		t.Errorf("busy wave should zoom in further than sine: busy=%d sine=%d", zBusy, zSine)
	}
	if zBusy < 1 {
		t.Errorf("zoom must show at least 1 cycle, got %d", zBusy)
	}
}

// TestAutoZoomCycles_Bounds — the result is always within [1, cyclesRendered].
func TestAutoZoomCycles_Bounds(t *testing.T) {
	for _, h := range []float64{0.5, 1, 2, 4, 8, 16} {
		w := makeWave(6, 96, h)
		z := autoZoomCycles(w, 6)
		if z < 1 || z > 6 {
			t.Errorf("harmonic %v: zoom %d out of [1,6]", h, z)
		}
	}
}

// TestAutoZoomWindow_PrefixLen — the display window is a whole number of cycles.
func TestAutoZoomWindow_PrefixLen(t *testing.T) {
	w := makeWave(6, 96, 4)
	win := autoZoomWindow(w, 6)
	if len(win) == 0 || len(win)%96 != 0 {
		t.Fatalf("window len %d is not a whole number of 96-pt cycles", len(win))
	}
	if len(win) > len(w) {
		t.Fatalf("window %d longer than source %d", len(win), len(w))
	}
}
