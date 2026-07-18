//go:build test

package ui

import (
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestSynthKnobUndoButtonReactsSameFrame proves the transport Undo button reacts
// to a Synth-tab value edit in REAL TIME. Task-5 value edits are the desktop
// wheel nudge; its single undo step commits when the debounce elapses (inside an
// Update's synthTabUpdate). On the very frame the commit lands, that frame's Draw
// must already render the button lit — not one or more frames later.
func TestSynthKnobUndoButtonReactsSameFrame(t *testing.T) {
	g := newModularSynthTabGame(t)
	expandSynthPanelForTest(t, g)
	idx := synthIdxByParam(t, g, "filter_cutoff")
	selectSectionForKnobIdx(t, g, "ut-chip-modular", idx)
	g.Update()

	img := newTrackedImage("test.knobrealtime", 1280, 720)
	defer releaseImage(img)
	g.Draw(img)

	k := g.drum.SynthTabKnobs()[idx]
	r := k.Rect()
	if r.Empty() {
		t.Fatal("knob rect empty")
	}
	cx, cy := r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2

	wheelActive := false
	restore := SetInputForTest(
		func() (int, int) { return cx, cy },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) {
			if wheelActive {
				return 0, 1
			}
			return 0, 0
		},
		func() (int, int) { return 1280, 720 },
	)
	defer restore()

	g.drum.eqPanelZone.Invalidate()
	g.undoManager.OnExternalLoad()
	tz := g.drum.transportZone

	// Baseline: nothing to undo -> greyed.
	g.Update()
	g.Draw(img)
	if iconColorOf(tz.undoBtn) != color.Color(colTextDisabled) {
		t.Fatalf("precondition: undo should be greyed before any edit, got %v", iconColorOf(tz.undoBtn))
	}

	// Spin a few wheel notches (mutates live audio, records nothing yet).
	for s := 0; s < 3; s++ {
		g.drum.eqPanelZone.Invalidate()
		wheelActive = true
		g.Update()
		wheelActive = false
		g.Update()
	}
	if g.undoManager.CanUndo() {
		t.Fatal("undo step must NOT be recorded mid-spin (before the debounce elapses)")
	}

	// Run the debounce down; the commit frame's Draw must show the button lit.
	committed := false
	for i := 0; i < wheelNudgeCommitFrames+8; i++ {
		g.Update()
		if g.undoManager.CanUndo() {
			committed = true
			g.Draw(img) // SAME frame as the commit
			if got := iconColorOf(tz.undoBtn); got != color.Color(colTextPrimary) {
				t.Fatalf("undo button did not react on the commit frame (got %v, want lit %v)", got, color.Color(colTextPrimary))
			}
			break
		}
	}
	if !committed {
		t.Fatal("wheel nudge never recorded an undo step after the debounce elapsed")
	}
}
