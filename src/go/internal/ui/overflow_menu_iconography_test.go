//go:build test

package ui

import (
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestOverflowMenu_AllNonHeaderItemsHaveIcons enforces Theme 5 of the
// mobile UI consistency pass: meaning is communicated through the
// iconography channel (DESIGN.md single-chrome-accent invariant
// forbids per-item accent colors).
func TestOverflowMenu_AllNonHeaderItemsHaveIcons(t *testing.T) {
	assertDefaultParityState(t)
	setupMobileTest(t, true)

	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 700)
	advanceFrames(g, 2)
	dv := g.drum

	for _, item := range dv.OverflowItemsForTest() {
		if item.header {
			continue
		}
		if item.iconID == "" {
			t.Errorf("overflow item %q has no iconID; meaning must come from icons not colors", item.label)
		}
	}
}

// TestOverflowMenu_SpecificIconAssignments pins each known action to
// its semantic icon per DESIGN.md:1717-1733.
func TestOverflowMenu_SpecificIconAssignments(t *testing.T) {
	assertDefaultParityState(t)
	setupMobileTest(t, true)

	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 700)
	advanceFrames(g, 2)
	dv := g.drum

	want := map[string]IconID{
		"Upload": IconUpload,
		"Import": IconImport,
		"Export": IconExport,
	}
	for _, item := range dv.OverflowItemsForTest() {
		if item.header {
			continue
		}
		expected, ok := want[item.label]
		if !ok {
			continue
		}
		if item.iconID != expected {
			t.Errorf("overflow item %q has iconID=%q, want %q", item.label, item.iconID, expected)
		}
	}
}
