package ui

import (
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// One row's edit must not drop an already-scheduled beat on an unchanged row. Deterministic: edit first, then
// enqueue a beat stamped with the pre-edit generation and a generation-exempt sentinel; once the sentinel
// plays, the beat has been decided.
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

	preGen := g.audioGen.Load()

	// Bumps the generation the audioLoop drop-filter reads.
	extra := g.tryAddNode(2, 0, model.NodeTypeRegular)
	g.deleteEdgeNoRefresh(start, mid)
	g.addEdgeNoRefresh(start, extra)
	g.addEdgeNoRefresh(extra, mid)
	g.updateBeatInfos()

	if g.rowsPathChanged != nil && len(g.rowsPathChanged) > 1 && g.rowsPathChanged[1] {
		t.Fatalf("row 1 path unexpectedly marked changed by a row-0 edit")
	}

	// Scheduled just before the edit, for the unchanged row 1.
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

	// Exempt (row < 0) and after the in-flight beat.
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
