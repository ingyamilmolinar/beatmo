package ui

import (
	"bytes"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Playback history (the played snapshot, timeline commits, the playhead, solo)
// is per-session runtime state: it is ephemeral and must never reach the
// project JSON, in either direction. Exporting the same document before and
// after a session has played, soloed and accumulated history must produce the
// same bytes, and importing a file must start with no history at all.
func TestExportImportCarryNoPlaybackHistory(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.SetPlayFunc(func(instrument.ID, float64, ...float64) {})
	g.Layout(1024, 720)

	ns := []*uiNode{
		g.tryAddNode(0, 0, model.NodeTypeRegular),
		g.tryAddNode(2, 0, model.NodeTypeRegular),
		g.tryAddNode(2, 1, model.NodeTypeRegular),
		g.tryAddNode(0, 1, model.NodeTypeRegular),
	}
	for k := range ns {
		g.addEdge(ns[k], ns[(k+1)%len(ns)])
	}
	g.start = ns[0]
	g.graph.StartNodeID = ns[0].ID
	g.drum.SetLength(64)
	g.drum.SetBPM(120)
	g.SetAppliedBPMForTest(120)
	g.updateBeatInfos()
	g.refreshDrumRow()

	fresh, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export before playback: %v", err)
	}

	// A session: play, solo, keep playing, pause — accumulating history.
	g.SetPlaying(true)
	for abs := 0; abs < 120; abs++ {
		setPlayStartForAbs(g, abs)
		g.Update()
		if abs == 60 {
			g.drum.toggleSolo(0)
		}
	}
	if g.timelineService().PlayedMax(0) < 100 {
		t.Fatalf("setup: no played history accumulated (PlayedMax=%d)", g.timelineService().PlayedMax(0))
	}
	g.drum.toggleSolo(0) // solo is session state too; restore the audible set

	afterSession, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export after playback: %v", err)
	}
	if !bytes.Equal(fresh, afterSession) {
		t.Fatalf("export changed after a playback session — runtime state leaked into the project JSON:\nbefore=%s\nafter=%s", fresh, afterSession)
	}

	// Import into a stopped game: the file brings no history with it.
	h := New(testLogger)
	t.Cleanup(h.CloseForTest)
	h.Layout(1024, 720)
	if err := h.Import(afterSession); err != nil {
		t.Fatalf("import: %v", err)
	}
	for row := range h.drum.Rows {
		if m := h.timelineService().PlayedMax(row); m != -1 {
			t.Fatalf("row %d has played history right after import (PlayedMax=%d)", row, m)
		}
		if last := h.timelineService().LastCommitted(row); last != -1 {
			t.Fatalf("row %d has timeline commits right after import (last=%d)", row, last)
		}
	}
}
