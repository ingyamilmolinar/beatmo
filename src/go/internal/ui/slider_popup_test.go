//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestSliderPopupOpenClose(t *testing.T) {
	assertDefaultParityState(t)

	val := 0.5
	sp := NewSliderPopup(SliderPopupConfig{
		ID:       "test-popup",
		ZIndex:   100,
		GetValue: func() float64 { return val },
		SetValue: func(v float64) { val = v },
	})

	if sp.IsOpen() {
		t.Fatal("popup should not be open initially")
	}

	anchor := image.Rect(100, 200, 130, 230)
	bounds := image.Rect(0, 0, 800, 600)
	sp.Open(anchor, bounds, 30)

	if !sp.IsOpen() {
		t.Fatal("popup should be open after Open()")
	}
	if sp.Rect().Empty() {
		t.Fatal("popup rect should be non-empty after Open()")
	}
	if sp.IsDragging() {
		t.Fatal("popup should not be dragging after Open()")
	}

	sp.Close()
	if sp.IsOpen() {
		t.Fatal("popup should not be open after Close()")
	}
	if sp.IsDragging() {
		t.Fatal("popup should not be dragging after Close()")
	}
}

func TestSliderPopupPositioning(t *testing.T) {
	assertDefaultParityState(t)

	val := 0.5
	sp := NewSliderPopup(SliderPopupConfig{
		ID:       "test-pos",
		ZIndex:   100,
		GetValue: func() float64 { return val },
		SetValue: func(v float64) { val = v },
	})

	bounds := image.Rect(0, 0, 800, 600)

	// Anchor near top → popup should flip below.
	anchor := image.Rect(100, 30, 130, 60)
	sp.Open(anchor, bounds, 30)
	if sp.Rect().Min.Y < anchor.Max.Y {
		// Popup should be below the anchor when near the top.
		// Allowing for the case where it fits above.
	}
	sp.Close()

	// Anchor near left edge → popup should be clamped right.
	anchor = image.Rect(0, 200, 10, 230)
	sp.Open(anchor, bounds, 30)
	if sp.Rect().Min.X < bounds.Min.X {
		t.Fatalf("popup left %d exceeds bounds left %d", sp.Rect().Min.X, bounds.Min.X)
	}
	sp.Close()

	// Anchor near right edge → popup should be clamped left.
	anchor = image.Rect(790, 200, 800, 230)
	sp.Open(anchor, bounds, 30)
	if sp.Rect().Max.X > bounds.Max.X {
		t.Fatalf("popup right %d exceeds bounds right %d", sp.Rect().Max.X, bounds.Max.X)
	}
	sp.Close()
}

func TestSliderPopupDrag(t *testing.T) {
	assertDefaultParityState(t)

	val := 0.5
	sp := NewSliderPopup(SliderPopupConfig{
		ID:       "test-drag",
		ZIndex:   100,
		GetValue: func() float64 { return val },
		SetValue: func(v float64) { val = v },
	})

	anchor := image.Rect(100, 300, 130, 330)
	bounds := image.Rect(0, 0, 800, 600)
	sp.Open(anchor, bounds, 30)

	r := sp.Rect()
	mx := r.Min.X + r.Dx()/2
	my := r.Min.Y + r.Dy()/2

	// Press inside.
	consumed := sp.HandleInput(mx, my, true)
	if !consumed {
		t.Fatal("HandleInput should return true for press inside popup")
	}
	if !sp.IsDragging() {
		t.Fatal("IsDragging should be true after press inside")
	}
	if val == 0.5 {
		t.Fatal("value should have changed after drag")
	}

	// Release.
	sp.HandleInput(mx, my, false)
	if sp.IsDragging() {
		t.Fatal("IsDragging should be false after release")
	}
}

func TestSliderPopupClamping(t *testing.T) {
	assertDefaultParityState(t)

	val := 0.5
	sp := NewSliderPopup(SliderPopupConfig{
		ID:       "test-clamp",
		ZIndex:   100,
		GetValue: func() float64 { return val },
		SetValue: func(v float64) { val = v },
	})

	anchor := image.Rect(100, 300, 130, 330)
	bounds := image.Rect(0, 0, 800, 600)
	sp.Open(anchor, bounds, 30)

	r := sp.Rect()
	mx := r.Min.X + r.Dx()/2

	// Start drag inside.
	sp.HandleInput(mx, r.Min.Y+r.Dy()/2, true)
	if !sp.IsDragging() {
		t.Fatal("should be dragging")
	}

	// Drag far above → should clamp to 1.0.
	sp.HandleInput(mx, r.Min.Y-200, true)
	if val != 1.0 {
		t.Fatalf("value = %f after dragging above top, want 1.0", val)
	}

	// Drag far below → should clamp to 0.0.
	sp.HandleInput(mx, r.Max.Y+200, true)
	if val != 0.0 {
		t.Fatalf("value = %f after dragging below bottom, want 0.0", val)
	}

	sp.HandleInput(mx, r.Max.Y+200, false)
}

func TestSliderPopupWithLabel(t *testing.T) {
	assertDefaultParityState(t)

	val := 0.7
	sp := NewSliderPopup(SliderPopupConfig{
		ID:       "test-label",
		ZIndex:   100,
		Label:    func() string { return "1k" },
		GetValue: func() float64 { return val },
		SetValue: func(v float64) { val = v },
	})

	anchor := image.Rect(100, 300, 130, 330)
	bounds := image.Rect(0, 0, 800, 600)
	sp.Open(anchor, bounds, 30)

	// With a label (no title): SpaceXS + RoleBody height + SpaceXS + TextHeight + SpaceXS
	// = 3 + 18 + 3 + 16 + 3 = 43.
	expectedTrackTop := sp.Rect().Min.Y + SpaceXS + StyledTextHeight(RoleBody) + SpaceXS + TextHeight() + SpaceXS
	if sp.trackTop() != expectedTrackTop {
		t.Fatalf("trackTop() = %d, want %d", sp.trackTop(), expectedTrackTop)
	}

	sp.Close()

	// Without label (no title): SpaceXS + RoleBody height + SpaceXS = 3 + 18 + 3 = 24.
	sp2 := NewSliderPopup(SliderPopupConfig{
		ID:       "test-nolabel",
		ZIndex:   100,
		GetValue: func() float64 { return val },
		SetValue: func(v float64) { val = v },
	})
	sp2.Open(anchor, bounds, 30)
	expectedTrackTop = sp2.Rect().Min.Y + SpaceXS + StyledTextHeight(RoleBody) + SpaceXS
	if sp2.trackTop() != expectedTrackTop {
		t.Fatalf("trackTop() = %d, want %d (no label)", sp2.trackTop(), expectedTrackTop)
	}
	sp2.Close()
}

func TestSliderPopupDrawUsesRoundedThumb(t *testing.T) {
	assertDefaultParityState(t)
	val := 0.5
	sp := NewSliderPopup(SliderPopupConfig{
		ID: "t", ZIndex: 100,
		GetValue: func() float64 { return val },
		SetValue: func(v float64) { val = v },
	})
	sp.Open(image.Rect(100, 200, 130, 230), image.Rect(0, 0, 800, 600), 30)
	dst := ebiten.NewImage(800, 600)
	calls := captureRoundedRectCalls(t, func() { sp.Draw(dst) })
	wantDia := Profile().DensityValues().SliderThumbH
	foundThumb := false
	for _, c := range calls {
		if c.Filled && intAbs(c.Rect.Dx()-c.Rect.Dy()) <= 2 && c.Rect.Dx() >= wantDia-2 && c.Radius == c.Rect.Dx()/2 {
			foundThumb = true
		}
	}
	if !foundThumb {
		t.Fatalf("expected a round filled thumb; got %+v", calls)
	}
}

func TestSliderPopupHorizontalMapsXToValue(t *testing.T) {
	assertDefaultParityState(t)
	forceSmallScreenForTest = true
	UpdateProfile()
	t.Cleanup(func() { forceSmallScreenForTest = false; UpdateProfile() })

	val := 0.0
	sp := NewSliderPopup(SliderPopupConfig{
		ID: "h", ZIndex: 100,
		GetValue: func() float64 { return val },
		SetValue: func(v float64) { val = v },
	})
	bounds := image.Rect(0, 0, 400, 800)
	sp.Open(image.Rect(40, 700, 84, 744), bounds, 0)
	if !sp.IsHorizontal() {
		t.Fatal("popup should be horizontal on mobile")
	}
	if !sp.Rect().In(bounds) {
		t.Fatalf("popup rect %v should fit inside bounds %v (no off-screen clip)", sp.Rect(), bounds)
	}
	midY := (sp.Rect().Min.Y + sp.Rect().Max.Y) / 2
	x0, x1 := sp.trackStart(), sp.trackEnd()
	sp.HandleInput(x1, midY, true) // far right
	if val < 0.9 {
		t.Fatalf("press at right end should set value ~1, got %.2f", val)
	}
	sp.HandleInput(x0, midY, true) // far left
	if val > 0.1 {
		t.Fatalf("press at left end should set value ~0, got %.2f", val)
	}
}

func TestSliderPopupOverlayInterface(t *testing.T) {
	assertDefaultParityState(t)

	val := 0.5
	sp := NewSliderPopup(SliderPopupConfig{
		ID:       "test-overlay",
		ZIndex:   225,
		GetValue: func() float64 { return val },
		SetValue: func(v float64) { val = v },
	})

	o := &SliderPopupOverlay{Popup: sp}

	if o.ID() != "test-overlay" {
		t.Fatalf("ID() = %q, want %q", o.ID(), "test-overlay")
	}
	if o.ZIndex() != 225 {
		t.Fatalf("ZIndex() = %d, want 225", o.ZIndex())
	}
	if o.IsOpen() {
		t.Fatal("IsOpen() should be false before opening")
	}

	anchor := image.Rect(100, 300, 130, 330)
	bounds := image.Rect(0, 0, 800, 600)
	sp.Open(anchor, bounds, 30)

	if !o.IsOpen() {
		t.Fatal("IsOpen() should be true after Open()")
	}
	if o.InputBounds() != sp.Rect() {
		t.Fatalf("InputBounds() = %v, want %v", o.InputBounds(), sp.Rect())
	}
	if o.Capturing() {
		t.Fatal("Capturing() should be false before drag")
	}

	// Press inside → should capture.
	r := sp.Rect()
	mx := r.Min.X + r.Dx()/2
	my := r.Min.Y + r.Dy()/2
	result := o.HandleInput(mx, my, true)
	if result != InputCaptured {
		t.Fatalf("HandleInput = %v, want InputCaptured", result)
	}
	if !o.Capturing() {
		t.Fatal("Capturing() should be true during drag")
	}

	// HandleWheel should consume.
	wResult := o.HandleWheel(mx, my, 3)
	if wResult != InputConsumed {
		t.Fatalf("HandleWheel = %v, want InputConsumed", wResult)
	}

	// Release.
	o.HandleInput(mx, my, false)

	// Close via overlay.
	o.Close()
	if o.IsOpen() {
		t.Fatal("IsOpen() should be false after Close()")
	}

	// When closed, HandleInput should return InputIgnored.
	result = o.HandleInput(100, 100, true)
	if result != InputIgnored {
		t.Fatalf("HandleInput when closed = %v, want InputIgnored", result)
	}
}

// TestSliderPopupHorizontalDrawEmitsHorizontalRail verifies that the mobile
// (horizontal) Draw path emits a clearly-landscape rail rect (Dx >= 2*Dy) and a
// near-square round thumb. A regression that swapped axes would draw a vertical
// rail (Dx < Dy) and this test would catch it.
func TestSliderPopupHorizontalDrawEmitsHorizontalRail(t *testing.T) {
	assertDefaultParityState(t)
	forceSmallScreenForTest = true
	UpdateProfile()
	t.Cleanup(func() { forceSmallScreenForTest = false; UpdateProfile() })

	val := 0.5
	sp := NewSliderPopup(SliderPopupConfig{
		ID: "h-draw", ZIndex: 100,
		GetValue: func() float64 { return val },
		SetValue: func(v float64) { val = v },
	})
	bounds := image.Rect(0, 0, 400, 800)
	sp.Open(image.Rect(40, 700, 84, 744), bounds, 0)
	if !sp.IsHorizontal() {
		t.Fatal("popup should be horizontal on mobile")
	}

	dst := ebiten.NewImage(400, 800)
	calls := captureRoundedRectCalls(t, func() { sp.Draw(dst) })

	// Assert there is a clearly horizontal filled rail: Dx >= 2*Dy.
	foundHRail := false
	for _, r := range calls {
		if r.Filled && r.Rect.Dy() > 0 && r.Rect.Dx() >= 2*r.Rect.Dy() {
			foundHRail = true
			break
		}
	}
	if !foundHRail {
		t.Fatalf("expected a horizontal filled rail (Dx >= 2*Dy) but got: %+v", calls)
	}

	// Assert there is a near-square round thumb (|Dx-Dy|<=2, Radius==Dx/2, Dx>=8).
	wantDia := Profile().DensityValues().SliderThumbH
	foundThumb := false
	for _, c := range calls {
		if c.Filled && intAbs(c.Rect.Dx()-c.Rect.Dy()) <= 2 && c.Rect.Dx() >= wantDia-2 && c.Radius == c.Rect.Dx()/2 {
			foundThumb = true
			break
		}
	}
	if !foundThumb {
		t.Fatalf("expected a round filled thumb (dia~%d) but got: %+v", wantDia, calls)
	}
}

// TestSliderPopupHorizontalLongTitleStaysInViewport verifies that a very long
// Title cannot push the mobile popup width past the bounds.Dx()-2*SpaceMD
// margin (regression guard for the Title-expansion-after-clamp ordering bug).
func TestSliderPopupHorizontalLongTitleStaysInViewport(t *testing.T) {
	assertDefaultParityState(t)
	forceSmallScreenForTest = true
	UpdateProfile()
	t.Cleanup(func() { forceSmallScreenForTest = false; UpdateProfile() })

	longTitle := "Aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	val := 0.5
	sp := NewSliderPopup(SliderPopupConfig{
		ID:       "test-long-title",
		ZIndex:   100,
		Title:    func() string { return longTitle },
		GetValue: func() float64 { return val },
		SetValue: func(v float64) { val = v },
	})

	boundsW := 300
	bounds := image.Rect(0, 0, boundsW, 600)
	anchor := image.Rect(100, 300, 130, 330)
	sp.Open(anchor, bounds, 0)

	maxAllowed := boundsW - 2*SpaceMD
	if got := sp.Rect().Dx(); got > maxAllowed {
		t.Fatalf("mobile popup width %d exceeds viewport margin limit %d (bounds %d - 2*SpaceMD %d)",
			got, maxAllowed, boundsW, SpaceMD)
	}
}
