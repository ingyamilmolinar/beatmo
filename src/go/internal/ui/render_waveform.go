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

// colWaveTraceFill is the soft synthwave "outrun" wash painted from the
// waveform trace down to the centerline at AlphaSubtle. Package-level so the
// color is served from pixelCache (no per-column color alloc); drawWaveTrace
// emits one fill rect per column above/below the midline, the trace line on
// top.
var colWaveTraceFill = WithAlpha(colWaveTrace, AlphaSubtle)

// drawAnalyzerWaveform renders a waveform from the given channel metrics into
// the rectangle. It picks frozen capture data when available, otherwise
// falls back to the channel's rolling waveform.
//
// beatGrid is an optional slice of fractional X positions (in [0,1)) where
// vertical beat markers should be drawn; nil disables the overlay. Fed by
// dv.beatGridFractions() via EQCallbacks.BeatGridFrac.
//
// The channel name is no longer drawn inside the waveform — the sticky bar
// above the panel owns channel identity (see audio_sticky_bar.go). Restating
// it here was redundant chrome.
func drawAnalyzerWaveform(dst *ebiten.Image, rect image.Rectangle, ch *analyzer.ChannelMetrics, capture *analyzer.CaptureBuffer, beatGrid []float64, gain float64, autoOn bool, tabFrozen bool) {
	if ch == nil {
		drawRect(dst, rect, colButtonBorder, false)
		return
	}

	// Choose waveform source.
	var wave []float64
	frozen := capture != nil && capture.Frozen
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

	// Reserve left margin for amplitude labels + bottom strip for the ms
	// axis (tick marks + labels), matching the Chain tab's convention so
	// time-correlation reads the same way across both signal views. The
	// strip height holds one full caption text row so label baselines are
	// never clipped by the panel's bottom edge.
	waveRect := image.Rect(rect.Min.X+Profile().DensityValues().AudioLabelMarginW, rect.Min.Y, rect.Max.X, rect.Max.Y-waveTimeAxisHeight())

	// Label scale.
	captionScale := FontSizeCaption / FontSizeBody
	lh := int(float64(TextHeight()) * captionScale)

	if gain <= 0 {
		gain = 1.0
	}
	edge := waveEdgeAmplitude(gain)

	// Draw amplitude labels in the left margin (SpaceXS inset from the
	// panel's left edge so they never render flush at x=0).
	// Edge labels reflect the TRUE amplitude the panel edges represent at the
	// current Y-gain (adaptive auto-scale): top/bottom = +/- 1/gain, center 0.
	DrawTextColorAtScale(dst, fmt.Sprintf("+%.2g", edge), rect.Min.X+SpaceXS, waveRect.Min.Y+2, colTextSecondary, captionScale)
	DrawTextColorAtScale(dst, "0", rect.Min.X+SpaceXS, waveRect.Min.Y+waveRect.Dy()/2-lh/2, colTextSecondary, captionScale)
	DrawTextColorAtScale(dst, fmt.Sprintf("-%.2g", edge), rect.Min.X+SpaceXS, waveRect.Max.Y-lh-2, colTextSecondary, captionScale)

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

	// Beat-grid overlay: 1-px vertical ticks at fractional positions (AlphaSubtle).
	// Drawn before the trace so the wave stays visually on top.
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

	// Phase 5: when stereo data is present, paint L in the top half and
	// R in the bottom half so the user sees per-channel content. Mono
	// signals continue to render as a single trace down the centerline.
	if ch.HasStereo() && len(ch.WaveformL) > 0 && len(ch.WaveformR) > 0 {
		topRect := image.Rect(waveRect.Min.X, waveRect.Min.Y, waveRect.Max.X, midY)
		botRect := image.Rect(waveRect.Min.X, midY, waveRect.Max.X, waveRect.Max.Y)
		topMid := topRect.Min.Y + topRect.Dy()/2
		botMid := botRect.Min.Y + botRect.Dy()/2
		drawWaveTrace(dst, ch.WaveformL, topRect, topMid, width, colWaveTrace, gain, colWaveTraceFill)
		drawWaveTrace(dst, ch.WaveformR, botRect, botMid, width, colWaveTrace, gain, colWaveTraceFill)
		// Small L / R labels along the left margin so the split reads
		// clearly without relying on context.
		DrawTextColorAtScale(dst, "L", rect.Min.X+2, topRect.Min.Y+2, colTextSecondary, captionScale)
		DrawTextColorAtScale(dst, "R", rect.Min.X+2, botRect.Min.Y+2, colTextSecondary, captionScale)
		overdrawWaveClips(dst, ch.WaveformL, topRect, width, meterClip)
		overdrawWaveClips(dst, ch.WaveformR, botRect, width, meterClip)
	} else {
		drawWaveTrace(dst, wave, waveRect, midY, width, colWaveTrace, gain, colWaveTraceFill)
		// Clip flash: overdraw any column whose samples saturate (|v| > 1.0)
		// with meterClip, full waveRect height. Gives instant DAW-style
		// headroom warning that's impossible to miss against the steady
		// trace color.
		overdrawWaveClips(dst, wave, waveRect, width, meterClip)
	}

	// Time-axis: tick marks + ms labels along the reserved bottom strip.
	axisRect := image.Rect(waveRect.Min.X, waveRect.Max.Y, waveRect.Max.X, waveRect.Max.Y+waveTimeAxisHeight())
	drawWaveTimeAxis(dst, axisRect, waveWindowMs(len(wave), audio.SampleRate()), captionScale)

	// Top-right status slot: FROZEN takes priority (global freeze reachable
	// from other tabs); otherwise show the live Y-scale badge so the user
	// always knows the current vertical scale.
	if frozen || tabFrozen {
		frozenW := int(float64(TextWidth(i18n.T(i18n.KeyCapFrozen))) * captionScale)
		DrawTextColorAtScale(dst, i18n.T(i18n.KeyCapFrozen), waveRect.Max.X-frozenW-4, waveRect.Min.Y+2, colAccentBright, captionScale)
	} else {
		badge := waveGainBadgeText(gain, autoOn)
		bw := int(float64(TextWidth(badge)) * captionScale)
		DrawTextColorAtScale(dst, badge, waveRect.Max.X-bw-4, waveRect.Min.Y+2, colTextSecondary, captionScale)
	}
}

// waveClipThreshold is the |v| level at which a waveform sample counts
// as clipped for the DAW-style red overlay. Aligns with the audio
// engine's hard-clip point at ±1.0; samples that reach this magnitude
// have already saturated the int16 output stage.
const waveClipThreshold = 1.0

// overdrawWaveClips paints columns whose source samples saturate
// (|v| ≥ waveClipThreshold) in clipCol, full rect height. Same
// min/max-per-pixel column math as drawWaveTrace so the painted column
// lines up exactly with the offending trace span.
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

// waveTimeAxisHeight is the reserved bottom strip (in px) for the Wave tab's
// ms-axis tick marks + labels: one caption text row plus padding, so label
// baselines are never clipped by the panel's bottom edge.
func waveTimeAxisHeight() int {
	captionScale := FontSizeCaption / FontSizeBody
	return int(float64(TextHeight())*captionScale) + SpaceSM
}

// drawWaveTimeAxis paints tick marks + ms labels across the bottom
// strip of the wave panel. Mirrors the Chain tab's bottom-axis style
// so time correlation reads identically across both signal surfaces.
func drawWaveTimeAxis(dst *ebiten.Image, axisRect image.Rectangle, windowMs float64, captionScale float64) {
	if axisRect.Empty() || windowMs <= 0 {
		return
	}
	w := axisRect.Dx()
	lh := int(float64(TextHeight()) * captionScale)
	for _, lbl := range waveAxisLabels(windowMs) {
		x := axisRect.Min.X + int(lbl.Frac*float64(w))
		if x >= axisRect.Max.X {
			x = axisRect.Max.X - 1
		}
		// 1px wide × 3px tall tick mark anchored to the BOTTOM of the axis
		// strip so it always lands in the panel's bottom band (where the
		// time-base is read), regardless of how tall the reserved strip is.
		drawRect(dst, image.Rect(x, axisRect.Max.Y-3, x+1, axisRect.Max.Y), colTextSecondary, true)
		// Label sits ABOVE the tick, fully inside the strip, so its baseline
		// is never clipped by the panel's bottom edge (the original bug).
		// Anchor first label to the left edge; centre the rest under their tick.
		lw := int(float64(TextWidth(lbl.Text)) * captionScale)
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
		DrawTextColorAtScale(dst, lbl.Text, lx, ly, colTextSecondary, captionScale)
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

		// Synthwave "outrun" fill from the trace down to the centerline:
		// a single soft wash at the caller's fillCol. The wash always spans
		// one contiguous vertical run from the far trace edge to the
		// centerline, i.e. [min(y0,midY) .. max(y1,midY)] — drawing it as ONE
		// rect per column instead of a top half + bottom half is
		// pixel-identical (same span, same fillCol) but halves the per-column
		// blit count on the audio panel's hottest renderer. That blit count
		// is the dominant Draw cost on the Chain tab (two full-width traces),
		// and on the single WASM thread an oversized Draw starves the
		// sequencer goroutine → choppy audio. See profile_chain_tab.mjs.
		// The fillCol is a fixed package-level color served from pixelCache,
		// so no per-column color alloc is incurred.
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

// drawWaveformCursor renders a vertical crosshair at cursorX inside the
// waveform area, plus an "amplitude · ms-in-window" readout label anchored
// above. cursorX is in screen pixels; nothing renders if cursorX falls
// outside the waveform rect.
//
// Sample lookup uses the same fractional offset the waveform renderer uses
// to map pixels → samples, so the readout reflects what the user sees on
// screen. When the capture buffer is frozen we read from the frozen copy;
// otherwise we read the rolling channel waveform.
func drawWaveformCursor(dst *ebiten.Image, rect image.Rectangle, ch *analyzer.ChannelMetrics, capture *analyzer.CaptureBuffer, cursorX int) {
	// Mirror drawAnalyzerWaveform's waveRect derivation (reserve 12px
	// bottom strip for the ms axis) so the cursor crosshair stops at
	// the wave area, not the axis labels below it.
	const waveMSAxisH = 12
	waveRect := image.Rect(rect.Min.X+Profile().DensityValues().AudioLabelMarginW, rect.Min.Y, rect.Max.X, rect.Max.Y-waveMSAxisH)
	if cursorX < waveRect.Min.X || cursorX >= waveRect.Max.X {
		return
	}
	if waveRect.Dx() <= 0 || waveRect.Dy() <= 0 {
		return
	}

	// Vertical line. Phase 6 audio-panel redesign: density-driven
	// stroke (Compact 1 / Comfortable 2 / Spacious 3) so the cursor
	// stays visible on a 360-px portrait phone where a 1-px stroke
	// disappears against the trace.
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
	captionScale := FontSizeCaption / FontSizeBody
	tw := int(float64(TextWidth(label)) * captionScale)
	th := int(float64(TextHeight()) * captionScale)
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
	DrawTextColorAtScale(dst, label, lx, ly, colTextSecondary, captionScale)
}
