package ui

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
)

// drawAnalyzerWaveform renders a waveform from the analyzer State into the
// given rectangle. It picks frozen capture data when available, otherwise
// falls back to the master channel's rolling waveform.
func drawAnalyzerWaveform(dst *ebiten.Image, rect image.Rectangle, state *analyzer.State) {
	if state == nil {
		drawRect(dst, rect, colButtonBorder, false)
		return
	}

	// Choose waveform source.
	var wave []float64
	switch {
	case state.Capture != nil && state.Capture.Frozen:
		wave = state.Capture.Wave.Samples
	case state.Master.Active:
		wave = state.Master.Waveform
	}

	if len(wave) == 0 {
		drawRect(dst, rect, colButtonBorder, false)
		return
	}

	// Draw midline at vertical center.
	midY := rect.Min.Y + rect.Dy()/2
	drawRect(dst, image.Rect(rect.Min.X, midY, rect.Max.X, midY+1), colWaveMid, true)

	width := rect.Dx()
	if width <= 0 {
		return
	}

	drawWaveTrace(dst, wave, rect, midY, width, colWaveTrace)
}

// drawWaveTrace draws a single waveform trace using the min/max-per-pixel-column
// technique. For each pixel column it finds the minimum and maximum sample values
// that map to that column and draws a vertical line spanning the range.
func drawWaveTrace(dst *ebiten.Image, wave []float64, rect image.Rectangle, midY, width int, col color.Color) {
	halfHeight := float64(rect.Dy()) * 0.48
	step := float64(len(wave)) / float64(width)

	for x := 0; x < width; x++ {
		start := int(float64(x) * step)
		end := int(float64(x+1) * step)
		if start >= len(wave) {
			start = len(wave) - 1
		}
		if end <= start {
			end = start + 1
		}
		if end > len(wave) {
			end = len(wave)
		}

		minV, maxV := 1.0, -1.0
		for i := start; i < end; i++ {
			v := wave[i]
			if v < minV {
				minV = v
			}
			if v > maxV {
				maxV = v
			}
		}

		y0 := midY - int(maxV*halfHeight)
		y1 := midY - int(minV*halfHeight)
		if y0 > y1 {
			y0, y1 = y1, y0
		}
		if y1 < rect.Min.Y {
			y1 = rect.Min.Y
		}
		if y0 > rect.Max.Y {
			y0 = rect.Max.Y
		}
		if y0 == y1 {
			y1 = y0 + 1
		}

		drawRect(dst, image.Rect(rect.Min.X+x, y0, rect.Min.X+x+1, y1), col, true)
	}
}
