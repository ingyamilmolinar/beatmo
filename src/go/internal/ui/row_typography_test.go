//go:build test

package ui

import (
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestMobileRowNamesDoNotTruncate verifies that typical (short) drum-kit row
// names render without ellipsis at the iPhone-portrait viewport (390×844).
//
// Reality of the mobile layout: the row rack lives in a ~192px left column
// (rows-left / timeline-right), so the label cell is ~44px — wide enough for
// the typical short kit names ("Snare", "Hi-Hat", "Clap", "Cowbell",
// "Kick-1", "Rimshot") but NOT for long two-word / 8+-char names like
// "FM Snare" or "Sidestick". Those long names live behind the tap-to-open
// label sheet on mobile and are expected to abbreviate in the narrow column;
// this guard only pins the short, common case so a future regression that
// shrinks the label below the short-name width fails loudly.
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

	// Short names representative of the default Beatmo kit. All fit the ~44px
	// mobile label cell verbatim. (Longer names such as "FM Snare"/"Sidestick"
	// genuinely cannot fit the 192px mobile half-column and abbreviate there.)
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
