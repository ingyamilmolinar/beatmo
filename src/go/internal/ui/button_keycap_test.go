package ui

import (
	"image"
	"testing"
)

func TestKeycapCapRect_RestPressedRaised(t *testing.T) {
	r := image.Rect(10, 20, 60, 50) // 50x30
	wall := genGeomKeycapWallDepth   // 4
	// Rest: cap occupies the top (height - wall), wall band below.
	rest := keycapCapRect(r, 0, false)
	if rest.Min != image.Pt(10, 20) {
		t.Fatalf("rest cap min = %v, want (10,20)", rest.Min)
	}
	if rest.Dy() != r.Dy()-wall {
		t.Fatalf("rest cap height = %d, want %d", rest.Dy(), r.Dy()-wall)
	}
	if rest.Dx() != r.Dx() {
		t.Fatalf("rest cap width = %d, want %d", rest.Dx(), r.Dx())
	}
	// Fully pressed (travel == wall): cap bottom meets rect bottom (bottom-out).
	pressed := keycapCapRect(r, wall, false)
	if pressed.Max.Y != r.Max.Y {
		t.Fatalf("pressed cap max.Y = %d, want %d (bottom-out)", pressed.Max.Y, r.Max.Y)
	}
	if pressed.Dy() != rest.Dy() {
		t.Fatalf("cap height must be travel-invariant: %d vs %d", pressed.Dy(), rest.Dy())
	}
	// Active raised: cap sits keycap-active-raise px above rest.
	raised := keycapCapRect(r, 0, true)
	if raised.Min.Y != r.Min.Y-genGeomKeycapActiveRaise {
		t.Fatalf("raised cap min.Y = %d, want %d", raised.Min.Y, r.Min.Y-genGeomKeycapActiveRaise)
	}
}

func TestButtonPressDepthTravel(t *testing.T) {
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
	if px := b.pressTravelPx(); px < genGeomKeycapWallDepth-1 {
		t.Fatalf("held travel = %dpx, want ~%d", px, genGeomKeycapWallDepth)
	}
	// Release: overshoot UP (negative depth) then settle to 0.
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
