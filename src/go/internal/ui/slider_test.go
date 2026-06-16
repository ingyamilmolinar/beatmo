//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
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

func TestSliderDrawUsesRoundedThumb(t *testing.T) {
	s := NewSlider(0.5)
	s.SetRect(image.Rect(0, 0, 200, 24))
	dst := ebiten.NewImage(200, 24)
	calls := captureRoundedRectCalls(t, func() { s.Draw(dst) })
	foundThumb := false
	for _, c := range calls {
		if c.Filled && intAbs(c.Rect.Dx()-c.Rect.Dy()) <= 2 && c.Radius == c.Rect.Dx()/2 && c.Rect.Dx() >= 8 {
			foundThumb = true
		}
	}
	if !foundThumb {
		t.Fatalf("param slider should draw a round thumb; got %+v", calls)
	}
}

func TestSliderThumbStaysWithinTrackAtExtremes(t *testing.T) {
	for _, v := range []float64{0.0, 1.0} {
		s := NewSlider(v)
		s.SetRect(image.Rect(0, 0, 200, 24))
		dst := ebiten.NewImage(200, 24)
		track := s.TrackRect()
		calls := captureRoundedRectCalls(t, func() { s.Draw(dst) })
		for _, c := range calls {
			// the thumb is the filled near-square rounded rect with radius == Dx/2
			if c.Filled && intAbs(c.Rect.Dx()-c.Rect.Dy()) <= 2 && c.Radius == c.Rect.Dx()/2 && c.Rect.Dx() >= 8 {
				if c.Rect.Min.X < track.Min.X {
					t.Errorf("value=%.1f: thumb left edge %d crosses track.Min.X %d", v, c.Rect.Min.X, track.Min.X)
				}
				if c.Rect.Max.X > track.Max.X {
					t.Errorf("value=%.1f: thumb right edge %d crosses track.Max.X %d", v, c.Rect.Max.X, track.Max.X)
				}
			}
		}
	}
}
