package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
)

// isoBands defines the 10 ISO frequency bands used for the spectrum display.
// Each pair is [lowHz, highHz).
var isoBands = [10][2]float64{
	{20, 44},
	{44, 88},
	{88, 177},
	{177, 355},
	{355, 710},
	{710, 1420},
	{1420, 2840},
	{2840, 5680},
	{5680, 11360},
	{11360, 22000},
}

const (
	spectrumMinDB = -80.0
	spectrumMaxDB = 0.0
)

// drawAnalyzerSpectrum renders a 10-band ISO spectrum analyzer into the given
// rectangle using the master channel's FFT data from the analyzer State.
func drawAnalyzerSpectrum(dst *ebiten.Image, rect image.Rectangle, state *analyzer.State) {
	// Empty state: draw border only.
	if state == nil || !state.Master.Active || len(state.Master.FFTBins) == 0 {
		drawRect(dst, rect, colButtonBorder, false)
		return
	}

	fftBins := state.Master.FFTBins
	freqBins := state.Master.FreqBins

	// FreqBins and FFTBins must be the same length for pairing.
	if len(freqBins) != len(fftBins) {
		drawRect(dst, rect, colButtonBorder, false)
		return
	}

	// Group FFT bins into 10 ISO bands and compute average dB for each.
	var bandDB [10]float64
	for b := range isoBands {
		lo, hi := isoBands[b][0], isoBands[b][1]
		sum := 0.0
		count := 0
		for i, hz := range freqBins {
			if hz >= lo && hz < hi {
				sum += fftBins[i]
				count++
			}
		}
		if count > 0 {
			bandDB[b] = sum / float64(count)
		} else {
			bandDB[b] = spectrumMinDB
		}
	}

	// Draw bars.
	numBands := len(isoBands)
	totalWidth := rect.Dx()
	totalHeight := rect.Dy()
	if totalWidth <= 0 || totalHeight <= 0 {
		return
	}

	// Each bar gets an equal share of the width, minus 1px gap between bars.
	// Total gaps = numBands - 1. Remaining pixels go to bars.
	gaps := numBands - 1
	barSpace := totalWidth - gaps
	if barSpace < numBands {
		// Not enough room for even 1px per bar.
		return
	}
	barWidth := barSpace / numBands

	for b := 0; b < numBands; b++ {
		// Normalize dB to 0-1 range.
		db := bandDB[b]
		if db < spectrumMinDB {
			db = spectrumMinDB
		}
		if db > spectrumMaxDB {
			db = spectrumMaxDB
		}
		norm := (db - spectrumMinDB) / (spectrumMaxDB - spectrumMinDB)

		barHeight := int(norm * float64(totalHeight))
		if barHeight < 1 && norm > 0 {
			barHeight = 1
		}

		x0 := rect.Min.X + b*(barWidth+1)
		x1 := x0 + barWidth
		y1 := rect.Max.Y
		y0 := y1 - barHeight

		if barHeight > 0 {
			drawRect(dst, image.Rect(x0, y0, x1, y1), colWaveTrace, true)
		}
	}
}
