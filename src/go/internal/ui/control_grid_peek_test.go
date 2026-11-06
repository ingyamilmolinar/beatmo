//go:build test

package ui

import (
	"image"
	"testing"
)

// peekTestGrid builds a single-column ControlGrid (maxCols=1 so row index == cell
// index) that overflows its content rect, using rowH=20 and a 40px-tall rect so a
// no-peek layout fits 2 rows but the reserved peek band drops the window to 1.
func peekTestGrid(t *testing.T, count int) *ControlGrid {
	t.Helper()
	g := NewControlGrid(DefaultScrollbarStyle)
	g.SetMaxCols(1)
	// cellH=20, vGap=0 → rowH=20; rect Dy=40 → fullRows=2; peekH=int(0.4*20)=8;
	// visRows=(40-8)/20=1.
	g.Layout(image.Rect(0, 100, 60, 140), count, 40, 20, 0, 0)
	return g
}

func TestControlGridPeek_ReservedOnOverflow(t *testing.T) {
	g := peekTestGrid(t, 10)
	if g.PeekH() <= 0 {
		t.Fatalf("expected a reserved peek band on overflow, got PeekH()=%d", g.PeekH())
	}
	if got := g.PeekH(); got != 8 {
		t.Fatalf("PeekH()=%d, want 8 (0.4*rowH)", got)
	}
	// The peek band steals a row: visible window is 1, not the no-peek 2.
	if g.visRows != 1 {
		t.Fatalf("visRows=%d, want 1 (peek band reserved)", g.visRows)
	}
}

func TestControlGridPeek_NoneWhenFits(t *testing.T) {
	g := peekTestGrid(t, 1) // single control — fits, no scroll
	if g.HasScroll() {
		t.Fatal("single-control grid should not scroll")
	}
	if g.PeekH() != 0 {
		t.Fatalf("PeekH()=%d, want 0 when the grid fits", g.PeekH())
	}
	if _, ok := g.PeekRect(0); ok {
		t.Fatal("PeekRect must be false when the grid fits")
	}
	if g.ShowTopFade() || g.ShowBottomFade() {
		t.Fatal("no fades when the grid fits")
	}
}

func TestControlGridPeek_RectIsClippedFirstOffWindowRow(t *testing.T) {
	g := peekTestGrid(t, 10) // First=0, visRows=1 → visible row 0, peek row 1
	// Visible cell publishes no peek rect.
	if _, ok := g.PeekRect(0); ok {
		t.Fatal("visible cell 0 must not be a peek cell")
	}
	// First off-window row (index 1) is the peek row.
	r, ok := g.PeekRect(1)
	if !ok {
		t.Fatal("cell 1 (first off-window row) should be a peek cell")
	}
	if r.Dy() != g.PeekH() {
		t.Fatalf("peek rect height=%d, want clipped to peek band %d", r.Dy(), g.PeekH())
	}
	wantTop := 100 + g.visRows*g.rowH // rect.Min.Y + visRows*rowH
	if r.Min.Y != wantTop {
		t.Fatalf("peek rect top=%d, want %d", r.Min.Y, wantTop)
	}
	// A row further down is not the peek row.
	if _, ok := g.PeekRect(2); ok {
		t.Fatal("cell 2 is not the first off-window row; must not be a peek cell")
	}
}

func TestControlGridPeek_PublishesNoHitArea(t *testing.T) {
	g := peekTestGrid(t, 10)
	// The peek cell (index 1) shows a sliver via PeekRect but is off-window for
	// CellRect — so the caller never registers a hit area for it (draw-only).
	if _, ok := g.CellRect(1); ok {
		t.Fatal("peek cell must be off-window for CellRect (no hit area)")
	}
	if _, ok := g.PeekRect(1); !ok {
		t.Fatal("peek cell must still render via PeekRect")
	}
}

func TestControlGridPeek_EmptyAtBottom(t *testing.T) {
	g := peekTestGrid(t, 10)
	g.scroll.VS.First = g.scroll.VS.Total - g.visRows // scroll fully to bottom
	// No next row below the last visible one → no peek cell anywhere.
	for i := 0; i < 10; i++ {
		if _, ok := g.PeekRect(i); ok {
			t.Fatalf("at bottom, cell %d must not be a peek cell", i)
		}
	}
	if g.ShowBottomFade() {
		t.Fatal("bottom fade must hide when scrolled to the bottom")
	}
	if !g.ShowTopFade() {
		t.Fatal("top fade must show when scrolled away from the top")
	}
}

func TestControlGridPeek_FadesTrackScrollEnds(t *testing.T) {
	g := peekTestGrid(t, 10)
	// At the top: no top fade, bottom fade present (more rows below).
	if g.ShowTopFade() {
		t.Fatal("top fade must be absent at First==0")
	}
	if !g.ShowBottomFade() {
		t.Fatal("bottom fade must be present when more rows exist below")
	}
	// After scrolling down one row: top fade appears.
	g.scroll.VS.First = 1
	if !g.ShowTopFade() {
		t.Fatal("top fade must appear after scrolling down")
	}
}

func TestControlGridPeek_WindowStableAcrossScroll(t *testing.T) {
	g := peekTestGrid(t, 10)
	top := g.visRows
	g.scroll.VS.First = 4
	// Re-layout at the scrolled position; window height must not change.
	g.Layout(image.Rect(0, 100, 60, 140), 10, 40, 20, 0, 0)
	if g.visRows != top {
		t.Fatalf("visRows changed across scroll: was %d, now %d", top, g.visRows)
	}
}

// SetPeekBand(false) gives the stolen row back: the same overflowing grid shows
// both rows and reserves no band (so no fade paints over a visible row either).
// The Synth root opts out — on a phone its band cost the second dial card.
func TestControlGridPeek_OptOutKeepsEveryFullRow(t *testing.T) {
	g := NewControlGrid(DefaultScrollbarStyle)
	g.SetMaxCols(1)
	g.SetPeekBand(false)
	g.Layout(image.Rect(0, 100, 60, 140), 10, 40, 20, 0, 0)
	if g.visRows != 2 || g.PeekH() != 0 {
		t.Fatalf("opted out: visRows=%d peekH=%d, want 2 and 0", g.visRows, g.PeekH())
	}
	if !g.HasScroll() {
		t.Fatal("opting out of the band must not hide the overflow")
	}
	g.SetPeekBand(true)
	g.Layout(image.Rect(0, 100, 60, 140), 10, 40, 20, 0, 0)
	if g.visRows != 1 || g.PeekH() != 8 {
		t.Fatalf("opted back in: visRows=%d peekH=%d, want 1 and 8", g.visRows, g.PeekH())
	}
}
