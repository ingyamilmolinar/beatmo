package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Every collapsed section shows its current value as a badge, defaults included, formatted like edited values.

// setNodeParams is a tiny helper that overwrites a node's params via the graph.
func setNodeParams(t *testing.T, g *Game, id model.NodeID, p model.NodeParams) {
	t.Helper()
	g.graph.SetNodeParams(id, p)
}

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

	// Against the note label, not a literal: a retune of the row's instrument must not fail a badge-format test.
	gotPit := g.sidebar.sectionBadgeText(secPit)
	if want := g.nodeNoteLabel(n.ID, 0); want != "" && gotPit != want {
		t.Errorf("default badge for section \"pit\": got %q, want the sounding note %q", gotPit, want)
	}
	if gotPit == "" {
		t.Error("default badge for section \"pit\" is empty; every category must surface a value")
	}
}

// An unpitched row has no note, so its badge keeps the signed semitone form.
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

// Regular reads "Audible".
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
