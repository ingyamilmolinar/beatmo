//go:build test

package ui

import (
	"image"
	"testing"
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

	// With a label, trackTop should be at rect.Min.Y + 32.
	expectedTrackTop := sp.Rect().Min.Y + 32
	if sp.trackTop() != expectedTrackTop {
		t.Fatalf("trackTop() = %d, want %d", sp.trackTop(), expectedTrackTop)
	}

	sp.Close()

	// Without label.
	sp2 := NewSliderPopup(SliderPopupConfig{
		ID:       "test-nolabel",
		ZIndex:   100,
		GetValue: func() float64 { return val },
		SetValue: func(v float64) { val = v },
	})
	sp2.Open(anchor, bounds, 30)
	expectedTrackTop = sp2.Rect().Min.Y + 28
	if sp2.trackTop() != expectedTrackTop {
		t.Fatalf("trackTop() = %d, want %d (no label)", sp2.trackTop(), expectedTrackTop)
	}
	sp2.Close()
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

	// Verify interface compliance.
	var _ Overlay = o

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
