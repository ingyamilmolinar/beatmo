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
// visible against the dark grid background.
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

// TestDrumCellHighlightUsesWhiteNotInstrumentColor verifies that
// DrumCellStyle.Draw uses colHighlight (white) when highlighted, NOT
// the instrument color. If the highlight fill equals the instrument color,
// the playback cursor is invisible.
func TestDrumCellHighlightUsesWhiteNotInstrumentColor(t *testing.T) {
	assertDefaultParityState(t)

	instColor := color.RGBA{50, 170, 100, 255} // kick green
	style := DrumCellUI

	r := image.Rect(10, 10, 30, 30)
	dst := ebiten.NewImage(40, 40)

	var fills []color.RGBA
	orig := drawRect
	drawRect = func(d *ebiten.Image, rect image.Rectangle, c color.Color, filled bool) {
		if filled && rect == r {
			fills = append(fills, color.RGBAModel.Convert(c).(color.RGBA))
		}
		orig(d, rect, c, filled)
	}
	defer func() { drawRect = orig }()

	// Draw a highlighted cell with an instrument color.
	style.Draw(dst, r, true, true, instColor)

	if len(fills) == 0 {
		t.Fatal("no filled rect drawn for highlighted cell")
	}

	// The fill should NOT be the instrument color — it should be colHighlight.
	hlExpected := color.RGBAModel.Convert(colHighlight).(color.RGBA)
	fillActual := fills[0]

	if fillActual == color.RGBAModel.Convert(instColor).(color.RGBA) {
		t.Fatalf("highlighted cell fill is the instrument color %v — should be colHighlight %v (white flash)", fillActual, hlExpected)
	}

	// Verify it IS the highlight color.
	if fillActual != hlExpected {
		t.Fatalf("highlighted cell fill = %v, want colHighlight %v", fillActual, hlExpected)
	}
}

// --- Node type highlight color distinction tests ---

// highlightColorForCellType is a test helper that sets up a TimelineZone with
// one row, assigns the given NodeType to cell index 2, triggers a non-mute
// highlight at that cell, and returns the fill colors drawn by drawHighlights.
func highlightColorForCellType(t *testing.T, cellType model.NodeType) []color.RGBA {
	t.Helper()

	z, _ := newTestTimelineZone()
	rows := []*DrumRow{{
		Steps:     make([]bool, 8),
		CellTypes: make([]model.NodeType, 8),
		Color:     color.RGBA{50, 170, 100, 255},
	}}
	rows[0].CellTypes[2] = cellType
	rows[0].Steps[2] = true
	z.callbacks.Rows = func() []*DrumRow { return rows }
	z.callbacks.RowHeight = func() int { return 24 }
	z.callbacks.VisibleRows = func() int { return 1 }
	z.callbacks.Length = func() int { return 8 }

	rect := image.Rect(0, 0, 800, 200)
	registerTimelineZone(z, rect)
	z.Layout(rect)

	// Non-mute highlight at cell index 2.
	z.highlightsByRow = [][]highlightEntry{{{idx: 2, val: 1}}}

	dst := ebiten.NewImage(800, 200)

	var fills []color.RGBA
	orig := drawRect
	drawRect = func(d *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		if filled {
			fills = append(fills, color.RGBAModel.Convert(c).(color.RGBA))
		}
		orig(d, r, c, filled)
	}
	defer func() { drawRect = orig }()

	z.drawHighlights(dst, false)
	return fills
}

// TestTimelineHighlightSilentNodeUsesGrey verifies that a silent node's
// highlight uses the grey colMuteHighlight, not the white colHighlight.
func TestTimelineHighlightSilentNodeUsesGrey(t *testing.T) {
	assertDefaultParityState(t)
	fills := highlightColorForCellType(t, model.NodeTypeSilent)

	hlWhite := color.RGBAModel.Convert(colHighlight).(color.RGBA)
	hlGrey := color.RGBAModel.Convert(colMuteHighlight).(color.RGBA)

	for _, f := range fills {
		if f == hlWhite {
			t.Fatalf("silent node highlight used white %v — should use grey %v", hlWhite, hlGrey)
		}
	}
	foundGrey := false
	for _, f := range fills {
		if f == hlGrey {
			foundGrey = true
			break
		}
	}
	if !foundGrey {
		t.Fatalf("silent node highlight did not produce grey %v; fills: %v", hlGrey, fills)
	}
}

// TestTimelineHighlightInvisibleNodeUsesGrey verifies that an invisible node's
// highlight uses the grey colMuteHighlight.
func TestTimelineHighlightInvisibleNodeUsesGrey(t *testing.T) {
	assertDefaultParityState(t)
	fills := highlightColorForCellType(t, model.NodeTypeInvisible)

	hlWhite := color.RGBAModel.Convert(colHighlight).(color.RGBA)
	hlGrey := color.RGBAModel.Convert(colMuteHighlight).(color.RGBA)

	for _, f := range fills {
		if f == hlWhite {
			t.Fatalf("invisible node highlight used white %v — should use grey %v", hlWhite, hlGrey)
		}
	}
	foundGrey := false
	for _, f := range fills {
		if f == hlGrey {
			foundGrey = true
			break
		}
	}
	if !foundGrey {
		t.Fatalf("invisible node highlight did not produce grey %v; fills: %v", hlGrey, fills)
	}
}

// TestTimelineHighlightRegularNodeUsesWhite is a regression guard that
// verifies regular (audible) nodes still use the bright white highlight.
func TestTimelineHighlightRegularNodeUsesWhite(t *testing.T) {
	assertDefaultParityState(t)
	fills := highlightColorForCellType(t, model.NodeTypeRegular)

	hlWhite := color.RGBAModel.Convert(colHighlight).(color.RGBA)

	foundWhite := false
	for _, f := range fills {
		if f == hlWhite {
			foundWhite = true
			break
		}
	}
	if !foundWhite {
		t.Fatalf("regular node highlight did not produce white %v; fills: %v", hlWhite, fills)
	}
}

// TestHighlightSpriteIsWhiteNotRowColor verifies that the production sprite
// path for non-muted highlights uses the white highlight, not the row color.
func TestHighlightSpriteIsWhiteNotRowColor(t *testing.T) {
	assertDefaultParityState(t)

	instColor := color.RGBA{50, 170, 100, 255}
	z, _ := newTestTimelineZone()
	rows := []*DrumRow{{
		Steps:     make([]bool, 8),
		CellTypes: make([]model.NodeType, 8),
		Color:     instColor,
	}}
	z.callbacks.Rows = func() []*DrumRow { return rows }
	z.callbacks.RowHeight = func() int { return 24 }
	z.callbacks.VisibleRows = func() int { return 1 }
	z.callbacks.Length = func() int { return 8 }

	rect := image.Rect(0, 0, 800, 200)
	registerTimelineZone(z, rect)
	z.Layout(rect)

	z.highlightsByRow = [][]highlightEntry{{{idx: 2, val: 1}}}

	dst := ebiten.NewImage(800, 200)

	// Intercept drawRect to capture what the highlight draws.
	var highlightFills []color.RGBA
	orig := drawRect
	drawRect = func(d *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		if filled {
			rgba := color.RGBAModel.Convert(c).(color.RGBA)
			highlightFills = append(highlightFills, rgba)
		}
		orig(d, r, c, filled)
	}
	defer func() { drawRect = orig }()

	// The test path (runningUnderGoTest=true, simpleDraw=false)
	// calls DrumCellUI.Draw with highlighted=true.
	z.drawHighlights(dst, false)

	instRGBA := color.RGBAModel.Convert(instColor).(color.RGBA)
	hlRGBA := color.RGBAModel.Convert(colHighlight).(color.RGBA)

	// At least one fill should be the highlight color (white), not the instrument color.
	foundHighlight := false
	for _, f := range highlightFills {
		if f == hlRGBA {
			foundHighlight = true
			break
		}
	}
	if !foundHighlight {
		// Check if we got instrument color instead (the bug).
		foundInst := false
		for _, f := range highlightFills {
			if f == instRGBA {
				foundInst = true
				break
			}
		}
		if foundInst {
			t.Fatalf("highlight used instrument color %v instead of colHighlight %v — cursor is invisible", instRGBA, hlRGBA)
		}
		t.Fatalf("highlight did not produce colHighlight %v; fills: %v", hlRGBA, highlightFills)
	}
}
