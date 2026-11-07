//go:build test

package ui

import (
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// The badge names the note the node sounds; nodeNoteLabel reads live audio state and caches nothing, so edits show
// with no invalidation of its own.

// Transposed +3, so pitch 0 sounds C4.
func badgeNodeOnRow0(t *testing.T, g *Game, inst string) *uiNode {
	t.Helper()
	node := noteTestNode(t, g)
	g.drum.Rows[0].Instrument = instrument.ID(inst)
	audio.SetSampleEdit(instrument.ID(inst), audio.SampleEdit{TransposeSemis: 3})
	t.Cleanup(func() { audio.ClearSampleEdit(instrument.ID(inst)) })
	return node
}

func TestCoordBadgeText_IncludesNoteForPitchedNode(t *testing.T) {
	g := newTestGameForUndo(t)
	node := badgeNodeOnRow0(t, g, "test-badge-inst-mel")

	got := g.coordBadgeText(node)
	if !strings.HasPrefix(got, "C4 ") {
		t.Errorf("badge = %q, want it to lead with the sounding note C4", got)
	}
	// The coordinates must survive unchanged — the note is additive.
	if !strings.Contains(got, g.grid.BeatAxisLabel(node.I)) ||
		!strings.Contains(got, g.grid.BeatAxisLabel(node.J)) {
		t.Errorf("badge = %q, want it to still carry both beat-axis coordinates", got)
	}
}

// The note must follow the node's pitch, which is the whole point.
func TestCoordBadgeText_TracksPitchChanges(t *testing.T) {
	g := newTestGameForUndo(t)
	node := badgeNodeOnRow0(t, g, "test-badge-inst-shift")

	before := g.coordBadgeText(node)

	mn, ok := g.graph.GetNodeByID(node.ID)
	if !ok {
		t.Fatal("node not in graph")
	}
	mn.Params.Pitch = 12 // one octave up
	g.graph.SetNodeParams(node.ID, mn.Params)

	after := g.coordBadgeText(node)
	if after == before {
		t.Fatalf("badge did not change after a +12 pitch shift (still %q)", before)
	}
	if !strings.HasPrefix(after, "C5 ") {
		t.Errorf("badge after +12 = %q, want it to lead with C5", after)
	}
}

// A kick's fundamental is a real measured frequency, so the badge leads with it as for a melodic instrument.
func TestCoordBadgeText_PercussionLeadsWithItsNote(t *testing.T) {
	g := newTestGameForUndo(t)
	node := noteTestNode(t, g)

	// A shipped instrument, tuned to 48 Hz.
	g.drum.Rows[0].Instrument = roster.DnbKick

	coords := "(" + g.grid.BeatAxisLabel(node.I) + ", " + g.grid.BeatAxisLabel(node.J) + ")"
	got := g.coordBadgeText(node)
	if got == coords {
		t.Errorf("percussion badge = %q, want it to lead with a note name now that drums resolve one", got)
	}
	if want := "G1 · " + coords; got != want {
		t.Errorf("percussion badge = %q, want %q (dnb-kick is tuned to 48 Hz = G1)", got, want)
	}
}

// A hi-hat has no fundamental (confidence 0), so a note there would be invented.
func TestCoordBadgeText_UnpitchedNoiseKeepsCoordinatesOnly(t *testing.T) {
	g := newTestGameForUndo(t)
	node := noteTestNode(t, g)

	g.drum.Rows[0].Instrument = roster.Hihat

	got := g.coordBadgeText(node)
	want := "(" + g.grid.BeatAxisLabel(node.I) + ", " + g.grid.BeatAxisLabel(node.J) + ")"
	if got != want {
		t.Errorf("hi-hat badge = %q, want plain coordinates %q", got, want)
	}
}

// No row, no instrument: no empty separator.
func TestCoordBadgeText_OffCircuitKeepsCoordinatesOnly(t *testing.T) {
	g, _, b := buildLoopGame(t)
	delete(g.nodeRows, b.ID)

	got := g.coordBadgeText(b)
	if strings.Contains(got, "·") {
		t.Errorf("off-circuit badge = %q, want no note separator", got)
	}
	if !strings.HasPrefix(got, "(") {
		t.Errorf("off-circuit badge = %q, want the plain coordinate form", got)
	}
}

// The draw call's own metric, not a fixed per-character estimate that drifts with a proportional font.
func TestCoordBadgePillWidthUsesRealTextMetrics(t *testing.T) {
	cases := []string{
		"(0:1/2, 0)",
		"C4 · (0:1/2, 0)",
		"A#4 · (12:3/4, 7:1/2)",
	}
	for _, s := range cases {
		got := coordBadgePillWidth(s)
		want := TextWidth(s)
		if got < want {
			t.Errorf("pill width for %q = %d, narrower than the text it must hold (%d)", s, got, want)
		}
		// Padding should be real but not wild.
		if got > want+40 {
			t.Errorf("pill width for %q = %d, absurdly wider than the text (%d)", s, got, want)
		}
	}
}

// Clamped horizontally too, so a node near an edge keeps the whole readout on screen.
func TestCoordBadgePillRectStaysOnScreen(t *testing.T) {
	const (
		screenW = 400
		w       = 120
		top     = 40
	)
	cases := []struct {
		name string
		cx   int
	}{
		{"far left", 0},
		{"near left", 10},
		{"centre", 200},
		{"near right", 390},
		{"far right", 480},
	}
	for _, c := range cases {
		r := coordBadgePillRect(c.cx, top, w, screenW)
		if r.Min.X < 0 {
			t.Errorf("%s: pill starts off-screen at x=%d", c.name, r.Min.X)
		}
		if r.Max.X > screenW {
			t.Errorf("%s: pill ends past the right edge (x=%d, screen=%d)", c.name, r.Max.X, screenW)
		}
		if r.Dx() != w {
			t.Errorf("%s: pill width %d, want %d — clamping must move the pill, not shrink it", c.name, r.Dx(), w)
		}
	}
}

// Both edges cannot hold; never a negative origin.
func TestCoordBadgePillRectWiderThanScreenPinsLeft(t *testing.T) {
	r := coordBadgePillRect(50, 40, 500, 300)
	if r.Min.X < 0 {
		t.Errorf("over-wide pill starts at x=%d, want a non-negative origin", r.Min.X)
	}
}

// With no known width only the left edge can be enforced.
func TestCoordBadgePillRectUnknownScreenWidthClampsLeftOnly(t *testing.T) {
	r := coordBadgePillRect(0, 40, 120, 0)
	if r.Min.X < 0 {
		t.Errorf("pill starts at x=%d with unknown screen width, want >= 0", r.Min.X)
	}
}

// The guard belongs in the formatter too, which other callers may use.
func TestCoordBadgeTextNilNode(t *testing.T) {
	g := newTestGameForUndo(t)
	if got := g.coordBadgeText(nil); got != "" {
		t.Errorf("coordBadgeText(nil) = %q, want empty", got)
	}
}
