package ui

import (
	"image"
	"testing"
)

func TestSliderClamp(t *testing.T) {
	s := NewSlider(0)
	s.SetRect(image.Rect(0, 0, 100, 10))
	// start drag inside
	if !s.Handle(1, 5, true) {
		t.Fatalf("expected handle to start drag")
	}
	// drag beyond max width
	s.Handle(150, 5, true)
	if s.Value < 0.99 || s.Value > 1 {
		t.Fatalf("expected value clamped to 1 got %f", s.Value)
	}
	// release
	s.Handle(150, 5, false)
}
