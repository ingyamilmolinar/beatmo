//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// sampler_card_test.go — Task 6. The Sampler tab knob is now the SAME universal
// ParamCard control the Synth tab uses: a tap anywhere on the card opens the
// value-wheel popup on EVERY platform (the old desktop rotary drag + step-badge
// tap + readout-editor tap are retired), and a desktop scroll-wheel notch over
// the card nudges the value in place (debounced single-undo commit).
//
// These drive the REAL production tree dispatch (SetInputForTest → g.Update →
// HitIndex → samplerKnobHitAdapter), mirroring param_card_input_test.go.

// samplerCardGame builds a DESKTOP Sampler tab (1280×720 — matching the window
// the shared pressAndReleaseAt / dispatchWheelAt helpers drive) with a captured
// buffer so the knob cards lay out, and draws one frame to populate the card
// rects the input helpers read.
func samplerCardGame(t *testing.T) *Game {
	t.Helper()
	g := samplerLayoutGame(t)
	g.drum.sampler.captureFromSynth(roster.Kick)
	// Sampler knob edits are LIVE (audio.ApplySampleEditLive), so any test that
	// turns a knob here writes into the PROCESS-GLOBAL sample-edit store. Clear
	// it on the way out or the descriptor rides into the next test — that is how
	// a wheel nudge here made TestImportClearsStaleSampleEdit export a project
	// that already carried the edit it was asserting had been cleared.
	t.Cleanup(func() { audio.ClearSampleEdit(roster.Kick) })
	layoutSamplerTab(t, g)
	img := newTrackedImage("test.samplercard", 1280, 720)
	defer releaseImage(img)
	g.Draw(img)
	return g
}

// Desktop sampler card tap opens the wheel popup (was: rotary drag capture).
func TestSamplerCardClickOpensPopupOnDesktop(t *testing.T) {
	g := samplerCardGame(t)
	dv := g.drum
	r := dv.sampler.knobs[0].Rect()
	if r.Empty() {
		t.Fatal("knob 0 has no card rect")
	}
	pressAndReleaseAt(t, g, r.Min.X+4, r.Min.Y+4)
	if dv.samplerWheelPopup == nil || !dv.samplerWheelPopup.IsOpen() {
		t.Fatal("desktop sampler card tap must open the wheel popup")
	}
}

// Wheel over a sampler card nudges the value + records a single undo commit
// after the debounce; the old dial-wheel and readout-editor special cases are
// gone.
func TestSamplerCardWheelNudges(t *testing.T) {
	g := samplerCardGame(t)
	dv := g.drum
	k := dv.sampler.knobs[0]
	before := k.Value
	r := k.Rect()
	dispatchWheelAt(t, g, r.Min.X+4, r.Min.Y+4, +1)
	if k.Value == before {
		t.Fatalf("sampler card wheel nudge did not change value (%.4f)", before)
	}
	if dv.samplerWheelPopup != nil && dv.samplerWheelPopup.IsOpen() {
		t.Fatal("sampler wheel nudge must not open the popup")
	}
}
