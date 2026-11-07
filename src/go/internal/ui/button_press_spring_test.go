package ui

import (
	"image"
	"testing"
)

// The spring drives no geometry but is the settle-to-silence contract: a released button eases back to exactly 0 so
// an idle control does no per-frame work.
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
