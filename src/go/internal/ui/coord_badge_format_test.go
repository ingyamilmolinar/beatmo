//go:build test

package ui

import (
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// The badge uses the cursor label's beat:subdivision form (Grid.BeatAxisLabel, "0:1/2"), never raw subdivision
// indices; coordBadgeText is its only formatter.

func TestCoordBadgeTextUsesBeatSubdivisionFormat(t *testing.T) {
	// (4, 0) at subdiv 8: half a beat on X ("0:1/2"), on the beat on Y ("0").
	g, _, b := buildLoopGame(t)

	// The suffix only: the badge leads with the sounding note, which a retune changes.
	if got, want := g.coordBadgeText(b), "(0:1/2, 0)"; !strings.HasSuffix(got, want) {
		t.Fatalf("coordBadgeText(b) = %q, want it to end with %q (must match desktop BeatAxisLabel format, not raw units)", got, want)
	}
}

func TestCoordBadgeTextMatchesDesktopCursorFormat(t *testing.T) {
	// (4, 12): "0:1/2" and "1:1/2".
	g, _, _ := buildLoopGame(t)
	n := g.tryAddNode(4, 12, model.NodeTypeRegular)
	if n == nil {
		t.Fatal("tryAddNode returned nil")
	}

	got := g.coordBadgeText(n)
	// Exactly the cursor label's form.
	want := "(" + g.grid.BeatAxisLabel(n.I) + ", " + g.grid.BeatAxisLabel(n.J) + ")"
	if got != want {
		t.Fatalf("coordBadgeText = %q, want %q (badge must share the desktop cursor coordinate system)", got, want)
	}
	if got != "(0:1/2, 1:1/2)" {
		t.Fatalf("coordBadgeText = %q, want %q", got, "(0:1/2, 1:1/2)")
	}
}

// An off-beat axis never renders as its raw index.
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
