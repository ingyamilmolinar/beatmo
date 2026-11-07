//go:build test

package ui

import (
	"image"
	"sort"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// The pill each tab renders, so a new feature has to make the case for its control instead of appending one.

// Through the shared drawPillTabAt seam.
func renderedPillLabels(t *testing.T, draw func(dst *ebiten.Image)) []string {
	t.Helper()
	var got []string
	orig := drawPillTabAt
	drawPillTabAt = func(dst *ebiten.Image, btn *Button, active bool) {
		if btn != nil {
			// The icon freeze pill has no text but still takes a slot in the tab's budget.
			label := btn.Text
			if label == "" {
				label = "<icon>"
			}
			got = append(got, label)
		}
		orig(dst, btn, active)
	}
	t.Cleanup(func() { drawPillTabAt = orig })
	draw(ebiten.NewImage(900, 400))
	sort.Strings(got)
	return got
}

func assertSameLabels(t *testing.T, tab string, got, want []string) {
	t.Helper()
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("%s pills = %v, want %v", tab, got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("%s pills = %v, want %v", tab, got, want)
		}
	}
}

// A toggle for framing the signal properly is a defect, so only the A/B comparison and freeze remain.
func TestChainPillInventory(t *testing.T) {
	z := NewChainPanelZone(ChainCallbacks{})
	z.Layout(image.Rect(0, 100, 900, 400))
	got := renderedPillLabels(t, func(dst *ebiten.Image) { z.Draw(dst) })
	assertSameLabels(t, "chain", got, []string{"OVR", "SPL", "DIF", "FRZ"})
}

// Freeze only: the other pills were expert controls whose effect is invisible unless you know the term, and one
// hid the numbers the tab exists to show.
func TestSpectrumPillInventory(t *testing.T) {
	c := newSpectrumControls(0, func() bool { return false })
	c.Layout(image.Rect(0, 0, 900, 40))
	got := renderedPillLabels(t, func(dst *ebiten.Image) { c.Draw(dst) })
	assertSameLabels(t, "spectrum", got, []string{"<icon>"}) // <icon> = the shared freeze pill
}

// The axis follows the signal, so the dB numbers, note anchors and peak marks sit against full-height bars and
// there is nothing to opt out of.
func TestSpectrumDetailIsAlwaysOn(t *testing.T) {
	c := newSpectrumControls(0, func() bool { return false })
	if !c.Detail() {
		t.Error("spectrum detail is off; the dB numbers and note anchors are hidden with no way to reveal them")
	}
	c.Layout(image.Rect(0, 0, 900, 40))
	got := renderedPillLabels(t, func(dst *ebiten.Image) { c.Draw(dst) })
	assertSameLabels(t, "spectrum", got, []string{"<icon>"}) // freeze only
}

// The watermark never decays, so the discontinuity invalidation is its only reset and must clear it.
func TestSpectrumMaxWatermarkStillHasAResetPath(t *testing.T) {
	z := NewEQPanelZone(EQCallbacks{})
	z.spectrumPeaks.MaxPeaksDB[4] = -20

	z.InvalidateSpectrumReadout()

	if z.spectrumPeaks.MaxPeaksDB[4] != spectrumNoPeakDB {
		t.Error("the MAX watermark survives every reset path there is; it can never be cleared")
	}
}
