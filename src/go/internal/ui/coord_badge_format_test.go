//go:build test

package ui

import (
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// The (i,j) coordinate badge that appears next to a node must present the grid
// position in the SAME beat/subdivision coordinate system the desktop cursor
// label uses (Grid.BeatAxisLabel: "beat:fraction", e.g. "0:1/2"), never raw
// whole subdivision indices. This was wrong on mobile — the badge is the
// primary coordinate readout there and it printed plain integers like "(4, 0)".
//
// coordBadgeText is the single formatter the badge draw path uses, so we assert
// its output directly (mirrors how node_coords_test.go pins formatNodeCoordHome).

func TestCoordBadgeTextUsesBeatSubdivisionFormat(t *testing.T) {
	// buildLoopGame places node b at grid (4, 0) with subdiv 8. On axis X that
	// is exactly half a beat past the origin -> BeatAxisLabel(4) == "0:1/2".
	// On axis Y it lands on the beat -> BeatAxisLabel(0) == "0". Raw formatting
	// would wrongly print "(4, 0)".
	g, _, b := buildLoopGame(t)

	// The subject here is the COORDINATE format, not the note. Since note
	// display went universal the badge leads with the sounding note for almost
	// every instrument, so assert on the coordinate SUFFIX: pinning the whole
	// string would make this test fail whenever an instrument is retuned,
	// which has nothing to do with what it is checking.
	if got, want := g.coordBadgeText(b), "(0:1/2, 0)"; !strings.HasSuffix(got, want) {
		t.Fatalf("coordBadgeText(b) = %q, want it to end with %q (must match desktop BeatAxisLabel format, not raw units)", got, want)
	}
}

func TestCoordBadgeTextMatchesDesktopCursorFormat(t *testing.T) {
	// A node fractional on BOTH axes: grid (4, 12) at subdiv 8.
	// X: BeatAxisLabel(4) == "0:1/2"; Y: BeatAxisLabel(12) == "1:1/2".
	g, _, _ := buildLoopGame(t)
	n := g.tryAddNode(4, 12, model.NodeTypeRegular)
	if n == nil {
		t.Fatal("tryAddNode returned nil")
	}

	got := g.coordBadgeText(n)
	// The badge must be exactly what the desktop cursor label produces for the
	// same grid coordinates: "(BeatAxisLabel(i), BeatAxisLabel(j))".
	want := "(" + g.grid.BeatAxisLabel(n.I) + ", " + g.grid.BeatAxisLabel(n.J) + ")"
	if got != want {
		t.Fatalf("coordBadgeText = %q, want %q (badge must share the desktop cursor coordinate system)", got, want)
	}
	if got != "(0:1/2, 1:1/2)" {
		t.Fatalf("coordBadgeText = %q, want %q", got, "(0:1/2, 1:1/2)")
	}
}

// Guard against regressing to raw whole-unit output: a node off the beat must
// never render its axis as a bare integer that equals the raw subdivision index.
func TestCoordBadgeTextNotRawUnits(t *testing.T) {
	g, _, _ := buildLoopGame(t)
	_ = game_log.LevelError
	n := g.tryAddNode(6, 0, model.NodeTypeRegular) // 6/8 of a beat on X
	if n == nil {
		t.Fatal("tryAddNode returned nil")
	}
	if got := g.coordBadgeText(n); got == "(6, 0)" {
		t.Fatalf("coordBadgeText = %q — regressed to raw whole-unit subdivision indices", got)
	}
}
