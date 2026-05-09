//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestParityDecisionsBounded asserts that g.paritySeqDecisions does not
// grow without bound during sustained playback. The cap mirrors the
// existing parityAudioMax = 1024 bound in game_parity_state.go: per-row
// scheduler decisions should be subject to the same retention discipline.
//
// Currently fails: paritySeqDecisions has no hard cap and is the primary
// suspect for the production OOM seen at ~14s of WASM playback. Flips
// green once a pruning policy lands.
func TestParityDecisionsBounded(t *testing.T) {
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
	g.Layout(1280, 720)
	g.parityWatch = parityWatchLog
	g.SetPlayFunc(func(string, float64, ...float64) {})

	if err := g.SetSubdivisions(16); err != nil {
		t.Fatalf("set subdiv 16: %v", err)
	}
	g.drum.SetBPM(240)

	const rows = 6
	buildSoakScene(t, g, rows, 8)

	screen := ebiten.NewImage(1280, 720)
	g.SetPlaying(true)
	advanceFrames(g, 5)

	const steps = 5000
	for i := 0; i < steps; i++ {
		advancePlaybackByAbs(g, 1)
		g.Draw(screen)
	}

	const perRowCap = 1024 // mirrors parityAudioMax in game_parity_state.go
	maxAllowed := rows * perRowCap

	var total int
	g.parityMu.Lock()
	for _, m := range g.paritySeqDecisions {
		total += len(m)
	}
	rowCount := len(g.paritySeqDecisions)
	g.parityMu.Unlock()

	if total > maxAllowed {
		t.Errorf("paritySeqDecisions grew to %d entries across %d rows after %d abs steps; "+
			"want ≤ %d (rows=%d × per-row cap=%d). The map needs a retention bound matching parityAudioMax.",
			total, rowCount, steps, maxAllowed, rows, perRowCap)
		t.Logf("parityScan invocations:              %d", g.parityScanCallsForTest)
		earlyReturnLabels := []string{
			"parityWatch=off & no fatals",
			"importDialog/importing/lengthChanging",
			"pendingStartRow >= 0",
			"!Playing && !Paused & off & no fatals",
			"!Playing && !Paused & off & test",
			"!Playing && !Paused & runtime",
			"drum==nil || !renderReady",
			"!Playing && reason==refresh",
			"!parityScanDue(every)",
			"renderLength <= 0",
		}
		for i, n := range g.parityScanReturnsForTest {
			if n > 0 {
				t.Logf("  early-return [%d] %s: %d", i, earlyReturnLabels[i], n)
			}
		}
		t.Logf("parityPrune call count during soak: %d", g.parityPruneCallsForTest)
		t.Logf("parityPrune max minAbs seen:        %d", g.parityPruneMaxMinAbsForTest)
		t.FailNow()
	}
}
