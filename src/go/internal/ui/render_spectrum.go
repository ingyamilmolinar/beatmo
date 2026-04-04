package ui

import (
	"fmt"
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
)

// isoLabels are the center-frequency labels for the 10 ISO bands.
var isoLabels = [10]string{"31", "62", "125", "250", "500", "1k", "2k", "4k", "8k", "16k"}

// SpectrumPeakState tracks peak-hold values for the 10-band spectrum display.
type SpectrumPeakState struct {
	Peaks  [10]float64 // normalized 0-1 peak values
	Ages   [10]int     // frames since peak was set
	MaxAge int         // frames before peak decays (default 30 ≈ 500ms)
}

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
// rectangle using the channel's FFT data. It draws frequency labels along the
// bottom, a dB scale on the left, and optional peak-hold indicators when peaks
// is non-nil.
func drawAnalyzerSpectrum(dst *ebiten.Image, rect image.Rectangle, ch *analyzer.ChannelMetrics, peaks *SpectrumPeakState) {
	// Empty state: draw border only.
	if ch == nil || !ch.Active || len(ch.FFTBins) == 0 {
		drawRect(dst, rect, colButtonBorder, false)
		return
	}

	fftBins := ch.FFTBins
	freqBins := ch.FreqBins

	// FreqBins and FFTBins must be the same length for pairing.
	if len(freqBins) != len(fftBins) {
		drawRect(dst, rect, colButtonBorder, false)
		return
	}

	// Reserve 28px left for dB scale, 14px bottom for frequency labels.
	barRect := image.Rect(rect.Min.X+28, rect.Min.Y, rect.Max.X, rect.Max.Y-14)

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

	// Draw dB reference lines and labels.
	captionScale := FontSizeCaption / FontSizeBody
	dbRefs := []float64{0, -20, -40, -60}
	for _, db := range dbRefs {
		norm := (db - spectrumMinDB) / (spectrumMaxDB - spectrumMinDB)
		y := barRect.Max.Y - int(norm*float64(barRect.Dy()))
		// Dashed line (3px on, 3px off).
		for x := barRect.Min.X; x < barRect.Max.X; x += 6 {
			endX := x + 3
			if endX > barRect.Max.X {
				endX = barRect.Max.X
			}
			drawRect(dst, image.Rect(x, y, endX, y+1), color.NRGBA{255, 255, 255, 30}, true)
		}
		// Label on left margin.
		label := fmt.Sprintf("%.0f", db)
		lh := int(float64(TextHeight()) * captionScale)
		DrawTextColorAtScale(dst, label, rect.Min.X+2, y-lh/2, colTextSecondary, captionScale)
	}

	// Draw bars.
	numBands := len(isoBands)
	totalWidth := barRect.Dx()
	totalHeight := barRect.Dy()
	if totalWidth <= 0 || totalHeight <= 0 {
		return
	}

	// Each bar gets an equal share of the width, minus 1px gap between bars.
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

		x0 := barRect.Min.X + b*(barWidth+1)
		x1 := x0 + barWidth
		y1 := barRect.Max.Y
		y0 := y1 - barHeight

		if barHeight > 0 {
			drawRect(dst, image.Rect(x0, y0, x1, y1), colWaveTrace, true)
		}

		// Peak hold.
		if peaks != nil {
			if peaks.MaxAge == 0 {
				peaks.MaxAge = 30
			}
			if norm > peaks.Peaks[b] {
				peaks.Peaks[b] = norm
				peaks.Ages[b] = 0
			} else {
				peaks.Ages[b]++
				if peaks.Ages[b] > peaks.MaxAge {
					peaks.Peaks[b] = norm
					peaks.Ages[b] = 0
				}
			}
			// Draw peak marker.
			peakHeight := int(peaks.Peaks[b] * float64(barRect.Dy()))
			if peakHeight > 0 {
				py := barRect.Max.Y - peakHeight
				drawRect(dst, image.Rect(x0, py, x1, py+1), color.NRGBA{200, 240, 255, 200}, true)
			}
		}
	}

	// Draw frequency labels below bars.
	for b := 0; b < numBands; b++ {
		x0 := barRect.Min.X + b*(barWidth+1)
		label := isoLabels[b]
		lw := int(float64(TextWidth(label)) * captionScale)
		lx := x0 + (barWidth-lw)/2
		ly := barRect.Max.Y + 1
		DrawTextColorAtScale(dst, label, lx, ly, colTextSecondary, captionScale)
	}
}
