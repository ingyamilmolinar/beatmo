//go:build test

package ui

import (
	"image"
	"testing"
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
// multiple callbacks with accelerating intervals.
func TestButtonHoldRepeat(t *testing.T) {
	b := NewButton("+", ButtonStyle{}, nil)
	b.Repeat = true
	b.SetRect(image.Rect(0, 0, 10, 10))

	var frame int
	var calls []int
	b.OnClick = func() { calls = append(calls, frame) }

	for frame = 0; frame < 60; frame++ {
		b.Handle(5, 5, true)
	}
	b.Handle(5, 5, false)

	if len(calls) < 3 {
		t.Fatalf("expected multiple callbacks, got %d", len(calls))
	}
	for i := 2; i < len(calls); i++ {
		if calls[i]-calls[i-1] > calls[i-1]-calls[i-2] {
			t.Fatalf("intervals not accelerating: %v", calls)
		}
	}
}
