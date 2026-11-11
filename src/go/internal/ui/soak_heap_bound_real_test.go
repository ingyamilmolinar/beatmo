//go:build !test

package ui

import (
	"os"
	"testing"
)

// TestSoakHeapBounded_Real is the real-Ebiten counterpart to the stub-build
// soak suite. It reproduces the same scenarios under the actual vector.Path
// allocator (which the stub no-ops). Gated by RUN_SOAK_REAL=1 so the
// `make test-real` run isn't paying the soak cost on every invocation.
//
// The body shares runSoakHeapBound with the stub variant — see
// soak_heap_bound_helper_test.go for parameters and assertions.
func TestSoakHeapBounded_Real(t *testing.T) {
	if os.Getenv("RUN_SOAK_REAL") != "1" {
		t.Skip("set RUN_SOAK_REAL=1 to enable the real-Ebiten soak test")
	}
	scenarios := []soakScenario{
		{name: "idle"},
		{name: "playing-moderate", playing: true},
		{name: "playing-with-edits", playing: true, editEvery: 600},
		// FX churn under real-Ebiten — the production OOM stack landed in
		// RowRackZone.drawRowControlsToCache → vector.Path tessellation, which
		// only the real-Ebiten allocator exercises. Stub-build coverage of the
		// FX path (TestSoakHeapBounded_PlayingWithFXChurn) misses the
		// vector.Path side; this real-soak entry catches it.
		{name: "playing-with-fx-churn", playing: true, fxChurnEvery: 200},
		{name: "playing-with-fx-param-churn", playing: true, fxParamChurnEvery: 5},
		{name: "playing-with-graph-edits", playing: true, graphEditEvery: 600},
		{
			name:              "playing-production-like",
			playing:           true,
			editEvery:         600,
			fxChurnEvery:      200,
			fxParamChurnEvery: 5,
			graphEditEvery:    600,
		},
	}
	for _, sc := range scenarios {
		sc := sc
		t.Run(sc.name, func(t *testing.T) {
			runSoakHeapBound(t, sc, 8400, 300, 4, 1.5, 1.3)
		})
	}
}
