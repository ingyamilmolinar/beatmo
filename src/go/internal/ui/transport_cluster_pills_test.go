//go:build test

package ui

import (
	"image"
	"testing"
)

// TestMobileTransportPillRectsArePopulated asserts the structural
// preconditions for the cluster pills on mobile:
// - the transport-cluster pill rect (play+stop+record) is non-empty
//   and contains all three of those button rects;
// - the BPM pill rect contains the BPM input box and both ± buttons;
// - the subdivision button rect (which gets its own surface-2 pill +
//   chevron-down hint via drawSubdivPillOffset / drawSubdivChevronOffset)
//   is non-empty and lives in the right portion of the transport row.
//
// Pixel-level verification of the surface-2 fill color is left to the
// regenerated mobile screenshot baseline; ebitenstub does not support
// reliable GPU pixel reads in the fast-test path.
func TestMobileTransportPillRectsArePopulated(t *testing.T) {
	assertDefaultParityState(t)
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false })

	z, _ := newTestTransportZone()
	// Simulate the standard mobile flow: DrumView allocates the bottom
	// action bar and tells the zone via SetUseBottomBar(true) so row 1
	// (vol/view/overflow) is suppressed inside the top toolbar. Without
	// this, the zone correctly falls back to a two-row layout (used on
	// ultra-short viewports where the bar collapses).
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

	// 4. After B3 the mobile transport collapses to a single row: vol /
	//    view-switch / overflow have moved to DrumView.bottomActionBarRect
	//    and are NOT placed by the zone's mobile layout. Assert they are
	//    explicitly empty here so the new contract stays enforced — any
	//    regression that re-adds row 1 inside the top bar will trip this.
	if !z.mainVolIconRect.Empty() {
		t.Errorf("mainVolIconRect should be empty after zone.Layout on mobile (DrumView places it in bottom action bar); got %v", z.mainVolIconRect)
	}
	if z.viewSwitchBtn != nil && !z.viewSwitchBtn.Rect().Empty() {
		t.Errorf("viewSwitchBtn rect should be empty after zone.Layout on mobile (DrumView places it in bottom action bar); got %v", z.viewSwitchBtn.Rect())
	}
	if z.overflowBtn != nil && !z.overflowBtn.Rect().Empty() {
		t.Errorf("overflowBtn rect should be empty after zone.Layout on mobile (DrumView places it in bottom action bar); got %v", z.overflowBtn.Rect())
	}
}
