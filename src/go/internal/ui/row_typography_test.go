//go:build test

package ui

import (
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// The short kit names fit the ~44 px mobile label cell verbatim; long ones abbreviate there by design. The test
// replicates clipTextToWidth, which the button draw applies.
func TestMobileRowNamesDoNotTruncate(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	g.Layout(390, 844)
	advanceFrames(g, 2)

	dv := g.drum

	names := []string{"Kick-1", "Snare", "Hi-Hat", "Clap", "Cowbell", "Rimshot"}

	if len(dv.rowLabels()) == 0 {
		t.Fatal("no row labels available after default game start")
	}
	rect := dv.rowLabels()[0].Rect()
	if rect.Empty() {
		t.Fatalf("row 0 label rect is empty after layout")
	}
	cellW := rect.Dx() - 2*SpaceXS

	for _, name := range names {
		clipped := clipTextToWidth(name, cellW)
		if strings.Contains(clipped, "...") {
			t.Errorf("name %q renders as %q at cell width %d — label cell too narrow",
				name, clipped, cellW)
		}
		if clipped != name {
			t.Errorf("name %q renders as %q; expected verbatim", name, clipped)
		}
	}
}
