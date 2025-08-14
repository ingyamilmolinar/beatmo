//go:build test

package ui

import "testing"

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
