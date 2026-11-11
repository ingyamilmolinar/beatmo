package ui

import (
	"image"
	"testing"
)

func TestControlGridGeom(t *testing.T) {
	g := NewControlGrid(controlGridScrollbarStyle())
	g.SetMaxCols(3)
	g.Layout(image.Rect(0, 0, 320, 200), 6, 90, 40, 8, 8)
	if g.Cols() != 3 {
		t.Fatalf("cols want 3 got %d", g.Cols())
	}
	if g.CellH() != 40 {
		t.Fatalf("cellH want 40 got %d", g.CellH())
	}
	if g.RowH() != 48 {
		t.Fatalf("rowH want 48 got %d", g.RowH())
	}
	c00, ok := g.CellAt(0, 0)
	if !ok {
		t.Fatal("cell (0,0) should be visible")
	}
	c01, _ := g.CellAt(0, 1)
	if c01.Min.X != c00.Max.X+g.HGap() {
		t.Fatalf("cell (0,1) x=%d want %d", c01.Min.X, c00.Max.X+g.HGap())
	}
	r0, _ := g.CellRect(0)
	if c00 != r0 {
		t.Fatalf("CellAt(0,0) %v must match CellRect(0) %v", c00, r0)
	}
}

func TestControlGridSetContentRows(t *testing.T) {
	g := NewControlGrid(controlGridScrollbarStyle())
	g.SetMaxCols(2)
	g.Layout(image.Rect(0, 0, 200, 60), 2, 90, 40, 8, 8) // 1 row, fits
	if g.HasScroll() {
		t.Fatal("2 controls in 2 cols fit one row; no scroll expected")
	}
	g.SetContentRows(5) // reflow now needs 5 rows
	if !g.HasScroll() {
		t.Fatal("after SetContentRows(5) the grid must report scrollable")
	}
}
