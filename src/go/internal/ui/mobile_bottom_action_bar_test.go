//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestBottomActionBar_AllocatedOnMobile verifies that on the mobile profile
// the DrumView allocates a bottom action bar rect that anchors to the
// drum-pane bottom, spans the drum-pane width, and is at least
// TouchMinTarget tall. This is the host for the volume icon, view-switch,
// and overflow buttons in the mobile bottom-sheet redesign (B3 critique).
func TestBottomActionBar_AllocatedOnMobile(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 700)

	r := g.drum.bottomActionBarRect
	if r.Empty() {
		t.Fatalf("expected bottom action bar rect non-empty on mobile, got empty")
	}
	if r.Dy() < TouchMinTarget() {
		t.Fatalf("bottom action bar height %d < TouchMinTarget %d", r.Dy(), TouchMinTarget())
	}
	if r.Max.Y != g.drum.Bounds.Max.Y {
		t.Fatalf("bottom action bar should anchor to drum-pane bottom: rect.Max.Y=%d bounds.Max.Y=%d", r.Max.Y, g.drum.Bounds.Max.Y)
	}
	if r.Min.X != g.drum.Bounds.Min.X || r.Max.X != g.drum.Bounds.Max.X {
		t.Fatalf("bottom action bar should span drum-pane width: rect=%v bounds=%v", r, g.drum.Bounds)
	}
}

// TestBottomActionBar_EmptyOnDesktop verifies the bottom action bar is NOT
// allocated on the desktop profile — it is a mobile-only affordance.
func TestBottomActionBar_EmptyOnDesktop(t *testing.T) {
	setupMobileTest(t, false) // desktop
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 800)

	if !g.drum.bottomActionBarRect.Empty() {
		t.Fatalf("expected empty bottom action bar rect on desktop, got %v", g.drum.bottomActionBarRect)
	}
}
