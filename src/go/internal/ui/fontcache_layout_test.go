//go:build !test

package ui

import (
	"image"
	"image/color"
	"testing"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

// Real fonts only (fontcache.go is !test): height and baseline are content-independent, the pen starts at x 0 so the
// caret lands on glyph boundaries, and the width covers the advance so a trailing space fits.

func TestSpriteLayoutHeightContentIndependent(t *testing.T) {
	face := fontFaceBold(FontSizeBody)
	if face == nil {
		t.Skip("no TrueType face")
	}
	_, wantH, _, _ := textSpriteLayout(face, "x")
	for _, s := range []string{"a", "A", "abc", "ABC", "Wave", "wave", "Agjy", "l", " "} {
		if _, h, _, _ := textSpriteLayout(face, s); h != wantH {
			t.Fatalf("height for %q = %d, want constant %d", s, h, wantH)
		}
	}
}

func TestSpriteLayoutBaselineContentIndependent(t *testing.T) {
	face := fontFaceBold(FontSizeBody)
	if face == nil {
		t.Skip("no TrueType face")
	}
	_, _, _, wantB := textSpriteLayout(face, "x")
	for _, s := range []string{"X", "Hx", "abc", "Agjy", "ice", "Nice"} {
		if _, _, _, b := textSpriteLayout(face, s); b != wantB {
			t.Fatalf("baseline for %q = %d, want constant %d (delete-leading-cap must not move text)", s, b, wantB)
		}
	}
}

func TestSpriteLayoutPenOriginZero(t *testing.T) {
	face := fontFaceBold(FontSizeBody)
	if face == nil {
		t.Skip("no TrueType face")
	}
	for _, s := range []string{"Wave", "abc", " token", "MM"} {
		if _, _, penX, _ := textSpriteLayout(face, s); penX != 0 {
			t.Fatalf("penX for %q = %d, want 0 (caret at x+TextWidth must align with the glyph)", s, penX)
		}
	}
}

func TestSpriteLayoutWidthCoversAdvance(t *testing.T) {
	face := fontFaceBold(FontSizeBody)
	if face == nil {
		t.Skip("no TrueType face")
	}
	for _, s := range []string{"hi ", "Wave", " x", "token "} {
		w, _, _, _ := textSpriteLayout(face, s)
		adv := font.MeasureString(face, s).Ceil()
		if w < adv {
			t.Fatalf("width for %q = %d < advance %d — end caret falls outside the sprite", s, w, adv)
		}
	}
}

// Drawn to a plain image.RGBA (At() is safe there): a baseline glyph lands on the same row with or without a leading
// capital.
func TestRenderedBaselineIsAbsolute(t *testing.T) {
	face := fontFaceBold(FontSizeBody)
	if face == nil {
		t.Skip("no TrueType face")
	}
	bottomInk := func(s string) int {
		w, h, penX, baseline := textSpriteLayout(face, s)
		img := image.NewRGBA(image.Rect(0, 0, w, h))
		d := font.Drawer{Dst: img, Src: image.NewUniform(color.White), Face: face, Dot: fixed.P(penX, baseline)}
		d.DrawString(s)
		bottom := -1
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				if _, _, _, a := img.At(x, y).RGBA(); a > 0 {
					bottom = y
					break
				}
			}
		}
		return bottom
	}
	bx, bX, bHx := bottomInk("x"), bottomInk("X"), bottomInk("Hx")
	if bx != bX || bx != bHx {
		t.Fatalf("baseline (bottom ink row) x=%d X=%d Hx=%d — must be identical", bx, bX, bHx)
	}
}
