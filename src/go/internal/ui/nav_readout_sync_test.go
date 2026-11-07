//go:build test

package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

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

// Both are readouts of the SAME playhead, so beat and elapsed seconds agree at every position; two derivations
// drift apart around beat boundaries.
func TestNavMenuReadoutMatchesBeatCounter(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)

	g.drum.SetBPM(120)
	g.SetAppliedBPMForTest(120)
	// One draw syncs secPerBeat from the BPM, as the live loop does before either readout renders.
	g.drawDrumPane(ebiten.NewImage(1, 1))

	if g.drum.navPanel == nil {
		t.Fatal("nav panel not constructed")
	}

	// Straddling beat boundaries, where independent derivations disagree.
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
