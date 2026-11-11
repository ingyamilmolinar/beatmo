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

// TestDrumCellIsFlat asserts drum cells are FLAT (DESIGN.md Rule 2 — nothing
// is 3D): an ON cell fills its rect exactly once in the instrument color and
// strokes the 1 px separating border — no gradient bands, no lit top strip.
// The former 4-band "backlit" gradient read as banding stripes.
func TestDrumCellIsFlat(t *testing.T) {
	dst := ebiten.NewImage(40, 20)
	rowCol := color.RGBA{200, 80, 120, 255}
	cell := image.Rect(0, 0, 30, 20)
	for _, tc := range []struct {
		name string
		on   bool
		col  color.Color
		mute bool
		want color.Color
	}{
		{"on", true, rowCol, false, rowCol},
		{"off", false, rowCol, false, DrumCellUI.Off},
		{"mute", true, colMuteCell, true, colMuteCell},
	} {
		var rec drawCallRecorder // fresh per case: record() accumulates
		rec.record(t, func() { drawDrumCell(dst, cell, tc.on, tc.col, tc.mute) })
		fills := 0
		for _, c := range rec.calls {
			if c.Kind != drawCallRect || !c.Filled {
				continue
			}
			fills++
			if c.Rect != cell || !rgbaEq(c.Color, tc.want) {
				t.Errorf("%s cell: fill %v %v, want one flat fill of the whole cell in %v", tc.name, c.Rect, c.Color, tc.want)
			}
		}
		if fills != 1 {
			t.Errorf("%s cell: %d filled rects, want exactly 1 (flat)", tc.name, fills)
		}
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

// TestRowLabelCarriesIdentitySwatch asserts a rack label with a LeadSwatch
// paints a rounded swatch in the row's instrument color at its leading edge,
// ON the label body (drawn after it) — the instrument menu's swatch + text
// row. The former faint wash + accent stripe were drawn BEFORE the opaque
// label body, which covered both: the rack showed no identity at all.
func TestRowLabelCarriesIdentitySwatch(t *testing.T) {
	var rec drawCallRecorder
	dst := ebiten.NewImage(160, 30)
	rowCol := color.RGBA{72, 180, 255, 255}
	b := NewButton("Kick", InstButtonStyle, nil)
	b.SetRect(image.Rect(0, 0, 150, 30))
	b.LeadSwatch = rowCol

	rec.record(t, func() { b.Draw(dst) })
	body, swatch := -1, -1
	for i, c := range rec.calls {
		if !c.Filled {
			continue
		}
		if c.Rect == b.Rect() && body < 0 {
			body = i
		}
		if rgbaEq(c.Color, rowCol) && c.Rect.Dx() == SwatchSizeMD && c.Rect.Min.X == SpaceSM {
			swatch = i
		}
	}
	if body < 0 || swatch < 0 {
		t.Fatalf("want label body and identity swatch drawn; body=%d swatch=%d (calls %+v)", body, swatch, rec.calls)
	}
	if swatch < body {
		t.Fatalf("swatch drawn before the body (%d < %d) — the body would cover it", swatch, body)
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
