//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

// TestHighlightSpriteAlphaSufficientForVisibility verifies that the highlight
// sprite rendered by ensureHighlightSprites has enough opacity to be clearly
// visible against the dark grid background. A white highlight at 50% alpha
// blends to ~{132,133,139} over colStepOff — barely perceptible.
func TestHighlightSpriteAlphaSufficientForVisibility(t *testing.T) {
	assertDefaultParityState(t)

	z, _ := newTestTimelineZone()
	z.ensureHighlightSprites()

	if z.hlSpriteReg == nil {
		t.Fatal("highlight sprite not created")
	}

	// Sample the sprite pixel to get the actual rendered alpha.
	at := z.hlSpriteReg.At(0, 0)
	_, _, _, a := at.RGBA()
	alpha := a >> 8

	// The highlight must have alpha >= 200 to produce a visible flash
	// against the dark grid background (~{14-24, 14-24, 18-30}).
	const minAlpha = 200
	if alpha < minAlpha {
		t.Fatalf("highlight sprite alpha %d is too low for visibility against dark backgrounds (need >= %d)", alpha, minAlpha)
	}
}

// TestHighlightDrawUsesVisibleColor verifies that when a non-muted highlight
// is drawn via the test path, the rendered color has sufficient alpha to be
// distinguishable from the background.
func TestHighlightDrawUsesVisibleColor(t *testing.T) {
	assertDefaultParityState(t)

	z, _ := newTestTimelineZone()
	rows := []*DrumRow{{
		Steps:     make([]bool, 8),
		CellTypes: make([]model.NodeType, 8),
		Color:     color.RGBA{50, 170, 100, 255}, // kick-like color
	}}
	z.callbacks.Rows = func() []*DrumRow { return rows }
	z.callbacks.RowHeight = func() int { return 24 }
	z.callbacks.VisibleRows = func() int { return 1 }
	z.callbacks.Length = func() int { return 8 }

	rect := image.Rect(0, 0, 800, 200)
	tree := registerTimelineZone(z, rect)
	_ = tree
	z.Layout(rect)

	z.highlightsByRow = [][]highlightEntry{{{idx: 2, val: 1}}}

	dst := ebiten.NewImage(800, 200)

	// Intercept drawRect to capture the highlight color.
	var highlightAlpha uint32
	var gotHighlight bool
	orig := drawRect
	drawRect = func(d *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		_, _, _, a := c.RGBA()
		if filled && a>>8 > 0 {
			// DrumCellUI.Draw renders the fill as one of its first calls.
			// The fill color for a highlighted cell should be bright enough.
			cr, cg, cb, _ := c.RGBA()
			cr8, cg8, cb8 := cr>>8, cg>>8, cb>>8
			// Check if this looks like a highlight fill (bright, near-white or row-colored).
			if cr8 > 150 && cg8 > 150 && cb8 > 150 && a>>8 >= 200 {
				highlightAlpha = a >> 8
				gotHighlight = true
			}
		}
		orig(d, r, c, filled)
	}
	defer func() { drawRect = orig }()

	z.drawHighlights(dst, false)

	if !gotHighlight {
		t.Fatal("highlight draw did not produce a visible (alpha >= 200) fill color")
	}
	if highlightAlpha < 200 {
		t.Fatalf("highlight fill alpha %d too low (need >= 200)", highlightAlpha)
	}
}
