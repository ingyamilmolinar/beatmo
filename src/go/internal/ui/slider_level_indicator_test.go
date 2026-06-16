//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// captureDrawRects swaps the package-level drawRect var for one that records
// every filled rect drawn during fn(). Returns recorded rects and the map of
// the color used for each rect (one slice entry per drawRect call).
func captureDrawRects(t *testing.T, fn func()) ([]image.Rectangle, []color.Color) {
	t.Helper()
	var rects []image.Rectangle
	var cols []color.Color
	orig := drawRect
	drawRect = func(d *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		if filled {
			rects = append(rects, r)
			cols = append(cols, c)
		}
		orig(d, r, c, filled)
	}
	t.Cleanup(func() { drawRect = orig })
	fn()
	return rects, cols
}

// indicatorIconRect is a stand-in icon rect used by helper tests.
func indicatorIconRect() image.Rectangle { return image.Rect(20, 30, 60, 70) } // 40x40 icon

// findIndicatorRoundedRects filters captureRoundedRectCalls results to
// the filled rounded-rect calls that correspond to the indicator rail/fill:
// they sit at or below iconR.Max.Y, within icon's X span, and are thin (<=6px).
func findIndicatorRoundedRects(calls []roundedDraw, iconR image.Rectangle) []roundedDraw {
	var found []roundedDraw
	for _, c := range calls {
		if !c.Filled {
			continue
		}
		if c.Rect.Min.Y >= iconR.Max.Y &&
			c.Rect.Max.X <= iconR.Max.X+1 &&
			c.Rect.Min.X >= iconR.Min.X-1 &&
			c.Rect.Dy() <= 6 {
			found = append(found, c)
		}
	}
	return found
}

func TestSliderLevelIndicator_OffStateDrawsTrackOnly(t *testing.T) {
	assertDefaultParityState(t)
	dst := ebiten.NewImage(200, 200)
	iconR := indicatorIconRect()
	btnR := image.Rect(iconR.Min.X-10, iconR.Min.Y-10, iconR.Max.X+10, iconR.Max.Y+10)

	calls := captureRoundedRectCalls(t, func() {
		drawSliderLevelIndicator(dst, btnR, iconR, 0.7, colTextPrimary, true)
	})
	got := findIndicatorRoundedRects(calls, iconR)
	if countFilled(got) != 1 {
		t.Fatalf("off-state indicator: want 1 filled rounded rect (track only), got %d (%v)", countFilled(got), got)
	}
	// Track should span full icon width.
	if got[0].Rect.Dx() != iconR.Dx() {
		t.Errorf("off-state track width: want %d, got %d", iconR.Dx(), got[0].Rect.Dx())
	}
}

func TestSliderLevelIndicator_ZeroValueDrawsTrackOnly(t *testing.T) {
	assertDefaultParityState(t)
	dst := ebiten.NewImage(200, 200)
	iconR := indicatorIconRect()
	btnR := image.Rect(iconR.Min.X-10, iconR.Min.Y-10, iconR.Max.X+10, iconR.Max.Y+10)

	calls := captureRoundedRectCalls(t, func() {
		drawSliderLevelIndicator(dst, btnR, iconR, 0.0, colTextPrimary, false)
	})
	got := findIndicatorRoundedRects(calls, iconR)
	if countFilled(got) != 1 {
		t.Fatalf("zero-value indicator: want 1 filled rounded rect (track only), got %d", countFilled(got))
	}
}

func TestSliderLevelIndicator_FullValueDrawsRailAndFill(t *testing.T) {
	assertDefaultParityState(t)
	dst := ebiten.NewImage(200, 200)
	iconR := indicatorIconRect()
	btnR := image.Rect(iconR.Min.X-10, iconR.Min.Y-10, iconR.Max.X+10, iconR.Max.Y+10)

	calls := captureRoundedRectCalls(t, func() {
		drawSliderLevelIndicator(dst, btnR, iconR, 1.0, colTextPrimary, false)
	})
	got := findIndicatorRoundedRects(calls, iconR)
	if countFilled(got) < 2 {
		t.Fatalf("full value should draw rail + fill (>=2 filled rounded rects), got %d", countFilled(got))
	}
}

func TestSliderLevelIndicator_HalfValueDrawsHalfFill(t *testing.T) {
	assertDefaultParityState(t)
	dst := ebiten.NewImage(200, 200)
	iconR := indicatorIconRect()
	btnR := image.Rect(iconR.Min.X-10, iconR.Min.Y-10, iconR.Max.X+10, iconR.Max.Y+10)

	calls := captureRoundedRectCalls(t, func() {
		drawSliderLevelIndicator(dst, btnR, iconR, 0.5, colTextPrimary, false)
	})
	got := findIndicatorRoundedRects(calls, iconR)
	if countFilled(got) < 2 {
		t.Fatalf("half-value indicator: want >=2 filled rounded rects, got %d", countFilled(got))
	}
	// The fill (second filled) should be roughly half the icon width.
	// Find the narrowest filled rect that is less than full width (the fill).
	want := iconR.Dx() / 2
	var fillW int
	for _, c := range got {
		if c.Rect.Dx() < iconR.Dx() {
			fillW = c.Rect.Dx()
		}
	}
	if abs(fillW-want) > 2 {
		t.Errorf("half fill width: want ~%d, got %d", want, fillW)
	}
}

// TestVolIconColorMatchesInstrumentNodeColor pins that an active per-row volume
// slider paints the SAME color as that instrument's grid nodes. The node fill
// for a regular node is the row's Color at full opacity (grid_pane_draw.go:
// fillCol = base, where base = g.drum.Rows[rowIdx].Color). DrumRow.Color is the
// single source of truth both surfaces read live, so they must agree exactly —
// no alpha dimming on the slider that would make it read as a different shade.
func TestVolIconColorMatchesInstrumentNodeColor(t *testing.T) {
	assertDefaultParityState(t)
	rowColor := color.RGBA{R: 220, G: 40, B: 90, A: 255}
	got := volIconColor(0.8, false, rowColor)
	wr, wg, wb, wa := rgba8(rowColor)
	gr, gg, gb, ga := rgba8(got)
	if gr != wr || gg != wg || gb != wb || ga != wa {
		t.Errorf("volIconColor active: want node color (%d,%d,%d,%d), got (%d,%d,%d,%d)",
			wr, wg, wb, wa, gr, gg, gb, ga)
	}
	// Muted / zero volume still reads as the disabled (silent) color.
	if got := volIconColor(0, false, rowColor); got != colTextDisabled {
		t.Errorf("volIconColor zero: want colTextDisabled, got %v", got)
	}
	if got := volIconColor(0.8, true, rowColor); got != colTextDisabled {
		t.Errorf("volIconColor muted: want colTextDisabled, got %v", got)
	}
}

// TestSliderLevelIndicator_NRGBAFillUsesIconColor pins the contract that the
// fill carries the icon's foreground color even when that color arrives as a
// color.NRGBA (the common case: per-row volume cells get their tint from
// WithAlphaFromColor, which returns color.NRGBA). A naive
// fgCol.(color.RGBA) assertion would miss this and silently substitute the
// cyan fallback — so we assert the fill is NOT the fallback and DOES match
// the converted icon color.
func TestSliderLevelIndicator_NRGBAFillUsesIconColor(t *testing.T) {
	assertDefaultParityState(t)
	dst := ebiten.NewImage(200, 200)
	iconR := indicatorIconRect()
	btnR := image.Rect(iconR.Min.X-10, iconR.Min.Y-10, iconR.Max.X+10, iconR.Max.Y+10)

	// A distinctly non-cyan row tint, delivered as color.NRGBA (as production does).
	iconCol := WithAlphaFromColor(color.RGBA{R: 220, G: 40, B: 90, A: 255}, AlphaOverlay)
	wantR, wantG, wantB, _ := color.RGBAModel.Convert(iconCol).RGBA()

	calls := captureRoundedRectCalls(t, func() {
		drawSliderLevelIndicator(dst, btnR, iconR, 0.8, iconCol, false)
	})
	got := findIndicatorRoundedRects(calls, iconR)
	if countFilled(got) < 2 {
		t.Fatalf("NRGBA fill: want rail+fill (>=2 filled), got %d", countFilled(got))
	}
	// The fill is the narrowest filled rect (< full icon width); its color must
	// be the converted icon color, not the cyan fallback genColorPrimary.
	var fillCol color.Color
	for _, c := range got {
		if c.Rect.Dx() < iconR.Dx() {
			fillCol = c.Color
		}
	}
	if fillCol == nil {
		t.Fatal("NRGBA fill: no partial-width fill rect found")
	}
	cr, cg, cb, _ := genColorPrimary.RGBA()
	gr, gg, gb, _ := fillCol.RGBA()
	if gr == cr && gg == cg && gb == cb {
		t.Fatalf("NRGBA fill: fell back to cyan genColorPrimary instead of the row color")
	}
	if gr != wantR || gg != wantG || gb != wantB {
		t.Errorf("NRGBA fill: color mismatch: want (%d,%d,%d), got (%d,%d,%d)",
			wantR>>8, wantG>>8, wantB>>8, gr>>8, gg>>8, gb>>8)
	}
}

func TestSliderLevelIndicator_ClampsValueAbove1(t *testing.T) {
	assertDefaultParityState(t)
	dst := ebiten.NewImage(200, 200)
	iconR := indicatorIconRect()
	btnR := image.Rect(iconR.Min.X-10, iconR.Min.Y-10, iconR.Max.X+10, iconR.Max.Y+10)

	calls := captureRoundedRectCalls(t, func() {
		drawSliderLevelIndicator(dst, btnR, iconR, 1.5, colTextPrimary, false)
	})
	got := findIndicatorRoundedRects(calls, iconR)
	if countFilled(got) < 2 {
		t.Fatalf("clamp-above: want >=2 filled rounded rects, got %d", countFilled(got))
	}
	// The fill must not exceed icon width.
	for _, c := range got {
		if c.Rect.Dx() > iconR.Dx() {
			t.Errorf("clamp above-1: fill width %d exceeds icon width %d", c.Rect.Dx(), iconR.Dx())
		}
	}
}

func TestSliderLevelIndicator_ClampsValueBelow0(t *testing.T) {
	assertDefaultParityState(t)
	dst := ebiten.NewImage(200, 200)
	iconR := indicatorIconRect()
	btnR := image.Rect(iconR.Min.X-10, iconR.Min.Y-10, iconR.Max.X+10, iconR.Max.Y+10)

	calls := captureRoundedRectCalls(t, func() {
		drawSliderLevelIndicator(dst, btnR, iconR, -0.3, colTextPrimary, false)
	})
	got := findIndicatorRoundedRects(calls, iconR)
	if countFilled(got) != 1 {
		t.Fatalf("clamp-below: want 1 filled rounded rect (no fill), got %d", countFilled(got))
	}
}

func TestSliderLevelIndicator_EmptyRectIsNoop(t *testing.T) {
	assertDefaultParityState(t)
	dst := ebiten.NewImage(200, 200)

	calls := captureRoundedRectCalls(t, func() {
		drawSliderLevelIndicator(dst, image.Rect(0, 0, 100, 100), image.Rectangle{}, 0.7, colTextPrimary, false)
	})
	if countFilled(calls) != 0 {
		t.Fatalf("empty-rect indicator: want 0 draw calls, got %d filled rounded rects", countFilled(calls))
	}
}

func TestSliderLevelIndicator_PositionedBelowIcon(t *testing.T) {
	assertDefaultParityState(t)
	dst := ebiten.NewImage(200, 200)
	iconR := indicatorIconRect()
	btnR := image.Rect(iconR.Min.X-10, iconR.Min.Y-10, iconR.Max.X+10, iconR.Max.Y+10)

	calls := captureRoundedRectCalls(t, func() {
		drawSliderLevelIndicator(dst, btnR, iconR, 0.5, colTextPrimary, false)
	})
	got := findIndicatorRoundedRects(calls, iconR)
	if len(got) == 0 {
		t.Fatal("no indicator rounded rects drawn")
	}
	track := got[0]
	if track.Rect.Min.Y < iconR.Max.Y {
		t.Errorf("indicator must sit beneath icon: track Min.Y=%d, iconR.Max.Y=%d", track.Rect.Min.Y, iconR.Max.Y)
	}
	if track.Rect.Min.Y-iconR.Max.Y > 6 {
		t.Errorf("indicator gap too large: %dpx (want <=6)", track.Rect.Min.Y-iconR.Max.Y)
	}
	if track.Rect.Dy() < 1 || track.Rect.Dy() > 6 {
		t.Errorf("indicator thickness %dpx is out of range (want 1-6)", track.Rect.Dy())
	}
}

// TestSliderLevelIndicator_StaysInsideBtnRect pins the contract that the
// indicator never draws outside the button's hit-area — otherwise a tightly
// laid-out toolbar (master volume on dense desktop) clips it and the user
// sees no level marker at all.
func TestSliderLevelIndicator_StaysInsideBtnRect(t *testing.T) {
	assertDefaultParityState(t)
	dst := ebiten.NewImage(200, 200)
	// Tight button: 32x32, icon centered, only ~6px below icon for the bar.
	btnR := image.Rect(50, 50, 82, 82)
	iconH := 32 * 60 / 100 // 19
	cx := btnR.Min.X + btnR.Dx()/2
	cy := btnR.Min.Y + btnR.Dy()/2
	iconR := image.Rect(cx-iconH/2, cy-iconH/2, cx+iconH/2, cy+iconH/2)

	calls := captureRoundedRectCalls(t, func() {
		drawSliderLevelIndicator(dst, btnR, iconR, 0.5, colTextPrimary, false)
	})
	got := findIndicatorRoundedRects(calls, iconR)
	if len(got) == 0 {
		t.Fatal("indicator should still render inside a tight button rect")
	}
	for i, c := range got {
		if c.Rect.Max.Y > btnR.Max.Y {
			t.Errorf("indicator rect %d y=%d exceeds btnR.Max.Y=%d (would be clipped by surface)", i, c.Rect.Max.Y, btnR.Max.Y)
		}
		if c.Rect.Min.Y < btnR.Min.Y {
			t.Errorf("indicator rect %d y=%d above btnR.Min.Y=%d", i, c.Rect.Min.Y, btnR.Min.Y)
		}
	}
}

// TestVolumeButton_RowAndMasterRenderIdentically_TightLayout repeats the
// row vs master parity check for a small, dense button — catches
// path-divergence that only shows up in real toolbar sizes.
func TestVolumeButton_RowAndMasterRenderIdentically_TightLayout(t *testing.T) {
	assertDefaultParityState(t)
	btnR := image.Rect(10, 10, 42, 42) // 32x32
	const vol = 0.6

	dstA := ebiten.NewImage(200, 200)
	s := NewSlider(vol)
	s.SetRect(btnR)

	dstB := ebiten.NewImage(200, 200)
	tz, _ := newTestTransportZone()
	tz.mainVolIconRect = btnR
	if tz.mainVolSlider == nil {
		tz.mainVolSlider = NewSlider(vol)
	}
	tz.mainVolSlider.Value = vol

	// The indicator (and rail/fill) is drawn via drawRoundedRect; compare the
	// full rounded-rect call sequence (geometry + color + radius + filled) so
	// the two paths render byte-identically — not just "both nonzero".
	callsA := captureRoundedRectCalls(t, func() {
		drawVolIcon(dstA, s, vol, false, nil)
	})
	callsB := captureRoundedRectCalls(t, func() {
		tz.drawMasterVolIconOffset(dstB, 0, 0)
	})

	if len(callsA) != len(callsB) {
		t.Fatalf("tight-layout rounded-rect count mismatch: row=%d, master=%d", len(callsA), len(callsB))
	}
	for i := range callsA {
		if callsA[i] != callsB[i] {
			t.Errorf("tight-layout rounded rect[%d] mismatch: row=%v master=%v", i, callsA[i], callsB[i])
		}
	}

	iconH := 32 * 60 / 100 // 19
	cx := btnR.Min.X + btnR.Dx()/2
	cy := btnR.Min.Y + btnR.Dy()/2
	iconR := image.Rect(cx-iconH/2, cy-iconH/2, cx+iconH/2, cy+iconH/2)
	if len(findIndicatorRoundedRects(callsA, iconR)) < 1 {
		t.Errorf("tight-layout: expected at least one indicator rounded rect inside btnR; got 0")
	}
}

func TestSliderLevelIndicator_TooSmallIconIsNoop(t *testing.T) {
	assertDefaultParityState(t)
	dst := ebiten.NewImage(200, 200)
	tinyR := image.Rect(0, 0, 3, 3)

	calls := captureRoundedRectCalls(t, func() {
		drawSliderLevelIndicator(dst, image.Rect(0, 0, 100, 100), tinyR, 0.7, colTextPrimary, false)
	})
	if countFilled(calls) != 0 {
		t.Fatalf("tiny-icon indicator should be no-op, got %d filled rounded rects", countFilled(calls))
	}
}

// Integration: drawVolIcon (per-row) calls the indicator below the speaker glyph.
func TestDrawVolIcon_DrawsIndicatorBeneathIcon(t *testing.T) {
	assertDefaultParityState(t)
	dst := ebiten.NewImage(400, 100)
	s := NewSlider(0.6)
	s.SetRect(image.Rect(0, 0, 60, 60)) // generous so icon is large enough

	// The indicator is now drawn via drawRoundedRect (via drawSliderRail).
	// Capture rounded rects and look for thin ones below the vertical midpoint.
	midY := 30
	calls := captureRoundedRectCalls(t, func() {
		drawVolIcon(dst, s, 0.6, false, nil)
	})
	belowCount := 0
	for _, c := range calls {
		if c.Filled && c.Rect.Min.Y > midY && c.Rect.Dy() <= 6 {
			belowCount++
		}
	}
	if belowCount < 1 {
		t.Fatalf("drawVolIcon vol=0.6 must draw indicator below mid-line via drawRoundedRect; got 0 thin filled rects below y=%d (total calls %d)", midY, len(calls))
	}
}

func TestDrawVolIcon_MutedDrawsIndicatorButNoFill(t *testing.T) {
	assertDefaultParityState(t)
	dst := ebiten.NewImage(400, 100)
	s := NewSlider(0.6)
	s.SetRect(image.Rect(0, 0, 60, 60))

	// Muted: indicator is present (track/rail) but no fill — so exactly 1 thin
	// filled rounded rect (the rail) below mid-line.
	midY := 30
	calls := captureRoundedRectCalls(t, func() {
		drawVolIcon(dst, s, 0.6, true /* muted */, nil)
	})
	belowThin := 0
	for _, c := range calls {
		if c.Filled && c.Rect.Min.Y > midY && c.Rect.Dy() <= 6 {
			belowThin++
		}
	}
	if belowThin != 1 {
		t.Errorf("muted vol icon: want exactly 1 thin filled rounded rect (rail only), got %d", belowThin)
	}
}

// TestVolumeButton_RowAndMasterRenderIdentically pins the contract that the
// master volume button is the same component as the per-row volume button,
// hooked to a different channel. Same input rect + level + neutral state must
// yield identical draw calls (count, geometry, color).
func TestVolumeButton_RowAndMasterRenderIdentically(t *testing.T) {
	assertDefaultParityState(t)
	btnR := image.Rect(20, 20, 80, 80)
	const vol = 0.6

	// Path A: per-row, no row tint, not muted (matches master semantics).
	dstA := ebiten.NewImage(200, 200)
	s := NewSlider(vol)
	s.SetRect(btnR)
	rectsA, colsA := captureDrawRects(t, func() {
		drawVolIcon(dstA, s, vol, false, nil)
	})

	// Path B: master volume.
	dstB := ebiten.NewImage(200, 200)
	tz, _ := newTestTransportZone()
	tz.mainVolIconRect = btnR
	if tz.mainVolSlider == nil {
		tz.mainVolSlider = NewSlider(vol)
	}
	tz.mainVolSlider.Value = vol
	rectsB, colsB := captureDrawRects(t, func() {
		tz.drawMasterVolIconOffset(dstB, 0, 0)
	})

	if len(rectsA) != len(rectsB) {
		t.Fatalf("draw-call count mismatch: row=%d, master=%d (must be the same component)", len(rectsA), len(rectsB))
	}
	for i := range rectsA {
		if rectsA[i] != rectsB[i] {
			t.Errorf("rect[%d] mismatch: row=%v master=%v", i, rectsA[i], rectsB[i])
		}
		ar, ag, ab, aa := colsA[i].RGBA()
		br, bg, bb, ba := colsB[i].RGBA()
		if ar != br || ag != bg || ab != bb || aa != ba {
			t.Errorf("color[%d] mismatch: row=(%d,%d,%d,%d) master=(%d,%d,%d,%d)",
				i, ar>>8, ag>>8, ab>>8, aa>>8, br>>8, bg>>8, bb>>8, ba>>8)
		}
	}
}

// Integration: drawMasterVolIconOffset draws indicator on the master volume icon.
func TestDrawMasterVolIcon_DrawsIndicator(t *testing.T) {
	assertDefaultParityState(t)
	tz, _ := newTestTransportZone()
	tz.mainVolIconRect = image.Rect(100, 100, 160, 160)
	if tz.mainVolSlider == nil {
		tz.mainVolSlider = NewSlider(0.5)
	}
	tz.mainVolSlider.Value = 0.5

	dst := ebiten.NewImage(400, 400)
	midY := 130
	calls := captureRoundedRectCalls(t, func() {
		tz.drawMasterVolIconOffset(dst, 0, 0)
	})
	belowThin := 0
	for _, c := range calls {
		if c.Filled && c.Rect.Min.Y > midY && c.Rect.Dy() <= 6 {
			belowThin++
		}
	}
	if belowThin < 1 {
		t.Fatalf("master vol icon should draw an indicator via drawRoundedRect; got 0 thin filled rects below y=%d (total calls %d)", midY, len(calls))
	}
}
