package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// These tests pin the behavior requested in the node pop-up menu: every
// collapsed category section should surface its current value as a badge —
// including when the value is at its default — using the same formatting that
// non-default / updated values already use. Previously sectionBadgeText()
// returned "" for defaults, so default categories showed no badge at all.

// setNodeParams is a tiny helper that overwrites a node's params via the graph.
func setNodeParams(t *testing.T, g *Game, id model.NodeID, p model.NodeParams) {
	t.Helper()
	g.graph.SetNodeParams(id, p)
}

// TestSectionBadgeShowsDefaultValues verifies that a freshly-added regular node
// (all params at their defaults) shows a value badge for every category instead
// of an empty string.
func TestSectionBadgeShowsDefaultValues(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	if n == nil {
		t.Fatal("failed to add node")
	}
	g.sidebar.Open(n)

	cases := []struct {
		section sbSection
		want    string
	}{
		{secVol, "0 dB"},
		{secDur, "1.00x"},
		{secLogic, "None"},
		{secGroove, "None"},
		{secAud, "Audible"},
	}
	for _, c := range cases {
		got := g.sidebar.sectionBadgeText(c.section)
		if got != c.want {
			t.Errorf("default badge for section %q: got %q, want %q", c.section, got, c.want)
		}
	}

	// "pit" is asserted against the note label rather than a literal. Since note
	// display went universal the badge shows the sounding NOTE for almost every
	// instrument (this node's row is the demo's snare, so "F#3"), and the bare
	// "+0" semitone form survives only for unpitched noise. Pinning the literal
	// here would make an unrelated instrument retune fail this test, which is
	// about badge formatting; TestInstrumentFundamentalsUpToDate is what guards
	// the pitch itself.
	gotPit := g.sidebar.sectionBadgeText(secPit)
	if want := g.nodeNoteLabel(n.ID, 0); want != "" && gotPit != want {
		t.Errorf("default badge for section \"pit\": got %q, want the sounding note %q", gotPit, want)
	}
	if gotPit == "" {
		t.Error("default badge for section \"pit\" is empty; every category must surface a value")
	}
}

// TestSectionBadgePitchFallsBackToSemitones pins the other half of the pitch
// badge: an UNPITCHED row has no note to show, so the badge keeps the signed
// semitone form. This is what stops "universal note names" from meaning the
// semitone readout was simply deleted.
func TestSectionBadgePitchFallsBackToSemitones(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	if n == nil {
		t.Fatal("failed to add node")
	}
	g.drum.Rows[0].Instrument = roster.Hihat // no fundamental: nothing to name
	g.sidebar.Open(n)

	setNodeParams(t, g, n.ID, model.NodeParams{Volume: 1, Pitch: 3, Duration: 1})

	if got, want := g.sidebar.sectionBadgeText(secPit), "+3"; got != want {
		t.Errorf("unpitched pitch badge = %q, want %q", got, want)
	}
}

// TestSectionBadgeNonDefaultValuesUnchanged locks the existing non-default
// formatting so the default-value change above does not regress updated values.
func TestSectionBadgeNonDefaultValuesUnchanged(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	if n == nil {
		t.Fatal("failed to add node")
	}
	g.sidebar.Open(n)

	setNodeParams(t, g, n.ID, model.NodeParams{
		Volume:     0.75,
		Pitch:      3,
		Duration:   1.50,
		LogicKind:  model.LogicProbability,
		LogicP:     0.5,
		GrooveKind: model.GrooveDelay,
		GroovePct:  0.25,
	})

	cases := []struct {
		section sbSection
		want    string
	}{
		{secVol, "-2 dB"}, // 20*log10(0.75) = -2.50 -> -2
		{secDur, "1.50x"},
		{secLogic, "P 50%"},
		{secGroove, "Delay 25%"},
	}
	for _, c := range cases {
		got := g.sidebar.sectionBadgeText(c.section)
		if got != c.want {
			t.Errorf("non-default badge for section %q: got %q, want %q", c.section, got, c.want)
		}
	}

	// Pitch +3 on a pitched row reads as the note three semitones up, not "+3".
	gotPit := g.sidebar.sectionBadgeText(secPit)
	if want := g.nodeNoteLabel(n.ID, 3); want != "" && gotPit != want {
		t.Errorf("non-default badge for section \"pit\": got %q, want the sounding note %q", gotPit, want)
	}
}

// TestSectionBadgeAudibleVariants verifies the audible category badge reflects
// each node type, including the default (regular → "Audible").
func TestSectionBadgeAudibleVariants(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	cases := []struct {
		nodeType model.NodeType
		i, j     int
		want     string
	}{
		{model.NodeTypeRegular, 0, 0, "Audible"},
		{model.NodeTypeSilent, 4, 0, "Silent"},
		{model.NodeTypeMute, 8, 0, "Muted"},
	}
	for _, c := range cases {
		n := g.tryAddNode(c.i, c.j, c.nodeType)
		if n == nil {
			t.Fatalf("failed to add node of type %v", c.nodeType)
		}
		g.sidebar.Open(n)
		got := g.sidebar.sectionBadgeText(secAud)
		if got != c.want {
			t.Errorf("audible badge for node type %v: got %q, want %q", c.nodeType, got, c.want)
		}
	}
}
