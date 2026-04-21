package ui

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
)

// drawAnalyzerWaveform renders a waveform from the given channel metrics into
// the rectangle. It picks frozen capture data when available, otherwise
// falls back to the channel's rolling waveform.
func drawAnalyzerWaveform(dst *ebiten.Image, rect image.Rectangle, ch *analyzer.ChannelMetrics, capture *analyzer.CaptureBuffer) {
	if ch == nil {
		drawRect(dst, rect, colButtonBorder, false)
		return
	}

	// Choose waveform source and channel name.
	var wave []float64
	frozen := capture != nil && capture.Frozen
	channelName := ch.Name
	if channelName == "" {
		channelName = "Master"
	}
	switch {
	case frozen:
		wave = capture.Wave.Samples
	case ch.Active:
		wave = ch.Waveform
	}

	if len(wave) == 0 {
		drawRect(dst, rect, colButtonBorder, false)
		return
	}

	// Reserve left margin for amplitude labels.
	waveRect := image.Rect(rect.Min.X+28, rect.Min.Y, rect.Max.X, rect.Max.Y)

	// Label scale.
	captionScale := FontSizeCaption / FontSizeBody
	lh := int(float64(TextHeight()) * captionScale)

	// Draw amplitude labels in the left margin.
	DrawTextColorAtScale(dst, "+1", rect.Min.X+2, waveRect.Min.Y+2, colTextSecondary, captionScale)
	DrawTextColorAtScale(dst, "0", rect.Min.X+2, waveRect.Min.Y+waveRect.Dy()/2-lh/2, colTextSecondary, captionScale)
	DrawTextColorAtScale(dst, "-1", rect.Min.X+2, waveRect.Max.Y-lh-2, colTextSecondary, captionScale)

	// Draw midline at vertical center.
	midY := waveRect.Min.Y + waveRect.Dy()/2
	drawRect(dst, image.Rect(waveRect.Min.X, midY, waveRect.Max.X, midY+1), colWaveMid, true)

	// Draw dashed grid lines at +0.5 and -0.5.
	halfH := float64(waveRect.Dy()) * 0.48
	gridCol := color.NRGBA{255, 255, 255, 20}
	for _, yOff := range []int{
		midY - int(0.5*halfH),
		midY + int(0.5*halfH),
	} {
		for x := waveRect.Min.X; x < waveRect.Max.X; x += 6 {
			endX := x + 3
			if endX > waveRect.Max.X {
				endX = waveRect.Max.X
			}
			drawRect(dst, image.Rect(x, yOff, endX, yOff+1), gridCol, true)
		}
	}

	width := waveRect.Dx()
	if width <= 0 {
		return
	}

	drawWaveTrace(dst, wave, waveRect, midY, width, colWaveTrace, 1.0, nil)

	// Channel name at top-left of waveform area.
	DrawTextColorAtScale(dst, channelName, waveRect.Min.X+4, waveRect.Min.Y+2, colTextSecondary, captionScale)

	// Frozen indicator at top-right.
	if frozen {
		frozenW := int(float64(TextWidth("FROZEN")) * captionScale)
		DrawTextColorAtScale(dst, "FROZEN", waveRect.Max.X-frozenW-4, waveRect.Min.Y+2, colAccentBright, captionScale)
	}
}

// drawWaveTrace draws a single waveform trace using the min/max-per-pixel-column
// technique. For each pixel column it finds the minimum and maximum sample values
// that map to that column and draws a vertical line spanning the range.
// yGain scales the amplitude display (1.0 = normal, >1 = zoomed in).
// fillCol, if non-nil, draws a semi-transparent fill from the trace to the centerline.
func drawWaveTrace(dst *ebiten.Image, wave []float64, rect image.Rectangle, midY, width int, col color.Color, yGain float64, fillCol color.Color) {
	halfHeight := float64(rect.Dy()) * 0.48 * yGain
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

		// Fill from trace to centerline.
		if fillCol != nil {
			px := rect.Min.X + x
			if y0 < midY {
				drawRect(dst, image.Rect(px, y0, px+1, midY), fillCol, true)
			}
			if y1 > midY {
				drawRect(dst, image.Rect(px, midY, px+1, y1), fillCol, true)
			}
		}

		drawRect(dst, image.Rect(rect.Min.X+x, y0, rect.Min.X+x+1, y1), col, true)
	}
}
