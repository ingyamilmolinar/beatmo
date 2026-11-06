//go:build test

package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// parseBeatReadout extracts (beat, seconds) from a position readout string of
// the shared "Beat <n> · <M:SS>" form (or the mobile-compact "<n> · <M:SS>").
// Returns ok=false when the string is not a position readout at all.
func parseBeatReadout(t *testing.T, s string) (beat, sec int, ok bool) {
	t.Helper()
	p := strings.Index(s, "· ")
	if p < 0 {
		return 0, 0, false
	}
	head := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s[:p]), "Beat"))
	if _, err := fmt.Sscanf(head, "%d", &beat); err != nil {
		return 0, 0, false
	}
	var m, sc int
	if _, err := fmt.Sscanf(strings.TrimSpace(s[p+len("· "):]), "%d:%d", &m, &sc); err != nil {
		return 0, 0, false
	}
	return beat, m*60 + sc, true
}

// TestNavMenuReadoutMatchesBeatCounter pins the navigation menu's position
// title to the transport beat counter: both are readouts of the SAME playhead,
// so both must render the same beat number and the same elapsed second count at
// every clock position.
//
// Regression guard for the desync where the two surfaces each derived their own
// numbers from the transport clock: the counter formatted int(displayBeat)+1
// with continuous elapsed seconds, while the nav menu formatted
// round(displayBeat*div)/div with the *whole-beat boundary* seconds — so at
// displayBeat=3.9 / 120 BPM the counter read "Beat 4 · 0:01" while the nav menu
// read "Beat 3 · 0:02". Both the beat AND the second disagreed.
func TestNavMenuReadoutMatchesBeatCounter(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)

	g.drum.SetBPM(120)
	g.SetAppliedBPMForTest(120)
	// One draw pass syncs DrumView.secPerBeat from the applied BPM, exactly as
	// the live draw loop does before either readout renders.
	g.drawDrumPane(ebiten.NewImage(1, 1))

	if g.drum.navPanel == nil {
		t.Fatal("nav panel not constructed")
	}

	// Positions chosen to straddle beat boundaries, where two independent
	// derivations drift apart: just under a beat, mid-beat, and far along.
	for _, beats := range []float64{0, 3.9, 4.5, 15.99, 130.25} {
		g.SetLastDisplayBeatForTest(beats)

		counterText := g.drum.timelineZone.timelineInfoCached(g.displayBeat())
		navText := g.drum.navPanel.ReadoutText()

		cBeat, cSec, ok := parseBeatReadout(t, counterText)
		if !ok {
			t.Fatalf("displayBeat=%v: unparseable beat-counter readout %q", beats, counterText)
		}
		nBeat, nSec, ok := parseBeatReadout(t, navText)
		if !ok {
			t.Fatalf("displayBeat=%v: unparseable nav readout %q", beats, navText)
		}
		if cBeat != nBeat || cSec != nSec {
			t.Errorf("displayBeat=%v: nav menu readout %q out of sync with beat counter %q (beat %d vs %d, sec %d vs %d)",
				beats, navText, counterText, nBeat, cBeat, nSec, cSec)
		}
	}
}
