//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestFocusRing_BPMBlursOnResize verifies that resizing the viewport clears
// focus on the BPM text input. Regression for the orange focus ring that
// persisted across orientation changes (A4 in the screenshot critique).
func TestFocusRing_BPMBlursOnResize(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	g.Layout(800, 600)
	tz := g.drum.transportZone
	if tz == nil || tz.bpmBox == nil {
		t.Fatal("transport zone / bpm box not initialized")
	}
	tz.bpmBox.focused = true

	g.Layout(400, 800)

	if tz.bpmBox.focused {
		t.Fatalf("expected bpmBox.focused=false after resize, got true")
	}
}

// TestFocusRing_BPMBlursOnOrientationFlip verifies focus is cleared when the
// viewport flips landscape ↔ portrait at the same total area.
func TestFocusRing_BPMBlursOnOrientationFlip(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	g.Layout(800, 400) // landscape
	tz := g.drum.transportZone
	if tz == nil || tz.bpmBox == nil {
		t.Fatal("transport zone / bpm box not initialized")
	}
	tz.bpmBox.focused = true

	g.Layout(400, 800) // portrait

	if tz.bpmBox.focused {
		t.Fatalf("expected bpmBox.focused=false after orientation flip, got true")
	}
}

// TestFocusRing_NotBlurredOnIdenticalLayout verifies focus is preserved when
// Layout is called with the same dimensions (defensive regression: don't
// blur on every Update/Draw cycle).
func TestFocusRing_NotBlurredOnIdenticalLayout(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	g.Layout(400, 800)
	tz := g.drum.transportZone
	if tz == nil || tz.bpmBox == nil {
		t.Fatal("transport zone / bpm box not initialized")
	}
	tz.bpmBox.focused = true

	g.Layout(400, 800) // identical dims

	if !tz.bpmBox.focused {
		t.Fatalf("expected bpmBox.focused=true after no-op Layout; blur fired on identical dims")
	}
}
