//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Real clicks through Game.Update, never direct HandleInput calls: unit tests passed while a press on the
// floating panel never reached the sidebar at all. clickAt (press + release over two frames) is the only path
// that proves the wiring.

// Sidebar buttons dispatch through g.enqueueUI, which runs on the NEXT Update; stopping at the release frame
// would never see the effect.
func nmClick(g *Game, x, y int) {
	clickAt(g, x, y)
	_ = g.Update()
}

// nmCenter is the midpoint of a rect.
func nmCenter(r image.Rectangle) (int, int) {
	return r.Min.X + r.Dx()/2, r.Min.Y + r.Dy()/2
}

func matrixTestGame(t *testing.T) (*Game, *uiNode) {
	t.Helper()
	g := newTestGameForUndo(t)
	g.Layout(1280, 720)
	node := noteTestNode(t, g)
	g.drum.Rows[0].Instrument = roster.OrganChurch

	g.sidebar.Open(node)
	g.sidebar.ExpandAllSections()
	g.sidebar.layout()
	return g, node
}

// Clicking the note chip must open the matrix.
func TestNoteMatrixE2E_ChipClickOpensPicker(t *testing.T) {
	g, _ := matrixTestGame(t)

	chip := g.sidebar.rects[sbPitchNote]
	if chip.Empty() {
		t.Fatal("no note chip rect")
	}
	cx, cy := nmCenter(chip)
	nmClick(g, cx, cy)

	if !g.sidebar.noteMatrixIsOpen() {
		t.Fatal("clicking the note chip did not open the note matrix")
	}
}

// The click must reach the sidebar through NodeSidebar.Hit, which must know the matrix panel.
func TestNoteMatrixE2E_ClickingACellSetsThePitch(t *testing.T) {
	g, node := matrixTestGame(t)

	cx, cy := nmCenter(g.sidebar.rects[sbPitchNote])
	nmClick(g, cx, cy)
	if !g.sidebar.noteMatrixIsOpen() {
		t.Fatal("picker did not open")
	}

	m := g.sidebar.noteMatrix
	// Pick a target two octaves up from the current note.
	const want = 12.0
	col, row, ok := m.Model().CellFor(want)
	if !ok {
		t.Fatalf("pitch %v has no cell", want)
	}
	cell, visible := m.cellRect(col, row)
	if !visible {
		t.Fatalf("cell for pitch %v is not in the visible row window", want)
	}

	ccx, ccy := nmCenter(cell)
	nmClick(g, ccx, ccy)

	mn, ok := g.graph.GetNodeByID(node.ID)
	if !ok {
		t.Fatal("node vanished")
	}
	if mn.Params.Pitch != want {
		t.Errorf("clicking the cell for pitch %v left the node at %v — "+
			"the click never reached the picker", want, mn.Params.Pitch)
	}
	// Selecting applies and keeps the picker open; re-selecting the same note confirms.
	if !g.sidebar.noteMatrixIsOpen() {
		t.Error("picker closed on a single pick; it must stay open for the next audition")
	}
}

// Three views of one value.
func TestNoteMatrixE2E_ChipLabelFollowsThePick(t *testing.T) {
	g, node := matrixTestGame(t)

	cx, cy := nmCenter(g.sidebar.rects[sbPitchNote])
	nmClick(g, cx, cy)

	m := g.sidebar.noteMatrix
	const want = 7.0
	col, row, ok := m.Model().CellFor(want)
	if !ok {
		t.Fatalf("pitch %v has no cell", want)
	}
	cell, visible := m.cellRect(col, row)
	if !visible {
		t.Skip("target cell scrolled out of view")
	}
	wantLabel := m.Model().NoteLabelAt(col, row)

	ccx, ccy := nmCenter(cell)
	nmClick(g, ccx, ccy)

	g.sidebar.layout()
	btn := g.sidebar.btns[sbPitchNote]
	if btn == nil {
		t.Fatal("note chip button vanished after the pick")
	}
	if btn.Text != wantLabel {
		t.Errorf("chip reads %q after picking %q", btn.Text, wantLabel)
	}
	if got := g.nodeNoteLabel(node.ID, want); got != wantLabel {
		t.Errorf("nodeNoteLabel says %q but the matrix cell said %q", got, wantLabel)
	}
}

func TestNoteMatrixE2E_ChipClickTogglesClosed(t *testing.T) {
	g, _ := matrixTestGame(t)
	cx, cy := nmCenter(g.sidebar.rects[sbPitchNote])

	nmClick(g, cx, cy)
	if !g.sidebar.noteMatrixIsOpen() {
		t.Fatal("first chip click did not open the picker")
	}
	nmClick(g, cx, cy)
	if g.sidebar.noteMatrixIsOpen() {
		t.Error("second chip click did not close the picker")
	}
}

// It must not change the pitch, dismiss the picker, or fall through to the grid and close the sidebar.
func TestNoteMatrixE2E_DimmedCellIsInert(t *testing.T) {
	g, node := matrixTestGame(t)
	cx, cy := nmCenter(g.sidebar.rects[sbPitchNote])
	nmClick(g, cx, cy)
	if !g.sidebar.noteMatrixIsOpen() {
		t.Fatal("picker did not open")
	}

	before, _ := g.graph.GetNodeByID(node.ID)

	var target image.Rectangle
	for _, c := range g.sidebar.noteMatrix.VisibleCells() {
		if !c.Selectable {
			target = c.Rect
			break
		}
	}
	if target.Empty() {
		t.Skip("no dimmed cell visible at this layout")
	}
	dx, dy := nmCenter(target)
	nmClick(g, dx, dy)

	after, _ := g.graph.GetNodeByID(node.ID)
	if after.Params.Pitch != before.Params.Pitch {
		t.Errorf("a dimmed cell changed the pitch %v → %v", before.Params.Pitch, after.Params.Pitch)
	}
	if !g.sidebar.IsOpen() {
		t.Error("a dimmed cell click closed the node sidebar — it fell through to the grid")
	}
	if !g.sidebar.noteMatrixIsOpen() {
		t.Error("a dimmed cell click dismissed the picker")
	}
}

// ─── Keyboard + wheel, end to end ──────────────────────────────────────────

// The edge shape MenuKeyNav keys off.
func nmKey(g *Game, k ebiten.Key) {
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(kk ebiten.Key) bool { return kk == k },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return g.winW, g.winH },
	)
	_ = g.Update()
	restore()
	_ = g.Update()
}

// nmWheel sends a wheel event with the cursor at (x, y).
func nmWheel(g *Game, x, y int, steps float64) {
	restore := SetInputForTest(
		func() (int, int) { return x, y },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, steps },
		func() (int, int) { return g.winW, g.winH },
	)
	_ = g.Update()
	restore()
}

func openMatrixForKeys(t *testing.T) (*Game, *uiNode) {
	t.Helper()
	g, node := matrixTestGame(t)
	cx, cy := nmCenter(g.sidebar.rects[sbPitchNote])
	nmClick(g, cx, cy)
	if !g.sidebar.noteMatrixIsOpen() {
		t.Fatal("picker did not open")
	}
	return g, node
}

// Grid mode: Down moves a whole row, Right one cell.
func TestNoteMatrixE2E_ArrowKeysMoveTheCursor(t *testing.T) {
	g, _ := openMatrixForKeys(t)

	nmKey(g, ebiten.KeyArrowDown)
	first := g.sidebar.noteMatrixKeyNav.Selected()
	if first == MenuKeyNavNoSelection {
		t.Fatal("ArrowDown did not place a cursor in the matrix")
	}

	cols := noteMatrixNavHost{sb: g.sidebar}.MenuGridCols()
	if cols <= 1 {
		t.Fatal("matrix reported no grid columns; MenuKeyNav would treat it as a list")
	}

	nmKey(g, ebiten.KeyArrowDown)
	second := g.sidebar.noteMatrixKeyNav.Selected()
	if second == first {
		t.Error("a second ArrowDown did not move the cursor")
	}
	if (second-first)%cols != 0 {
		t.Errorf("ArrowDown moved by %d, which is not a whole row of %d columns", second-first, cols)
	}

	before := g.sidebar.noteMatrixKeyNav.Selected()
	nmKey(g, ebiten.KeyArrowRight)
	if after := g.sidebar.noteMatrixKeyNav.Selected(); after == before {
		t.Error("ArrowRight did not move the cursor")
	}
}

// Enter must commit the highlighted cell — the same mutation a click makes.
func TestNoteMatrixE2E_EnterPicksTheHighlightedNote(t *testing.T) {
	g, node := openMatrixForKeys(t)

	nmKey(g, ebiten.KeyArrowDown)
	nmKey(g, ebiten.KeyArrowRight)
	sel := g.sidebar.noteMatrixKeyNav.Selected()
	if sel == MenuKeyNavNoSelection {
		t.Fatal("no cursor after arrow keys")
	}
	h := noteMatrixNavHost{sb: g.sidebar}
	col, row, ok := h.cellAt(sel)
	if !ok {
		t.Fatal("cursor maps to no cell")
	}
	want, selectable := g.sidebar.noteMatrix.Model().PitchAt(col, row)
	if !selectable {
		t.Fatal("cursor sits on an unselectable cell")
	}

	nmKey(g, ebiten.KeyEnter)

	mn, _ := g.graph.GetNodeByID(node.ID)
	if mn.Params.Pitch != want {
		t.Errorf("Enter left the pitch at %v, want %v", mn.Params.Pitch, want)
	}
	if !g.sidebar.noteMatrixIsOpen() {
		t.Error("picker closed on the first Enter; a second Enter on the same note confirms")
	}
}

// The cursor must never land on an out-of-range cell.
func TestNoteMatrixE2E_CursorSkipsUnselectableCells(t *testing.T) {
	g, _ := openMatrixForKeys(t)
	h := noteMatrixNavHost{sb: g.sidebar}

	for i := 0; i < 20; i++ {
		nmKey(g, ebiten.KeyArrowDown)
		sel := g.sidebar.noteMatrixKeyNav.Selected()
		if sel == MenuKeyNavNoSelection {
			continue
		}
		if !h.MenuItemSelectable(sel) {
			t.Fatalf("cursor landed on unselectable cell %d", sel)
		}
	}
}

// The picker scrolls in ROWS while the cursor counts CELLS, which is why it implements MenuKeyNavRevealer.
func TestNoteMatrixE2E_CursorScrollsIntoView(t *testing.T) {
	g, _ := openMatrixForKeys(t)
	if !g.sidebar.noteMatrix.HasScroll() {
		t.Skip("matrix fits without scrolling at this size")
	}
	h := noteMatrixNavHost{sb: g.sidebar}

	for i := 0; i < 14; i++ {
		nmKey(g, ebiten.KeyArrowDown)
	}
	sel := g.sidebar.noteMatrixKeyNav.Selected()
	if sel == MenuKeyNavNoSelection {
		t.Fatal("no cursor")
	}
	col, row, ok := h.cellAt(sel)
	if !ok {
		t.Fatal("cursor maps to no cell")
	}
	if _, visible := g.sidebar.noteMatrix.cellRect(col, row); !visible {
		t.Error("cursor moved off the visible page without scrolling it into view")
	}
}

// The wheel over the picker must scroll ITS rows, not the sidebar behind it.
func TestNoteMatrixE2E_WheelOverPickerScrollsRows(t *testing.T) {
	g, _ := openMatrixForKeys(t)
	m := g.sidebar.noteMatrix
	if !m.HasScroll() {
		t.Skip("matrix fits without scrolling at this size")
	}
	before := m.scroll.VS.First
	sidebarBefore := g.sidebar.scroll.VS.First

	cx, cy := nmCenter(m.CellsRect())
	nmWheel(g, cx, cy, -3)

	if m.scroll.VS.First == before {
		t.Error("wheel over the picker did not scroll its rows")
	}
	if g.sidebar.scroll.VS.First != sidebarBefore {
		t.Error("wheel over the picker also scrolled the sidebar behind it")
	}
}

func TestNoteMatrixE2E_WheelOutsidePickerStillScrollsSidebar(t *testing.T) {
	g, _ := openMatrixForKeys(t)
	panel := g.sidebar.rects[sbPanel]
	pick := g.sidebar.noteMatrix.InputBounds()

	// A point in the sidebar card that the picker does not cover.
	px, py := panel.Min.X+4, panel.Max.Y-4
	if image.Pt(px, py).In(pick) {
		t.Skip("picker covers the sampled sidebar point at this layout")
	}
	if !g.sidebar.scroll.HasScroll() {
		t.Skip("sidebar does not scroll at this size")
	}
	before := g.sidebar.scroll.VS.First
	nmWheel(g, px, py, -3)
	if g.sidebar.scroll.VS.First == before {
		t.Error("wheel outside the picker no longer scrolls the sidebar")
	}
}
