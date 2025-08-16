//go:build test

package ui

import (
        "image"
        "testing"

        "github.com/hajimehoshi/ebiten/v2"
)

// TestControlButtonsClickable ensures that top-panel buttons respond to clicks
// when unobstructed.
func TestControlButtonsClickable(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	g.drum.recalcButtons()

	buttons := []*Button{g.drum.playBtn, g.drum.stopBtn, g.drum.bpmDecBtn, g.drum.bpmIncBtn, g.drum.lenDecBtn, g.drum.lenIncBtn, g.drum.uploadBtn}
	for i, btn := range buttons {
		called := false
		btn.OnClick = func() { called = true }
		r := btn.Rect()
		click(g, r.Min.X+1, r.Min.Y+1)
		if !called {
			t.Fatalf("button %d (%s) not clickable", i, btn.Text)
		}
	}
}

// TestButtonsDoNotOverlap verifies that control-panel buttons have disjoint
// rectangles so clicks are unambiguous.
func TestButtonsDoNotOverlap(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	g.drum.recalcButtons()

	buttons := []*Button{g.drum.playBtn, g.drum.stopBtn, g.drum.bpmDecBtn, g.drum.bpmIncBtn, g.drum.lenDecBtn, g.drum.lenIncBtn, g.drum.uploadBtn}
	for i := 0; i < len(buttons); i++ {
		ri := buttons[i].Rect()
		for j := i + 1; j < len(buttons); j++ {
			if ri.Overlaps(buttons[j].Rect()) {
				t.Fatalf("buttons %d and %d overlap", i, j)
			}
		}
	}
}

// TestButtonHoldRepeat verifies that holding a repeat-enabled button triggers
// repeats after a 1s delay and then every ~250ms.
func TestButtonHoldRepeat(t *testing.T) {
	b := NewButton("+", ButtonStyle{}, nil)
	b.Repeat = true
	b.SetRect(image.Rect(0, 0, 10, 10))

	var frame int
	var calls []int
	b.OnClick = func() { calls = append(calls, frame) }

	for frame = 0; frame < 100; frame++ {
		b.Handle(5, 5, true)
	}
	b.Handle(5, 5, false)

	want := []int{0, 74, 89}
	if len(calls) != len(want) {
		t.Fatalf("expected %d callbacks, got %v", len(want), calls)
	}
	for i, v := range want {
		if calls[i] != v {
			t.Fatalf("expected call at %d, got %v", v, calls)
		}
	}
}

func TestBPMHoldIncrements(t *testing.T) {
        g := New(testLogger)
        g.Layout(640, 480)
        dv := g.drum
        dv.recalcButtons()
        btn := dv.bpmIncBtn
        x, y := btn.Rect().Min.X+1, btn.Rect().Min.Y+1
        pressed := true
        restore := SetInputForTest(func() (int, int) { return x, y }, func(ebiten.MouseButton) bool { return pressed }, func(ebiten.Key) bool { return false }, func() []rune { return nil }, func() (float64, float64) { return 0, 0 }, func() (int, int) { return 800, 600 })
        for i := 0; i < 100; i++ {
                dv.Update()
        }
        pressed = false
        dv.Update()
        restore()
        if dv.bpm != 123 {
                t.Fatalf("expected BPM 123 got %d", dv.bpm)
        }
}
