//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// Through the real input path, not a direct call.
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
	k := dv.instEditorControls[target]
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

	if got := dv.synthSelectedControlIdx(inst, dv.synthSelectedSection()); got != target {
		t.Fatalf("after pressing knob %d, selected = %d", target, got)
	}
}

// There is no hover path by design: a hover-driven selection would thrash the graph as the cursor crosses dials.
func TestHover_DoesNotChangeSelection(t *testing.T) {
	g := newModularSynthTabGame(t)
	expandSynthPanelForTest(t, g)
	dv := g.drum
	inst := dv.resolveSynthInstrument(dv.synthTabActiveInstrument())
	sec := dv.synthSelectedSection()
	if inst == "" || sec == nil || len(sec.knobIdxs) < 2 {
		t.Skip("need a section with >=2 visible knobs")
	}
	dv.setSynthSelectedControl(inst, sec.knobIdxs[0])
	target := sec.knobIdxs[1]
	selectSectionForKnobIdx(t, g, inst, target)
	if dv.synthSelectedSection() == nil {
		t.Skip("section deselected after scroll")
	}
	k := dv.instEditorControls[target]
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

	if got := dv.synthSelectedControlIdx(inst, dv.synthSelectedSection()); got != sec.knobIdxs[0] {
		t.Fatalf("hover changed selection to %d (should stay %d)", got, sec.knobIdxs[0])
	}
}
