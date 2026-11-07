package ui

import (
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

func scheduleAbsNoHighlightForTest(g *Game, abs int) {
	setPlayStartForAbs(g, abs)
}

func ageSeqDecisionsForTest(g *Game, d time.Duration) {
	if g == nil {
		return
	}
	when := time.Unix(0, 0).Add(-d)
	g.parityMu.Lock()
	defer g.parityMu.Unlock()
	for row, m := range g.paritySeqDecisions {
		for abs, dec := range m {
			dec.RecordedAt = when
			m[abs] = dec
		}
		g.paritySeqDecisions[row] = m
	}
}

// Deleting a row mid-play must not reset the scheduler's per-row counters and trip an audio_missing
// mismatch.
func TestDeleteRowDuringPlayback_DoesNotTripParity(t *testing.T) {
	assertDefaultParityState(t)
	prevFatal := parityFatalEnabled.Load()
	SetParityFatal(true)
	t.Cleanup(func() { SetParityFatal(prevFatal) })

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	g.drum.SetLength(64)

	// Ensure at least two rows exist so we can delete one.
	for len(g.drum.Rows) < 2 {
		g.drum.AddRow()
	}

	buildLoop := func(row int, x int) {
		g.pendingStartRow = row
		a := g.tryAddNode(x, 0, model.NodeTypeRegular)
		b := g.tryAddNode(x+1, 0, model.NodeTypeRegular)
		g.addEdge(a, b)
		g.addEdge(b, a)
		if row == 0 {
			g.start = a
			g.graph.StartNodeID = a.ID
		}
		g.pendingStartRow = -1
	}
	buildLoop(0, 0)
	buildLoop(1, 4)

	g.updateBeatInfos()
	g.refreshDrumRow()

	// Force the test-only direct-play path so no parity audio events are recorded.
	g.SetPlayFunc(func(instrument.ID, float64, ...float64) {})
	g.SetPlaying(true)

	// Schedule far enough ahead to populate seq decisions beyond the initial window.
	targetAbs := 200
	scheduleAbsNoHighlightForTest(g, targetAbs)
	for i := 0; i < 128; i++ {
		g.seqScheduleTime()
		if len(g.seqNextIdxs) > 0 && g.seqNextIdxs[0] > targetAbs {
			break
		}
	}
	// Ensure the grace window doesn't mask the mismatch we want to reproduce.
	ageSeqDecisionsForTest(g, time.Second)

	// Simulate "close row" while playing.
	g.drum.DeleteRow(1)

	// The sequencer sees the row-count change and resizes its counters before the UI processes the delete.
	g.seqScheduleTime()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("unexpected parity panic after deleting a row during playback: %v", r)
		}
	}()
	_ = g.Update()
	if got := len(g.ParityMismatchSnapshot()); got != 0 {
		t.Fatalf("expected no parity mismatches after row deletion, got %d", got)
	}
}

// The sequencer may see the new row count before beat paths are rebuilt; that must not raise a
// scheduler_vs_view panic.
func TestDeleteRowDuringPlayback_DoesNotTripSequencerParityWhileRowsShift(t *testing.T) {
	assertDefaultParityState(t)
	prevFatal := parityFatalEnabled.Load()
	SetParityFatal(true)
	t.Cleanup(func() { SetParityFatal(prevFatal) })

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	g.drum.SetLength(64)
	g.parityWatch = parityWatchPanic

	// Ensure at least three rows so deleting a middle row shifts indices.
	for len(g.drum.Rows) < 3 {
		g.drum.AddRow()
	}

	// Row 0: regular self-loop (audible/visible=true).
	g.pendingStartRow = 0
	r0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.pendingStartRow = -1
	g.graph.StartNodeID = r0.ID
	g.start = r0
	g.addEdge(r0, r0)

	// Row 1: regular self-loop (audible/visible=true).
	g.pendingStartRow = 1
	r1 := g.tryAddNode(4, 0, model.NodeTypeRegular)
	g.pendingStartRow = -1
	g.addEdge(r1, r1)

	// Row 2: invisible self-loop so its rendered Steps are false at idx=0.
	r2 := g.tryAddNode(8, 0, model.NodeTypeInvisible)
	g.drum.Rows[2].Origin = r2.ID
	g.drum.Rows[2].Node = r2
	g.addEdge(r2, r2)

	g.updateBeatInfos()
	g.refreshDrumRow()
	if len(g.drum.Rows[2].Steps) == 0 {
		t.Fatalf("unexpected empty drum steps for row 2")
	}
	if g.drum.Rows[2].Steps[0] {
		t.Fatalf("expected row 2 step 0 to be false for an invisible origin")
	}

	// Use the direct-play path under tests so missing-instrument parity is not a factor.
	g.SetPlayFunc(func(instrument.ID, float64, ...float64) {})
	g.SetPlaying(true)
	g.ClearParityMismatches()
	scheduleAbsNoHighlightForTest(g, 0)

	// beatInfosByRow and nextBeatIdxs keep the old mapping until Update processes it.
	g.drum.DeleteRow(1)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("unexpected parity panic while rows were shifting: %v", r)
		}
	}()
	g.seqScheduleTime()

	_ = g.Update()
	if got := len(g.ParityMismatchSnapshot()); got != 0 {
		t.Fatalf("expected no parity mismatches after row deletion, got %d", got)
	}
	// Sequencer should resume normally once Update reconciles row state.
	scheduleAbsNoHighlightForTest(g, 0)
	g.seqScheduleTime()
	if len(g.seqNextIdxs) != len(g.drum.Rows) || len(g.seqNextIdxs) == 0 || g.seqNextIdxs[0] <= 0 {
		t.Fatalf("expected sequencer to advance after row reconciliation, seqNextIdxs=%v rows=%d", g.seqNextIdxs, len(g.drum.Rows))
	}
}
