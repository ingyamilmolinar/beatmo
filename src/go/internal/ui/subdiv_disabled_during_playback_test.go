package ui

import (
	"image"
	"testing"
)

// Changing the subdivision mid-flight is not supported.
func TestSubdivButtonDisabledDuringPlayback(t *testing.T) {
	assertDefaultParityState(t)
	dv := newTestDrumView(t, 1280, 720)

	if dv.subdivBtn().Disabled {
		t.Fatalf("subdiv button should be enabled when stopped")
	}

	dv.SetPlaying(true)
	if !dv.subdivBtn().Disabled {
		t.Fatalf("subdiv button should be disabled during playback")
	}

	dv.SetPlaying(false)
	if dv.subdivBtn().Disabled {
		t.Fatalf("subdiv button should be re-enabled when playback stops")
	}
}

func TestDisabledButtonIgnoresClick(t *testing.T) {
	clicked := 0
	b := NewButton("x", nil, func() { clicked++ })
	b.SetRect(image.Rect(0, 0, 20, 20))
	h := &buttonHitAdapter{btn: b}

	h.OnPress(1, 1)
	if clicked != 1 {
		t.Fatalf("enabled button should fire OnClick: got %d", clicked)
	}

	b.Disabled = true
	h.OnPress(1, 1)
	if clicked != 1 {
		t.Fatalf("disabled button must not fire OnClick: got %d", clicked)
	}
}
