//go:build test

package ui

import (
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestMobileRowNamesDoNotTruncate verifies that typical drum-kit row
// names render without ellipsis at the iPhone-portrait viewport
// (390×844). Before the rowControlWeights rebalance from [3,2,2,2,2]
// → [5,2,2,2,2] and the RowControlBtnSize shrink 36 → 32, names like
// "Hi-Hat", "Cowbell", "FM Snare" all truncated to "Hi-...", "Co...",
// "Fm-...". This regression-guard test ensures the label cell stays
// wide enough to fit them.
//
// Truncation is applied by clipTextToWidth at button-draw time
// (uigrid.go:162 and uigrid.go:256). Replicating that call here is
// behavior-equivalent to rendering and inspecting the rasterized text.
func TestMobileRowNamesDoNotTruncate(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	g.Layout(390, 844)
	advanceFrames(g, 2)

	dv := g.drum

	// Names representative of the default Beatmo kit and a few longer
	// edge cases. All should fit at 390px viewport once weights are
	// rebalanced.
	names := []string{"Kick-1", "Snare", "Hi-Hat", "Clap", "Cowbell", "FM Snare"}

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
