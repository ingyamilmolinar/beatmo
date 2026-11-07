//go:build test

package ui

import (
	"image"
	"testing"
)

// Below the waveform, so the readout never overlaps the signal.
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

// So enlarged text fits at Spacious.
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
