//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// noteTestNode adds a real graph node and maps it onto the existing row 0 so
// nodeNoteLabel can resolve node → row → instrument deterministically (the
// nodeRows map is exactly that resolution's input).
func noteTestNode(t *testing.T, g *Game) *uiNode {
	t.Helper()
	if len(g.drum.Rows) == 0 {
		t.Fatal("test game has no rows")
	}
	node := g.tryAddNode(7, 7, model.NodeTypeRegular)
	if node == nil {
		t.Fatal("failed to add node")
	}
	g.nodeRows[node.ID] = 0
	return node
}

func TestNodeNoteLabel_MelodicDerivesNote(t *testing.T) {
	g := newTestGameForUndo(t)
	node := noteTestNode(t, g)

	const inst = "test-note-inst-mel"
	g.drum.Rows[0].Instrument = inst
	// Unbound (sampler-style) instrument is pitched; a +3 semitone transpose
	// puts A3 (node pitch 0) at C4.
	audio.SetSampleEdit(inst, audio.SampleEdit{TransposeSemis: 3})
	t.Cleanup(func() { audio.ClearSampleEdit(inst) })

	if got := g.nodeNoteLabel(node.ID, 0); got != "C4" {
		t.Fatalf("note at pitch 0 = %q, want C4 (A3 + 3 semitones)", got)
	}
	// Node pitch stacks on top of the instrument offset: -3 cancels back to A3.
	if got := g.nodeNoteLabel(node.ID, -3); got != "A3" {
		t.Fatalf("note at pitch -3 = %q, want A3", got)
	}
	// +12 is one octave above the C4 baseline.
	if got := g.nodeNoteLabel(node.ID, 12); got != "C5" {
		t.Fatalf("note at pitch +12 = %q, want C5", got)
	}
}

func TestNodeNoteLabel_PercussionSuppressed(t *testing.T) {
	g := newTestGameForUndo(t)
	node := noteTestNode(t, g)

	const inst = "test-note-inst-drum"
	g.drum.Rows[0].Instrument = inst
	audio.BindInstrumentToRecipe(inst, "drum-kick") // drum category → not pitched
	t.Cleanup(func() { audio.BindInstrumentToRecipe(inst, "") })

	if got := g.nodeNoteLabel(node.ID, 5); got != "" {
		t.Fatalf("percussion note = %q, want empty (suppressed)", got)
	}
}

// TestNodeNoteLabel_RealDemoInstruments validates classification against the
// actual shipped instrument set (as bound by the real engine init), not
// synthetic test recipes: a melodic instrument yields a note, a percussion
// instrument is suppressed.
func TestNodeNoteLabel_RealDemoInstruments(t *testing.T) {
	g := newTestGameForUndo(t)
	node := noteTestNode(t, g)

	// organ-church is a melodic (modular) instrument in the default demo.
	g.drum.Rows[0].Instrument = "organ-church"
	if got := g.nodeNoteLabel(node.ID, 0); got == "" {
		t.Errorf("melodic organ-church should yield a note, got empty")
	}

	// dnb-kick is percussion — a musical note would mislead.
	g.drum.Rows[0].Instrument = "dnb-kick"
	if got := g.nodeNoteLabel(node.ID, 0); got != "" {
		t.Errorf("percussion dnb-kick should be suppressed, got %q", got)
	}
}

// TestNodeSidebar_DrawsNoteChip drives the real sidebar Draw with a melodic
// node and asserts the read-only note chip is actually painted in the pitch
// row's left gap (a filled rounded rect left of the "−" stepper button). This
// exercises the render integration, not just the derivation helper.
func TestNodeSidebar_DrawsNoteChip(t *testing.T) {
	g := newTestGameForUndo(t)
	g.Layout(800, 1200)
	node := noteTestNode(t, g)

	const inst = "test-note-render-inst"
	g.drum.Rows[0].Instrument = inst
	audio.SetSampleEdit(inst, audio.SampleEdit{TransposeSemis: 3}) // pitched → note "C4"
	t.Cleanup(func() { audio.ClearSampleEdit(inst) })

	g.sidebar.Open(node)
	g.sidebar.ExpandAllSections()
	g.sidebar.layout()

	pitMinus, ok := g.sidebar.rects["pit-"]
	if !ok || pitMinus.Empty() || !g.sidebar.inViewport(pitMinus) {
		t.Skip("pitch stepper not laid out / out of viewport")
	}

	orig := drawRoundedRect
	t.Cleanup(func() { drawRoundedRect = orig })
	found := false
	drawRoundedRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, radius int, filled bool) {
		// The note chip is the only filled rounded rect drawn LEFT of the "−"
		// button within the pitch row's Y band.
		if filled && !r.Empty() && r.Max.X <= pitMinus.Min.X &&
			r.Min.Y >= pitMinus.Min.Y-2 && r.Max.Y <= pitMinus.Max.Y+2 {
			found = true
		}
		orig(dst, r, c, radius, filled)
	}

	g.sidebar.Draw(ebiten.NewImage(800, 1200))

	if !found {
		t.Error("sidebar drew no note chip in the pitch row for a melodic instrument")
	}
}

func TestNodeNoteLabel_NoRowReturnsEmpty(t *testing.T) {
	g := newTestGameForUndo(t)
	// A node id with no nodeRows entry cannot resolve an instrument.
	if got := g.nodeNoteLabel(model.NodeID(99999), 0); got != "" {
		t.Fatalf("unmapped node note = %q, want empty", got)
	}
}
