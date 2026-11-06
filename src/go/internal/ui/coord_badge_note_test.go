//go:build test

package ui

import (
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// The coordinate badge above a selected node is the primary readout for "what
// is this node?" — on mobile it is the ONLY one when the sidebar is closed. It
// showed position but not the note the node actually sounds, so a user shifting
// pitch had to open the sidebar to see what they had done.
//
// The note is derived by nodeNoteLabel (node_note.go), which reads live audio
// state and caches nothing, so the badge tracks pitch/synth/sampler edits with
// no invalidation path of its own.

// badgeNodeOnRow0 places a node on row 0 and gives that row a pitched
// instrument transposed +3, so node pitch 0 sounds C4.
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

// Percussion names a note too, since note display went universal (2026-09-13):
// a kick's fundamental is a real, measured frequency, so the badge leads with
// it exactly as it does for a melodic instrument. This test asserted the
// OPPOSITE until then — that a note on a kick would be misinformation — which
// was true only while the engine had no answer for what a kick sounds.
func TestCoordBadgeText_PercussionLeadsWithItsNote(t *testing.T) {
	g := newTestGameForUndo(t)
	node := noteTestNode(t, g)

	// A real shipped percussion instrument, not a synthetic one: this is the
	// classification the user actually meets. dnb-kick is tuned to 48 Hz.
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

// Genuinely unpitched noise still keeps the plain coordinate form: a hi-hat has
// no fundamental at all (the detector returns confidence 0.000 on it), so a
// note there WOULD be misinformation. This is the half of the old contract that
// survives, and it is what stops "universal" from meaning "invented".
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

// A node with no row cannot resolve an instrument, so it also keeps the plain
// coordinate form rather than rendering an empty separator.
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

// The badge pill used a fixed 7px-per-character estimate to size itself, which
// is wrong for a proportional font and drifts further the longer the string
// gets — exactly what adding a note name does. Width must come from the same
// metric the draw call uses (TextWidth pairs with DrawTextAt).
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

// The pill is centred on its node and was clamped only vertically, so a node
// near the left or right edge pushed half the readout off-screen. Adding a note
// name widens the pill and makes that more likely.
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

// A pill wider than the screen cannot satisfy both edges; it must pin to the
// left rather than produce a negative origin.
func TestCoordBadgePillRectWiderThanScreenPinsLeft(t *testing.T) {
	r := coordBadgePillRect(50, 40, 500, 300)
	if r.Min.X < 0 {
		t.Errorf("over-wide pill starts at x=%d, want a non-negative origin", r.Min.X)
	}
}

// With no known screen width (an unlaid-out Game) only the left edge can be
// enforced.
func TestCoordBadgePillRectUnknownScreenWidthClampsLeftOnly(t *testing.T) {
	r := coordBadgePillRect(0, 40, 120, 0)
	if r.Min.X < 0 {
		t.Errorf("pill starts at x=%d with unknown screen width, want >= 0", r.Min.X)
	}
}

// The formatter must not panic on a nil node. The draw site guards against it,
// but the guard belongs in the formatter too — it is called from tests and
// could pick up another caller.
func TestCoordBadgeTextNilNode(t *testing.T) {
	g := newTestGameForUndo(t)
	if got := g.coordBadgeText(nil); got != "" {
		t.Errorf("coordBadgeText(nil) = %q, want empty", got)
	}
}
