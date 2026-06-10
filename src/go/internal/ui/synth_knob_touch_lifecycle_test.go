//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestSynthTab_KnobMobileTouchLifecycle drives a real mobile touch lifecycle
// through the DrumViewTree dispatcher to prove the mobile-knob bug fix.
//
// Unlike TestSynthTab_KnobDragEndToEndChangesAudioParam (which calls the hit
// adapter's OnPress/OnDrag/OnRelease directly), this test sets up the
// globalTouchState mock so the dispatcher reads input through the same code
// path a real mobile browser uses: globalTouchState.Update → updateTouchOverride
// → cursorPosition() → DrumViewTree.handleInput → synthKnobHitAdapter.OnRelease.
//
// Pre-fix, the release frame in this lifecycle handed (0,0) to OnRelease,
// which Knob.HandleInputResult re-ran through updateFromDrag, snapping Value
// to 1.0 (the original user-reported bug). Post-fix, the touch override
// holds the lift coords for one frame AND Knob.HandleInputResult treats
// release as a commit-only event — both defenses prevent the max-snap.
func TestSynthTab_KnobMobileTouchLifecycle(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)

	uiNode := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.drum.Rows[0].Origin = uiNode.ID
	g.drum.Rows[0].Node = uiNode
	g.drum.Rows[0].Name = "Snare"
	g.drum.Rows[0].Instrument = "snare"
	audio.BindInstrumentToRecipe("snare", "drum-snare")
	t.Cleanup(func() { audio.ResetInstrumentParams("snare") })

	// Switch to Synth tab + force the panel to its expanded synth height so the
	// detail pane is tall enough to give the selected stage's knobs real rects.
	expandSynthPanelForTest(t, g)

	knobs := g.drum.SynthTabKnobs()
	bindings := g.drum.SynthTabBindings()
	if len(knobs) == 0 {
		t.Fatal("no knobs wired on Synth tab")
	}
	// Find decay knob.
	decayIdx := -1
	for i, b := range bindings {
		if b.def.Name == "decay" {
			decayIdx = i
			break
		}
	}
	if decayIdx < 0 {
		t.Fatal("no decay knob found in bindings")
	}
	// Decay lives in ENVELOPE; open that stage so the knob gets a non-empty,
	// hit-testable rect under the chip-strip + expand-one-detail-pane layout.
	// Only the selected stage's knobs publish hit areas now.
	selectSectionForKnobIdx(t, g, "snare", decayIdx)
	knob := g.drum.SynthTabKnobs()[decayIdx]
	rect := knob.Rect()
	if rect.Empty() {
		t.Fatalf("decay knob has empty rect")
	}
	cx := (rect.Min.X + rect.Max.X) / 2
	cy := (rect.Min.Y + rect.Max.Y) / 2

	// Publish the EQ panel's synth-tab hit areas to the tree's hit index.
	// Mirrors what drumview_layout.go does on a Layout pass; needed here
	// because the test bypasses g.Update()'s full layout/draw cycle.
	g.drum.tree.HitIndexRef().Update("eq-panel", g.drum.eqPanelZone.HitAreas())

	// Set up the mock touch state driving globalTouchState. We do NOT call
	// SetInputForTest — that would set inputForTestActive=true and short-
	// circuit updateTouchOverride, defeating the point of this test.
	mock := newMockTouchState()
	restoreTouch := SetTouchForTest(mock.TouchIDs, mock.TouchPosition)
	t.Cleanup(restoreTouch)
	globalTouchState.Reset()
	resetTouchOverride()
	t.Cleanup(func() {
		globalTouchState.Reset()
		resetTouchOverride()
	})

	initialValue := knob.Value

	// --- Frame 1: finger lands on the knob centre. ---
	mock.addTouch(1, cx, cy)
	globalTouchState.Update()
	updateTouchOverride()
	g.drum.tree.Update()
	if !knob.Capturing() {
		t.Fatalf("frame 1 (touchstart at knob centre %d,%d): knob not capturing — "+
			"dispatcher did not route press through globalTouchState", cx, cy)
	}

	// --- Frame 2: finger drags 30 px RIGHT (knobs are horizontal-only). The
	// exact value delta depends on the knob's radius-scaled sensitivity; we
	// just assert it moved in the expected direction and stays bounded.
	mock.moveTouch(1, cx+30, cy)
	globalTouchState.Update()
	updateTouchOverride()
	g.drum.tree.Update()
	dragEndValue := knob.Value
	if dragEndValue <= initialValue {
		t.Fatalf("frame 2 (drag 30 px right): Value=%v, want > initial %v", dragEndValue, initialValue)
	}
	if dragEndValue >= 1.0 {
		t.Fatalf("frame 2 (drag 30 px right): Value=%v already pinned to max — sensitivity broken", dragEndValue)
	}

	// --- Frame 3: finger lifts. THIS is the bug-trigger frame: pre-fix the
	// dispatcher handed (0,0) to OnRelease and Value snapped to 1.0.
	mock.removeTouch(1)
	globalTouchState.Update()
	updateTouchOverride()
	g.drum.tree.Update()
	if knob.Capturing() {
		t.Error("knob still capturing after touchend")
	}

	// The fix: Value must equal the drag-end value, NOT max.
	if knob.Value >= 1.0 {
		t.Fatalf("RELEASE-FRAME REGRESSION: knob.Value=%v snapped to max on touch lift. "+
			"This is the original mobile-touch bug.", knob.Value)
	}
	if knob.Value < dragEndValue-0.02 || knob.Value > dragEndValue+0.02 {
		t.Errorf("after touchend: knob.Value=%v, want ≈ drag-end %v (commit, no re-evaluation)",
			knob.Value, dragEndValue)
	}

	// Audio param must reflect the drag-end value, not max.
	params := audio.GetInstrumentParams("snare")
	got, ok := params["decay"]
	if !ok {
		t.Fatalf("decay param not propagated to audio after touch lifecycle: %v", params)
	}
	// decay range = [0, 4], so max would be 4.0. Drag-end fractional ~0.25-0.35
	// → param ~1.0-1.4. Strict cap: must not be at or near the maximum.
	if got > 3.5 {
		t.Errorf("audio decay param=%v after touch lifecycle — pinned to/near max (range 0..4). "+
			"Pre-fix this would have been ~4.0.", got)
	}
}
