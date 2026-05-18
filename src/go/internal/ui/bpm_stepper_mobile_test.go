//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestBPMStepperMobile_HorizontalLayout verifies that the mobile BPM stepper
// renders as [−][BPM box][+] in a single horizontal row with both ± buttons
// at full row height. Regression for B1 in the screenshot critique: prior
// `stackVerticalTransport` halved the row to 22 px, violating DESIGN.md's
// 44 px touch minimum.
func TestBPMStepperMobile_HorizontalLayout(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)

	tz := g.drum.transportZone
	if tz == nil {
		t.Fatal("transport zone not initialized")
	}
	dec := tz.bpmDecBtn.Rect()
	box := tz.bpmBox.Rect
	inc := tz.bpmIncBtn.Rect()

	if dec.Empty() || box.Empty() || inc.Empty() {
		t.Fatalf("unexpected empty rects: dec=%v box=%v inc=%v", dec, box, inc)
	}
	// Horizontal order: dec to the left of box, box to the left of inc.
	if !(dec.Max.X <= box.Min.X+1 && box.Max.X <= inc.Min.X+1) {
		t.Fatalf("expected horizontal [dec][box][inc] order; got dec=%v box=%v inc=%v", dec, box, inc)
	}
	// Top edges aligned within 1 px (single row).
	tops := []int{dec.Min.Y, box.Min.Y, inc.Min.Y}
	for i := 1; i < len(tops); i++ {
		if absInt(tops[i]-tops[0]) > 1 {
			t.Fatalf("expected aligned top edges within 1 px; got %v", tops)
		}
	}
}

// TestBPMStepperMobile_NotHalvedVertically verifies that the BPM ± buttons
// are NOT vertically halved. The prior `stackVerticalTransport` cut the
// row in half, leaving each button ~22 px tall — direct violation of
// DESIGN.md §"Touch sizing". The horizontal stepper must give each button
// the FULL row height (matching the BPM box rect).
func TestBPMStepperMobile_NotHalvedVertically(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)

	tz := g.drum.transportZone
	if tz == nil {
		t.Fatal("transport zone not initialized")
	}
	boxH := tz.bpmBox.Rect.Dy()
	if h := tz.bpmDecBtn.Rect().Dy(); h < boxH {
		t.Fatalf("bpmDecBtn height %d px < bpmBox height %d px — vertical halving regression", h, boxH)
	}
	if h := tz.bpmIncBtn.Rect().Dy(); h < boxH {
		t.Fatalf("bpmIncBtn height %d px < bpmBox height %d px — vertical halving regression", h, boxH)
	}
}

// TestBPMStepperDesktop_VerticalStackUnchanged verifies the desktop layout
// keeps the vertical ± stack — the mobile redesign must not regress
// desktop pixel layout.
func TestBPMStepperDesktop_VerticalStackUnchanged(t *testing.T) {
	setupMobileTest(t, false) // desktop
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)

	tz := g.drum.transportZone
	if tz == nil {
		t.Fatal("transport zone not initialized")
	}
	inc := tz.bpmIncBtn.Rect()
	dec := tz.bpmDecBtn.Rect()
	if inc.Empty() || dec.Empty() {
		t.Fatalf("desktop ± buttons should be allocated; got inc=%v dec=%v", inc, dec)
	}
	// Desktop stack: inc on top, dec below — non-overlapping vertically.
	if inc.Max.Y > dec.Min.Y+1 {
		t.Fatalf("expected desktop vertical stack (inc above dec); got inc=%v dec=%v", inc, dec)
	}
}

// TestBPMStepperMobile_IncrementsBPM presses the [+] button and verifies the
// BPM value rises. Catches wiring regressions (button tied to wrong handler).
func TestBPMStepperMobile_IncrementsBPM(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)

	tz := g.drum.transportZone
	if tz == nil {
		t.Fatal("transport zone not initialized")
	}
	before := tz.bpm
	tz.bpmIncBtn.OnClick()
	tz.Update()
	if tz.bpm <= before {
		t.Fatalf("expected bpm to increment after Inc click: before=%d after=%d", before, tz.bpm)
	}
}

