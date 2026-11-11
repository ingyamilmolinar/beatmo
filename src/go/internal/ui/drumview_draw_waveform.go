package ui

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

func (dv *DrumView) drawWaveform(dst *ebiten.Image, snap audio.AnalyzerSnapshot) {
	wave := snap.Waveform
	if len(wave) == 0 {
		// Idle: render the modern axis chrome + hint instead of a dead
		// border-only rect (native desktop showed a blank Wave tab at boot).
		drawAnalyzerWaveform(dst, dv.eqRect, nil, nil, nil, 1.0, false, false)
		return
	}
	// Draw midline
	midY := dv.eqRect.Min.Y + dv.eqRect.Dy()/2
	drawRect(dst, image.Rect(dv.eqRect.Min.X, midY, dv.eqRect.Max.X, midY+1), colWaveMid, true)

	width := dv.eqRect.Dx()
	if width <= 0 {
		return
	}

	// Draw pre-EQ (dry) waveform first in dim color.
	preSnap := dv.preEQAnalyzerSnapshot()
	if len(preSnap.Waveform) > 0 {
		dv.drawWaveformTrace(dst, preSnap.Waveform, midY, width, colWaveTraceDry)
	}

	// Draw post-EQ (wet) waveform on top in bright color.
	dv.drawWaveformTrace(dst, wave, midY, width, colWaveTrace)
}

// drawWaveformTrace draws a single waveform trace onto dst.
func (dv *DrumView) drawWaveformTrace(dst *ebiten.Image, wave []float64, midY, width int, col color.Color) {
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
		y0 := midY - int(maxV*float64(dv.eqRect.Dy())*0.48)
		y1 := midY - int(minV*float64(dv.eqRect.Dy())*0.48)
		if y0 > y1 {
			y0, y1 = y1, y0
		}
		if y1 < dv.eqRect.Min.Y {
			y1 = dv.eqRect.Min.Y
		}
		if y0 > dv.eqRect.Max.Y {
			y0 = dv.eqRect.Max.Y
		}
		if y0 == y1 {
			y1 = y0 + 1
		}
		drawRect(dst, image.Rect(dv.eqRect.Min.X+x, y0, dv.eqRect.Min.X+x+1, y1), col, true)
	}
}
