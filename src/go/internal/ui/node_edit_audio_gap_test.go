package ui

import (
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// TestAddNodeDuringPlaybackDoesNotDropScheduledBeatOnUnchangedRow reproduces the
// "small gap of silence while placing a node" artifact.
//
// A structural graph edit runs through (*Game).updateBeatInfos, which bumps a
// SINGLE GLOBAL generation counter (g.audioGen). The audioLoop drop-filter
// (game_audio_loop.go) discards any already-scheduled sequencer beat whose
// stamped generation != the current global generation. Because the counter is
// global, editing ONE row invalidates in-flight (lookahead-scheduled, not yet
// played) beats on EVERY row — including rows whose path did not change. Those
// unchanged rows are not rewound/rescheduled (only changed rows get the
// seqNextIdxs rewind), so their dropped beat simply never plays → audible gap.
//
// The invariant: placing a node must only affect the edited node's trigger; an
// already-scheduled beat for an UNCHANGED row must still play.
//
// Determinism: the eager audioLoop goroutine dispatches on enqueue, so we can't
// interleave "enqueue then bump" through live scheduling. Instead we bump the
// generation first (via a real add-node), then enqueue a beat stamped with the
// pre-edit generation (exactly what a lookahead beat scheduled just before the
// edit would carry), then enqueue a generation-exempt fire-and-forget sentinel
// (row < 0) after it. When the sentinel plays, the audioLoop has finished
// deciding the earlier beat, so the assertion is race-free and deterministic.
func TestAddNodeDuringPlaybackDoesNotDropScheduledBeatOnUnchangedRow(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// Row 0: a real circuit we will edit.
	start := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.start = start
	g.graph.StartNodeID = start.ID
	mid := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(start, mid)
	g.addEdge(mid, start)
	g.drum.Rows[0].Origin = start.ID
	g.drum.Rows[0].Node = start
	for i := range g.drum.Rows[0].Steps {
		g.drum.Rows[0].Steps[i] = true
	}

	// Row 1: a second, independent row whose path we will NOT touch.
	g.drum.AddRow()
	if len(g.drum.Rows) < 2 {
		t.Fatalf("expected a second row, got %d", len(g.drum.Rows))
	}
	g.updateBeatInfos()

	g.SetPlaying(true)

	var mu sync.Mutex
	plays := map[string]int{}
	g.SetPlayFunc(func(id instrument.ID, _ float64, _ ...float64) {
		mu.Lock()
		plays[string(id)]++
		mu.Unlock()
	})

	unchangedInst := g.drum.Rows[1].Instrument
	const sentinelID = "gap-test-sentinel"

	// The generation an already-scheduled (in-flight) beat would carry if it was
	// enqueued just before the edit.
	preGen := g.audioGen.Load()

	// Perform a real structural edit on row 0 (insert a node onto its path). This
	// bumps the generation counter that the audioLoop drop-filter reads.
	extra := g.tryAddNode(2, 0, model.NodeTypeRegular)
	g.deleteEdgeNoRefresh(start, mid)
	g.addEdgeNoRefresh(start, extra)
	g.addEdgeNoRefresh(extra, mid)
	g.updateBeatInfos()

	if g.rowsPathChanged != nil && len(g.rowsPathChanged) > 1 && g.rowsPathChanged[1] {
		t.Fatalf("row 1 path unexpectedly marked changed by a row-0 edit")
	}

	// A lookahead beat for the UNCHANGED row 1, scheduled just before the edit
	// (carries preGen), now sitting in the audio channel.
	inflight := soundReq{
		id:    unchangedInst,
		vol:   1.0,
		pitch: 0,
		dur:   1,
		row:   1,
		abs:   0,
		gen:   preGen,
		enqAt: time.Now(),
	}
	sendLatest(g.audioCh, inflight, nil)

	// Generation-exempt sentinel (row < 0) enqueued AFTER the in-flight beat.
	// When it plays, the audioLoop has already decided the in-flight beat.
	sentinel := soundReq{id: sentinelID, vol: 1.0, dur: 1, row: -1, abs: -1, gen: g.audioGen.Load(), enqAt: time.Now()}
	sendLatest(g.audioCh, sentinel, nil)

	// Wait deterministically for the audioLoop to process through the sentinel.
	seen := false
	for i := 0; i < 200000 && !seen; i++ {
		mu.Lock()
		seen = plays[sentinelID] > 0
		mu.Unlock()
		if !seen {
			runtime.Gosched()
		}
	}
	if !seen {
		t.Fatalf("sentinel never dispatched; audioLoop did not drain")
	}

	mu.Lock()
	got := plays[string(unchangedInst)]
	mu.Unlock()
	if got == 0 {
		t.Fatalf("placing a node dropped an in-flight scheduled beat on an UNCHANGED row: "+
			"unchanged-row plays=%d (want >=1). The global audioGen bump on a structural "+
			"edit invalidated a beat for a row whose path did not change → audible gap.", got)
	}
}
