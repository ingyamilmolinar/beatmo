//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// The open picker OWNS the arrow keys: if the grid's pan also ran, one press would move the cursor and pan the
// camera. It declares itself through KeyboardClaimant.

func TestNoteMatrix_ArrowKeysDoNotPanTheGrid(t *testing.T) {
	g, _ := openMatrixForKeys(t)

	beforeX, beforeY := g.cam.OffsetX, g.cam.OffsetY
	for _, k := range []ebiten.Key{
		ebiten.KeyArrowDown, ebiten.KeyArrowUp, ebiten.KeyArrowLeft, ebiten.KeyArrowRight,
	} {
		nmKey(g, k)
	}
	if g.cam.OffsetX != beforeX || g.cam.OffsetY != beforeY {
		t.Errorf("arrows panned the camera while the note matrix was open: "+
			"(%v,%v) → (%v,%v); the picker must own the keyboard",
			beforeX, beforeY, g.cam.OffsetX, g.cam.OffsetY)
	}
}

// So the guard above cannot pass by breaking arrow-pan outright.
func TestNoteMatrix_ArrowKeysStillPanWhenPickerClosed(t *testing.T) {
	g, _ := matrixTestGame(t)
	g.sidebar.Close()

	beforeX := g.cam.OffsetX
	nmKey(g, ebiten.KeyArrowLeft)
	if g.cam.OffsetX == beforeX {
		t.Error("arrow-pan is dead with no picker open — the guard is too broad")
	}
}

// The predicate the grid consults: every gated grid shortcut yields while the picker is open.
func TestNoteMatrix_ClaimsKeyboardWhileOpen(t *testing.T) {
	g, _ := matrixTestGame(t)
	if g.KeyboardClaimed() {
		t.Fatal("keyboard is claimed before the picker opens")
	}

	cx, cy := nmCenter(g.sidebar.rects[sbPitchNote])
	nmClick(g, cx, cy)
	if !g.sidebar.noteMatrixIsOpen() {
		t.Fatal("picker did not open")
	}
	if !g.KeyboardClaimed() {
		t.Error("an open note matrix does not claim the keyboard, so the grid still handles arrows")
	}

	g.sidebar.closeNoteMatrix()
	if g.KeyboardClaimed() {
		t.Error("keyboard is still claimed after the picker closed")
	}
}

// ─── 2. Cursor starts on the current note ──────────────────────────────────

// The first arrow then moves RELATIVE to the current note, not from a corner.
func TestNoteMatrix_CursorStartsOnCurrentNote(t *testing.T) {
	for _, start := range []float64{0, -12, 7} {
		g, node := matrixTestGame(t)

		mn, _ := g.graph.GetNodeByID(node.ID)
		p := mn.Params
		p.Pitch = start
		g.graph.SetNodeParams(node.ID, p)
		g.sidebar.layout()

		cx, cy := nmCenter(g.sidebar.rects[sbPitchNote])
		nmClick(g, cx, cy)
		if !g.sidebar.noteMatrixIsOpen() {
			t.Fatalf("pitch %v: picker did not open", start)
		}

		sel := g.sidebar.noteMatrixNav().Selected()
		if sel == MenuKeyNavNoSelection {
			t.Errorf("pitch %v: picker opened with no cursor; it must start on the current note", start)
			continue
		}
		h := noteMatrixNavHost{sb: g.sidebar}
		col, row, ok := h.cellAt(sel)
		if !ok {
			t.Errorf("pitch %v: cursor maps to no cell", start)
			continue
		}
		got, selectable := g.sidebar.noteMatrix.Model().PitchAt(col, row)
		if !selectable || got != start {
			t.Errorf("pitch %v: cursor started on pitch %v, want the current note", start, got)
		}
	}
}

func TestNoteMatrix_EnterOnOpenKeepsThePitch(t *testing.T) {
	g, node := matrixTestGame(t)
	mn, _ := g.graph.GetNodeByID(node.ID)
	p := mn.Params
	p.Pitch = 5
	g.graph.SetNodeParams(node.ID, p)
	g.sidebar.layout()

	cx, cy := nmCenter(g.sidebar.rects[sbPitchNote])
	nmClick(g, cx, cy)
	nmKey(g, ebiten.KeyEnter)

	after, _ := g.graph.GetNodeByID(node.ID)
	if after.Params.Pitch != 5 {
		t.Errorf("Enter straight after opening changed the pitch 5 → %v", after.Params.Pitch)
	}
}

// ─── 3. Highlight fills the cell ───────────────────────────────────────────

// A PIXEL read, not a draw-call count: a highlight painted under the opaque keycap makes the call and changes
// no pixel.
func cellCenterPixel(t *testing.T, g *Game, r image.Rectangle) color.RGBA {
	t.Helper()
	img := ebiten.NewImage(g.winW, g.winH)
	g.sidebar.Draw(img)
	cx, cy := r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2
	rr, gg, bb, aa := img.At(cx, cy).RGBA()
	return color.RGBA{uint8(rr >> 8), uint8(gg >> 8), uint8(bb >> 8), uint8(aa >> 8)}
}

// nmSameColor reports whether two samples are visually indistinguishable.
func nmSameColor(a, b color.RGBA) bool {
	d := func(x, y uint8) int {
		if x > y {
			return int(x - y)
		}
		return int(y - x)
	}
	return d(a.R, b.R)+d(a.G, b.G)+d(a.B, b.B) < 12
}

// Compared against a plain neighbour, so the assertion is about what the user sees.
func TestNoteMatrix_SelectedCellIsFilledNotJustOutlined(t *testing.T) {
	g, _ := openMatrixForKeys(t)

	cursorID := g.sidebar.noteMatrixKeyCursorID()
	var sel, plain image.Rectangle
	for _, c := range g.sidebar.noteMatrix.VisibleCells() {
		if !c.Selectable {
			continue
		}
		if c.Selected && sel.Empty() {
			sel = c.Rect
			continue
		}
		if !c.Selected && c.ID != cursorID && plain.Empty() {
			plain = c.Rect
		}
	}
	if sel.Empty() || plain.Empty() {
		t.Fatal("need both a selected and a plain visible cell")
	}

	selPx := cellCenterPixel(t, g, sel)
	plainPx := cellCenterPixel(t, g, plain)
	if nmSameColor(selPx, plainPx) {
		t.Errorf("the current note renders the same as a plain cell (%v vs %v) — "+
			"it is outlined but not filled, so it does not read at a glance", selPx, plainPx)
	}
}

func TestNoteMatrix_KeyboardCursorCellIsFilled(t *testing.T) {
	g, _ := openMatrixForKeys(t)

	// Move the cursor off the current note.
	nmKey(g, ebiten.KeyArrowRight)
	cursorID := g.sidebar.noteMatrixKeyCursorID()
	if cursorID == (sbID{}) {
		t.Fatal("no keyboard cursor after ArrowRight")
	}

	var cur, sel, plain image.Rectangle
	for _, c := range g.sidebar.noteMatrix.VisibleCells() {
		if !c.Selectable {
			continue
		}
		switch {
		case c.ID == cursorID:
			cur = c.Rect
		case c.Selected:
			sel = c.Rect
		case plain.Empty():
			plain = c.Rect
		}
	}
	if cur.Empty() || plain.Empty() {
		t.Fatal("need both a cursor cell and a plain visible cell")
	}
	if cur.Eq(sel) {
		t.Skip("cursor is still on the selected note; nothing to distinguish")
	}

	curPx := cellCenterPixel(t, g, cur)
	if plainPx := cellCenterPixel(t, g, plain); nmSameColor(curPx, plainPx) {
		t.Errorf("the keyboard-cursor cell renders the same as a plain cell (%v vs %v) — "+
			"it is outlined but not filled", curPx, plainPx)
	}
	if !sel.Empty() {
		if selPx := cellCenterPixel(t, g, sel); nmSameColor(curPx, selPx) {
			t.Errorf("the keyboard cursor and the current note render identically (%v vs %v); "+
				"they sit on different cells and must be tellable apart", curPx, selPx)
		}
	}
}
