//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestControlGridFades_NoneWhenFits(t *testing.T) {
	g := peekTestGrid(t, 1) // fits, no scroll, no peek band
	calls := 0
	orig := drawRect
	drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, f bool) { calls++ }
	defer func() { drawRect = orig }()
	drawControlGridEdgeFades(nil, g, TokenSurface1())
	if calls != 0 {
		t.Fatalf("fits grid must draw no fade bands, got %d drawRect calls", calls)
	}
}

func TestControlGridFades_BottomOnlyAtTop(t *testing.T) {
	g := peekTestGrid(t, 10) // overflow, First=0
	r := g.ContentRect()
	h := g.PeekH()
	top, bottom := 0, 0
	orig := drawRect
	drawRect = func(dst *ebiten.Image, rr image.Rectangle, c color.Color, f bool) {
		mid := (rr.Min.Y + rr.Max.Y) / 2
		if mid >= r.Min.Y && mid < r.Min.Y+h {
			top++
		}
		if mid >= r.Max.Y-h && mid < r.Max.Y {
			bottom++
		}
	}
	defer func() { drawRect = orig }()
	drawControlGridEdgeFades(nil, g, TokenSurface1())
	if top != 0 {
		t.Fatalf("top fade must be absent at First==0, got %d bands", top)
	}
	if bottom == 0 {
		t.Fatal("bottom fade must be present when more rows exist below")
	}
}

func TestControlGridFades_BothWhenScrolled(t *testing.T) {
	g := peekTestGrid(t, 10)
	g.scroll.VS.First = 1 // scrolled down one row: top + bottom both show
	r := g.ContentRect()
	h := g.PeekH()
	top, bottom := 0, 0
	orig := drawRect
	drawRect = func(dst *ebiten.Image, rr image.Rectangle, c color.Color, f bool) {
		mid := (rr.Min.Y + rr.Max.Y) / 2
		if mid >= r.Min.Y && mid < r.Min.Y+h {
			top++
		}
		if mid >= r.Max.Y-h && mid < r.Max.Y {
			bottom++
		}
	}
	defer func() { drawRect = orig }()
	drawControlGridEdgeFades(nil, g, TokenSurface1())
	if top == 0 {
		t.Fatal("top fade must show when scrolled down")
	}
	if bottom == 0 {
		t.Fatal("bottom fade must show while more rows exist below")
	}
}
