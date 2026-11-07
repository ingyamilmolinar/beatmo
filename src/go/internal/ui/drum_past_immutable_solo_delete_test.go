package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// A played cell renders from the played snapshot, so no edit (on any row, audible or solo-gated, removing a row
// or not) rewrites it. Real Game.Update frames; rows compared by identity, so a row that moves index is checked
// against its own past.
func TestDrumView_PastImmutableAfterSoloThenNodeDelete(t *testing.T) {
	deletes := []struct {
		row  int
		kind string
	}{
		{0, "straight"}, {0, "corner"},
		{1, "straight"}, {1, "corner"},
		{1, "origin"}, // removes row 1: row 2 slides into its index
		{1, "straight+undo"},
		{1, "origin+undo"}, // undo re-inserts the deleted row
	}
	for _, follow := range []bool{false, true} {
		for _, run := range []int{40, 400} {
			if !follow && run > 40 {
				continue // a stationary 64-cell window cannot hold a long run
			}
			for _, soloRow := range []int{0, 1, 2} {
				for _, d := range deletes {
					name := fmt.Sprintf("follow=%v_run=%d_solo%d_delete%d_%s", follow, run, soloRow, d.row, d.kind)
					t.Run(name, func(t *testing.T) {
						runPastImmutableSoloDelete(t, follow, run, soloRow, d.row, d.kind)
					})
				}
			}
		}
	}
}

func runPastImmutableSoloDelete(t *testing.T, follow bool, run, soloRow, deleteRow int, kind string) {
	t.Helper()
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.SetPlayFunc(func(instrument.ID, float64, ...float64) {})
	g.Layout(1024, 720)

	// Deleting x2 reconnects x1→x3 (a shifted pattern), the corner x4 breaks the loop, the origin x0 deletes the row.
	loop := func(i0, j0 int, rect [][2]int) []*uiNode {
		ns := make([]*uiNode, len(rect))
		for k, p := range rect {
			ns[k] = g.tryAddNode(i0+p[0], j0+p[1], model.NodeTypeRegular)
		}
		for k := range ns {
			g.addEdge(ns[k], ns[(k+1)%len(ns)])
		}
		return ns
	}
	a := loop(0, 0, [][2]int{{0, 0}, {1, 0}, {2, 0}, {3, 0}, {3, 1}, {0, 1}})
	b := loop(6, 0, [][2]int{{0, 0}, {1, 0}, {2, 0}, {3, 0}, {3, 2}, {0, 2}})
	c := loop(12, 0, [][2]int{{0, 0}, {1, 0}, {2, 0}, {3, 0}, {3, 3}, {0, 3}})
	g.start = a[0]
	g.graph.StartNodeID = a[0].ID
	for _, spec := range []struct {
		inst   instrument.ID
		origin *uiNode
	}{{roster.Snare, b[0]}, {roster.Hihat, c[0]}} {
		g.drum.AddRow()
		g.drum.SetInstrument(spec.inst)
		r := g.drum.Rows[len(g.drum.Rows)-1]
		r.Origin = spec.origin.ID
		r.Node = g.nodeByID(spec.origin.ID)
	}

	g.drum.SetLength(64)
	g.drum.SetFollow(follow)
	g.drum.SetBPM(120)
	g.SetAppliedBPMForTest(120)
	g.updateBeatInfos()
	g.drum.Offset = 0
	g.refreshDrumRow()

	// The node delete is the only undoable step.
	if g.undoManager != nil {
		g.undoManager.OnExternalLoad()
	}

	g.SetPlaying(true)
	frame := func(abs int) {
		setPlayStartForAbs(g, abs)
		g.Update()
	}

	abs := 0
	for ; abs < run/2; abs++ {
		frame(abs)
	}
	g.drum.toggleSolo(soloRow)
	for ; abs < run; abs++ {
		frame(abs)
	}

	playhead := g.elapsedBeats
	if playhead < run-10 {
		t.Fatalf("playhead did not advance: elapsedBeats=%d", playhead)
	}
	if !follow && g.drum.Offset != 0 {
		t.Fatalf("window moved: offset=%d (test assumes a stationary window)", g.drum.Offset)
	}

	type cell struct {
		step bool
		typ  model.NodeType
	}
	// row identity -> abs -> cell, for every visible cell left of the playhead.
	snapshot := func() map[*DrumRow]map[int]cell {
		out := map[*DrumRow]map[int]cell{}
		off := g.drum.Offset
		for _, row := range g.drum.Rows {
			m := map[int]cell{}
			for i := range row.Steps {
				if a := off + i; a < playhead {
					m[a] = cell{row.Steps[i], row.CellTypes[i]}
				}
			}
			out[row] = m
		}
		return out
	}
	rowsBefore := append([]*DrumRow(nil), g.drum.Rows...)
	before := snapshot()
	if len(before[rowsBefore[deleteRow]]) < 8 {
		t.Fatalf("too few visible past cells to compare: %d", len(before[rowsBefore[deleteRow]]))
	}

	var victim *uiNode
	loops := [][]*uiNode{a, b}
	delKind, undo := strings.CutSuffix(kind, "+undo")
	switch delKind {
	case "straight":
		victim = loops[deleteRow][2]
	case "corner":
		victim = loops[deleteRow][4]
	case "origin":
		victim = loops[deleteRow][0]
	}
	replayFrom := playhead + 1<<30 // no replayed stretch unless an undo re-seeks
	preDeleteRows := append([]*DrumRow(nil), g.drum.Rows...)
	preDelete, preDeletePlayhead := before, playhead
	g.deleteNode(victim)
	if undo {
		// Undo restores the document and must neither wipe history nor re-derive the edited stretch from the restored
		// circuit.
		for k := 0; k < 16; k++ {
			frame(abs + k)
		}
		abs += 16
		playhead = g.elapsedBeats
		rowsBefore = append(rowsBefore[:0], g.drum.Rows...)
		before = snapshot()
		g.performUndo()
		if g.nodeByID(victim.ID) == nil {
			t.Fatalf("undo did not restore the deleted node")
		}
		// Undo re-seeks to the beat start, so [replayFrom, playhead) genuinely replays and re-records.
		replayFrom = g.elapsedBeats
		if replayFrom < playhead-g.grid.MaxDiv() {
			t.Fatalf("undo seek target %d rewinds more than a beat before playhead %d", replayFrom, playhead)
		}
	}

	wantRows := 3
	if delKind == "origin" && !undo {
		wantRows = 2
	}
	if len(g.drum.Rows) != wantRows {
		t.Fatalf("rows after delete = %d, want %d", len(g.drum.Rows), wantRows)
	}

	// The edit must disagree with the rendered past somewhere, or a rewrite would be invisible.
	disagree := 0
	if undo && delKind == "origin" {
		// Rows below the re-inserted one move index and keep their own past.
		for r, row := range g.drum.Rows {
			if old := rowsBeforeByOrigin(rowsBefore, row.Origin); old != nil && (r >= len(rowsBefore) || rowsBefore[r] != old) {
				disagree++
			}
		}
	} else if undo {
		// The restored circuit must disagree with what played under the edit.
		for r, row := range g.drum.Rows {
			for a, c := range before[rowsBeforeByOrigin(rowsBefore, row.Origin)] {
				if g.beatInfoAtRow(r, a).NodeType != c.typ {
					disagree++
				}
			}
		}
	} else if delKind == "origin" {
		moved := rowsBefore[deleteRow+1]
		for a, c := range before[moved] {
			if before[rowsBefore[deleteRow]][a] != c {
				disagree++
			}
		}
	} else {
		for a, c := range before[rowsBefore[deleteRow]] {
			if g.beatInfoAtRow(deleteRow, a).NodeType != c.typ {
				disagree++
			}
		}
	}
	if disagree == 0 {
		t.Fatalf("the delete cannot change any past cell of the observed row; test is vacuous")
	}

	// A few more frames at (and just after) the same playhead.
	frame(abs - 1)
	frame(abs - 1)
	frame(abs)
	frame(abs + 1)

	after := snapshot()
	// Mapped by origin node, which a restore preserves; a row the undo re-creates has no "before".
	for r, row := range g.drum.Rows {
		old := row
		if undo {
			if old = rowsBeforeByOrigin(rowsBefore, row.Origin); old == nil {
				continue
			}
		}
		for a, want := range before[old] {
			if a >= replayFrom {
				continue
			}
			// Every played cell is frozen exactly as it showed, and that renders.
			if pv, pt, ok := g.timelineService().Played(r, a); !ok || pv != want.step || pt != want.typ {
				t.Errorf("row %d (%s) PAST abs=%d played snapshot=(%v,%v,%v), want the cell as it played %+v",
					r, row.Instrument, a, pv, pt, ok, want)
			}
			got, ok := after[row][a]
			if !ok {
				continue
			}
			if got != want {
				t.Errorf("row %d (%s) PAST cell abs=%d changed: before=%+v after=%+v (playhead=%d off=%d)",
					r, row.Instrument, a, want, got, playhead, g.drum.Offset)
			}
		}
	}
	if undo && delKind == "origin" {
		// Exactly the past it played before its delete.
		deleted := preDeleteRows[deleteRow]
		r := -1
		for i, row := range g.drum.Rows {
			if row.Origin == deleted.Origin {
				r = i
			}
		}
		if r < 0 {
			t.Fatalf("undo did not re-create the deleted row")
		}
		n := 0
		for a, want := range preDelete[deleted] {
			if a >= preDeletePlayhead {
				continue
			}
			n++
			if v, typ, ok := g.timelineService().Played(r, a); !ok || v != want.step || typ != want.typ {
				t.Errorf("re-created row %d PAST abs=%d played=(%v,%v,%v), want its pre-delete %+v", r, a, v, typ, ok, want)
			}
		}
		if n == 0 {
			t.Fatalf("no pre-delete past to compare for the re-created row")
		}
	}
}

func rowsBeforeByOrigin(rows []*DrumRow, origin model.NodeID) *DrumRow {
	for _, r := range rows {
		if r.Origin == origin {
			return r
		}
	}
	return nil
}
