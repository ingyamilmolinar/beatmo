//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// The BPM readout opens the shared wheel: each detent changes the tempo live, closing records one undo step, Esc
// restores and records nothing.

func newBPMWheelGame(t *testing.T) (*Game, func(x, y, frames int)) {
	t.Helper()
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	advanceFrames(g, 2)
	g.Draw(ebiten.NewImage(1280, 720))
	advanceFrames(g, 1)
	return g, makeHoldTap(g, 1280, 720)
}

func openBPMWheelByTap(t *testing.T, g *Game, tap func(x, y, frames int)) {
	t.Helper()
	r := g.drum.bpmBox().Rect
	if r.Empty() {
		t.Fatal("BPM readout has no rect")
	}
	x, y := center(r)
	tap(x, y, 3)
	advanceFrames(g, 2)
	if g.drum.valueWheelPopup == nil || !g.drum.valueWheelPopup.IsOpen() {
		t.Fatal("tapping the BPM readout must open the value wheel")
	}
}

func TestBPMWheelAppliesLiveAndCommitsOnce(t *testing.T) {
	g, tap := newBPMWheelGame(t)
	start := g.drum.BPM()
	undo0 := len(g.undoManager.undo)
	openBPMWheelByTap(t, g, tap)
	for i := 0; i < 3; i++ {
		pressKeyThroughShortcuts(g, ebiten.KeyArrowUp)
	}
	if got := g.drum.BPM(); got != start+3 {
		t.Fatalf("each detent must change the tempo live: %d → %d", start, got)
	}
	if n := len(g.undoManager.undo); n != undo0 {
		t.Fatalf("no undo step may be recorded mid-gesture (%d → %d)", undo0, n)
	}
	pressKeyThroughShortcuts(g, ebiten.KeyEnter)
	advanceFrames(g, 1)
	if g.drum.valueWheelPopup.IsOpen() {
		t.Fatal("Enter must close the BPM wheel")
	}
	if n := len(g.undoManager.undo); n != undo0+1 {
		t.Fatalf("closing must record exactly ONE undo step (%d → %d)", undo0, n)
	}
	if got := g.drum.BPM(); got != start+3 {
		t.Fatalf("the committed tempo must stay: %d", got)
	}
}

func TestBPMWheelEscRestoresAndRecordsNothing(t *testing.T) {
	g, tap := newBPMWheelGame(t)
	start := g.drum.BPM()
	undo0 := len(g.undoManager.undo)
	openBPMWheelByTap(t, g, tap)
	pressKeyThroughShortcuts(g, ebiten.KeyArrowRight) // coarser: ±5
	for i := 0; i < 4; i++ {
		pressKeyThroughShortcuts(g, ebiten.KeyArrowUp)
	}
	if g.drum.BPM() == start {
		t.Fatal("the wheel did not move the tempo")
	}
	restore := stubKeys(nil, map[ebiten.Key]bool{ebiten.KeyEscape: true})
	advanceFrames(g, 1)
	restore()
	advanceFrames(g, 1)
	if g.drum.valueWheelPopup.IsOpen() {
		t.Fatal("Esc must close the BPM wheel")
	}
	if got := g.drum.BPM(); got != start {
		t.Fatalf("Esc must restore the tempo %d, got %d", start, got)
	}
	if n := len(g.undoManager.undo); n != undo0 {
		t.Fatalf("Esc must record no undo step (%d → %d)", undo0, n)
	}
}

func TestBPMWheelLadderAndRange(t *testing.T) {
	g, tap := newBPMWheelGame(t)
	openBPMWheelByTap(t, g, tap)
	m, ok := g.drum.valueWheelPopup.model().(*NumberModel)
	if !ok {
		t.Fatalf("BPM wheel model %T", g.drum.valueWheelPopup.model())
	}
	if m.Min != 1 || m.Max != maxBPM {
		t.Fatalf("range %v..%v, want 1..%d", m.Min, m.Max, maxBPM)
	}
	want := []string{"±1", "±5", "±10", "±50"}
	for i, w := range want {
		if got := m.LadderLabel(i); got != w {
			t.Errorf("rung %d label %q want %q", i, got, w)
		}
	}
	if m.LadderIndex() != 0 {
		t.Errorf("default rung ±1")
	}
}

func TestBPMPlusMinusButtonsStillStepByOne(t *testing.T) {
	g, tap := newBPMWheelGame(t)
	start := g.drum.BPM()
	r := g.drum.transportZone.bpmIncBtn.Rect()
	x, y := center(r)
	tap(x, y, 2)
	advanceFrames(g, 2)
	if got := g.drum.BPM(); got != start+1 {
		t.Fatalf("+ must still step the tempo by one: %d → %d", start, got)
	}
}
