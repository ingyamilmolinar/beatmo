//go:build test

package ui

import (
	"testing"
)

// Returning to Pads from any panel tab must republish the rack's control hit areas: the rack rect is the same
// in both modes, so nothing marked it dirty and taps hit the scroll catch-all. Asserts the controls are
// present and a real press toggles the row.
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

			// The mute control itself, not only the scroll catch-all.
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

			before := g.drum.Rows[0].Muted
			muteHit.Handler.OnPress(cx, cy)
			muteHit.Handler.OnRelease(cx, cy)
			if g.drum.Rows[0].Muted == before {
				t.Fatalf("pressing row-0 mute after %s→Pads did not toggle Muted (still %v)", pm.name, before)
			}
		})
	}
}
