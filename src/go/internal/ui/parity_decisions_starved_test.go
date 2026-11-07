//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// The sequencer records decisions at audio cadence while the UI goroutine that scans and prunes is starved, so
// recordSeqDecision needs its own inline cap; without the UI loop the per-row map would grow with abs.
func TestParityDecisionsBoundedUnderScanStarvation(t *testing.T) {
	withDefaultAudio(t)
	withDefaultStart(t, false)

	prevWatch := parityWatchDefault
	prevFatal := parityFatalEnabled.Load()
	parityWatchDefault = parityWatchLog
	parityFatalEnabled.Store(false)
	t.Cleanup(func() {
		parityWatchDefault = prevWatch
		parityFatalEnabled.Store(prevFatal)
	})

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.parityWatch = parityWatchLog

	// Writes with no parityScan, so the deferred prune never runs; far more steps than the per-row cap.
	const rows = 6
	const steps = 200_000
	for row := 0; row < rows; row++ {
		for abs := 0; abs < steps; abs++ {
			g.recordSeqDecision(row, abs, true, model.NodeTypeRegular, false)
		}
	}

	g.parityMu.Lock()
	defer g.parityMu.Unlock()
	for row := 0; row < rows; row++ {
		if got := len(g.paritySeqDecisions[row]); got > paritySeqDecisionsPerRowMax {
			t.Errorf("paritySeqDecisions[row=%d] = %d entries after %d writes; "+
				"want ≤ paritySeqDecisionsPerRowMax = %d. The inline cap in "+
				"recordSeqDecision should hold even when parityScan never runs.",
				row, got, steps, paritySeqDecisionsPerRowMax)
		}
	}
}
