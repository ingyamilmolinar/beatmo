//go:build test

package ui

import "testing"

// TestEQLiveBarsNeverEnterLabelStrip pins the invariant claimed in
// eq_panel_zone.go: the EQ live spectrum bars (eqLiveBarsRect) must never
// intersect the bottom label/dB-readout strip (eqLabelStripRect). Pre-fix the
// bars ran flush to the panel bottom and overpainted the per-band dB readouts.
// Verified across desktop and mobile viewports so the layout split holds at
// both densities.
func TestEQLiveBarsNeverEnterLabelStrip(t *testing.T) {
	withDefaultAudio(t)
	withDefaultStart(t, false)

	viewports := []struct {
		name string
		w, h int
	}{
		{"desktop", 1280, 720},
		{"mobile", 390, 780},
	}
	for _, vp := range viewports {
		t.Run(vp.name, func(t *testing.T) {
			g := New(testLogger)
			t.Cleanup(g.CloseForTest)
			g.Layout(vp.w, vp.h)

			z := g.drum.eqPanelZone
			if z == nil {
				t.Fatal("eqPanelZone nil; harness assumption broken")
			}
			z.tabState.SetActiveTab(TabEQ)
			z.Layout(z.PanelRect())

			bars := z.eqLiveBarsRect()
			labels := z.eqLabelStripRect()
			if bars.Empty() {
				t.Fatalf("eqLiveBarsRect empty; nothing to verify (panel=%v)", z.PanelRect())
			}
			if got := bars.Intersect(labels); !got.Empty() {
				t.Errorf("live bars %v intersect label strip %v (overlap %v): bars overpaint the dB readouts",
					bars, labels, got)
			}
			if bars.Max.Y > labels.Min.Y {
				t.Errorf("live bars bottom %d extends past label strip top %d", bars.Max.Y, labels.Min.Y)
			}
		})
	}
}
