//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// A resize clears the BPM focus, so the ring does not survive an orientation change.
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

// At the same area too.
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

// Not on every Update/Draw.
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
