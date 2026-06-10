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

func findIndicatorRects(rects []image.Rectangle, iconR image.Rectangle) []image.Rectangle {
	// Indicator rects sit BELOW the icon (Min.Y >= iconR.Max.Y) and within icon's X span.
	var found []image.Rectangle
	for _, r := range rects {
		if r.Min.Y >= iconR.Max.Y && r.Max.X <= iconR.Max.X+1 && r.Min.X >= iconR.Min.X-1 && r.Dy() <= 4 {
			found = append(found, r)
		}
	}
	return found
}

func TestSliderLevelIndicator_OffStateDrawsTrackOnly(t *testing.T) {
	assertDefaultParityState(t)
	dst := ebiten.NewImage(200, 200)
	iconR := indicatorIconRect()

	rects, _ := captureDrawRects(t, func() {
		drawSliderLevelIndicator(dst, image.Rect(iconR.Min.X-10, iconR.Min.Y-10, iconR.Max.X+10, iconR.Max.Y+10), iconR, 0.7, colTextPrimary, true)
	})
	got := findIndicatorRects(rects, iconR)
	if len(got) != 1 {
		t.Fatalf("off-state indicator: want 1 rect (track only), got %d (%v)", len(got), got)
	}
	if got[0].Dx() != iconR.Dx() {
		t.Errorf("off-state track width: want %d, got %d", iconR.Dx(), got[0].Dx())
	}
}

func TestSliderLevelIndicator_ZeroValueDrawsTrackOnly(t *testing.T) {
	assertDefaultParityState(t)
	dst := ebiten.NewImage(200, 200)
	iconR := indicatorIconRect()

	rects, _ := captureDrawRects(t, func() {
		drawSliderLevelIndicator(dst, image.Rect(iconR.Min.X-10, iconR.Min.Y-10, iconR.Max.X+10, iconR.Max.Y+10), iconR, 0.0, colTextPrimary, false)
	})
	got := findIndicatorRects(rects, iconR)
	if len(got) != 1 {
		t.Fatalf("zero-value indicator: want 1 rect (track only), got %d", len(got))
	}
}

func TestSliderLevelIndicator_FullValueDrawsFullFill(t *testing.T) {
	assertDefaultParityState(t)
	dst := ebiten.NewImage(200, 200)
	iconR := indicatorIconRect()

	rects, _ := captureDrawRects(t, func() {
		drawSliderLevelIndicator(dst, image.Rect(iconR.Min.X-10, iconR.Min.Y-10, iconR.Max.X+10, iconR.Max.Y+10), iconR, 1.0, colTextPrimary, false)
	})
	got := findIndicatorRects(rects, iconR)
	if len(got) != 2 {
		t.Fatalf("full-value indicator: want 2 rects (track+fill), got %d (%v)", len(got), got)
	}
	// Last drawn (fill) should span full icon width
	fill := got[len(got)-1]
	if fill.Dx() != iconR.Dx() {
		t.Errorf("full fill width: want %d, got %d", iconR.Dx(), fill.Dx())
	}
}

func TestSliderLevelIndicator_HalfValueDrawsHalfFill(t *testing.T) {
	assertDefaultParityState(t)
	dst := ebiten.NewImage(200, 200)
	iconR := indicatorIconRect()

	rects, _ := captureDrawRects(t, func() {
		drawSliderLevelIndicator(dst, image.Rect(iconR.Min.X-10, iconR.Min.Y-10, iconR.Max.X+10, iconR.Max.Y+10), iconR, 0.5, colTextPrimary, false)
	})
	got := findIndicatorRects(rects, iconR)
	if len(got) != 2 {
		t.Fatalf("half-value indicator: want 2 rects, got %d", len(got))
	}
	fill := got[len(got)-1]
	want := iconR.Dx() / 2
	if abs(fill.Dx()-want) > 1 {
		t.Errorf("half fill width: want ~%d, got %d", want, fill.Dx())
	}
}

func TestSliderLevelIndicator_ClampsValueAbove1(t *testing.T) {
	assertDefaultParityState(t)
	dst := ebiten.NewImage(200, 200)
	iconR := indicatorIconRect()

	rects, _ := captureDrawRects(t, func() {
		drawSliderLevelIndicator(dst, image.Rect(iconR.Min.X-10, iconR.Min.Y-10, iconR.Max.X+10, iconR.Max.Y+10), iconR, 1.5, colTextPrimary, false)
	})
	got := findIndicatorRects(rects, iconR)
	if len(got) != 2 {
		t.Fatalf("clamp-above: want 2 rects, got %d", len(got))
	}
	fill := got[len(got)-1]
	if fill.Dx() != iconR.Dx() {
		t.Errorf("clamp above-1 fill width: want %d, got %d (must not exceed icon)", iconR.Dx(), fill.Dx())
	}
}

func TestSliderLevelIndicator_ClampsValueBelow0(t *testing.T) {
	assertDefaultParityState(t)
	dst := ebiten.NewImage(200, 200)
	iconR := indicatorIconRect()

	rects, _ := captureDrawRects(t, func() {
		drawSliderLevelIndicator(dst, image.Rect(iconR.Min.X-10, iconR.Min.Y-10, iconR.Max.X+10, iconR.Max.Y+10), iconR, -0.3, colTextPrimary, false)
	})
	got := findIndicatorRects(rects, iconR)
	if len(got) != 1 {
		t.Fatalf("clamp-below: want 1 rect (no fill), got %d", len(got))
	}
}

func TestSliderLevelIndicator_EmptyRectIsNoop(t *testing.T) {
	assertDefaultParityState(t)
	dst := ebiten.NewImage(200, 200)

	rects, _ := captureDrawRects(t, func() {
		drawSliderLevelIndicator(dst, image.Rect(0, 0, 100, 100), image.Rectangle{}, 0.7, colTextPrimary, false)
	})
	if len(rects) != 0 {
		t.Fatalf("empty-rect indicator: want 0 draw calls, got %d", len(rects))
	}
}

func TestSliderLevelIndicator_PositionedBelowIcon(t *testing.T) {
	assertDefaultParityState(t)
	dst := ebiten.NewImage(200, 200)
	iconR := indicatorIconRect()

	rects, _ := captureDrawRects(t, func() {
		drawSliderLevelIndicator(dst, image.Rect(iconR.Min.X-10, iconR.Min.Y-10, iconR.Max.X+10, iconR.Max.Y+10), iconR, 0.5, colTextPrimary, false)
	})
	got := findIndicatorRects(rects, iconR)
	if len(got) == 0 {
		t.Fatal("no indicator rects drawn")
	}
	track := got[0]
	if track.Min.Y < iconR.Max.Y {
		t.Errorf("indicator must sit beneath icon: track Min.Y=%d, iconR.Max.Y=%d", track.Min.Y, iconR.Max.Y)
	}
	if track.Min.Y-iconR.Max.Y > 6 {
		t.Errorf("indicator gap too large: %dpx (want <=6)", track.Min.Y-iconR.Max.Y)
	}
	if track.Dy() < 1 || track.Dy() > 3 {
		t.Errorf("indicator thickness %dpx is not thin (want 1-3)", track.Dy())
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

	rects, _ := captureDrawRects(t, func() {
		drawSliderLevelIndicator(dst, btnR, iconR, 0.5, colTextPrimary, false)
	})
	if len(rects) == 0 {
		t.Fatal("indicator should still render inside a tight button rect")
	}
	for i, r := range rects {
		if r.Max.Y > btnR.Max.Y {
			t.Errorf("indicator rect %d y=%d exceeds btnR.Max.Y=%d (would be clipped by surface)", i, r.Max.Y, btnR.Max.Y)
		}
		if r.Min.Y < btnR.Min.Y {
			t.Errorf("indicator rect %d y=%d above btnR.Min.Y=%d", i, r.Min.Y, btnR.Min.Y)
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
	rectsA, _ := captureDrawRects(t, func() {
		drawVolIcon(dstA, s, vol, false, nil)
	})

	dstB := ebiten.NewImage(200, 200)
	tz, _ := newTestTransportZone()
	tz.mainVolIconRect = btnR
	if tz.mainVolSlider == nil {
		tz.mainVolSlider = NewSlider(vol)
	}
	tz.mainVolSlider.Value = vol
	rectsB, _ := captureDrawRects(t, func() {
		tz.drawMasterVolIconOffset(dstB, 0, 0)
	})

	if len(rectsA) != len(rectsB) {
		t.Fatalf("tight-layout draw-call count mismatch: row=%d, master=%d", len(rectsA), len(rectsB))
	}
	for i := range rectsA {
		if rectsA[i] != rectsB[i] {
			t.Errorf("tight-layout rect[%d] mismatch: row=%v master=%v", i, rectsA[i], rectsB[i])
		}
	}
	// And: at least one indicator rect must be inside the tight btnR.
	indicatorFound := 0
	for _, r := range rectsA {
		if r.Dy() <= 3 && r.Min.Y > btnR.Min.Y+btnR.Dy()/2 && r.Max.Y <= btnR.Max.Y {
			indicatorFound++
		}
	}
	if indicatorFound < 1 {
		t.Errorf("tight-layout: expected at least one indicator rect inside btnR; got 0 (rects=%v)", rectsA)
	}
}

func TestSliderLevelIndicator_TooSmallIconIsNoop(t *testing.T) {
	assertDefaultParityState(t)
	dst := ebiten.NewImage(200, 200)
	tinyR := image.Rect(0, 0, 3, 3)

	rects, _ := captureDrawRects(t, func() {
		drawSliderLevelIndicator(dst, image.Rect(0, 0, 100, 100), tinyR, 0.7, colTextPrimary, false)
	})
	if len(rects) != 0 {
		t.Fatalf("tiny-icon indicator should be no-op, got %d rects", len(rects))
	}
}

// Integration: drawVolIcon (per-row) calls the indicator below the speaker glyph.
func TestDrawVolIcon_DrawsIndicatorBeneathIcon(t *testing.T) {
	assertDefaultParityState(t)
	dst := ebiten.NewImage(400, 100)
	s := NewSlider(0.6)
	s.SetRect(image.Rect(0, 0, 60, 60)) // generous so icon is large enough

	rects, _ := captureDrawRects(t, func() {
		drawVolIcon(dst, s, 0.6, false, nil)
	})
	// At least one rect below the vertical mid-line of the slider's rect.
	midY := 30
	belowCount := 0
	for _, r := range rects {
		if r.Min.Y > midY && r.Dy() <= 4 {
			belowCount++
		}
	}
	if belowCount < 1 {
		t.Fatalf("drawVolIcon vol=0.6 must draw indicator below mid-line; got 0 thin rects below y=%d (total %d rects)", midY, len(rects))
	}
}

func TestDrawVolIcon_MutedDrawsIndicatorButNoFill(t *testing.T) {
	assertDefaultParityState(t)
	dst := ebiten.NewImage(400, 100)
	s := NewSlider(0.6)
	s.SetRect(image.Rect(0, 0, 60, 60))

	rects, _ := captureDrawRects(t, func() {
		drawVolIcon(dst, s, 0.6, true /* muted */, nil)
	})
	// Count thin rects below mid-line — muted should still show the track (1) but no fill.
	midY := 30
	belowThin := 0
	for _, r := range rects {
		if r.Min.Y > midY && r.Dy() <= 4 {
			belowThin++
		}
	}
	if belowThin != 1 {
		t.Errorf("muted vol icon: want exactly 1 thin rect (track only), got %d", belowThin)
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
	rects, _ := captureDrawRects(t, func() {
		tz.drawMasterVolIconOffset(dst, 0, 0)
	})
	midY := 130
	belowThin := 0
	for _, r := range rects {
		if r.Min.Y > midY && r.Dy() <= 4 {
			belowThin++
		}
	}
	if belowThin < 1 {
		t.Fatalf("master vol icon should draw an indicator; got 0 thin rects below y=%d (total %d rects)", midY, len(rects))
	}
}
