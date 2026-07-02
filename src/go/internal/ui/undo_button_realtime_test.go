//go:build test

package ui

import (
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestSynthKnobUndoButtonReactsSameFrame proves the transport Undo button
// reacts to a Synth-tab knob change in REAL TIME: on the very frame the knob
// gesture completes (its undo step is recorded), the button must render lit on
// THAT frame's Draw — not one (or more) frames later. The bug: syncUndoRedoVisual
// ran in Update's Tick phase, BEFORE the frame dispatched the knob release and
// committed the undo step (at the per-frame endGroup), so the toolbar painted the
// stale previous-frame CanUndo and the button only lit on a subsequent frame —
// perceived as the buttons lagging behind knob changes.
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
	cx, cy := r.Min.X+r.Dx()/2, r.Min.Y+r.Dx()/2 // dial center

	mx, my := cx, cy
	pressed := false
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(b ebiten.MouseButton) bool { return pressed && b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 1280, 720 },
	)
	defer restore()

	g.drum.eqPanelZone.Invalidate()
	g.undoManager.OnExternalLoad()
	tz := g.drum.transportZone

	// Baseline frame: nothing to undo → greyed.
	g.Update()
	g.Draw(img)
	if iconColorOf(tz.undoBtn) != color.Color(colTextDisabled) {
		t.Fatalf("precondition: undo should be greyed before any edit, got %v", iconColorOf(tz.undoBtn))
	}

	// Drag the knob horizontally, then release. The LAST Update processes the
	// release and records the undo step.
	pressed = true
	g.Update()
	for f := 1; f <= 5; f++ {
		mx = cx + f*10
		g.Update()
	}
	pressed = false
	g.Update() // release frame: records the undo step at endGroup

	if !g.undoManager.CanUndo() {
		t.Fatal("knob release should have recorded an undo step")
	}

	// SAME frame's Draw must show the button lit — no extra Update/frame allowed.
	g.Draw(img)
	if got := iconColorOf(tz.undoBtn); got != color.Color(colTextPrimary) {
		t.Fatalf("undo button did not react on the same frame as the knob change "+
			"(got %v, want lit %v) — transport reflects stale CanUndo, lagging behind user events",
			got, color.Color(colTextPrimary))
	}
}
