package ui

import (
	"image"
	"testing"
)

func TestSliderFullRange(t *testing.T) {
	assertDefaultParityState(t)
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = false })
	s := NewSlider(0)
	s.SetRect(image.Rect(0, 0, 100, 10))
	centerX := s.Rect().Min.X + s.Rect().Dx()/2
	if s.HandleInputResult(centerX, 5, true) == InputIgnored {
		t.Fatalf("expected handle to start drag")
	}
	s.HandleInputResult(s.Rect().Max.X-1, 5, true)
	if s.Value != 1 {
		t.Fatalf("expected value 1 got %f", s.Value)
	}
	s.HandleInputResult(s.Rect().Min.X, 5, true)
	if s.Value != 0 {
		t.Fatalf("expected value 0 got %f", s.Value)
	}
	s.HandleInputResult(centerX, 5, false)
}
