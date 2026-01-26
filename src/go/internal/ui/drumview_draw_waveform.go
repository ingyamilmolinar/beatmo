package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/tunkul/internal/audio"
)

func (dv *DrumView) drawWaveform(dst *ebiten.Image, snap audio.AnalyzerSnapshot) {
	wave := snap.Waveform
	if len(wave) == 0 {
		drawRect(dst, dv.eqRect, colButtonBorder, false)
		return
	}
	// Draw midline
	midY := dv.eqRect.Min.Y + dv.eqRect.Dy()/2
	drawRect(dst, image.Rect(dv.eqRect.Min.X, midY, dv.eqRect.Max.X, midY+1), colWaveMid, true)

	width := dv.eqRect.Dx()
	if width <= 0 {
		return
	}
	// Downsample waveform to panel width. Use min/max per bucket for clarity.
	step := float64(len(wave)) / float64(width)
	if step < 1 {
		step = 1
	}
	for x := 0; x < width; x++ {
		start := int(float64(x) * step)
		end := int(float64(x+1) * step)
		if start >= len(wave) {
			break
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
		drawRect(dst, image.Rect(dv.eqRect.Min.X+x, y0, dv.eqRect.Min.X+x+1, y1), colWaveTrace, true)
	}
}
