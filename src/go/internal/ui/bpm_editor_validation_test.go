package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// Typing non-digits should not change BPM on commit and should trigger error highlight.
func TestBPMEditorRejectsNonDigits(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	_ = g.Update()

	// Focus editor, clear, type a mix of digits and letters, then press Enter.
	focusTextInput(t, g.drum, g.drum.bpmBox())
	g.drum.bpmBox().SetText("")
	// Try to type mix of digits and letters; final commit should be rejected.
	chars := []rune{'1'}
	enter := false
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return enter && k == ebiten.KeyEnter },
		func() []rune { c := chars; chars = nil; return c },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	defer restore()
	_ = g.Update()
	// feed 'a', '9', 'X'
	chars = []rune{'a', '9', 'X'}
	_ = g.Update()
	// Trigger Enter via isKeyPressed override on final update.
	enter = true
	_ = g.Update()

	if g.drum.BPM() != 120 {
		t.Fatalf("BPM changed unexpectedly: %d", g.drum.BPM())
	}
	if g.drum.bpmErrorAnim == 0 {
		t.Fatalf("expected bpm error highlight on invalid input")
	}
}

// Empty BPM input should be rejected and leave the current BPM untouched.
func TestBPMEditorRejectsEmpty(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	_ = g.Update()

	focusTextInput(t, g.drum, g.drum.bpmBox())
	g.drum.bpmBox().SetText("")

	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return k == ebiten.KeyEnter },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	defer restore()

	_ = g.Update()

	if g.drum.BPM() != 120 {
		t.Fatalf("BPM changed unexpectedly: %d", g.drum.BPM())
	}
	if g.drum.bpmErrorAnim != 0 {
		t.Fatalf("unexpected bpm error highlight on empty input")
	}
}
