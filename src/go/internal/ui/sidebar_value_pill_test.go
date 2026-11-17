//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestRecessedPillIsDarkerWellWithInnerShadow pins the recessed value-readout
// look: a fill DARKER than colSurface2 (sunk-in, not a flat raised tile) plus
// an inner top-shadow band — the tactile opposite of the raised keycaps it
// sits between. A regression to the old flat colSurface2 fill fails here.
func TestRecessedPillIsDarkerWellWithInnerShadow(t *testing.T) {
	r := image.Rect(20, 10, 80, 34)
	dst := ebiten.NewImage(100, 50)

	var rounded []roundedDraw
	origR := drawRoundedRect
	drawRoundedRect = func(d *ebiten.Image, rr image.Rectangle, c color.Color, rad int, filled bool) {
		rounded = append(rounded, roundedDraw{Rect: rr, Color: c, Radius: rad, Filled: filled})
		origR(d, rr, c, rad, filled)
	}
	t.Cleanup(func() { drawRoundedRect = origR })

	type rectDraw struct {
		Rect   image.Rectangle
		Color  color.Color
		Filled bool
	}
	var rects []rectDraw
	origRect := drawRect
	drawRect = func(d *ebiten.Image, rr image.Rectangle, c color.Color, filled bool) {
		rects = append(rects, rectDraw{Rect: rr, Color: c, Filled: filled})
		origRect(d, rr, c, filled)
	}
	t.Cleanup(func() { drawRect = origRect })

	drawRecessedPill(dst, r, "0.50")

	// (a) The well fill is a filled rounded rect at r, DARKER than colSurface2.
	sr, sg, sb, _ := colSurface2.RGBA()
	foundWell := false
	for _, c := range rounded {
		if c.Filled && c.Rect == r {
			wr, wg, wb, _ := c.Color.RGBA()
			if wr == sr && wg == sg && wb == sb {
				t.Fatalf("well fill equals colSurface2 — flat, not recessed")
			}
			if wr <= sr && wg <= sg && wb <= sb && (wr < sr || wg < sg || wb < sb) {
				foundWell = true
			}
		}
	}
	if !foundWell {
		t.Fatalf("no recessed (darker-than-surface2) well fill found: %+v", rounded)
	}

	// (b) An inner top-shadow band: a filled rect hugging the top inner edge.
	foundShadow := false
	for _, c := range rects {
		if c.Filled && c.Rect.Min.Y == r.Min.Y+1 && c.Rect.Max.Y <= r.Min.Y+1+genGeomButtonInnerShadowPx {
			foundShadow = true
		}
	}
	if !foundShadow {
		t.Fatalf("no inner top-shadow band drawn for the recessed readout: %+v", rects)
	}
}
