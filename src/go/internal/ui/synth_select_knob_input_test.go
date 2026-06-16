//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestKnobPress_SelectsKnobForFocusGraph_ViaTreeDispatch proves that pressing a
// synth knob makes it the SELECTED knob (the value the focus graph explains).
// The press is dispatched through the REAL production input path —
// SetInputForTest + g.Update() → DrumViewTree.handleInput → HitIndex.At →
// synthKnobHitAdapter.OnPress — NOT a synthetic call to a dead handler.
func TestKnobPress_SelectsKnobForFocusGraph_ViaTreeDispatch(t *testing.T) {
	g := newModularSynthTabGame(t)
	expandSynthPanelForTest(t, g)
	dv := g.drum
	inst := dv.resolveSynthInstrument(dv.synthTabActiveInstrument())
	sec := dv.synthSelectedSection()
	if inst == "" || sec == nil || len(sec.knobIdxs) < 2 {
		t.Skip("need a section with >=2 visible knobs")
	}
	target := sec.knobIdxs[1]
	// Bring the target knob on-screen so it has a non-empty, hit-testable rect.
	selectSectionForKnobIdx(t, g, inst, target)
	sec = dv.synthSelectedSection()
	if sec == nil {
		t.Skip("section deselected after scroll")
	}
	k := dv.instEditorKnobs[target]
	if k.Rect().Empty() {
		t.Skip("target knob not laid out")
	}
	cx := k.Rect().Min.X + k.Rect().Dx()/2
	cy := k.Rect().Min.Y + k.Rect().Dy()/2

	mouseX, mouseY := cx, cy
	pressed := false
	restore := SetInputForTest(
		func() (int, int) { return mouseX, mouseY },
		func(b ebiten.MouseButton) bool { return pressed && b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 1280, 720 },
	)
	defer restore()

	// Press at the knob center through the production tree dispatcher.
	pressed = true
	g.Update()
	pressed = false
	g.Update()

	if got := dv.synthSelectedKnobIdx(inst, dv.synthSelectedSection()); got != target {
		t.Fatalf("after pressing knob %d, selected = %d", target, got)
	}
}

// TestHover_DoesNotChangeSelection guards that merely having the pointer over a
// DIFFERENT knob (no press) never reassigns the focus-graph selection — there
// is no hover path by design, and a hover-driven selection would thrash the
// graph as the cursor crosses dials. We position the cursor over the second
// knob and run a normal Update tick WITHOUT pressing the mouse; the selection
// must stay on the first knob we set explicitly.
func TestHover_DoesNotChangeSelection(t *testing.T) {
	g := newModularSynthTabGame(t)
	expandSynthPanelForTest(t, g)
	dv := g.drum
	inst := dv.resolveSynthInstrument(dv.synthTabActiveInstrument())
	sec := dv.synthSelectedSection()
	if inst == "" || sec == nil || len(sec.knobIdxs) < 2 {
		t.Skip("need a section with >=2 visible knobs")
	}
	dv.setSynthSelectedKnob(inst, sec.knobIdxs[0])
	target := sec.knobIdxs[1]
	selectSectionForKnobIdx(t, g, inst, target)
	if dv.synthSelectedSection() == nil {
		t.Skip("section deselected after scroll")
	}
	k := dv.instEditorKnobs[target]
	if k.Rect().Empty() {
		t.Skip("target not laid out")
	}
	cx := k.Rect().Min.X + k.Rect().Dx()/2
	cy := k.Rect().Min.Y + k.Rect().Dy()/2

	// Cursor over the second knob, mouse button NOT pressed (pure hover).
	restore := SetInputForTest(
		func() (int, int) { return cx, cy },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 1280, 720 },
	)
	defer restore()
	g.Update()

	if got := dv.synthSelectedKnobIdx(inst, dv.synthSelectedSection()); got != sec.knobIdxs[0] {
		t.Fatalf("hover changed selection to %d (should stay %d)", got, sec.knobIdxs[0])
	}
}
