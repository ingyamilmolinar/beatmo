//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// The grid silently drops swatches past colorSwatchRows, so it must hold exactly every swatch.
func TestColorPickerGridMatchesSwatchCount(t *testing.T) {
	if got, want := len(SuggestedInstrumentSwatches), colorSwatchRows*colorSwatchCols; got != want {
		t.Fatalf("picker grid is %d×%d=%d cells but there are %d swatches; "+
			"resize colorSwatchRows/colorSwatchCols (or the swatch set) so every swatch is reachable",
			colorSwatchRows, colorSwatchCols, want, got)
	}
}

// A press, then a release, through the real DrumView input loop.
func pressReleaseAtThenUpdate(t *testing.T, dv *DrumView, x, y, w, h int) {
	t.Helper()
	// Press frame.
	r := SetInputForTest(
		func() (int, int) { return x, y },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return w, h },
	)
	dv.Update()
	r()
	// Release frame.
	r = SetInputForTest(
		func() (int, int) { return x, y },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return w, h },
	)
	dv.Update()
	r()
}

// Kebab → "Color" → swatch → the row recolours with an undo step, and Undo restores the old colour, all through real
// layout and Game.Update.
func TestEllipsisColorOpensPickerAndRecolors(t *testing.T) {
	g := newTestGameForUndo(t) // real Game + g.undoManager, Layout(800,600) = desktop popup
	dv := g.drum
	const W, H = 800, 600

	row := 0
	if len(dv.Rows) == 0 {
		t.Fatal("expected at least one drum row in the default document")
	}
	before := dv.Rows[row].Color

	// Warm-up frame so row buttons (kebab) are laid out.
	warmUp := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	warmUp()

	// Its OnClick is the production openContextMenu path.
	if len(dv.rowMenuBtns()) == 0 {
		t.Fatal("rowMenuBtns not created after warm-up")
	}
	kebabRect := dv.rowMenuBtns()[row].Rect()
	if kebabRect.Empty() {
		t.Skip("kebab button rect empty (not visible in this layout)")
	}
	kx := kebabRect.Min.X + kebabRect.Dx()/2
	ky := kebabRect.Min.Y + kebabRect.Dy()/2
	// A full tap: the release clears the suppression opening the menu set, so the next item tap is not swallowed.
	pressReleaseAtThenUpdate(t, dv, kx, ky, W, H)
	if !dv.IsContextMenuOpen() {
		t.Fatal("context menu did not open after kebab tap")
	}

	// Its OnClick closes the menu and opens the picker.
	var colorRect image.Rectangle
	foundColor := false
	for _, btn := range dv.contextMenuBtns {
		if btn.Text == "Color" {
			colorRect = btn.Rect()
			foundColor = true
			break
		}
	}
	if !foundColor {
		t.Fatal(`context menu has no "Color" item`)
	}
	if colorRect.Empty() {
		t.Fatal(`"Color" item rect is empty`)
	}
	cx := colorRect.Min.X + colorRect.Dx()/2
	cy := colorRect.Min.Y + colorRect.Dy()/2
	pressReleaseAtThenUpdate(t, dv, cx, cy, W, H)

	if dv.colorWheelComp == nil || !dv.colorWheelComp.IsOpen() {
		t.Fatal(`"Color" item did not open the color picker`)
	}

	// A swatch of a different colour, through the picker's portal hit area.
	cells := dv.colorWheelComp.SwatchCells()
	if len(cells) == 0 {
		t.Fatal("picker has no swatch cells")
	}
	var target image.Rectangle
	found := false
	for _, cell := range cells {
		mx := (cell.Min.X + cell.Max.X) / 2
		my := (cell.Min.Y + cell.Max.Y) / 2
		// It must differ from the current colour, or the recolour check is meaningless.
		col := dv.colorWheelComp.pickColorAt(mx, my)
		if col == nil {
			continue
		}
		if dv.colorKey(col) != dv.colorKey(before) {
			target = cell
			found = true
			break
		}
	}
	if !found {
		t.Fatal("no swatch cell with a color different from the current row color")
	}
	sx := (target.Min.X + target.Max.X) / 2
	sy := (target.Min.Y + target.Max.Y) / 2
	pressReleaseAtThenUpdate(t, dv, sx, sy, W, H)

	// User-visible outcome: the row recolored.
	if dv.colorKey(dv.Rows[row].Color) == dv.colorKey(before) {
		t.Fatalf("row color did not change after picking a swatch (still %v)", dv.Rows[row].Color)
	}
	// An undo step must have been recorded by SetRowColorManual.
	if !g.undoManager.CanUndo() {
		t.Fatal("picking a swatch should record an undo step")
	}

	// 4. Undo restores the previous color.
	g.undoManager.Undo()
	if dv.colorKey(g.drum.Rows[row].Color) != dv.colorKey(before) {
		t.Fatalf("undo did not restore color: got %v want %v", g.drum.Rows[row].Color, before)
	}
}
