package ui

import (
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

// Ensure that clicking the BPM + button updates the game's BPM and that a
// draw call between frames does not revert the change.
func TestBPMButtonsSurviveDrawLoop(t *testing.T) {
	g := New(testLogger)
	g.Layout(800, 600)

	// Click the + button via its handler to avoid input geometry flakiness.
	g.drum.bpmIncBtn.OnClick()
	_ = g.Update()

	// Capture the new BPM after the click.
	afterClick := g.drum.BPM()
	if afterClick <= 120 {
		t.Fatalf("expected BPM to increase after + click; got %d", afterClick)
	}

	// Simulate a draw call (end of frame). This must not reset the UI BPM.
	g.drawDrumPane(ebiten.NewImage(1, 1))

	// Next frame: Update should read the DrumView BPM and keep it.
	_ = g.Update()
	if g.bpm != afterClick || g.drum.BPM() != afterClick {
		t.Fatalf("BPM reverted across draw: g=%d dv=%d want %d", g.bpm, g.drum.BPM(), afterClick)
	}

	// Engine/applied BPM should converge asynchronously.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		_ = g.Update()
		if g.appliedBPM == afterClick && g.engine.BPM() == afterClick {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	if g.appliedBPM != afterClick || g.engine.BPM() != afterClick {
		t.Fatalf("engine/applied BPM not updated: engine=%d applied=%d target=%d", g.engine.BPM(), g.appliedBPM, afterClick)
	}
}

// Ensure that committing a BPM value via the text box survives a draw call
// during the same frame and affects playback.
func TestBPMEditorSurvivesDrawLoop(t *testing.T) {
	g := New(testLogger)
	g.Layout(800, 600)

	// Simulate a user edit by setting BPM directly, then propagate.
	g.drum.SetBPM(200)
	_ = g.Update()

	// Simulate draw and ensure it doesn't override the UI BPM value.
	g.drawDrumPane(ebiten.NewImage(1, 1))
	_ = g.Update()
	if g.bpm != 200 || g.drum.BPM() != 200 {
		t.Fatalf("BPM reverted after draw: g=%d dv=%d", g.bpm, g.drum.BPM())
	}

	// Wait briefly for engine/applied BPM to converge.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		_ = g.Update()
		if g.appliedBPM == 200 && g.engine.BPM() == 200 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("engine/applied BPM not updated: engine=%d applied=%d", g.engine.BPM(), g.appliedBPM)
}
