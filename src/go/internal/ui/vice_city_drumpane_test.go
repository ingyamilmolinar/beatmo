//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// Vice City / outrun aesthetic regression tests for the three main drum-view
// surfaces: drum cells (backlit micro-gradient + 808 group separators), the
// timeline ribbon (faint amber track fill + brighter playhead core), and the
// row rack (faint instrument-color label tint). All treatments are token-
// driven and reuse existing alpha buckets; these tests pin that they actually
// draw so the polish can't silently regress.

// rgbaEq compares two color.Color via their premultiplied RGBA model. The
// drawCallRecorder stores captured colors as color.RGBA (premultiplied), so
// `want` NRGBA tokens must be compared in the same space.
func rgbaEq(a, b color.Color) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	ar := color.RGBAModel.Convert(a).(color.RGBA)
	br := color.RGBAModel.Convert(b).(color.RGBA)
	return ar == br
}

// TestDrumCellGradientHelper asserts that an ON, non-mute, wide cell draws a
// multi-band vertical gradient in the instrument color (backlit-hardware
// look), while an OFF cell does not. The gradient bands must all be distinct
// shades derived from the instrument color via adjustColor.
func TestDrumCellGradientHelper(t *testing.T) {
	var rec drawCallRecorder
	dst := ebiten.NewImage(40, 20)
	rowCol := color.RGBA{200, 80, 120, 255}
	cell := image.Rect(0, 0, 30, 20)

	rec.record(t, func() {
		drawDrumCell(dst, cell, true, rowCol, false)
	})
	// Gradient bakes drumCellGradientBands filled rects covering the cell
	// width plus a 1px border stroke. Count the filled full-width band rects.
	bands := 0
	for _, c := range rec.calls {
		if c.Kind == drawCallRect && c.Filled && c.Rect.Dx() == cell.Dx() {
			bands++
		}
	}
	if bands < drumCellGradientBands {
		t.Fatalf("ON cell: expected >= %d gradient bands, got %d", drumCellGradientBands, bands)
	}

	// OFF cell: no gradient — defers to the flat DrumCellUI.Draw path which
	// fills the cell exactly once with the off color.
	rec.record(t, func() {
		drawDrumCell(dst, cell, false, rowCol, false)
	})
	fullFills := 0
	for _, c := range rec.calls {
		if c.Kind == drawCallRect && c.Filled && c.Rect.Dx() == cell.Dx() {
			fullFills++
		}
	}
	if fullFills >= drumCellGradientBands {
		t.Fatalf("OFF cell must not draw a gradient; got %d full-width fills", fullFills)
	}
}

// TestDrumCellMuteNoGradient asserts mute cells stay flat (the gradient is an
// ON-instrument-color treatment only; mute uses the dedicated mute fill).
func TestDrumCellMuteNoGradient(t *testing.T) {
	var rec drawCallRecorder
	dst := ebiten.NewImage(40, 20)
	cell := image.Rect(0, 0, 30, 20)
	rec.record(t, func() {
		drawDrumCell(dst, cell, true, colMuteCell, true)
	})
	fills := 0
	for _, c := range rec.calls {
		if c.Kind == drawCallRect && c.Filled && c.Rect.Dx() == cell.Dx() {
			fills++
		}
	}
	if fills >= drumCellGradientBands {
		t.Fatalf("mute cell must not draw a gradient; got %d full-width fills", fills)
	}
}

// TestBeatGroupSeparators asserts the faint per-beat group separators bake one
// vertical line at each group boundary so the row reads as an 808 step row.
func TestBeatGroupSeparators(t *testing.T) {
	var rec drawCallRecorder
	dst := ebiten.NewImage(64, 20)
	want := WithAlpha(genColorOnSurface, AlphaFaint)
	// w=64, n=16, group=4 -> separators at j=4,8,12 (3 lines).
	rec.record(t, func() {
		drawBeatGroupSeparators(dst, 64, 20, 16, 4)
	})
	seps := 0
	for _, c := range rec.calls {
		if c.Kind == drawCallRect && c.Filled && c.Rect.Dx() == 1 && rgbaEq(c.Color, want) {
			seps++
		}
	}
	if seps != 3 {
		t.Fatalf("expected 3 group separators, got %d", seps)
	}

	// group <= 1 disables the separators entirely.
	rec.record(t, func() {
		drawBeatGroupSeparators(dst, 64, 20, 16, 1)
	})
	for _, c := range rec.calls {
		if c.Kind == drawCallRect && c.Filled && c.Rect.Dx() == 1 && rgbaEq(c.Color, want) {
			t.Fatalf("group<=1 must draw no separators")
		}
	}
}

// TestRowLabelTintUsesRowColor asserts the row-rack label tint draws a faint
// wash in the row's instrument color across the label rect, threading the
// same hue the graph cluster uses.
func TestRowLabelTintUsesRowColor(t *testing.T) {
	var rec drawCallRecorder
	dst := ebiten.NewImage(120, 24)
	rowCol := color.RGBA{72, 180, 255, 255}
	lbl := image.Rect(0, 0, 100, 24)
	want := WithAlphaFromColor(rowCol, AlphaFaint)

	rec.record(t, func() {
		drawRowLabelTint(dst, lbl, rowCol)
	})
	found := false
	for _, c := range rec.calls {
		if c.Kind == drawCallRect && c.Filled && c.Rect == lbl && rgbaEq(c.Color, want) {
			found = true
		}
	}
	if !found {
		t.Fatalf("row-label tint not drawn in row color at AlphaFaint")
	}

	// nil color is a no-op (test paths build DrumRow values directly).
	rec.record(t, func() {
		drawRowLabelTint(dst, lbl, nil)
	})
	if len(rec.calls) != 0 {
		t.Fatalf("nil row color must be a no-op, got %d draws", len(rec.calls))
	}
}

// TestTimelineTrackFillDraws asserts the timeline ribbon bakes a faint amber
// track fill behind the view-rect so the bright window reads as a window onto
// a recessed warm "sunset" track instead of floating in the near-black bg.
// The fill is baked into TlCache, so we intercept drawRect during a full
// Game.Draw and look for the faint-amber full-bar fill.
func TestTimelineTrackFillDraws(t *testing.T) {
	w, h := 1000, 700
	g, cleanup := newRibbonCursorGame(t, w, h)
	defer cleanup()
	screen := ebiten.NewImage(w, h)

	want := WithAlpha(genColorTimelineView, AlphaFaint)
	trackFillSeen := false
	orig := drawRect
	drawRect = func(dst *ebiten.Image, r image.Rectangle, col color.Color, filled bool) {
		if filled && rgbaEq(col, want) {
			trackFillSeen = true
		}
		orig(dst, r, col, filled)
	}
	defer func() { drawRect = orig }()

	_ = g.Update()
	g.Draw(screen)

	if !trackFillSeen {
		t.Fatalf("timeline track fill (faint amber) not drawn behind view-rect")
	}
}

// TestTimelinePlayheadBrightCore asserts the playhead cursor draws a 1px
// brighter core (colAccentBright) so it stays findable against the warm ticks.
func TestTimelinePlayheadBrightCore(t *testing.T) {
	w, h := 1000, 700
	g, cleanup := newRibbonCursorGame(t, w, h)
	defer cleanup()
	screen := ebiten.NewImage(w, h)

	coreSeen := false
	orig := drawRect
	drawRect = func(dst *ebiten.Image, r image.Rectangle, col color.Color, filled bool) {
		// The bright core is a 1px-wide accent fill inside the bar.
		if filled && r.Dx() == 1 && rgbaEq(col, colAccentBright) {
			coreSeen = true
		}
		orig(dst, r, col, filled)
	}
	defer func() { drawRect = orig }()

	_ = g.Update()
	g.Draw(screen)

	if !coreSeen {
		t.Fatalf("timeline playhead bright core not drawn")
	}
}
