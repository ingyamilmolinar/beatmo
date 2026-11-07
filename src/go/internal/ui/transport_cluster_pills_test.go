//go:build test

package ui

import (
	"image"
	"testing"
)

// The cluster pill contains play/stop/record, the BPM pill its box and ±, and subdiv sits in the right of the
// row. The surface fill is left to the screenshot: the stub has no reliable GPU reads.
func TestMobileTransportPillRectsArePopulated(t *testing.T) {
	assertDefaultParityState(t)
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false })

	z, _ := newTestTransportZone()
	// As DrumView does when it allocates the bottom bar; without it the zone falls back to the two-row layout.
	z.SetUseBottomBar(true)
	z.Layout(image.Rect(0, 0, 390, 96))

	// 1. Transport pill must contain play, stop, and record.
	if z.transportGroupRect.Empty() {
		t.Fatal("transportGroupRect empty — play+stop+record cluster has no pill container")
	}
	for _, b := range []struct {
		name string
		r    image.Rectangle
	}{{"play", z.playBtn.Rect()}, {"stop", z.stopBtn.Rect()}, {"record", z.recordBtn.Rect()}} {
		if !b.r.In(z.transportGroupRect) {
			t.Errorf("transport pill %v should contain %s rect %v", z.transportGroupRect, b.name, b.r)
		}
	}

	// 2. BPM pill must contain the BPM box + both ± arrows.
	if z.bpmGroupRect.Empty() {
		t.Fatal("bpmGroupRect empty — BPM cluster has no pill container")
	}
	for _, b := range []struct {
		name string
		r    image.Rectangle
	}{{"bpmBox", z.bpmBox.Rect}, {"bpmIncBtn", z.bpmIncBtn.Rect()}, {"bpmDecBtn", z.bpmDecBtn.Rect()}} {
		if !b.r.In(z.bpmGroupRect) {
			t.Errorf("BPM pill %v should contain %s rect %v", z.bpmGroupRect, b.name, b.r)
		}
	}

	// 3. Subdiv button must be non-empty and to the right of the BPM cluster.
	subdiv := z.subdivBtn.Rect()
	if subdiv.Empty() {
		t.Fatal("subdivBtn rect empty — subdivision pill cannot be drawn")
	}
	if subdiv.Min.X < z.bpmGroupRect.Max.X {
		t.Errorf("subdiv button %v should start to the right of BPM pill %v", subdiv, z.bpmGroupRect)
	}

	// The top toolbar hosts volume and overflow; the binary view switch stays hidden in favour of the segmented
	// control.
	if z.mainVolIconRect.Empty() {
		t.Errorf("mainVolIconRect should be non-empty after zone.Layout on mobile (Theme 4 places it in top toolbar)")
	}
	if z.viewSwitchBtn != nil && !z.viewSwitchBtn.Rect().Empty() {
		t.Errorf("viewSwitchBtn rect should be empty on mobile (segmented replaces it); got %v", z.viewSwitchBtn.Rect())
	}
}
