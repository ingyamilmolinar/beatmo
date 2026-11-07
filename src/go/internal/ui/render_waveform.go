package ui

import (
	"fmt"
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// Package-level so pixelCache serves it without a per-column alloc. Derived in rebuildThemeAliases.
var colWaveTraceFill color.NRGBA // derived in rebuildThemeAliases (theme.go)

// beatGrid holds beat-marker X fractions in [0,1); nil disables them. No channel name here: the sticky
// bar owns channel identity.
func drawAnalyzerWaveform(dst *ebiten.Image, rect image.Rectangle, ch *analyzer.ChannelMetrics, capture *analyzer.CaptureBuffer, beatGrid []float64, gain float64, autoOn bool, tabFrozen bool) {
	// Nil-safe: an idle panel still draws its axis chrome.
	var wave []float64
	frozen := capture != nil && capture.Frozen
	switch {
	case frozen:
		wave = capture.Wave.Samples
	case ch != nil && ch.Active:
		wave = ch.Waveform
	}
	idle := len(wave) == 0

	// The same margins as the Chain tab so time reads alike in both; the axis strip holds a full caption row
	// so its baselines are never clipped.
	waveRect := image.Rect(rect.Min.X+Profile().DensityValues().AudioLabelMarginW, rect.Min.Y, rect.Max.X, rect.Max.Y-waveTimeAxisHeight())

	lh := StyledTextHeight(RoleMicro)

	if gain <= 0 {
		gain = 1.0
	}
	edge := waveEdgeAmplitude(gain)

	// The edge labels show the TRUE amplitude at the current gain: ±1/gain.
	DrawTextStyled(dst, fmt.Sprintf("+%.2g", edge), rect.Min.X+SpaceXS, waveRect.Min.Y+2, RoleMicro, colTextSecondary)
	DrawTextStyled(dst, "0", rect.Min.X+SpaceXS, waveRect.Min.Y+waveRect.Dy()/2-lh/2, RoleMicro, colTextSecondary)
	DrawTextStyled(dst, fmt.Sprintf("-%.2g", edge), rect.Min.X+SpaceXS, waveRect.Max.Y-lh-2, RoleMicro, colTextSecondary)

	// Draw midline at vertical center.
	midY := waveRect.Min.Y + waveRect.Dy()/2
	drawRect(dst, image.Rect(waveRect.Min.X, midY, waveRect.Max.X, midY+1), colWaveMid, true)

	// Draw dashed grid lines at +0.5 and -0.5.
	halfH := float64(waveRect.Dy()) * 0.48
	gridCol := WithAlpha(genColorBorder, genAlphaBorderPanel)
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

	if idle {
		// A quiet caption so the idle panel reads as "waiting for sound", never as dead.
		hint := i18n.T(i18n.KeyAnalyzerIdle)
		hx := waveRect.Min.X + (waveRect.Dx()-StyledTextWidth(hint, RoleCaption))/2
		DrawTextStyled(dst, hint, hx, midY+SpaceMD, RoleCaption, colTextSecondary)
		return
	}

	// Before the trace so the wave stays on top.
	if len(beatGrid) > 0 {
		beatCol := WithAlpha(genColorBorder, AlphaSubtle)
		for _, frac := range beatGrid {
			if frac < 0 || frac >= 1 {
				continue
			}
			x := waveRect.Min.X + int(frac*float64(width))
			drawRect(dst, image.Rect(x, waveRect.Min.Y, x+1, waveRect.Max.Y), beatCol, true)
		}
	}

	// Stereo: L in the top half, R in the bottom.
	if ch != nil && ch.HasStereo() && len(ch.WaveformL) > 0 && len(ch.WaveformR) > 0 {
		topRect := image.Rect(waveRect.Min.X, waveRect.Min.Y, waveRect.Max.X, midY)
		botRect := image.Rect(waveRect.Min.X, midY, waveRect.Max.X, waveRect.Max.Y)
		topMid := topRect.Min.Y + topRect.Dy()/2
		botMid := botRect.Min.Y + botRect.Dy()/2
		drawWaveTrace(dst, ch.WaveformL, topRect, topMid, width, colWaveTrace, gain, colWaveTraceFill)
		drawWaveTrace(dst, ch.WaveformR, botRect, botMid, width, colWaveTrace, gain, colWaveTraceFill)
		DrawTextStyled(dst, "L", rect.Min.X+2, topRect.Min.Y+2, RoleCaption, colTextSecondary)
		DrawTextStyled(dst, "R", rect.Min.X+2, botRect.Min.Y+2, RoleCaption, colTextSecondary)
		overdrawWaveClips(dst, ch.WaveformL, topRect, width, meterClip)
		overdrawWaveClips(dst, ch.WaveformR, botRect, width, meterClip)
	} else {
		drawWaveTrace(dst, wave, waveRect, midY, width, colWaveTrace, gain, colWaveTraceFill)
		// Saturated columns flash full height in the clip color: a headroom warning that cannot be missed.
		overdrawWaveClips(dst, wave, waveRect, width, meterClip)
	}

	// Time-axis: tick marks + ms labels along the reserved bottom strip.
	axisRect := image.Rect(waveRect.Min.X, waveRect.Max.Y, waveRect.Max.X, waveRect.Max.Y+waveTimeAxisHeight())
	drawWaveTimeAxis(dst, axisRect, waveWindowMs(len(wave), audio.SampleRate()))

	// FROZEN wins (it can be global from another tab); otherwise the Y-scale badge, so the scale is always known.
	if frozen || tabFrozen {
		frozenW := StyledTextWidth(i18n.T(i18n.KeyCapFrozen), RoleCaption)
		DrawTextStyled(dst, i18n.T(i18n.KeyCapFrozen), waveRect.Max.X-frozenW-4, waveRect.Min.Y+2, RoleCaption, colAccentBright)
	} else {
		badge := waveGainBadgeText(gain, autoOn)
		bw := StyledTextWidth(badge, RoleCaption)
		DrawTextStyled(dst, badge, waveRect.Max.X-bw-4, waveRect.Min.Y+2, RoleCaption, colTextSecondary)
	}
}

// The engine's hard-clip point: a sample this large has already saturated the output stage.
const waveClipThreshold = 1.0

// The same per-column min/max math as drawWaveTrace, so the flash lines up with the offending span.
func overdrawWaveClips(dst *ebiten.Image, wave []float64, rect image.Rectangle, width int, clipCol color.Color) {
	if width <= 0 || len(wave) == 0 || clipCol == nil {
		return
	}
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
		clipped := false
		for i := start; i < end; i++ {
			v := wave[i]
			if v > waveClipThreshold || v < -waveClipThreshold {
				clipped = true
				break
			}
		}
		if clipped {
			px := rect.Min.X + x
			drawRect(dst, image.Rect(px, rect.Min.Y, px+1, rect.Max.Y), clipCol, true)
		}
	}
}

func waveTimeAxisHeight() int {
	return StyledTextHeight(RoleMicro) + SpaceSM
}

// The Chain tab's axis style, so time reads alike on both.
func drawWaveTimeAxis(dst *ebiten.Image, axisRect image.Rectangle, windowMs float64) {
	if axisRect.Empty() || windowMs <= 0 {
		return
	}
	w := axisRect.Dx()
	lh := StyledTextHeight(RoleMicro)
	for _, lbl := range waveAxisLabels(windowMs) {
		x := axisRect.Min.X + int(lbl.Frac*float64(w))
		if x >= axisRect.Max.X {
			x = axisRect.Max.X - 1
		}
		// Anchored to the strip bottom, where the time base is read, however tall the strip is.
		drawRect(dst, image.Rect(x, axisRect.Max.Y-3, x+1, axisRect.Max.Y), colTextSecondary, true)
		// Above the tick and inside the strip so its baseline is never clipped; the first label anchors left.
		lw := StyledTextWidth(lbl.Text, RoleMicro)
		lx := x - lw/2
		if lbl.Frac == 0 {
			lx = axisRect.Min.X
		}
		if lx+lw > axisRect.Max.X {
			lx = axisRect.Max.X - lw
		}
		if lx < axisRect.Min.X {
			lx = axisRect.Min.X
		}
		ly := axisRect.Max.Y - 3 - lh
		if ly < axisRect.Min.Y {
			ly = axisRect.Min.Y
		}
		DrawTextStyled(dst, lbl.Text, lx, ly, RoleMicro, colTextSecondary)
	}
}

// Min/max per pixel column. yGain > 1 zooms in; a non-nil fillCol washes from the trace to the centerline.
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

		// ONE wash rect per column, pixel-identical to two halves but half the blits on the panel's hottest
		// renderer: on the single WASM thread an oversized Draw starves the sequencer.
		if fillCol != nil {
			px := rect.Min.X + x
			fy0, fy1 := y0, y1
			if midY < fy0 {
				fy0 = midY
			}
			if midY > fy1 {
				fy1 = midY
			}
			if fy1 > fy0 {
				drawRect(dst, image.Rect(px, fy0, px+1, fy1), fillCol, true)
			}
		}

		drawRect(dst, image.Rect(rect.Min.X+x, y0, rect.Min.X+x+1, y1), col, true)
	}
}

// cursorX is in screen pixels; the sample lookup uses the renderer's own pixel-to-sample map, so the
// readout matches what is on screen.
func drawWaveformCursor(dst *ebiten.Image, rect image.Rectangle, ch *analyzer.ChannelMetrics, capture *analyzer.CaptureBuffer, cursorX int) {
	// Must match drawAnalyzerWaveform's axis strip so the crosshair stops at the wave area.
	const waveMSAxisH = 12
	waveRect := image.Rect(rect.Min.X+Profile().DensityValues().AudioLabelMarginW, rect.Min.Y, rect.Max.X, rect.Max.Y-waveMSAxisH)
	if cursorX < waveRect.Min.X || cursorX >= waveRect.Max.X {
		return
	}
	if waveRect.Dx() <= 0 || waveRect.Dy() <= 0 {
		return
	}

	// Density-driven so a 1 px line does not vanish against the trace on a phone.
	cursorW := Profile().DensityValues().WaveCursorStroke
	if cursorW < 1 {
		cursorW = 1
	}
	drawRect(dst, image.Rect(cursorX, waveRect.Min.Y, cursorX+cursorW, waveRect.Max.Y), colTextSecondary, true)

	// Resolve sample source (frozen wins).
	var wave []float64
	if capture != nil && capture.Frozen {
		wave = capture.Wave.Samples
	}
	if wave == nil && ch != nil {
		wave = ch.Waveform
	}
	if len(wave) == 0 {
		return
	}

	// Map cursorX → sample index → amplitude.
	frac := float64(cursorX-waveRect.Min.X) / float64(waveRect.Dx())
	idx := int(frac * float64(len(wave)))
	if idx < 0 {
		idx = 0
	}
	if idx >= len(wave) {
		idx = len(wave) - 1
	}
	amp := wave[idx]
	sr := audio.SampleRate()
	if sr <= 0 {
		sr = 44100
	}
	msInWindow := float64(idx) * 1000.0 / float64(sr)

	label := fmt.Sprintf("%.2f · %.1f ms", amp, msInWindow)
	tw := StyledTextWidth(label, RoleMicro)
	th := StyledTextHeight(RoleMicro)
	lx := cursorX + 4
	if lx+tw+4 > waveRect.Max.X {
		lx = cursorX - tw - 4
	}
	if lx < waveRect.Min.X {
		lx = waveRect.Min.X
	}
	ly := waveRect.Min.Y + 2
	bg := image.Rect(lx-2, ly-1, lx+tw+2, ly+th+2)
	drawRect(dst, bg, WithAlpha(colSurface2, AlphaStrong), true)
	DrawTextStyled(dst, label, lx, ly, RoleMicro, colTextSecondary)
}
