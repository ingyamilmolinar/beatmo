//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// Both pickers: selecting applies and keeps the picker open for auditioning; activating the value already current
// confirms and closes. Positional, not a double-click timer, so pointer and keyboard behave identically.

// ─── Note matrix ───────────────────────────────────────────────────────────

// Scrolled into view first, so the click is exercised rather than skipped when the row is outside the window.
func clickMatrixCellForPitch(t *testing.T, g *Game, pitch float64) bool {
	t.Helper()
	m := g.sidebar.noteMatrix
	col, row, ok := m.Model().CellFor(pitch)
	if !ok {
		t.Fatalf("pitch %v has no cell", pitch)
	}
	m.RevealRow(row)
	g.sidebar.layout() // republish cell rects for the new scroll position

	r, visible := m.cellRect(col, row)
	if !visible {
		return false
	}
	cx, cy := nmCenter(r)
	nmClick(g, cx, cy)
	return true
}

// A first click on a DIFFERENT note applies it and leaves the picker open.
func TestNoteMatrix_FirstClickAppliesAndStaysOpen(t *testing.T) {
	g, node := openMatrixForKeys(t)

	const want = 5.0
	if !clickMatrixCellForPitch(t, g, want) {
		t.Skip("target cell not visible at this layout")
	}

	mn, _ := g.graph.GetNodeByID(node.ID)
	if mn.Params.Pitch != want {
		t.Errorf("first click set pitch %v, want %v", mn.Params.Pitch, want)
	}
	if !g.sidebar.noteMatrixIsOpen() {
		t.Error("picker closed on the FIRST click; selecting must apply and stay open " +
			"so several notes can be tried without reopening it")
	}
}

// Clicking the note that is ALREADY current confirms and closes.
func TestNoteMatrix_SecondClickOnSameCellCloses(t *testing.T) {
	g, node := openMatrixForKeys(t)

	const want = 5.0
	if !clickMatrixCellForPitch(t, g, want) {
		t.Skip("target cell not visible at this layout")
	}
	if !g.sidebar.noteMatrixIsOpen() {
		t.Fatal("picker closed on the first click")
	}
	if !clickMatrixCellForPitch(t, g, want) {
		t.Skip("target cell scrolled out after the first click")
	}

	if g.sidebar.noteMatrixIsOpen() {
		t.Error("re-selecting the current note did not confirm and close the picker")
	}
	mn, _ := g.graph.GetNodeByID(node.ID)
	if mn.Params.Pitch != want {
		t.Errorf("confirming changed the pitch to %v, want it to stay %v", mn.Params.Pitch, want)
	}
}

func TestNoteMatrix_AuditionSeveralNotesWithoutReopening(t *testing.T) {
	g, node := openMatrixForKeys(t)

	for _, want := range []float64{2, -3, 9} {
		if !clickMatrixCellForPitch(t, g, want) {
			t.Skipf("cell for pitch %v not visible", want)
		}
		mn, _ := g.graph.GetNodeByID(node.ID)
		if mn.Params.Pitch != want {
			t.Fatalf("click set pitch %v, want %v", mn.Params.Pitch, want)
		}
		if !g.sidebar.noteMatrixIsOpen() {
			t.Fatalf("picker closed after selecting pitch %v; it must stay open for a different value", want)
		}
	}
}

// Otherwise the user cannot see what they just selected.
func TestNoteMatrix_SelectionHighlightFollowsWhileOpen(t *testing.T) {
	g, _ := openMatrixForKeys(t)

	const want = 4.0
	if !clickMatrixCellForPitch(t, g, want) {
		t.Skip("target cell not visible")
	}
	if !g.sidebar.noteMatrixIsOpen() {
		t.Fatal("picker closed on the first click")
	}

	wantCol, wantRow, _ := g.sidebar.noteMatrix.Model().CellFor(want)
	wantID := sbNoteCell(wantCol, wantRow)
	found := false
	for _, c := range g.sidebar.noteMatrix.VisibleCells() {
		if c.Selected {
			if c.ID != wantID {
				t.Errorf("highlight is on %s, want the just-applied %s", c.ID, wantID)
			}
			found = true
		}
	}
	if !found {
		t.Error("no cell is highlighted after applying a note")
	}
}

func TestNoteMatrix_EnterAppliesThenConfirms(t *testing.T) {
	g, node := openMatrixForKeys(t)

	// Move the cursor off the current note, then Enter.
	nmKey(g, ebiten.KeyArrowRight)
	sel := g.sidebar.noteMatrixNav().Selected()
	h := noteMatrixNavHost{sb: g.sidebar}
	col, row, ok := h.cellAt(sel)
	if !ok {
		t.Fatal("cursor maps to no cell")
	}
	want, selectable := g.sidebar.noteMatrix.Model().PitchAt(col, row)
	if !selectable {
		t.Skip("cursor is on an unselectable cell")
	}

	nmKey(g, ebiten.KeyEnter)
	mn, _ := g.graph.GetNodeByID(node.ID)
	if mn.Params.Pitch != want {
		t.Errorf("first Enter set pitch %v, want %v", mn.Params.Pitch, want)
	}
	if !g.sidebar.noteMatrixIsOpen() {
		t.Fatal("first Enter closed the picker; it must apply and stay open")
	}

	nmKey(g, ebiten.KeyEnter)
	if g.sidebar.noteMatrixIsOpen() {
		t.Error("second Enter on the same note did not confirm and close")
	}
}

// ─── Colour picker ─────────────────────────────────────────────────────────

func openColorPicker(t *testing.T) *DrumView {
	t.Helper()
	logger := game_log.New(nil, game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 800, 600), nil, logger)
	dv.calcLayout()
	dv.openColorPickerForRow(0)
	if dv.colorWheelComp == nil || !dv.colorWheelComp.IsOpen() {
		t.Fatal("colour picker did not open")
	}
	dv.colorWheelComp.ClearHold()
	return dv
}

// clickSwatch presses and releases on the swatch at index i.
func clickSwatch(dv *DrumView, i int) {
	cells := dv.colorWheelComp.SwatchCells()
	if i < 0 || i >= len(cells) {
		return
	}
	r := cells[i]
	cx, cy := r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2
	dv.colorWheelComp.HandleInput(cx, cy, true)
	dv.colorWheelComp.HandleInput(cx, cy, false)
}

// swatchColorAt returns the palette colour of swatch i.
func swatchColorAt(dv *DrumView, i int) color.RGBA {
	return SuggestedInstrumentSwatches[i].RGBA
}

func TestColorPicker_FirstClickAppliesAndStaysOpen(t *testing.T) {
	dv := openColorPicker(t)

	// Pick a swatch that is not already the row's colour.
	idx := -1
	for i := range SuggestedInstrumentSwatches {
		if !colorsEqualRGB(dv.rowColorAt(0), SuggestedInstrumentSwatches[i].RGBA) {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Skip("every swatch equals the row colour")
	}
	want := swatchColorAt(dv, idx)

	clickSwatch(dv, idx)

	if !colorsEqualRGB(dv.rowColorAt(0), want) {
		t.Errorf("first click applied %v, want %v", dv.rowColorAt(0), want)
	}
	if !dv.colorWheelComp.IsOpen() {
		t.Error("colour picker closed on the FIRST click; selecting must apply and stay open")
	}
}

// Clicking the swatch that is ALREADY applied confirms and closes.
func TestColorPicker_SecondClickOnSameSwatchCloses(t *testing.T) {
	dv := openColorPicker(t)

	idx := -1
	for i := range SuggestedInstrumentSwatches {
		if !colorsEqualRGB(dv.rowColorAt(0), SuggestedInstrumentSwatches[i].RGBA) {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Skip("every swatch equals the row colour")
	}
	want := swatchColorAt(dv, idx)

	clickSwatch(dv, idx)
	if !dv.colorWheelComp.IsOpen() {
		t.Fatal("picker closed on the first click")
	}
	clickSwatch(dv, idx)

	if dv.colorWheelComp.IsOpen() {
		t.Error("re-selecting the applied colour did not confirm and close the picker")
	}
	if !colorsEqualRGB(dv.rowColorAt(0), want) {
		t.Errorf("confirming changed the colour to %v, want it to stay %v", dv.rowColorAt(0), want)
	}
}

// Several different colours in a row: each applies, the picker stays open.
func TestColorPicker_AuditionSeveralColoursWithoutReopening(t *testing.T) {
	dv := openColorPicker(t)

	picked := 0
	for i := 0; i < len(SuggestedInstrumentSwatches) && picked < 3; i++ {
		if colorsEqualRGB(dv.rowColorAt(0), SuggestedInstrumentSwatches[i].RGBA) {
			continue // would be the confirm gesture
		}
		want := swatchColorAt(dv, i)
		clickSwatch(dv, i)
		picked++
		if !colorsEqualRGB(dv.rowColorAt(0), want) {
			t.Fatalf("click %d applied %v, want %v", picked, dv.rowColorAt(0), want)
		}
		if !dv.colorWheelComp.IsOpen() {
			t.Fatalf("picker closed after applying colour %d; it must stay open for a different value", picked)
		}
	}
	if picked < 2 {
		t.Skip("not enough distinct swatches to exercise the loop")
	}
}

// Its props were seeded at open time and would otherwise go stale.
func TestColorPicker_SelectedRingFollowsWhileOpen(t *testing.T) {
	dv := openColorPicker(t)

	idx := -1
	for i := range SuggestedInstrumentSwatches {
		if !colorsEqualRGB(dv.rowColorAt(0), SuggestedInstrumentSwatches[i].RGBA) {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Skip("every swatch equals the row colour")
	}
	want := swatchColorAt(dv, idx)
	clickSwatch(dv, idx)
	if !dv.colorWheelComp.IsOpen() {
		t.Fatal("picker closed on the first click")
	}

	_, got, ok := dv.colorWheelComp.selectedSwatchName()
	if !ok {
		t.Fatal("no swatch reports as selected after applying a colour")
	}
	if !colorsEqualRGB(got, want) {
		t.Errorf("selected ring is on %v, want the just-applied %v", got, want)
	}
}

// After a click the cursor sits on the cell, so Enter confirms it rather than re-applying wherever the cursor was.
func TestNoteMatrix_CursorFollowsAPointerPickSoEnterConfirms(t *testing.T) {
	g, node := openMatrixForKeys(t)

	const want = 4.0
	if !clickMatrixCellForPitch(t, g, want) {
		t.Skip("target cell not visible")
	}
	if !g.sidebar.noteMatrixIsOpen() {
		t.Fatal("picker closed on the first click")
	}

	sel := g.sidebar.noteMatrixNav().Selected()
	if sel == MenuKeyNavNoSelection {
		t.Fatal("no keyboard cursor after a pointer pick")
	}
	h := noteMatrixNavHost{sb: g.sidebar}
	col, row, ok := h.cellAt(sel)
	if !ok {
		t.Fatal("cursor maps to no cell")
	}
	got, _ := g.sidebar.noteMatrix.Model().PitchAt(col, row)
	if got != want {
		t.Errorf("after clicking pitch %v the cursor is on %v; it must follow the pick", want, got)
	}

	// Enter is therefore the confirm.
	nmKey(g, ebiten.KeyEnter)
	if g.sidebar.noteMatrixIsOpen() {
		t.Error("Enter after a pointer pick did not confirm and close")
	}
	mn, _ := g.graph.GetNodeByID(node.ID)
	if mn.Params.Pitch != want {
		t.Errorf("confirming changed the pitch to %v, want %v", mn.Params.Pitch, want)
	}
}
