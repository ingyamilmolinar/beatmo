//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// The Sampler knob is the same ParamCard as Synth: a tap opens the value wheel on every platform, and a desktop
// wheel nudges in place with one debounced undo commit. Real tree dispatch.

// 1280×720, the window the shared input helpers drive, with a captured buffer so the cards lay out.
func samplerCardGame(t *testing.T) *Game {
	t.Helper()
	g := samplerLayoutGame(t)
	g.drum.sampler.captureFromSynth(roster.Kick)
	// Knob edits apply live into a process-global store; clear it or the edit rides into the next test's export.
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

// One undo commit after the debounce.
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
