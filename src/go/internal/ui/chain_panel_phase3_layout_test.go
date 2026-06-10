//go:build test

package ui

import (
	"image"
	"testing"
)

// TestChainAGInChromeRow — the auto-gain button must live in the top chrome
// row, NOT the trace's lower-right corner where it collided with the
// time-axis labels. Assert its rect sits above the content (trace) area.
func TestChainAGInChromeRow(t *testing.T) {
	z := NewChainPanelZone(ChainCallbacks{})
	z.Layout(image.Rect(0, 0, 600, 200))
	ag := z.autoGainBtn.Rect()
	if ag.Empty() {
		t.Fatal("AG button has no rect")
	}
	cr := z.contentRect()
	if ag.Max.Y > cr.Min.Y {
		t.Errorf("AG rect %v overlaps the trace content area (top %d) — must be in the chrome row", ag, cr.Min.Y)
	}
}

// TestChainFitButtonInChromeRowAndHitArea — the FIT toggle is laid out in the
// chrome row, registered as a hit area, and clicking it flips auto-fit.
func TestChainFitButtonInChromeRowAndHitArea(t *testing.T) {
	z := NewChainPanelZone(ChainCallbacks{})
	z.Layout(image.Rect(0, 0, 600, 200))

	fit := z.fitBtn.Rect()
	if fit.Empty() {
		t.Fatal("FIT button has no rect")
	}
	cr := z.contentRect()
	if fit.Max.Y > cr.Min.Y {
		t.Errorf("FIT rect %v overlaps the trace content area — must be in the chrome row", fit)
	}

	var found bool
	for _, a := range z.HitAreas() {
		if a.Tag == "scope-fit-btn" {
			found = true
		}
	}
	if !found {
		t.Error("missing hit area with tag 'scope-fit-btn'")
	}

	// Click toggles auto-fit (default on → off).
	before := z.AutoFit()
	z.fitBtn.OnClick()
	if z.AutoFit() == before {
		t.Error("FIT button click did not toggle auto-fit")
	}
}

// TestChainLegendStripBelowWaveform — the reserved legend strip must sit
// entirely below the waveform so the Pk/RMS readout never overlaps the
// signal (the pre-redesign legend was painted on top of the trace).
func TestChainLegendStripBelowWaveform(t *testing.T) {
	content := image.Rect(60, 0, 600, 200)
	waveRect, legendStrip := chainTraceRects(content)
	if legendStrip.Min.Y < waveRect.Max.Y {
		t.Errorf("legend strip (top %d) overlaps waveform (bottom %d)", legendStrip.Min.Y, waveRect.Max.Y)
	}
	if legendStrip.Dy() <= 0 {
		t.Errorf("legend strip has no height: %v", legendStrip)
	}
	// The strip must leave room for the time-axis labels below it (bottom margin).
	if legendStrip.Max.Y > content.Max.Y {
		t.Errorf("legend strip bottom %d exceeds content bottom %d", legendStrip.Max.Y, content.Max.Y)
	}
}

// TestChainLegendMarginsGrowWithDensity — the left gutter / bottom margin /
// legend strip all widen at Spacious so enlarged text fits.
func TestChainLegendMarginsGrowWithDensity(t *testing.T) {
	content := image.Rect(0, 0, 600, 300)
	measure := func(d Density) (leftGutter, stripH int) {
		restore := SetDensityForTest(d)
		defer restore()
		wave, strip := chainTraceRects(content)
		return wave.Min.X - content.Min.X, strip.Dy()
	}
	cLeft, cStrip := measure(DensityCompact)
	sLeft, sStrip := measure(DensitySpacious)
	if sLeft <= cLeft {
		t.Errorf("left gutter should widen with density: compact=%d spacious=%d", cLeft, sLeft)
	}
	if sStrip <= cStrip {
		t.Errorf("legend strip should grow with density: compact=%d spacious=%d", cStrip, sStrip)
	}
}
