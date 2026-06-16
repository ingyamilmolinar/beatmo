package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
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
		section string
		want    string
	}{
		{"vol", "100%"},
		{"pit", "+0"},
		{"dur", "1.00x"},
		{"logic", "None"},
		{"groove", "None"},
		{"aud", "Audible"},
	}
	for _, c := range cases {
		got := g.sidebar.sectionBadgeText(c.section)
		if got != c.want {
			t.Errorf("default badge for section %q: got %q, want %q", c.section, got, c.want)
		}
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
		LogicKind:  "probability",
		LogicP:     0.5,
		GrooveKind: "delay",
		GroovePct:  0.25,
	})

	cases := []struct {
		section string
		want    string
	}{
		{"vol", "75%"},
		{"pit", "+3"},
		{"dur", "1.50x"},
		{"logic", "P 50%"},
		{"groove", "Delay 25%"},
	}
	for _, c := range cases {
		got := g.sidebar.sectionBadgeText(c.section)
		if got != c.want {
			t.Errorf("non-default badge for section %q: got %q, want %q", c.section, got, c.want)
		}
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
		got := g.sidebar.sectionBadgeText("aud")
		if got != c.want {
			t.Errorf("audible badge for node type %v: got %q, want %q", c.nodeType, got, c.want)
		}
	}
}
