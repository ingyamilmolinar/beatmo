//go:build test

package ui

import (
	"image"
	"strings"
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// This file pins the tab-isolation contract across EVERY ordered pair of
// mobile view-mode transitions. The user reported that switching from the
// Sampler tab to the Pads tab made all input stop working — the existing
// alive-test only covered Pads→{EQ..Synth} (one direction, omitting
// Sampler), so the regression slipped through. These tests walk all 56
// ordered (from,to) pairs and assert, after each transition:
//
//	(a) the destination's interactive controls are reachable through the
//	    tree's HitIndex (top hit at a real control belongs to the dest);
//	(b) no hidden tab's exclusive hit area leaks into the destination;
//	(c) the bottom-nav tab switcher is always on top at the bar, so the
//	    user can never get stranded.
//
// Plus a dedicated sub-test pinning the headline cause: the Sampler tab's
// modal Save-As dialog must be torn down on a view-mode switch (it was
// leaking, holding keyboard/IME focus past the tab change).

// allViewModeCases enumerates every mobile view mode with the bottom-nav
// segment index a user taps to reach it and a label for failure messages.
var allViewModeCases = []struct {
	name string
	mode viewMode
	seg  int
}{
	{"Pads", viewModeRows, 0},
	{"EQ", viewModeEQ, 1},
	{"Wave", viewModeWave, 2},
	{"Spectrum", viewModeSpectrum, 3},
	{"Levels", viewModeMeters, 4},
	{"Chain", viewModeChain, 5},
	{"Synth", viewModeSynth, 6},
	{"Sampler", viewModeSampler, 7},
}

// tabExclusiveOwner returns the human tab name that EXCLUSIVELY owns a hit
// tag, or "" for tags that are shared chrome (sticky bar, row rack, the
// tab switcher itself). The special owner "audio-any" marks the audio
// panel's catch-all, which legitimately appears on every audio tab but
// must vanish on Pads.
func tabExclusiveOwner(tag string) string {
	switch {
	case strings.HasPrefix(tag, "synth-"):
		return "Synth"
	case strings.HasPrefix(tag, "sampler-"), strings.HasPrefix(tag, "save-as-dialog-"):
		return "Sampler"
	case strings.HasPrefix(tag, "scope-"), tag == "chain-panel-capture":
		return "Chain"
	case tag == "eq-panel-capture":
		return "audio-any"
	default:
		return ""
	}
}

// tagForbiddenInMode reports whether a hit tag must NOT be present while
// the named view mode is active.
func tagForbiddenInMode(tag, modeName string) bool {
	owner := tabExclusiveOwner(tag)
	switch owner {
	case "":
		return false
	case "audio-any":
		// The audio-panel catch-all is allowed on any audio tab; only Pads
		// (rows view) must be free of it.
		return modeName == "Pads"
	default:
		return owner != modeName
	}
}

// destControlRect returns a representative interactive control rect for the
// destination mode, derived from the live layout (never magic coordinates).
func destControlRect(dv *DrumView, mode viewMode) image.Rectangle {
	if mode == viewModeRows {
		if dv.rowRackZone == nil {
			return image.Rectangle{}
		}
		for _, h := range dv.rowRackZone.HitAreas() {
			if !h.Rect.Empty() {
				return h.Rect
			}
		}
		return image.Rectangle{}
	}
	// The channel pill is present on every audio tab (incl. Synth/Sampler).
	if dv.eqPanelZone == nil || dv.eqPanelZone.stickyBar == nil {
		return image.Rectangle{}
	}
	return dv.eqPanelZone.stickyBar.ChannelBtn().Rect()
}

// TestViewModeTransitionMatrix drives all ordered (from,to) view-mode pairs
// on mobile and asserts the three legs of the tab-isolation contract after
// each transition.
func TestViewModeTransitionMatrix(t *testing.T) {
	assertDefaultParityState(t)
	setupMobileTest(t, true)

	const w, h = 360, 700

	for _, from := range allViewModeCases {
		for _, to := range allViewModeCases {
			if from.mode == to.mode {
				continue
			}
			t.Run(from.name+"_to_"+to.name, func(t *testing.T) {
				// Fresh game per pair: each transition is an independent
				// reproduction, no cross-tab state contamination.
				logger := game_log.New(testLogOutput(), game_log.LevelError)
				g := New(logger)
				t.Cleanup(g.CloseForTest)
				g.Layout(w, h)
				advanceFrames(g, 2)
				dv := g.drum
				tap := makeFullGameTap(g, w, h)

				if dv.viewSwitchSegmented == nil {
					t.Fatal("precondition: viewSwitchSegmented non-nil on mobile")
				}

				tapSegment := func(seg int) {
					r := dv.viewSwitchSegmented.SegmentRect(seg)
					if r.Empty() {
						t.Fatalf("segment %d rect empty — bottom-nav not laid out", seg)
					}
					tap((r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2)
				}

				// Walk the real user flow: tap `from`, then tap `to`.
				tapSegment(from.seg)
				if dv.currentViewMode != from.mode {
					t.Fatalf("could not reach 'from' mode: tapped %s segment, got %v", from.name, dv.currentViewMode)
				}
				tapSegment(to.seg)
				advanceFrames(g, 2)
				if dv.currentViewMode != to.mode {
					t.Fatalf("%s->%s: after tapping the %s segment, currentViewMode=%v — the tab switch did not take. "+
						"If the bottom-nav stopped responding, input is stranded.", from.name, to.name, to.name, dv.currentViewMode)
				}

				// (c) The tab switcher must be the top hit at every segment.
				for _, seg := range allViewModeCases {
					r := dv.viewSwitchSegmented.SegmentRect(seg.seg)
					if r.Empty() {
						continue
					}
					cx, cy := (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2
					hits := dv.tree.HitIndexRef().At(cx, cy)
					if len(hits) == 0 {
						t.Fatalf("%s->%s: no hit at the %s segment (%d,%d) — the user cannot switch tabs from here",
							from.name, to.name, seg.name, cx, cy)
					}
					if hits[0].Tag != "transport-view-segmented" {
						t.Fatalf("%s->%s: top hit at the %s segment is %q (z=%d), not the tab switcher — the switcher is occluded",
							from.name, to.name, seg.name, hits[0].Tag, hits[0].ZIndex)
					}
				}

				// (b) No hidden tab's exclusive hit area may be published.
				if dv.eqPanelZone != nil {
					for _, ha := range dv.eqPanelZone.HitAreas() {
						if tagForbiddenInMode(ha.Tag, to.name) {
							t.Fatalf("%s->%s: hidden-tab hit %q leaked into the %s view (rect=%v z=%d). "+
								"Only the active tab's hit areas may be published.",
								from.name, to.name, ha.Tag, to.name, ha.Rect, ha.ZIndex)
						}
					}
				}

				// (a) The destination's controls are reachable.
				ctrl := destControlRect(dv, to.mode)
				if ctrl.Empty() {
					t.Fatalf("%s->%s: no interactive control rect for the %s view — layout not realised",
						from.name, to.name, to.name)
				}
				cx, cy := (ctrl.Min.X+ctrl.Max.X)/2, (ctrl.Min.Y+ctrl.Max.Y)/2
				hits := dv.tree.HitIndexRef().At(cx, cy)
				if len(hits) == 0 {
					t.Fatalf("%s->%s: destination control at (%d,%d) has no hit area — input is dead on the %s view",
						from.name, to.name, cx, cy, to.name)
				}
				if tagForbiddenInMode(hits[0].Tag, to.name) {
					t.Fatalf("%s->%s: top hit at the %s control (%d,%d) is %q — a hidden tab is swallowing input",
						from.name, to.name, to.name, cx, cy, hits[0].Tag)
				}
			})
		}
	}
}

// TestSamplerSaveAsDialog_ClosedOnTabSwitch pins the headline cause: the
// Sampler tab's modal Save-As dialog (a focused TextInput) must be torn
// down when the user switches view modes. Left open, it keeps consuming
// keyboard/IME input from behind a now-invisible, unreachable panel — the
// user is stranded.
func TestSamplerSaveAsDialog_ClosedOnTabSwitch(t *testing.T) {
	assertDefaultParityState(t)
	setupMobileTest(t, true)

	const w, h = 360, 700
	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(w, h)
	advanceFrames(g, 2)
	dv := g.drum
	tap := makeFullGameTap(g, w, h)

	tapSegment := func(seg int) {
		r := dv.viewSwitchSegmented.SegmentRect(seg)
		tap((r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2)
	}

	// Go to the Sampler tab and load a working buffer so the Save-As dialog
	// can open.
	tapSegment(7)
	advanceFrames(g, 2)
	if dv.currentViewMode != viewModeSampler {
		t.Fatalf("precondition: expected viewModeSampler, got %v", dv.currentViewMode)
	}
	dv.sampler.loadFromInstrument("kick", []float32{0.1, 0.2, -0.3, 0.4, -0.1}, 48000, samplerSourceSynth)
	if !dv.sampler.hasBuffer() {
		t.Fatal("precondition: sampler buffer not loaded")
	}
	dv.openSamplerSaveAsDialog()
	if dv.saveAsDialog == nil {
		t.Fatal("precondition: openSamplerSaveAsDialog did not open the dialog")
	}

	// Switch back to Pads — the modal must be gone, capture cleared.
	tapSegment(0)
	advanceFrames(g, 2)
	if dv.currentViewMode != viewModeRows {
		t.Fatalf("after tapping Pads, currentViewMode=%v — the tab switch did not take (input stranded by the leaked modal)", dv.currentViewMode)
	}
	if dv.saveAsDialog != nil {
		t.Fatal("Sampler Save-As dialog survived the Sampler->Pads switch — it keeps eating keyboard/IME input behind the hidden panel, stranding the user")
	}
	if dv.tree.Capturing() {
		t.Fatalf("tree still has a captured handler (%q) after the tab switch — transient capture must be cleared on setViewMode", dv.tree.CapturedTag())
	}

	// And the user can still switch tabs afterward.
	tapSegment(6)
	advanceFrames(g, 2)
	if dv.currentViewMode != viewModeSynth {
		t.Fatalf("after the leaked-modal scenario, tapping the Synth segment did not switch (got %v) — input is still stranded", dv.currentViewMode)
	}
}

// TestViewModeTransition_TransientOverlayTornDown is the general regression
// for the reported Sampler->Pads "input dead" bug. The existing matrix test
// only switched between tabs with NOTHING open, so it never exercised the
// real failure: a transient overlay (portal/popup/dropdown) opened on the
// departing tab survived the switch and swallowed input on the destination.
//
// Root cause: setViewMode — the single documented chokepoint for EVERY view
// transition (segmented control, toolbar button, EQ-peek tap) — only called
// CloseAllPopups() inside `if dv.MobileEQMode()` (false when the destination
// is Pads), and resetTransientTabState() only tore down the Save-As dialog +
// pointer capture — not portals. So a transition into Pads leaked any open
// overlay; a full-screen MODAL overlay (the naming portal) then filters the
// HitIndex to itself and blocks ALL input.
//
// This drives setViewMode directly (the fix site) across all 56 ordered
// pairs with a full-screen modal portal open on `from`, and asserts the
// overlay is gone and the destination is reachable afterward.
func TestViewModeTransition_TransientOverlayTornDown(t *testing.T) {
	assertDefaultParityState(t)
	setupMobileTest(t, true)

	const w, h = 360, 700

	for _, from := range allViewModeCases {
		for _, to := range allViewModeCases {
			if from.mode == to.mode {
				continue
			}
			t.Run(from.name+"_to_"+to.name, func(t *testing.T) {
				logger := game_log.New(testLogOutput(), game_log.LevelError)
				g := New(logger)
				t.Cleanup(g.CloseForTest)
				g.Layout(w, h)
				advanceFrames(g, 2)
				dv := g.drum

				// Reach the `from` tab.
				dv.setViewMode(from.mode)
				advanceFrames(g, 2)
				if dv.currentViewMode != from.mode {
					t.Fatalf("could not reach 'from' mode %s, got %v", from.name, dv.currentViewMode)
				}

				// Open a full-screen MODAL portal on the departing tab. It is
				// self-persisting (isOpenFn = Has("naming"), ShouldClose=false),
				// so it cannot self-heal and mask the bug.
				dv.openNamingPortal()
				if !dv.tree.Portal().HasModal() {
					t.Fatal("precondition: modal naming portal did not open")
				}

				// Switch to the destination through the canonical chokepoint.
				dv.setViewMode(to.mode)
				advanceFrames(g, 2)
				if dv.currentViewMode != to.mode {
					t.Fatalf("%s->%s: setViewMode did not switch (got %v)", from.name, to.name, dv.currentViewMode)
				}

				// The departing tab's overlay must NOT survive the switch.
				if dv.tree.Portal().IsOpen() {
					t.Fatalf("%s->%s: a transient overlay (top=%q) survived the tab switch — it swallows input on the %s view. "+
						"setViewMode must tear down ALL popups/portals on every transition, not only when entering an audio tab.",
						from.name, to.name, dv.tree.Portal().TopID(), to.name)
				}
				if dv.tree.Portal().HasModal() {
					t.Fatalf("%s->%s: a modal overlay survived the switch — it blocks ALL input on the %s view", from.name, to.name, to.name)
				}

				// The destination's controls must be reachable (a leaked modal
				// portal filters them out of the HitIndex entirely).
				ctrl := destControlRect(dv, to.mode)
				if ctrl.Empty() {
					t.Fatalf("%s->%s: no interactive control rect for the %s view", from.name, to.name, to.name)
				}
				cx, cy := (ctrl.Min.X+ctrl.Max.X)/2, (ctrl.Min.Y+ctrl.Max.Y)/2
				hits := dv.tree.HitIndexRef().At(cx, cy)
				if len(hits) == 0 {
					t.Fatalf("%s->%s: destination control at (%d,%d) has no hit area — input is dead on the %s view",
						from.name, to.name, cx, cy, to.name)
				}
				if tagForbiddenInMode(hits[0].Tag, to.name) {
					t.Fatalf("%s->%s: top hit at the %s control (%d,%d) is %q — a hidden tab is swallowing input",
						from.name, to.name, to.name, cx, cy, hits[0].Tag)
				}
			})
		}
	}
}

// TestSamplerToPads_PortalSurvives_StrandsInput is the headline reproduction
// of the exact user report: on the Sampler tab, with a transient overlay
// open, switching to Pads leaves the overlay alive and input dead. It uses
// the full-screen modal naming portal — the worst case, which blocks ALL
// input until torn down — and switches via setViewMode (a modal would block
// a bottom-nav tap, so the teardown must be invocation-path-agnostic).
func TestSamplerToPads_PortalSurvives_StrandsInput(t *testing.T) {
	assertDefaultParityState(t)
	setupMobileTest(t, true)

	const w, h = 360, 700
	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(w, h)
	advanceFrames(g, 2)
	dv := g.drum

	// Reach the Sampler tab and load a buffer (mirrors the existing Save-As test).
	dv.setViewMode(viewModeSampler)
	advanceFrames(g, 2)
	if dv.currentViewMode != viewModeSampler {
		t.Fatalf("precondition: expected viewModeSampler, got %v", dv.currentViewMode)
	}
	dv.sampler.loadFromInstrument("kick", []float32{0.1, 0.2, -0.3, 0.4, -0.1}, 48000, samplerSourceSynth)

	// Open both a Save-As dialog and a full-screen MODAL portal — the kind
	// of transient state a real session accumulates on the Sampler tab.
	dv.openSamplerSaveAsDialog()
	dv.openNamingPortal()
	if dv.saveAsDialog == nil {
		t.Fatal("precondition: Save-As dialog did not open")
	}
	if !dv.tree.Portal().HasModal() {
		t.Fatal("precondition: modal naming portal did not open")
	}

	// Switch to Pads.
	dv.setViewMode(viewModeRows)
	advanceFrames(g, 2)

	// Everything transient from the Sampler tab must be torn down.
	if dv.tree.Portal().IsOpen() {
		t.Fatalf("Sampler->Pads: portal (top=%q) survived — it blocks input on the Pads view", dv.tree.Portal().TopID())
	}
	if dv.tree.Portal().HasModal() {
		t.Fatal("Sampler->Pads: modal portal survived — ALL input on the Pads view is blocked (the reported bug)")
	}
	if dv.saveAsDialog != nil {
		t.Fatal("Sampler->Pads: Save-As dialog survived the switch")
	}
	if dv.tree.Capturing() {
		t.Fatalf("Sampler->Pads: tree still has a captured handler (%q)", dv.tree.CapturedTag())
	}

	// A Pads row control must be reachable.
	ctrl := destControlRect(dv, viewModeRows)
	if ctrl.Empty() {
		t.Fatal("Sampler->Pads: no Pads row control rect — layout not realised")
	}
	cx, cy := (ctrl.Min.X+ctrl.Max.X)/2, (ctrl.Min.Y+ctrl.Max.Y)/2
	hits := dv.tree.HitIndexRef().At(cx, cy)
	if len(hits) == 0 || tagForbiddenInMode(hits[0].Tag, "Pads") {
		got := "<none>"
		if len(hits) > 0 {
			got = hits[0].Tag
		}
		t.Fatalf("Sampler->Pads: Pads control at (%d,%d) not reachable (top hit=%q) — input is dead", cx, cy, got)
	}
}
