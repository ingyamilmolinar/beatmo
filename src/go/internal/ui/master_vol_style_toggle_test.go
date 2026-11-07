//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// The master volume icon matches the row volume icons: the button-secondary keycap body with the speaker glyph
// and level indicator, no overlay fill bar, the same on both platforms; a second press on any volume icon
// closes its popup.

// --- Helpers ---

type styleDrawCall struct {
	Rect   image.Rectangle
	Color  color.Color
	Filled bool
}

func captureRoundedRect(t *testing.T, fn func()) []styleDrawCall {
	t.Helper()
	var calls []styleDrawCall
	orig := drawRoundedRect
	drawRoundedRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, radius int, filled bool) {
		calls = append(calls, styleDrawCall{Rect: r, Color: c, Filled: filled})
		orig(dst, r, c, radius, filled)
	}
	t.Cleanup(func() { drawRoundedRect = orig })
	fn()
	return calls
}

func colorEqRGB(a, b color.Color) bool {
	ar, ag, ab, _ := a.RGBA()
	br, bg, bb, _ := b.RGBA()
	return ar == br && ag == bg && ab == bb
}

// --- Style consistency tests ---

func TestMasterVolIcon_KeycapBodyOnMobile(t *testing.T) {
	forceSmallScreenForTest = true
	activeProfile = nil
	defer func() {
		forceSmallScreenForTest = false
		activeProfile = nil
	}()
	z, _ := newTestTransportZone()
	z.Layout(image.Rect(0, 0, 720, 240))
	if z.mainVolIconRect.Empty() {
		t.Skip("mainVolIconRect empty after mobile layout")
	}
	if z.mainVolSlider == nil {
		t.Skip("mainVolSlider nil")
	}
	z.mainVolSlider.Value = 0.5
	dst := ebiten.NewImage(720, 240)
	calls := captureRoundedRect(t, func() {
		z.drawMasterVolIconOffset(dst, 0, 0)
	})
	icon := z.mainVolIconRect
	want := Spec(ComponentButtonSecondary).Fill
	body := false
	for _, c := range calls {
		if c.Filled && c.Rect == icon && colorEqRGB(c.Color, want) {
			body = true
		}
		if c.Filled && c.Rect == icon && colorEqRGB(c.Color, colSurface1) {
			t.Fatalf("master vol icon drew a surface-1 body (the bed tone), not the keycap body: %+v", c)
		}
	}
	if !body {
		t.Fatalf("master vol icon drew no keycap body %v at %v: %+v", want, icon, calls)
	}
}

// The level is read in the popup, as on the row icons.
func TestMasterVolIcon_NoOverlayFillBar(t *testing.T) {
	assertDefaultParityState(t)
	z, _ := newTestTransportZone()
	z.Layout(image.Rect(0, 0, 1280, 96))
	if z.mainVolSlider == nil {
		t.Skip("mainVolSlider nil")
	}
	z.mainVolSlider.Value = 0.5

	dst := ebiten.NewImage(1280, 96)
	calls := captureFilledDrawRect(t, func() {
		z.drawMasterVolIconOffset(dst, 0, 0)
	})
	icon := z.mainVolIconRect
	for _, c := range calls {
		if !c.Rect.In(icon) {
			continue
		}
		// A fill bar would be primary-coloured, hugging the bottom of the icon.
		if isPrimaryColor(c.Color) && c.Rect.Max.Y == icon.Max.Y && c.Rect.Min.X == icon.Min.X {
			t.Fatalf("master vol icon drew a proportional overlay fill bar: %+v", c)
		}
		// A short full-width bottom track is the deprecated overlay.
		if c.Rect.Max.Y == icon.Max.Y && c.Rect.Dx() == icon.Dx() && c.Rect.Dy() <= 4 {
			t.Fatalf("master vol icon drew a track overlay bar at the bottom: %+v", c)
		}
	}
}

// TestMasterVolIcon_NoOverlayFillBarFullVol covers the v=1.0 edge case.
func TestMasterVolIcon_NoOverlayFillBarFullVol(t *testing.T) {
	assertDefaultParityState(t)
	z, _ := newTestTransportZone()
	z.Layout(image.Rect(0, 0, 1280, 96))
	if z.mainVolSlider == nil {
		t.Skip("mainVolSlider nil")
	}
	z.mainVolSlider.Value = 1.0

	dst := ebiten.NewImage(1280, 96)
	calls := captureFilledDrawRect(t, func() {
		z.drawMasterVolIconOffset(dst, 0, 0)
	})
	icon := z.mainVolIconRect
	for _, c := range calls {
		if !c.Rect.In(icon) {
			continue
		}
		if isPrimaryColor(c.Color) && c.Rect.Max.Y == icon.Max.Y && c.Rect.Min.X == icon.Min.X {
			t.Fatalf("master vol icon drew a fill bar at full vol: %+v", c)
		}
	}
}

func TestMasterVolIcon_StyleParityWithRowIcon_Mobile(t *testing.T) {
	forceSmallScreenForTest = true
	activeProfile = nil
	defer func() {
		forceSmallScreenForTest = false
		activeProfile = nil
	}()

	z, _ := newTestTransportZone()
	z.Layout(image.Rect(0, 0, 720, 240))
	if z.mainVolIconRect.Empty() || z.mainVolSlider == nil {
		t.Skip("layout not ready")
	}
	z.mainVolSlider.Value = 0.5

	// Both go through drawVolumeButton, so a different count of filled rounded rects means extra chrome on the
	// master path; at least one must draw (the rail).
	dstM := ebiten.NewImage(720, 240)
	roundedM := captureRoundedRect(t, func() {
		z.drawMasterVolIconOffset(dstM, 0, 0)
	})
	dstR := ebiten.NewImage(720, 240)
	roundedR := captureRoundedRect(t, func() {
		drawVolumeButton(dstR, z.mainVolIconRect, 0.5, false, nil)
	})
	countFilledRoundedRects := func(calls []styleDrawCall) int {
		n := 0
		for _, c := range calls {
			if c.Filled {
				n++
			}
		}
		return n
	}
	mCount, rCount := countFilledRoundedRects(roundedM), countFilledRoundedRects(roundedR)
	if mCount != rCount {
		t.Fatalf("master vol icon drew %d filled rounded rect(s); per-row icon draws %d — must match for style parity", mCount, rCount)
	}
	if mCount == 0 {
		t.Fatalf("expected the level indicator to draw a rounded rail; got 0 rounded rects")
	}
}

// --- Toggle behavior tests ---

func TestMasterVolIcon_SecondClickClosesPopupDesktop(t *testing.T) {
	assertDefaultParityState(t)
	forceSmallScreenForTest = false
	activeProfile = nil
	defer func() { activeProfile = nil }()

	dv := NewDrumView(image.Rect(0, 0, 1280, 800), nil, testLogger)
	dv.recalcButtons()
	dv.calcLayout()
	if dv.tree == nil {
		t.Skip("tree not initialized")
	}
	if dv.mainVolIconRect.Empty() {
		t.Skip("mainVolIconRect empty after layout")
	}
	if dv.mainVolSlider() == nil {
		t.Skip("mainVolSlider not initialized")
	}

	clickAtIcon(t, dv, dv.mainVolIconRect)
	if !dv.masterVolPopup.IsOpen() {
		t.Fatal("master vol popup should be open after first click")
	}

	clickAtIcon(t, dv, dv.mainVolIconRect)
	if dv.masterVolPopup.IsOpen() {
		t.Fatal("master vol popup should be closed after second click on icon (toggle)")
	}
}

// On mobile the popup's touch-expanded hit area used to swallow the icon press.
func TestMasterVolIcon_SecondClickClosesPopupMobile(t *testing.T) {
	assertDefaultParityState(t)
	forceSmallScreenForTest = true
	activeProfile = nil
	defer func() {
		forceSmallScreenForTest = false
		activeProfile = nil
	}()

	dv := NewDrumView(image.Rect(0, 0, 420, 840), nil, testLogger)
	dv.recalcButtons()
	dv.calcLayout()
	if dv.tree == nil {
		t.Skip("tree not initialized")
	}
	if dv.mainVolIconRect.Empty() {
		t.Skip("mainVolIconRect empty after layout")
	}
	if dv.mainVolSlider() == nil {
		t.Skip("mainVolSlider not initialized")
	}

	clickAtIcon(t, dv, dv.mainVolIconRect)
	if !dv.masterVolPopup.IsOpen() {
		t.Fatal("master vol popup should be open after first click (mobile)")
	}

	clickAtIcon(t, dv, dv.mainVolIconRect)
	if dv.masterVolPopup.IsOpen() {
		t.Fatal("master vol popup should be closed after second click on icon (mobile toggle)")
	}
}

func TestRowVolIcon_SecondClickClosesPopup(t *testing.T) {
	assertDefaultParityState(t)
	forceSmallScreenForTest = true
	activeProfile = nil
	defer func() {
		forceSmallScreenForTest = false
		activeProfile = nil
	}()

	dv := NewDrumView(image.Rect(0, 0, 420, 840), nil, testLogger)
	dv.recalcButtons()
	dv.calcLayout()
	if dv.tree == nil {
		t.Skip("tree not initialized")
	}
	if len(dv.Rows) == 0 {
		t.Fatal("expected at least one row")
	}
	if len(dv.rowVolSliders()) == 0 || dv.rowVolSliders()[0].Rect().Empty() {
		t.Skip("row vol slider not laid out")
	}

	r := dv.rowVolSliders()[0].Rect()
	clickAtIcon(t, dv, r)
	if !dv.volPopup.IsOpen() {
		t.Fatal("row vol popup should be open after first click")
	}

	clickAtIcon(t, dv, r)
	if dv.volPopup.IsOpen() {
		t.Fatal("row vol popup should be closed after second click on the same icon (toggle)")
	}
}

// Through the real tree input pipeline.
func clickAtIcon(t *testing.T, dv *DrumView, r image.Rectangle) {
	t.Helper()
	if r.Empty() {
		t.Fatal("clickAtIcon: rect empty")
	}
	mx := r.Min.X + r.Dx()/2
	my := r.Min.Y + r.Dy()/2

	// Press frame.
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return dv.Bounds.Dx(), dv.Bounds.Dy() },
	)
	dv.Update()
	restore()

	// Release frame.
	restore = SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return dv.Bounds.Dx(), dv.Bounds.Dy() },
	)
	dv.Update()
	restore()
}
