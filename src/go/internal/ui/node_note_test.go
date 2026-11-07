//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Mapped onto row 0 so node → row → instrument resolves deterministically.
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
	// An unbound (sampler-style) instrument is pitched; +3 semitones puts A3 (pitch 0) at C4.
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
	// Real noise: any document that declares a fundamental names a note, user-made or not.
	_ = audio.InstallInstrumentDocForTest(inst, roster.Hihat)
	t.Cleanup(func() { audio.RemoveInstrumentDocForTest(inst) })

	if got := g.nodeNoteLabel(node.ID, 5); got != "" {
		t.Fatalf("unpitched-noise note = %q, want empty (suppressed)", got)
	}
}

// The shipped instruments: melodic and tuned percussion name a note, only unpitched noise does not.
func TestNodeNoteLabel_RealDemoInstruments(t *testing.T) {
	g := newTestGameForUndo(t)
	node := noteTestNode(t, g)

	// organ-church is a melodic (modular) instrument in the default demo.
	g.drum.Rows[0].Instrument = roster.OrganChurch
	if got := g.nodeNoteLabel(node.ID, 0); got == "" {
		t.Errorf("melodic organ-church should yield a note, got empty")
	}

	// Tuned to 48 Hz, G1.
	g.drum.Rows[0].Instrument = roster.DnbKick
	if got := g.nodeNoteLabel(node.ID, 0); got != "G1" {
		t.Errorf("percussion dnb-kick = %q, want \"G1\" (48 Hz)", got)
	}

	// A hi-hat has no fundamental for a note name to refer to.
	g.drum.Rows[0].Instrument = roster.Hihat
	if got := g.nodeNoteLabel(node.ID, 0); got != "" {
		t.Errorf("unpitched hihat = %q, want empty (no fundamental to name)", got)
	}
}

// Through the real sidebar Draw: the chip is painted in the pitch row's gap, left of the "−" stepper.
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

	pitMinus, ok := g.sidebar.rects[sbMinus(fieldPit)]
	if !ok || pitMinus.Empty() || !g.sidebar.inViewport(pitMinus) {
		t.Skip("pitch stepper not laid out / out of viewport")
	}

	// The chip is a Button drawn like the steppers beside it, so assert what it is: registered, labelled with the
	// note, left of "−".
	chip, ok := g.sidebar.rects[sbPitchNote]
	if !ok || chip.Empty() {
		t.Fatal("sidebar reserved no note-chip rect in the pitch row for a melodic instrument")
	}
	if chip.Max.X > pitMinus.Min.X {
		t.Errorf("note chip %v overlaps the − button %v; it must sit to its left", chip, pitMinus)
	}
	if chip.Min.Y < pitMinus.Min.Y-2 || chip.Max.Y > pitMinus.Max.Y+2 {
		t.Errorf("note chip %v is not aligned with the pitch row %v", chip, pitMinus)
	}
	btn := g.sidebar.btns[sbPitchNote]
	if btn == nil {
		t.Fatal("note chip is not a button — it must open the note matrix")
	}
	if want := g.nodeNoteLabel(node.ID, 0); btn.Text != want {
		t.Errorf("note chip label = %q, want %q", btn.Text, want)
	}

	// Drawing must not panic and must reach the button.
	g.sidebar.Draw(ebiten.NewImage(800, 1200))
}

// dnb-kick sounds 48 Hz (G1): a real note to show and a real octave range to pick from.
func TestNodeSidebar_NoteChipForTunedPercussion(t *testing.T) {
	g := newTestGameForUndo(t)
	g.Layout(800, 1200)
	node := noteTestNode(t, g)
	g.drum.Rows[0].Instrument = roster.DnbKick

	g.sidebar.Open(node)
	g.sidebar.ExpandAllSections()
	g.sidebar.layout()

	btn := g.sidebar.btns[sbPitchNote]
	if btn == nil {
		t.Fatal("tuned percussion row registered no note-chip button")
	}
	if want := "G1"; btn.Text != want {
		t.Errorf("note chip label = %q, want %q", btn.Text, want)
	}
}

// A picker there would offer a choice with nothing behind it.
func TestNodeSidebar_NoNoteChipForUnpitchedNoise(t *testing.T) {
	g := newTestGameForUndo(t)
	g.Layout(800, 1200)
	node := noteTestNode(t, g)
	g.drum.Rows[0].Instrument = roster.Hihat

	g.sidebar.Open(node)
	g.sidebar.ExpandAllSections()
	g.sidebar.layout()

	if btn := g.sidebar.btns[sbPitchNote]; btn != nil {
		t.Error("unpitched-noise row registered a note-chip button")
	}
	_ = node
}

func TestNodeNoteLabel_NoRowReturnsEmpty(t *testing.T) {
	g := newTestGameForUndo(t)
	// A node id with no nodeRows entry cannot resolve an instrument.
	if got := g.nodeNoteLabel(model.NodeID(99999), 0); got != "" {
		t.Fatalf("unmapped node note = %q, want empty", got)
	}
}
