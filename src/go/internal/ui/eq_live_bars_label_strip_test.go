//go:build test

package ui

import "testing"

// The live bars never enter the label/dB-readout strip, on desktop and mobile.
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
