//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// Each drum-pane treatment is token-driven and reuses existing alpha buckets; these pin that it actually draws.

// The recorder stores premultiplied color.RGBA, so NRGBA tokens are compared in that space.
func rgbaEq(a, b color.Color) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	ar := color.RGBAModel.Convert(a).(color.RGBA)
	br := color.RGBAModel.Convert(b).(color.RGBA)
	return ar == br
}

// Nothing is 3D: one fill in the instrument colour and the 1 px border, no gradient bands or lit strip.
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

// So the row reads as an 808 step row.
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

// Drawn AFTER the opaque label body: anything painted before it is covered and the rack shows no identity.
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

// The bright window reads as a window onto a recessed warm track. The fill is baked into TlCache, so it is caught
// during a full Game.Draw.
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

// It stays findable against the warm ticks.
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
