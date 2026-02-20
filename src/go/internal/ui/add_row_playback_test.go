package ui

import (
	"sync/atomic"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// TestAddRowDuringPlayback_SchedulingContinues verifies that adding a drum row
// while playing does not halt audio scheduling for existing rows. Regression
// test for two bugs:
//   - game_update.go did not set needsBeatInfos on row addition, leaving the
//     path snapshot stale (wrong row count), causing the sequencer gate to
//     return early every tick.
//   - game_graph_update_beat_infos.go reallocated pathSigByRow without copying
//     old entries, making every existing row appear path-changed and triggering
//     audio.Stop() for all instruments.
func TestAddRowDuringPlayback_SchedulingContinues(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultAudio(t)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// Build a 2-node loop on row 0.
	n0 := g.tryAddNode(0, 0, 0)
	g.start = n0
	g.graph.StartNodeID = n0.ID
	n1 := g.tryAddNode(1, 0, 0)
	g.addEdge(n0, n1)
	g.addEdge(n1, n0)
	g.updateBeatInfos()

	// Track plays.
	var plays int32
	g.SetPlayFunc(func(string, float64, ...float64) { atomic.AddInt32(&plays, 1) })

	// Start playback and advance scheduling to verify it works.
	g.SetPlaying(true)
	scheduleAbsForMuteTest(g, 0)
	scheduleAbsForMuteTest(g, 1)

	if len(g.seqNextIdxs) == 0 {
		t.Fatalf("missing sequencer counters after initial scheduling")
	}
	if g.seqNextIdxs[0] < 1 {
		t.Fatalf("sequencer did not advance initially: seqNextIdxs[0]=%d", g.seqNextIdxs[0])
	}

	// Record state before adding row.
	preAddIdx := g.seqNextIdxs[0]

	// Add a row mid-playback.
	g.drum.AddRow()
	_ = g.Update() // processes the addition, calls updateBeatInfos

	// Snapshot must now match new row count.
	snap := g.seqPathSnapshot()
	if snap == nil {
		t.Fatalf("nil path snapshot after adding row")
	}
	if len(snap.beatInfosByRow) != len(g.drum.Rows) {
		t.Fatalf("snapshot row count mismatch: got %d, want %d",
			len(snap.beatInfosByRow), len(g.drum.Rows))
	}

	// Ensure sequencer indices are resized.
	if len(g.seqNextIdxs) != len(g.drum.Rows) {
		g.seqNextIdxs = make([]int, len(g.drum.Rows))
		g.seqNextIdxs[0] = preAddIdx
	}
	if len(g.nextBeatIdxs) != len(g.drum.Rows) {
		old := g.nextBeatIdxs
		g.nextBeatIdxs = make([]int, len(g.drum.Rows))
		copy(g.nextBeatIdxs, old)
	}

	// Advance scheduling further — row 0 should continue advancing.
	scheduleAbsForMuteTest(g, preAddIdx+1)
	scheduleAbsForMuteTest(g, preAddIdx+2)

	if g.seqNextIdxs[0] <= preAddIdx {
		t.Fatalf("scheduling halted after adding row: seqNextIdxs[0]=%d, preAdd=%d",
			g.seqNextIdxs[0], preAddIdx)
	}

	// Verify that existing row 0's path was NOT marked as changed during the
	// add-row updateBeatInfos. If pathSigByRow were naively reallocated without
	// copying, row 0 would appear changed and audio.Stop would fire for it.
	if len(g.rowsPathChanged) > 0 && g.rowsPathChanged[0] {
		t.Fatalf("row 0 incorrectly marked as path-changed after adding row")
	}
}

// TestAddRowDuringPlayback_NoStopExistingInstrument verifies that adding a row
// does not call audio.Stop for an existing row's instrument via false path
// change detection.
func TestAddRowDuringPlayback_NoStopExistingInstrument(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultAudio(t)
	// Use a distinct instrument for the initial row to isolate stop tracking.
	audio.ResetCatalogForTest([]audio.SoundMeta{{ID: "kick808", Name: "Kick808"}})
	t.Cleanup(func() { audio.ResetCatalogForTest(nil) })

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// Override row 0 instrument to something unique.
	g.drum.Rows[0].Instrument = "kick808"

	// Build a 2-node loop on row 0.
	n0 := g.tryAddNode(0, 0, 0)
	g.start = n0
	g.graph.StartNodeID = n0.ID
	n1 := g.tryAddNode(1, 0, 0)
	g.addEdge(n0, n1)
	g.addEdge(n1, n0)
	g.updateBeatInfos()

	var plays int32
	g.SetPlayFunc(func(string, float64, ...float64) { atomic.AddInt32(&plays, 1) })

	var kick808Stops int32
	audio.SetStopHook(func(id string) {
		if id == "kick808" {
			atomic.AddInt32(&kick808Stops, 1)
		}
	})
	t.Cleanup(func() { audio.SetStopHook(nil) })

	g.SetPlaying(true)
	scheduleAbsForMuteTest(g, 0)

	// Add a row mid-playback and process it.
	g.drum.AddRow()
	_ = g.Update()

	if s := atomic.LoadInt32(&kick808Stops); s > 0 {
		t.Fatalf("audio.Stop(\"kick808\") called %d times after row add; existing instrument should not be stopped", s)
	}
}
