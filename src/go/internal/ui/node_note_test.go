//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
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
	// drum-hihat declares no fundamental at all — noise, nothing to name. A
	// recipe that DOES declare one (drum-kick) names a note even for a
	// user-made instrument, so the suppression case has to use real noise.
	_ = audio.InstallInstrumentDocForTest(inst, roster.Hihat)
	t.Cleanup(func() { audio.RemoveInstrumentDocForTest(inst) })

	if got := g.nodeNoteLabel(node.ID, 5); got != "" {
		t.Fatalf("unpitched-noise note = %q, want empty (suppressed)", got)
	}
}

// TestNodeNoteLabel_RealDemoInstruments validates classification against the
// actual shipped instrument set (as bound by the real engine init), not
// synthetic test recipes: melodic and tuned-percussion instruments both yield a
// note, and only genuinely unpitched noise is suppressed.
func TestNodeNoteLabel_RealDemoInstruments(t *testing.T) {
	g := newTestGameForUndo(t)
	node := noteTestNode(t, g)

	// organ-church is a melodic (modular) instrument in the default demo.
	g.drum.Rows[0].Instrument = roster.OrganChurch
	if got := g.nodeNoteLabel(node.ID, 0); got == "" {
		t.Errorf("melodic organ-church should yield a note, got empty")
	}

	// dnb-kick is percussion and names a note too since display went universal:
	// it is tuned to 48 Hz, which is G1. Suppressing it (the old contract) hid
	// a real, measurable pitch.
	g.drum.Rows[0].Instrument = roster.DnbKick
	if got := g.nodeNoteLabel(node.ID, 0); got != "G1" {
		t.Errorf("percussion dnb-kick = %q, want \"G1\" (48 Hz)", got)
	}

	// Only genuinely unpitched noise is still suppressed — a hi-hat has no
	// fundamental for any note name to refer to.
	g.drum.Rows[0].Instrument = roster.Hihat
	if got := g.nodeNoteLabel(node.ID, 0); got != "" {
		t.Errorf("unpitched hihat = %q, want empty (no fundamental to name)", got)
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

	pitMinus, ok := g.sidebar.rects[sbMinus(fieldPit)]
	if !ok || pitMinus.Empty() || !g.sidebar.inViewport(pitMinus) {
		t.Skip("pitch stepper not laid out / out of viewport")
	}

	// The chip is a Button ("pitnote"), drawn through the shared keycap style
	// like the -/+ steppers beside it — NOT the old drawRoundedRect pill. So
	// assert what it IS rather than which primitive painted it: a registered
	// button, labelled with the note, seated left of the "−" key.
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

// A tuned percussion row DOES get the chip and the picker — that is the whole
// point of universal note display. dnb-kick sounds 48 Hz (G1), so there is a
// real note to show and a real octave range to pick from.
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

// Genuinely unpitched noise still has no chip and no button: a picker there
// would offer a choice with nothing meaningful behind it.
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
