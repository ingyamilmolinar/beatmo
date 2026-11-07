//go:build !test

package ui

import (
	"os"
	"testing"
)

// The real vector.Path allocator, which the stub no-ops. RUN_SOAK_REAL=1 keeps `make test-real` from paying for it
// every time.
func TestSoakHeapBounded_Real(t *testing.T) {
	if os.Getenv("RUN_SOAK_REAL") != "1" {
		t.Skip("set RUN_SOAK_REAL=1 to enable the real-Ebiten soak test")
	}
	scenarios := []soakScenario{
		{name: "idle"},
		{name: "playing-moderate", playing: true},
		{name: "playing-with-edits", playing: true, editEvery: 600},
		// Only the real allocator exercises the vector.Path side of FX churn.
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
