//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Real-input coverage for the chord builder in the node sidebar
// (docs/superpowers/specs/2026-10-09-chord-nodes-design.md §3): every action
// is a click through Game.Update (nmClick), never a direct method call.

func chordSidebarGame(t *testing.T, inst instrument.ID) (*Game, *uiNode) {
	t.Helper()
	g := newTestGameForUndo(t)
	g.Layout(1280, 720)
	g.drum.Rows[0].Instrument = inst
	// A node ON row 0's circuit (a loop through the start node), so its row —
	// and with it the instrument's pitchedness — survives updateBeatInfos.
	node := g.tryAddNode(2, 0, model.NodeTypeRegular)
	if node == nil || g.start == nil {
		t.Fatal("fixture: no start node / test node")
	}
	g.addEdge(g.start, node)
	g.addEdge(node, g.start)
	g.updateBeatInfos()
	if row, ok := g.nodeRows[node.ID]; !ok || row != 0 {
		t.Fatalf("fixture: test node is not on row 0 (row=%d ok=%v)", row, ok)
	}
	g.sidebar.Open(node)
	g.sidebar.ExpandAllSections()
	g.sidebar.layout()
	return g, node
}

func sidebarClick(t *testing.T, g *Game, id sbID) {
	t.Helper()
	g.sidebar.layout()
	r, ok := g.sidebar.rects[id]
	if !ok || r.Empty() {
		t.Fatalf("sidebar control %q is not laid out", id)
	}
	if !g.sidebar.inViewport(r) {
		g.sidebar.scroll.VS.First += r.Min.Y - g.sidebar.scroll.VS.View.Min.Y
		g.sidebar.scroll.VS.Clamp()
		g.sidebar.layout()
		r = g.sidebar.rects[id]
	}
	x, y := nmCenter(r)
	nmClick(g, x, y)
	g.sidebar.layout()
}

func nodeTypeOf(t *testing.T, g *Game, id model.NodeID) model.NodeType {
	t.Helper()
	n, ok := g.graph.GetNodeByID(id)
	if !ok {
		t.Fatalf("node %d missing", id)
	}
	return n.Type
}

func chordOf(t *testing.T, g *Game, id model.NodeID) model.ChordSpec {
	t.Helper()
	n, _ := g.graph.GetNodeByID(id)
	return n.Params.Chord
}

func TestChordTypeCycleOrderOnPitchedRow(t *testing.T) {
	g, node := chordSidebarGame(t, roster.OrganChurch)
	want := []model.NodeType{model.NodeTypeChord, model.NodeTypeSilent, model.NodeTypeMute, model.NodeTypeRegular}
	for i, w := range want {
		sidebarClick(t, g, sbAudible)
		if got := nodeTypeOf(t, g, node.ID); got != w {
			t.Fatalf("click %d: type %s, want %s", i+1, got.String(), w.String())
		}
	}
}

// Review Focus 1: an unpitched row (hi-hat) has no notes to stack, so the
// cycle skips Chord.
func TestChordTypeCycleSkipsChordOnUnpitchedRow(t *testing.T) {
	g, node := chordSidebarGame(t, roster.Hihat)
	sidebarClick(t, g, sbAudible)
	if got := nodeTypeOf(t, g, node.ID); got != model.NodeTypeSilent {
		t.Fatalf("regular → %s on an unpitched row, want silent", got.String())
	}
}

// Entering Chord seeds a major triad; the spec then survives the whole
// cycle (Chord → Silent → Mute → Regular → Chord).
func TestChordSpecSurvivesTheTypeCycle(t *testing.T) {
	g, node := chordSidebarGame(t, roster.OrganChurch)
	sidebarClick(t, g, sbAudible)
	if c := chordOf(t, g, node.ID); c.Quality != model.ChordDefaultQuality {
		t.Fatalf("entering Chord seeded %+v, want a major triad", c)
	}
	sidebarClick(t, g, sbPlus(fieldChordInv))
	sidebarClick(t, g, sbPlus(fieldChordSpread))
	want := chordOf(t, g, node.ID)
	if want.Inversion != 1 || want.Spread != 1 {
		t.Fatalf("steppers left %+v", want)
	}
	for i := 0; i < 4; i++ {
		sidebarClick(t, g, sbAudible)
	}
	if got := nodeTypeOf(t, g, node.ID); got != model.NodeTypeChord {
		t.Fatalf("after a full cycle type is %s", got.String())
	}
	if got := chordOf(t, g, node.ID); got != want {
		t.Fatalf("spec after the cycle = %+v, want %+v", got, want)
	}
}

func TestChordSectionOnlyOnChordNodes(t *testing.T) {
	g, _ := chordSidebarGame(t, roster.OrganChurch)
	for _, id := range []sbID{sbSectionHeader(secChord), sbMinus(fieldChordInv), sbPlus(fieldChordSpread), sbChordSingle} {
		if r := g.sidebar.rects[id]; !r.Empty() {
			t.Fatalf("regular node lays out chord control %q", id)
		}
	}
	sidebarClick(t, g, sbAudible) // → chord
	for _, id := range []sbID{sbSectionHeader(secChord), sbMinus(fieldChordInv), sbPlus(fieldChordInv), sbMinus(fieldChordSpread), sbPlus(fieldChordSpread), sbChordSingle} {
		if r := g.sidebar.rects[id]; r.Empty() {
			t.Fatalf("chord node lacks chord control %q", id)
		}
	}
	sidebarClick(t, g, sbAudible) // → silent
	if r := g.sidebar.rects[sbMinus(fieldChordInv)]; !r.Empty() {
		t.Fatal("silent node still lays out the chord section")
	}
}

func TestChordSteppersClampToTheQuality(t *testing.T) {
	g, node := chordSidebarGame(t, roster.OrganChurch)
	sidebarClick(t, g, sbAudible)
	for i := 0; i < 5; i++ {
		sidebarClick(t, g, sbPlus(fieldChordInv))
	}
	if c := chordOf(t, g, node.ID); c.Inversion != 2 {
		t.Fatalf("a triad's inversion clamps at 2, got %d", c.Inversion)
	}
	for i := 0; i < 5; i++ {
		sidebarClick(t, g, sbMinus(fieldChordInv))
	}
	if c := chordOf(t, g, node.ID); c.Inversion != 0 {
		t.Fatalf("inversion clamps at root, got %d", c.Inversion)
	}
	for i := 0; i < 5; i++ {
		sidebarClick(t, g, sbPlus(fieldChordSpread))
	}
	if c := chordOf(t, g, node.ID); c.Spread != model.ChordSpreadWide {
		t.Fatalf("spread clamps at wide, got %d", c.Spread)
	}
	// A 7th chord has a 3rd inversion.
	g.setNodeChord(node.ID, model.ChordSpec{Quality: "maj7"})
	for i := 0; i < 5; i++ {
		sidebarClick(t, g, sbPlus(fieldChordInv))
	}
	if c := chordOf(t, g, node.ID); c.Inversion != 3 {
		t.Fatalf("maj7 inversion clamps at 3, got %d", c.Inversion)
	}
}

// Each stepper press is ONE undo step.
func TestChordStepperPressIsOneUndoStep(t *testing.T) {
	g, node := chordSidebarGame(t, roster.OrganChurch)
	sidebarClick(t, g, sbAudible)
	sidebarClick(t, g, sbPlus(fieldChordInv))
	sidebarClick(t, g, sbPlus(fieldChordInv))
	g.undoManager.Undo()
	n := g.nodeAt(node.I, node.J) // undo re-imports: find the node by position
	if n == nil {
		t.Fatal("test node gone after undo")
	}
	if c := chordOf(t, g, n.ID); c.Inversion != 1 {
		t.Fatalf("one undo after two presses: inversion %d, want 1", c.Inversion)
	}
}

func TestChordSingleNoteCollapsesAndKeepsTheSpec(t *testing.T) {
	g, node := chordSidebarGame(t, roster.OrganChurch)
	sidebarClick(t, g, sbAudible)
	sidebarClick(t, g, sbPlus(fieldChordSpread))
	want := chordOf(t, g, node.ID)
	sidebarClick(t, g, sbChordSingle)
	if got := nodeTypeOf(t, g, node.ID); got != model.NodeTypeRegular {
		t.Fatalf("Single note left type %s", got.String())
	}
	if got := chordOf(t, g, node.ID); got != want {
		t.Fatalf("Single note dropped the spec: %+v, want %+v", got, want)
	}
}

// Review Focus 1 (second half): a chord node whose row is switched to an
// unpitched instrument keeps its type and keeps sounding a stack.
func TestChordNodeOnRowSwitchedToUnpitchedKeepsPlaying(t *testing.T) {
	g, node := chordSidebarGame(t, roster.OrganChurch)
	sidebarClick(t, g, sbAudible)
	g.drum.SetInstrument(roster.Hihat) // row 0 is the selected row
	if got := nodeTypeOf(t, g, node.ID); got != model.NodeTypeChord {
		t.Fatalf("switching the row's instrument changed the node to %s", got.String())
	}
	info := model.BeatInfo{NodeID: node.ID, NodeType: model.NodeTypeChord}
	if l := g.playbackSlate.nodeLayersAt(info); len(l) != 3 {
		t.Fatalf("chord on an unpitched row sounds %d voices, want 3", len(l))
	}
}
