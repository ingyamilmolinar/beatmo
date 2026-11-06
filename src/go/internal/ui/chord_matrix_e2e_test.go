//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// End-to-end coverage for the chord matrix: real clicks and keys through
// Game.Update (nmClick / nmKey), like the note matrix's e2e suite.

func openChordMatrix(t *testing.T) (*Game, *uiNode) {
	t.Helper()
	g, node := chordSidebarGame(t, roster.OrganChurch)
	sidebarClick(t, g, sbAudible)   // → chord
	sidebarClick(t, g, sbPitchNote) // scrolled into view first
	if !g.sidebar.chordMatrixIsOpen() {
		t.Fatal("the chip on a chord node did not open the chord matrix")
	}
	if g.sidebar.noteMatrixIsOpen() {
		t.Fatal("the chip on a chord node opened the NOTE matrix")
	}
	return g, node
}

// clickChordCell clicks the visible cell (col,row) of the open chord matrix.
func clickChordCell(t *testing.T, g *Game, col, row int) {
	t.Helper()
	g.sidebar.chordMatrix.RevealRow(row)
	g.sidebar.layout()
	r, ok := g.sidebar.chordMatrix.cellRect(col, row)
	if !ok {
		t.Fatalf("chord cell (%d,%d) not visible", col, row)
	}
	x, y := nmCenter(r)
	nmClick(g, x, y)
}

func TestChordMatrixE2E_RegularChipStillOpensTheNoteMatrix(t *testing.T) {
	g, _ := chordSidebarGame(t, roster.OrganChurch)
	sidebarClick(t, g, sbPitchNote)
	if !g.sidebar.noteMatrixIsOpen() || g.sidebar.chordMatrixIsOpen() {
		t.Fatalf("regular node chip: note=%v chord=%v", g.sidebar.noteMatrixIsOpen(), g.sidebar.chordMatrixIsOpen())
	}
}

// Selecting applies and keeps the picker open; re-selecting confirms.
func TestChordMatrixE2E_PickAppliesThenRepickCloses(t *testing.T) {
	g, node := openChordMatrix(t)
	before, _ := g.graph.GetNodeByID(node.ID)
	clickChordCell(t, g, 1, 2) // Triads: m, row 2 = D
	n, _ := g.graph.GetNodeByID(node.ID)
	if n.Params.Chord.Quality != "m" {
		t.Fatalf("picked quality %q, want m", n.Params.Chord.Quality)
	}
	if got := ((g.nodeRootMIDI(node.ID, n.Params.Pitch) % 12) + 12) % 12; got != 2 {
		t.Fatalf("picked root pitch class %d, want D (2)", got)
	}
	if n.Params.Pitch == before.Params.Pitch && n.Params.Chord == before.Params.Chord {
		t.Fatal("the pick changed nothing")
	}
	if !g.sidebar.chordMatrixIsOpen() {
		t.Fatal("a pick must keep the picker open")
	}
	clickChordCell(t, g, 1, 2)
	if g.sidebar.chordMatrixIsOpen() {
		t.Fatal("re-picking the current chord must close the picker")
	}
}

// One undo step per pick.
func TestChordMatrixE2E_PickIsOneUndoStep(t *testing.T) {
	g, node := openChordMatrix(t)
	clickChordCell(t, g, 3, 0) // aug on C
	g.undoManager.Undo()
	n := g.nodeAt(node.I, node.J)
	if n == nil {
		t.Fatal("node gone after undo")
	}
	if c := chordOf(t, g, n.ID); c.Quality != model.ChordDefaultQuality {
		t.Fatalf("undo left %+v, want the seeded major chord back", c)
	}
}

func TestChordMatrixE2E_PageBarSwitchesPages(t *testing.T) {
	g, node := openChordMatrix(t)
	pages := g.sidebar.chordMatrix.Pages()
	if pages == nil {
		t.Fatal("chord matrix has no page bar")
	}
	x, y := nmCenter(pages.SegmentRect(1))
	nmClick(g, x, y)
	if g.sidebar.chordMatrix.Page() != 1 {
		t.Fatalf("tapping the 7ths segment left page %d", g.sidebar.chordMatrix.Page())
	}
	clickChordCell(t, g, 1, 0) // 7ths col 1 = maj7
	if c := chordOf(t, g, node.ID); c.Quality != "maj7" {
		t.Fatalf("7ths page pick = %q, want maj7", c.Quality)
	}
	nmKey(g, ebiten.KeyTab)
	if g.sidebar.chordMatrix.Page() != 2 {
		t.Fatalf("Tab moved to page %d, want 2", g.sidebar.chordMatrix.Page())
	}
}

// While open the picker owns the keyboard: arrows move its cursor and never
// pan the camera; Enter applies the cursor's chord.
func TestChordMatrixE2E_KeyboardOwnsArrowsAndEnterPicks(t *testing.T) {
	g, node := openChordMatrix(t)
	if !g.KeyboardClaimed() {
		t.Fatal("an open chord matrix must claim the keyboard")
	}
	bx, by := g.cam.OffsetX, g.cam.OffsetY
	nmKey(g, ebiten.KeyArrowRight) // maj → m on the root's row
	if g.cam.OffsetX != bx || g.cam.OffsetY != by {
		t.Fatal("arrows panned the camera while the chord matrix was open")
	}
	nmKey(g, ebiten.KeyEnter)
	if c := chordOf(t, g, node.ID); c.Quality != "m" {
		t.Fatalf("Enter applied %q, want m (cursor moved right from maj)", c.Quality)
	}
}

func TestChordMatrixE2E_OutsideTapCloses(t *testing.T) {
	g, _ := openChordMatrix(t)
	pr := g.sidebar.chordMatrix.InputBounds()
	panel := g.sidebar.rects[sbPanel]
	// A point inside the grid pane but outside both the picker and the card.
	x, y := 5, g.split.GridH(g.winH)-5
	if pr.Min.X <= x || panel.Min.X <= x {
		x = g.split.GridW(g.winW) - 5
	}
	nmClick(g, x, y)
	if g.sidebar.chordMatrixIsOpen() {
		t.Fatal("an outside tap must close the chord matrix")
	}
}

// The page bar is how three of the four pages are reached: on a short grid
// pane (the shared solver drops its band there) it must still get real
// height, inside the panel, with no cell drawn over it.
func TestChordMatrixPageBarIsNeverCoveredByCells(t *testing.T) {
	g, _ := chordSidebarGame(t, roster.OrganChurch)
	g.split.Y = 287 // the app's default grid pane height at 1280x720
	g.sidebar.layout()
	sidebarClick(t, g, sbAudible)
	sidebarClick(t, g, sbPitchNote)
	if !g.sidebar.chordMatrixIsOpen() {
		t.Fatal("chord matrix did not open")
	}
	pages := g.sidebar.chordMatrix.Pages()
	pr := pages.Rect()
	if pr.Dy() < 20 || !pr.In(g.sidebar.chordMatrix.InputBounds()) {
		t.Fatalf("page bar rect %v (panel %v): too short or outside the panel", pr, g.sidebar.chordMatrix.InputBounds())
	}
	for _, c := range g.sidebar.chordMatrix.visibleCells() {
		if c.Rect.Overlaps(pr) {
			t.Fatalf("cell (%d,%d) %v covers the page bar %v", c.Col, c.Row, c.Rect, pr)
		}
	}
}

// The keyboard cursor never stands in for the current chord on a page that
// does not hold it. It used to keep its (col, row) across a page switch, so a
// Gmaj7 node opened on 7ths and flipped to Triads showed "Gm" wearing the
// focus tint — read as the node's chord. Now a page switch (tap or Tab) puts
// the cursor on the current chord when that page holds it, and clears it
// otherwise.
func TestChordMatrixE2E_PageSwitchReseatsTheKeyCursor(t *testing.T) {
	g, node := openChordMatrix(t) // a fresh chord node: maj, on Triads
	cur := chordOf(t, g, node.ID)
	if model.ChordPageOf(cur.Quality) != g.sidebar.chordMatrix.Page() {
		t.Fatalf("fixture: matrix should open on the current chord's page")
	}
	wantOnCurrent := func(when string) {
		t.Helper()
		col, row, ok := g.sidebar.chordMatrix.Model().Selected()
		if !ok {
			t.Fatalf("%s: current chord not on this page", when)
		}
		if got, want := g.sidebar.chordMatrixKeyCursorID(), sbChordCell(col, row); got != want {
			t.Fatalf("%s: key cursor on %q, want the current chord %q", when, got, want)
		}
	}
	wantOnCurrent("on open")
	nmKey(g, ebiten.KeyArrowRight) // move it off the current chord first
	pages := g.sidebar.chordMatrix.Pages()
	x, y := nmCenter(pages.SegmentRect(1)) // tap → 7ths (no maj triad there)
	nmClick(g, x, y)
	if id := g.sidebar.chordMatrixKeyCursorID(); id != (sbID{}) {
		t.Fatalf("after tapping to a page without the current chord the cursor sits on %q", id)
	}
	nmKey(g, ebiten.KeyTab) // 7ths → Ext
	nmKey(g, ebiten.KeyTab) // Ext → Sus
	nmKey(g, ebiten.KeyTab) // Sus → Triads (wraps): the current chord again
	if g.sidebar.chordMatrix.Page() != 0 {
		t.Fatalf("Tab ×3 from 7ths left page %d, want 0", g.sidebar.chordMatrix.Page())
	}
	wantOnCurrent("after Tab back to the current chord's page")
}
