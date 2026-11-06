//go:build test

package ui

import (
	"image"
	"sort"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// The audio panel accumulated one pill per analyser feature. Chain shipped
// six and Spectrum six, most of them expert controls whose effect is not
// visible until you already know what they do. These tests pin the pill each
// tab actually renders, so a future feature has to make the case for its
// control instead of appending it silently.

// renderedPillLabels returns the labels of every pill a renderer paints,
// captured through the shared drawPillTabAt seam.
func renderedPillLabels(t *testing.T, draw func(dst *ebiten.Image)) []string {
	t.Helper()
	var got []string
	orig := drawPillTabAt
	drawPillTabAt = func(dst *ebiten.Image, btn *Button, active bool) {
		if btn != nil {
			// Wave/Spectrum/Levels use the shared ICON freeze pill, which has
			// no text; it still occupies a slot in the tab's control budget,
			// so the census must see it.
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

// TestChainPillInventory — FIT (auto-fit X) and AG (auto-gain Y) are gone:
// they are how the tab frames its own signal, and a toggle for "use the
// viewport properly" is a defect, not a feature. What is left is the A/B
// comparison the tab exists for, plus freeze.
func TestChainPillInventory(t *testing.T) {
	z := NewChainPanelZone(ChainCallbacks{})
	z.Layout(image.Rect(0, 100, 900, 400))
	got := renderedPillLabels(t, func(dst *ebiten.Image) { z.Draw(dst) })
	assertSameLabels(t, "chain", got, []string{"OVR", "SPL", "DIF", "FRZ"})
}

// TestSpectrumPillInventory — five of Spectrum's six pills were expert
// analyser controls whose effect is invisible unless you already know the
// term: a dB/oct slope tilt, a pre-EQ overlay, a peak-hold reset for marks
// that were themselves behind another toggle, a linear-frequency mode that
// no one reading an audio spectrum wants, and DTL, which hid the numbers the
// tab exists to show. Freeze is the only control left.
func TestSpectrumPillInventory(t *testing.T) {
	c := newSpectrumControls(0, func() bool { return false })
	c.Layout(image.Rect(0, 0, 900, 40))
	got := renderedPillLabels(t, func(dst *ebiten.Image) { c.Draw(dst) })
	assertSameLabels(t, "spectrum", got, []string{"<icon>"}) // <icon> = the shared freeze pill
}

// TestSpectrumDetailIsAlwaysOn — DTL gated the dB numbers, the note anchors
// and the peak-hold marks behind a toggle because, on the old fixed -80..0
// axis, those marks crowded a plot whose bars only reached the lower half.
// The axis is chosen from the signal now, so the marks sit against bars that
// fill the plot and there is nothing to opt out of. The pill is gone and the
// detail is unconditional.
func TestSpectrumDetailIsAlwaysOn(t *testing.T) {
	c := newSpectrumControls(0, func() bool { return false })
	if !c.Detail() {
		t.Error("spectrum detail is off; the dB numbers and note anchors are hidden with no way to reveal them")
	}
	c.Layout(image.Rect(0, 0, 900, 40))
	got := renderedPillLabels(t, func(dst *ebiten.Image) { c.Draw(dst) })
	assertSameLabels(t, "spectrum", got, []string{"<icon>"}) // freeze only
}

// TestSpectrumMaxWatermarkStillHasAResetPath — DTL's click was where the MAX
// watermark reset lived after RST was removed. The watermark never decays, so
// deleting the pill must not strand it a second time: the discontinuity
// invalidation is now its only reset, and it has to actually clear it.
func TestSpectrumMaxWatermarkStillHasAResetPath(t *testing.T) {
	z := NewEQPanelZone(EQCallbacks{})
	z.spectrumPeaks.MaxPeaksDB[4] = -20

	z.InvalidateSpectrumReadout()

	if z.spectrumPeaks.MaxPeaksDB[4] != spectrumNoPeakDB {
		t.Error("the MAX watermark survives every reset path there is; it can never be cleared")
	}
}
