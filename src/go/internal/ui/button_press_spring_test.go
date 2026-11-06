package ui

import (
	"image"
	"testing"
)

// TestButtonPressDepthSpring pins the press/release spring that survived the
// flat-tonal restyle. The spring no longer drives cap TRAVEL (there is no cap
// — see button_flat.go), but it is still the button's settle-to-silence
// contract: a released button must ease back to exactly 0 so an idle control
// performs zero per-frame work. Formerly TestButtonPressDepthTravel in
// button_keycap_test.go, minus the retired travel-in-pixels assertion.
func TestButtonPressDepthSpring(t *testing.T) {
	b := NewButton("Play", InstButtonStyle, nil)
	b.SetRect(image.Rect(0, 0, 60, 30))
	// Press edge targets full depth (1.0).
	b.HandleInputResult(10, 10, true)
	if b.pressTarget != 1 {
		t.Fatalf("press target = %v, want 1", b.pressTarget)
	}
	for i := 0; i < 12; i++ {
		b.AdvancePressAnim()
	}
	if b.pressDepth < 0.9 {
		t.Fatalf("held depth eased to %v, want ~1", b.pressDepth)
	}
	// Release: overshoot UP (negative depth) then settle to exactly 0.
	b.HandleInputResult(10, 10, false)
	if b.pressTarget != 0 {
		t.Fatalf("release target = %v, want 0", b.pressTarget)
	}
	if b.pressDepth >= 0 {
		t.Fatalf("release should kick depth negative (overshoot up), got %v", b.pressDepth)
	}
	for i := 0; i < 60; i++ {
		b.AdvancePressAnim()
	}
	if b.pressDepth != 0 {
		t.Fatalf("settled depth = %v, want exactly 0 (silence)", b.pressDepth)
	}
}
