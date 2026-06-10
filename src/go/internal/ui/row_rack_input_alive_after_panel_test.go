//go:build test

package ui

import (
	"testing"
)

// TestRowRackInputAliveAfterPanelToPads is the POSITIVE regression guard
// for the bug where, on mobile, returning to the Pads view after visiting
// any audio-panel tab (EQ / Wave / Spectrum / Levels / Chain / Synth /
// Sampler) left the row-rack mute / solo / FX / label controls
// unresponsive.
//
// Root cause: while a panel tab owns the mobile screen, MobileEQMode() is
// true so RowRackZone forces vis=0 and publishes ONLY its scroll catch-all
// (`row-rack-scroll`, z=119) into the tree's HitIndex. On returning to
// Pads the zone's LIVE HitAreas() are correct again (mute/solo/FX/label at
// z=120), but recalcButtons' republish gate
// (`NeedsLayout() || rect != rackRect`, drumview_layout.go) stays false —
// the rack rect is identical in both modes and nothing marked the zone
// dirty — so the tree keeps serving the stale scroll-only snapshot and
// every tap on a control falls through to the (ignoring) scroll handler.
//
// The user perceived this as "save in the Sampler breaks the Pads
// buttons", but Save is incidental: the Sampler is just one of the panel
// tabs, and ANY panel→Pads excursion reproduces it. The model toggle
// (dv.toggleMute) keeps working because it bypasses hit routing entirely.
//
// The pre-existing pads_input_regression_test.go only asserted the
// NEGATIVE (no `eq-panel-capture` catch-all leaking into the rack); it
// never asserted that the rack's own mute/solo/FX hit areas are PRESENT
// after the transition, so it stayed green while the controls were dead.
// This test closes that gap: after each panel→Pads transition it asserts
// the mute control is actually reachable in the dispatcher AND that
// pressing it through the real hit handler toggles the row.
func TestRowRackInputAliveAfterPanelToPads(t *testing.T) {
	assertDefaultParityState(t)

	panelModes := []struct {
		name string
		mode viewMode
	}{
		{"EQ", viewModeEQ},
		{"Wave", viewModeWave},
		{"Spectrum", viewModeSpectrum},
		{"Levels", viewModeMeters},
		{"Chain", viewModeChain},
		{"Synth", viewModeSynth},
		{"Sampler", viewModeSampler},
	}

	for _, pm := range panelModes {
		t.Run(pm.name, func(t *testing.T) {
			restore := SetForceSmallScreen(t, true)
			defer restore()

			g := New(testLogger)
			t.Cleanup(g.CloseForTest)
			g.Layout(360, 800)

			// User flow: go into a panel tab, then back to Pads.
			g.drum.setViewMode(pm.mode)
			g.Update()
			g.Update()
			if !g.drum.MobileEQMode() {
				t.Fatalf("MobileEQMode should be true in viewMode=%s on mobile", pm.name)
			}

			g.drum.setViewMode(viewModeRows)
			g.Update()
			g.Update()
			if g.drum.MobileEQMode() {
				t.Fatalf("MobileEQMode should be false after returning to Pads")
			}

			muteBtns := g.drum.rowMuteBtns()
			if len(muteBtns) == 0 || muteBtns[0] == nil {
				t.Fatal("no row-0 mute button after returning to Pads")
			}
			r := muteBtns[0].Rect()
			if r.Empty() {
				t.Fatal("row-0 mute button rect is empty after returning to Pads")
			}
			cx, cy := r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2

			// (1) The dispatcher must actually see the mute control at its
			// center — not just the scroll catch-all. This is the assertion
			// the old negative-only test lacked.
			hits := g.drum.tree.HitIndexRef().At(cx, cy)
			var muteHit *indexedHitArea
			for i := range hits {
				if hits[i].Tag == "row-rack-mute" {
					muteHit = &hits[i]
					break
				}
			}
			if muteHit == nil {
				tags := make([]string, len(hits))
				for i := range hits {
					tags[i] = hits[i].Tag
				}
				t.Fatalf("row-rack-mute hit area missing at mute center (%d,%d) after %s→Pads; "+
					"hit index is stale (controls unreachable). hits=%v", cx, cy, pm.name, tags)
			}

			// (2) Pressing the control through the real hit handler must
			// toggle the row — proving the wiring is live end-to-end.
			before := g.drum.Rows[0].Muted
			muteHit.Handler.OnPress(cx, cy)
			muteHit.Handler.OnRelease(cx, cy)
			if g.drum.Rows[0].Muted == before {
				t.Fatalf("pressing row-0 mute after %s→Pads did not toggle Muted (still %v)", pm.name, before)
			}
		})
	}
}
