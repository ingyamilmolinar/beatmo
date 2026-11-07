//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func openTestMatrix(t *testing.T, offset, current float64) (*NoteMatrixComponent, *float64) {
	t.Helper()
	var picked float64 = -999
	c := NewNoteMatrixComponent()
	c.SetProps(NoteMatrixProps{
		AnchorRect:       image.Rect(100, 100, 140, 124),
		Bounds:           image.Rect(0, 0, 800, 600),
		CurrentPitch:     current,
		InstrumentOffset: offset,
		OnPick:           func(p float64) { picked = p },
		OnClose:          func() {},
	})
	c.Open()
	if !c.IsOpen() {
		t.Fatal("picker did not open")
	}
	if c.Layout().Panel.Empty() {
		t.Fatal("picker solved an empty panel")
	}
	return c, &picked
}

// The component publishes cells for the sidebar to wire as Buttons; it no longer hit-tests them.
func TestNoteMatrixComponent_PublishesVisibleSelectableCells(t *testing.T) {
	c, _ := openTestMatrix(t, 0, 0)
	cells := c.VisibleCells()
	if len(cells) == 0 {
		t.Fatal("picker published no cells")
	}
	selectable, selected := 0, 0
	ids := map[sbID]bool{}
	for _, cell := range cells {
		if ids[cell.ID] {
			t.Errorf("duplicate cell id %q", cell.ID)
		}
		ids[cell.ID] = true
		if cell.Rect.Empty() {
			t.Errorf("cell %q has an empty rect", cell.ID)
		}
		if cell.Selectable {
			selectable++
			if cell.Label == "" {
				t.Errorf("selectable cell %q has no label", cell.ID)
			}
		}
		if cell.Selected {
			selected++
		}
	}
	if selectable == 0 {
		t.Error("no selectable cells published")
	}
	if selected != 1 {
		t.Errorf("%d cells marked selected, want exactly 1 (the current note)", selected)
	}
}

// The button wired from a cell sets what the user read.
func TestNoteMatrixComponent_CellPitchMatchesItsLabel(t *testing.T) {
	c, _ := openTestMatrix(t, 3, 0)
	for _, cell := range c.VisibleCells() {
		if !cell.Selectable {
			continue
		}
		col, row, ok := c.Model().CellFor(cell.Pitch)
		if !ok {
			t.Errorf("cell %q pitch %v maps to no cell", cell.ID, cell.Pitch)
			continue
		}
		if got := c.Model().NoteLabelAt(col, row); got != cell.Label {
			t.Errorf("cell %q is labelled %q but its pitch %v is %q", cell.ID, cell.Label, cell.Pitch, got)
		}
	}
}

// A press outside the panel closes it without picking.
func TestNoteMatrixComponent_PressOutsideCloses(t *testing.T) {
	c, picked := openTestMatrix(t, 0, 0)
	if got := c.HandleInput(5, 590, true); got != InputConsumed {
		t.Errorf("outside press returned %v, want InputConsumed", got)
	}
	if c.IsOpen() {
		t.Error("picker stayed open after a press outside")
	}
	if *picked != -999 {
		t.Error("a press outside picked a note")
	}
}

// No Button is wired for them, so a mis-tap costs nothing.
func TestNoteMatrixComponent_DimmedCellsAreNotSelectable(t *testing.T) {
	c, _ := openTestMatrix(t, 0, 0)
	for _, cell := range c.VisibleCells() {
		if cell.Selectable {
			continue
		}
		if _, _, ok := c.Model().CellFor(cell.Pitch); ok && cell.Pitch != 0 {
			t.Errorf("cell %q is marked unselectable but carries a reachable pitch %v", cell.ID, cell.Pitch)
		}
	}
}

// The picker never hides the note it is about to change.
func TestNoteMatrixComponent_ScrollsSelectionIntoView(t *testing.T) {
	for _, pitch := range []float64{-24, -12, 0, 11, 24} {
		c, _ := openTestMatrix(t, 0, pitch)
		col, row, ok := c.Model().CellFor(pitch)
		if !ok {
			t.Errorf("pitch %v has no cell", pitch)
			continue
		}
		if _, vis := c.cellRect(col, row); !vis {
			t.Errorf("pitch %v: selected cell is outside the visible row window", pitch)
		}
	}
}

// Why the grid is 12 rows × octave columns and scrolls rather than squeezing.
func TestNoteMatrixComponent_CellsMeetTouchMinimumOnMobile(t *testing.T) {
	restore := SetDensityForTest(DensitySpacious)
	defer restore()

	c := NewNoteMatrixComponent()
	c.SetProps(NoteMatrixProps{
		AnchorRect: image.Rect(10, 10, 50, 34),
		// A phone-sized grid pane: short enough that 12 rows cannot fit.
		Bounds:  image.Rect(0, 0, 390, 380),
		OnPick:  func(float64) {},
		OnClose: func() {},
	})
	c.Open()

	if c.lay.CellH < 20 {
		t.Errorf("cell height %d is unusably small", c.lay.CellH)
	}
	if c.visRows >= noteMatrixRows {
		t.Errorf("all %d rows claimed to fit in a 380px pane; they cannot at touch size", noteMatrixRows)
	}
	if !c.scroll.HasScroll() {
		t.Error("rows do not fit but the picker reports no scroll")
	}
}

func TestNoteMatrixComponent_DrawDoesNotPanic(t *testing.T) {
	c, _ := openTestMatrix(t, 0, 0)
	img := ebiten.NewImage(800, 600)
	c.DrawChrome(img)
	c.DrawOverlay(img)
	c.Close()
	c.DrawChrome(img) // closed: must be a no-op
	c.DrawOverlay(img)
}
