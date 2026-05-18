//go:build test

package ui

import (
	"testing"
)

// These tests reproduce the user-visible startup-chrome bugs in
// crop_drum_view_default that the existing beat_counter_visibility_test.go
// suite missed:
//   - beat counter pill is taller than its rect (renders behind the timeline bar)
//   - len +/− stack overflows the header band into the rows area
//   - track button has no minimum touch height inside the header
//
// They run at the production capture viewport (1280×720 desktop, 390×844
// mobile) — NOT the inflated 720-tall dv.Bounds the existing tests use —
// so the math reflects what the screenshot harness actually captures.

// minStartupChromeGap is the minimum vertical breathing room (in px) we
// require between the bottom of the beat counter chip and the top of the
// timeline progress bar. Without this, the pill background paints flush
// against the bar and the readout reads as part of the bar geometry.
const minStartupChromeGap = 2

// productionDrumView builds a Game at the given viewport and returns its
// laid-out drum view, mirroring the path the screenshot harness uses
// (`Game.Layout` → `Splitter.DrumRect`). All chrome rects are populated
// after this call.
func productionDrumView(t *testing.T, w, h int) *DrumView {
	t.Helper()
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(w, h)
	dv := g.drum
	dv.recalcButtons()
	return dv
}

// TestStartupHeader_BeatCounterRectFitsAboveTimeline_Desktop asserts the
// counter rect leaves at least minStartupChromeGap px of breathing room
// above the timeline progress bar at the production viewport.
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

// TestStartupHeader_BeatCounterPillFitsInRect_Desktop computes the same
// pill dimensions drawBeatCounter uses (`pillH = TextHeight() + 2*pillPadY`)
// and asserts the pill fits inside the counter rect — i.e. the chip cannot
// bleed below the rect into the timeline bar.
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

// TestStartupHeader_LenButtonsInsideHeader_Desktop asserts both inc and
// dec buttons fit inside dv.Bounds.Min.Y..+headerH so they cannot
// overflow into the rows area below the header.
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

// TestStartupHeader_TrackButtonInsideHeader_Desktop asserts the track button
// is fully inside the header band AND tall enough to be a usable click target
// (>= TouchMinTarget()/2). Today the rect spans the full timeline area but
// renders nearly invisibly — this guards against future shrinkage.
func TestStartupHeader_TrackButtonInsideHeader_Desktop(t *testing.T) {
	assertDefaultParityState(t)
	dv := productionDrumView(t, 1280, 720)
	r := dv.trackBtn().Rect()
	if r.Empty() {
		t.Fatal("trackBtn rect is empty on desktop")
	}
	headerBottom := dv.Bounds.Min.Y + dv.headerH
	if r.Min.Y < dv.Bounds.Min.Y || r.Max.Y > headerBottom {
		t.Errorf("trackBtn outside header band: rect=%v bounds=%v headerBottom=%d",
			r, dv.Bounds, headerBottom)
	}
	minH := TouchMinTarget() / 2
	if r.Dy() < minH {
		t.Errorf("trackBtn too short: dy=%d < min=%d (rect=%v)", r.Dy(), minH, r)
	}
}

// TestStartupHeader_BeatCounterRectFitsAboveTimeline_Mobile is the mobile
// variant — passes today thanks to the 56-px header, locks in the invariant
// for the regression-safety net.
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

// TestStartupHeader_HeaderChromeFitsOnMobile asserts mobile len buttons
// stay empty (per TestLenButtonsHiddenOnMobile) AND the beat counter rect
// fits inside the header band. The mobile track chip is exempt — it
// extends below the header floor by design (touch-ergonomics: the chip
// expands to TouchMinTarget for finger-sized hit area, locked in by
// TestTrackChip_VisibleOnMobile).
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
