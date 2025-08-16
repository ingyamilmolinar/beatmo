package ui

import (
	"image"
	"testing"
)

func TestSliderFullRange(t *testing.T) {
	s := NewSlider(0)
	s.SetRect(image.Rect(0, 0, 100, 10))
	if !s.Handle(1, 5, true) {
		t.Fatalf("expected handle to start drag")
	}
	s.Handle(99, 5, true)
	if s.Value != 1 {
		t.Fatalf("expected value 1 got %f", s.Value)
	}
	s.Handle(0, 5, true)
	if s.Value != 0 {
		t.Fatalf("expected value 0 got %f", s.Value)
	}
	s.Handle(0, 5, false)
}
