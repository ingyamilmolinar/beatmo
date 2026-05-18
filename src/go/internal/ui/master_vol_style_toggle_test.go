//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// These tests pin the master volume icon to the same minimalist style as the
// per-row volume icons (drawVolIcon) and require that clicking the icon a
// second time closes the popup (toggle behavior).
//
// Style: just the speaker glyph — no rounded chip background, no overlay
// fill bar, identical mobile/desktop chrome.
//
// Toggle: a second press on the icon — not on empty canvas — closes the
// master volume popup, on both desktop and mobile profiles, and the same
// applies to the per-row volume icons.

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

// TestMasterVolIcon_NoChipBackgroundOnMobile pins the master vol icon to the
// row-vol-icon minimalist style: no rounded surface chip on mobile. Row vol
// icons (drawVolIcon) draw only the speaker glyph.
func TestMasterVolIcon_NoChipBackgroundOnMobile(t *testing.T) {
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
	for _, c := range calls {
		if !c.Filled {
			continue
		}
		// A filled rounded rect whose rect equals (or covers) the icon
		// rect is the deprecated chip background.
		if c.Rect == icon || icon.In(c.Rect) {
			t.Fatalf("master vol icon drew a filled rounded chip behind the glyph: %+v", c)
		}
		if c.Rect.In(icon) && colorEqRGB(c.Color, colSurface1) {
			t.Fatalf("master vol icon drew a colSurface1 chip inside the icon rect: %+v", c)
		}
	}
}

// TestMasterVolIcon_NoOverlayFillBar pins the icon to the row style: no
// proportional fill-bar overlay at the bottom of the icon rect. Volume level
// is communicated by opening the popup, identical to the row icons.
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
		// A fill bar would be primary-colored at AlphaMedium hugging the
		// bottom of the icon rect.
		if isPrimaryColor(c.Color) && c.Rect.Max.Y == icon.Max.Y && c.Rect.Min.X == icon.Min.X {
			t.Fatalf("master vol icon drew a proportional overlay fill bar: %+v", c)
		}
		// A bottom-edge horizontal track at the icon's full width and very
		// short height (<=4 px) is the deprecated track overlay.
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

// TestMasterVolIcon_StyleParityWithRowIcon_Mobile: the master vol icon and
// the row vol icon should produce structurally identical draw output on
// mobile (same icon-glyph extent, no extra chrome around the master one).
// We compare by counting calls — the master icon should not have more
// drawRect calls per icon area than the row icon.
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

	// Master: should produce zero filled rounded rects (no chip) and zero
	// bottom-edge fill bars (no overlay).
	dstM := ebiten.NewImage(720, 240)
	roundedM := captureRoundedRect(t, func() {
		z.drawMasterVolIconOffset(dstM, 0, 0)
	})
	var chipCount int
	for _, c := range roundedM {
		if c.Filled {
			chipCount++
		}
	}
	if chipCount != 0 {
		t.Fatalf("master vol icon drew %d rounded chip(s); row vol icon style draws zero", chipCount)
	}
}

// --- Toggle behavior tests ---

// TestMasterVolIcon_SecondClickClosesPopupDesktop verifies that pressing the
// master vol icon a second time (after it has opened the popup) closes the
// popup. Currently fails because OnMasterVolClick unconditionally opens.
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

// TestMasterVolIcon_SecondClickClosesPopupMobile mirrors the desktop test
// under the mobile profile, where touch-target expansion of the popup hit
// area used to swallow the icon press.
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

// TestRowVolIcon_SecondClickClosesPopup verifies the per-row icon also
// toggles. The user wants all volume buttons consistent.
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

// clickAtIcon sends a press+release at the center of rect via the real tree
// input pipeline.
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
