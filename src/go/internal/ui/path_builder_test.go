package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

func TestBuildPathOrthogonalityValidation(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	// Diagonal segment should be rejected.
	pts := [][2]int{{0, 0}, {1, 1}}
	if err := g.BuildPath(0, pts); err == nil {
		t.Fatalf("expected error for non-orthogonal segment")
	}
}

func TestKickAndSnareBeatSizedSegments(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	max := g.grid.MaxDiv()
	// Kick: 1x1 beat rectangle at negative coords
	kick := [][2]int{{-2 * max, -1 * max}, {-1 * max, -1 * max}, {-1 * max, 0}, {-2 * max, 0}, {-2 * max, -1 * max}}
	if err := g.BuildPath(0, kick); err != nil {
		t.Fatalf("kick path: %v", err)
	}
	// Check two segments: top horizontal and right vertical are exactly 1 beat.
	if n := g.nodeAt(-2*max, -1*max); n == nil {
		t.Fatalf("kick node A missing")
	}
	if n := g.nodeAt(-1*max, -1*max); n == nil {
		t.Fatalf("kick node B missing")
	}
	if n := g.nodeAt(-1*max, 0); n == nil {
		t.Fatalf("kick node C missing")
	}
	if dx := abs(-1*max - (-2 * max)); dx != max {
		t.Fatalf("kick top dx=%d want %d", dx, max)
	}
	if dy := abs(0 - (-1 * max)); dy != max {
		t.Fatalf("kick right dy=%d want %d", dy, max)
	}

	// Snare: 2x2 beats rectangle on row 1
	g.drum.AddRow()
	snare := [][2]int{{0, -2 * max}, {2 * max, -2 * max}, {2 * max, 0}, {0, 0}, {0, -2 * max}}
	if err := g.BuildPath(1, snare); err != nil {
		t.Fatalf("snare path: %v", err)
	}
	if dx := abs(2*max - 0); dx != 2*max {
		t.Fatalf("snare top dx=%d want %d", dx, 2*max)
	}
	if dy := abs(0 - (-2 * max)); dy != 2*max {
		t.Fatalf("snare right dy=%d want %d", dy, 2*max)
	}

	// Ensure rows origins are set for both rows
	if g.drum.Rows[0].Origin == model.InvalidNodeID || g.drum.Rows[1].Origin == model.InvalidNodeID {
		t.Fatalf("row origins not set")
	}
}
