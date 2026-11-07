//go:build test

package ui

import (
	"testing"
)

// At the production capture viewport (1280×720, 390×844), not an inflated test bounds, so the math matches the
// screenshot harness.

// Without it the pill paints flush against the bar and reads as part of it.
const minStartupChromeGap = 2

// The screenshot harness's path (Game.Layout → Splitter.DrumRect).
func productionDrumView(t *testing.T, w, h int) *DrumView {
	t.Helper()
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(w, h)
	dv := g.drum
	dv.recalcButtons()
	return dv
}

func TestStartupHeader_BeatCounterRectFitsAboveTimeline_Desktop(t *testing.T) {
	assertDefaultParityState(t)
	dv := productionDrumView(t, 1280, 720)
	if dv.beatCounterRect.Empty() {
		t.Fatal("beatCounterRect is empty at production viewport")
	}
	if dv.timelineRect.Empty() {
		t.Fatal("timelineRect is empty at production viewport")
	}
	gap := dv.timelineRect.Min.Y - dv.beatCounterRect.Max.Y
	if gap < minStartupChromeGap {
		t.Fatalf("desktop counter/bar gap=%d (need >=%d). beatCounter=%v timeline=%v",
			gap, minStartupChromeGap, dv.beatCounterRect, dv.timelineRect)
	}
}

// The same pill dimensions as drawBeatCounter.
func TestStartupHeader_BeatCounterPillFitsInRect_Desktop(t *testing.T) {
	assertDefaultParityState(t)
	dv := productionDrumView(t, 1280, 720)
	if dv.beatCounterRect.Empty() {
		t.Fatal("beatCounterRect is empty at production viewport")
	}
	pillH := TextHeight() + 2*beatCounterPillPadY
	if pillH > dv.beatCounterRect.Dy() {
		t.Fatalf("pillH=%d > beatCounterRect.Dy=%d — pill would bleed below rect into timeline bar (rect=%v)",
			pillH, dv.beatCounterRect.Dy(), dv.beatCounterRect)
	}
}

func TestStartupHeader_LenButtonsInsideHeader_Desktop(t *testing.T) {
	assertDefaultParityState(t)
	dv := productionDrumView(t, 1280, 720)
	headerBottom := dv.Bounds.Min.Y + dv.headerH
	inc := dv.lenIncBtn.Rect()
	dec := dv.lenDecBtn.Rect()
	if inc.Empty() {
		t.Fatal("lenIncBtn rect is empty on desktop")
	}
	if dec.Empty() {
		t.Fatal("lenDecBtn rect is empty on desktop")
	}
	if inc.Max.Y > headerBottom {
		t.Errorf("lenIncBtn overflows header: rect=%v headerBottom=%d", inc, headerBottom)
	}
	if dec.Max.Y > headerBottom {
		t.Errorf("lenDecBtn overflows header: rect=%v headerBottom=%d", dec, headerBottom)
	}
}

// By design it extends below the header into the timeline's left gutter, while staying in bounds and usable.
func TestStartupHeader_TrackButtonInsideHeader_Desktop(t *testing.T) {
	assertDefaultParityState(t)
	dv := productionDrumView(t, 1280, 720)
	r := dv.trackBtn().Rect()
	if r.Empty() {
		t.Fatal("trackBtn rect is empty on desktop")
	}
	headerBottom := dv.Bounds.Min.Y + dv.headerH
	// Anchored within the header band at the top…
	if r.Min.Y < dv.Bounds.Min.Y || r.Min.Y > headerBottom {
		t.Errorf("trackBtn top not anchored in header: rect=%v bounds=%v headerBottom=%d",
			r, dv.Bounds, headerBottom)
	}
	// …extending down into the timeline but never past the drum surface.
	if r.Max.Y > dv.Bounds.Max.Y {
		t.Errorf("trackBtn escapes drum bounds: rect=%v bounds=%v", r, dv.Bounds)
	}
	minH := TouchMinTarget() / 2
	if r.Dy() < minH {
		t.Errorf("trackBtn too short: dy=%d < min=%d (rect=%v)", r.Dy(), minH, r)
	}
}

func TestStartupHeader_BeatCounterRectFitsAboveTimeline_Mobile(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)
	dv := productionDrumView(t, 390, 844)
	if dv.beatCounterRect.Empty() {
		t.Fatal("beatCounterRect is empty on mobile production viewport")
	}
	if dv.timelineRect.Empty() {
		t.Fatal("timelineRect is empty on mobile production viewport")
	}
	gap := dv.timelineRect.Min.Y - dv.beatCounterRect.Max.Y
	if gap < minStartupChromeGap {
		t.Fatalf("mobile counter/bar gap=%d (need >=%d). beatCounter=%v timeline=%v",
			gap, minStartupChromeGap, dv.beatCounterRect, dv.timelineRect)
	}
}

// TestStartupHeader_BeatCounterPillFitsInRect_Mobile mobile counterpart.
func TestStartupHeader_BeatCounterPillFitsInRect_Mobile(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)
	dv := productionDrumView(t, 390, 844)
	if dv.beatCounterRect.Empty() {
		t.Fatal("beatCounterRect is empty on mobile")
	}
	pillH := TextHeight() + 2*beatCounterPillPadY
	if pillH > dv.beatCounterRect.Dy() {
		t.Fatalf("mobile pillH=%d > beatCounterRect.Dy=%d (rect=%v)",
			pillH, dv.beatCounterRect.Dy(), dv.beatCounterRect)
	}
}

// The mobile track chip is exempt: it extends below the header to reach the touch minimum.
func TestStartupHeader_HeaderChromeFitsOnMobile(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)
	dv := productionDrumView(t, 390, 844)

	if r := dv.lenIncBtn.Rect(); !r.Empty() {
		t.Errorf("expected lenIncBtn empty on mobile, got %v", r)
	}
	if r := dv.lenDecBtn.Rect(); !r.Empty() {
		t.Errorf("expected lenDecBtn empty on mobile, got %v", r)
	}

	headerBottom := dv.Bounds.Min.Y + dv.headerH
	if dv.beatCounterRect.Max.Y > headerBottom {
		t.Errorf("mobile beatCounterRect overflows header: rect=%v headerBottom=%d",
			dv.beatCounterRect, headerBottom)
	}
}
