//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// TestParityDecisionsBoundedUnderScanStarvation reproduces the production WASM
// OOM where the sequencer goroutine writes to paritySeqDecisions at audio
// cadence while the UI goroutine — which calls parityScan and triggers
// parityPruneAroundPlayhead — is starved (long draws, slow refresh, the
// WARN [refresh] total slow elapsed=49–100ms messages just before the crash).
//
// The pre-existing TestParityDecisionsBounded passes today because in synthetic
// runs Draw/Update advance in lockstep with playback, so parityScan runs on
// every frame and pruning keeps up. This test omits the UI loop entirely and
// calls recordSeqDecision directly; without the inline cap in
// recordSeqDecision, paritySeqDecisions[row] grows linearly with abs and OOMs
// the WASM heap on long sessions.
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

	// Simulate the production failure mode: sequencer-goroutine writes happen
	// without ever calling parityScan (which is what would call the deferred
	// parityPruneAroundPlayhead). 200_000 abs steps comfortably exceeds the
	// per-row cap and matches the production crash horizon (~55 min playback).
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
