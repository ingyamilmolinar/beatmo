package ui

import (
	"fmt"
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	scope "github.com/ingyamilmolinar/beatmo/internal/scope"
)

// Scope colors are sourced from DESIGN.md `viz-scope-*` tokens. Trace
// alpha values are scope-internal (180/150 for the line, 18/15 for the
// fill, etc.) and stay as numeric arguments to WithAlpha; only the base
// hex moves to DESIGN.md.
var (
	colScopeA     = WithAlpha(genColorVizScopeTraceA, 180)
	colScopeB     = WithAlpha(genColorVizScopeTraceB, 150)
	colScopeAFill = WithAlpha(genColorVizScopeTraceA, 18)
	colScopeBFill = WithAlpha(genColorVizScopeTraceB, 15)
)

var (
	colScopeDiff     = WithAlpha(genColorVizScopeTraceDiff, 200)
	colScopeDiffFill = WithAlpha(genColorVizScopeTraceDiff, 15)
)

var (
	colScopeBg      = genColorVizScopeBg
	colScopeGrid    = WithAlpha(genColorBorder, 20)
	colScopeGridMid = WithAlpha(genColorBorder, 35)
	colScopeTrigger = genColorVizScopeTrigger
)

// colScopeFrozenBorder reuses destructive-confirm-border (#FF5050) at the
// scope-internal "frozen" alpha 160 — sits between AlphaStrong (180) and
// AlphaMedium (90) so it reads as a clear-but-not-loud halt indicator.
var colScopeFrozenBorder = WithAlpha(genColorDestructiveConfirmBorder, 160)

const (
	scopeLeftMargin   = 32
	scopeBottomMargin = 14
)

// drawChainTraces renders overlaid or split A/B waveform traces with grid,
// labels, trigger marker, and peak/RMS legend.
// yGain scales the amplitude display (1.0 = normal, >1 = zoomed in).
func drawChainTraces(dst *ebiten.Image, rect image.Rectangle, state *scope.State, windowMs float64, displayMode chainDisplayMode, frozen bool, yGain float64, showA, showB bool) {
	if rect.Dx() < 20 || rect.Dy() < 20 {
		return
	}

	// Dark background.
	drawRect(dst, rect, colScopeBg, true)

	// Waveform area: inset by left margin and bottom margin.
	waveRect := image.Rect(
		rect.Min.X+scopeLeftMargin, rect.Min.Y,
		rect.Max.X, rect.Max.Y-scopeBottomMargin,
	)
	if waveRect.Dx() <= 0 || waveRect.Dy() <= 0 {
		return
	}

	captionScale := FontSizeCaption / FontSizeBody
	lh := int(float64(TextHeight()) * captionScale)
	w := waveRect.Dx()

	// --- Time axis labels (bottom margin) ---
	numLabels := 6
	for i := 0; i < numLabels; i++ {
		frac := float64(i) / float64(numLabels-1)
		ms := frac * windowMs
		label := formatWindowMs(ms)
		lx := waveRect.Min.X + int(frac*float64(w)) - int(float64(TextWidth(label))*captionScale)/2
		if lx < waveRect.Min.X {
			lx = waveRect.Min.X
		}
		ly := waveRect.Max.Y + 2
		DrawTextColorAtScale(dst, label, lx, ly, colTextSecondary, captionScale)
	}

	switch {
	case displayMode == chainSplit && state != nil && state.TapA.Active && state.TapB.Active:
		// --- Split mode: A in top half, B in bottom half ---
		sepY := waveRect.Min.Y + waveRect.Dy()/2
		topRect := image.Rect(waveRect.Min.X, waveRect.Min.Y, waveRect.Max.X, sepY)
		botRect := image.Rect(waveRect.Min.X, sepY+1, waveRect.Max.X, waveRect.Max.Y)

		if showA {
			drawScopeHalf(dst, rect, topRect, &state.TapA, colScopeA, windowMs, captionScale, lh, yGain)
		}
		if showB {
			drawScopeHalf(dst, rect, botRect, &state.TapB, colScopeB, windowMs, captionScale, lh, yGain)
		}

		// Separator line.
		drawRect(dst, image.Rect(waveRect.Min.X, sepY, waveRect.Max.X, sepY+1), colScopeGridMid, true)

		// Legend in each half.
		drawScopeTapLegend(dst, topRect, &state.TapA, colScopeA, "A", captionScale, lh)
		drawScopeTapLegend(dst, botRect, &state.TapB, colScopeB, "B", captionScale, lh)

		drawScopeBorder(dst, waveRect)

	case displayMode == chainDiff && state != nil && state.TapA.Active && state.TapB.Active && showA && showB:
		// --- Difference mode: render A-B (requires both traces visible) ---
		drawScopeDiff(dst, rect, waveRect, state, windowMs, captionScale, lh, yGain)

	default:
		// --- Overlay mode ---
		drawChainOverlay(dst, rect, waveRect, state, windowMs, captionScale, lh, yGain, showA, showB)
	}

	// --- Frozen indicator ---
	if frozen {
		frozenText := "FROZEN"
		ftw := int(float64(TextWidth(frozenText)) * captionScale)
		cx := waveRect.Min.X + (waveRect.Dx()-ftw)/2
		DrawTextColorAtScale(dst, frozenText, cx, waveRect.Min.Y+2, colAccentBright, captionScale)
		// Red border glow to indicate frozen state.
		drawRect(dst, image.Rect(waveRect.Min.X, waveRect.Min.Y, waveRect.Max.X, waveRect.Min.Y+1), colScopeFrozenBorder, true)
		drawRect(dst, image.Rect(waveRect.Min.X, waveRect.Max.Y-1, waveRect.Max.X, waveRect.Max.Y), colScopeFrozenBorder, true)
		drawRect(dst, image.Rect(waveRect.Min.X, waveRect.Min.Y, waveRect.Min.X+1, waveRect.Max.Y), colScopeFrozenBorder, true)
		drawRect(dst, image.Rect(waveRect.Max.X-1, waveRect.Min.Y, waveRect.Max.X, waveRect.Max.Y), colScopeFrozenBorder, true)
	}
}

// drawChainOverlay renders both traces overlaid on the same waveRect.
func drawChainOverlay(dst *ebiten.Image, fullRect, waveRect image.Rectangle, state *scope.State, windowMs float64, captionScale float64, lh int, yGain float64, showA, showB bool) {
	midY := waveRect.Min.Y + waveRect.Dy()/2
	w := waveRect.Dx()
	h := waveRect.Dy()

	// --- Grid lines ---
	drawScopeGrid(dst, waveRect, w, h)

	// --- Trigger marker at 10% from left ---
	trigX := waveRect.Min.X + w/10
	drawRect(dst, image.Rect(trigX, waveRect.Min.Y, trigX+1, waveRect.Max.Y), colScopeTrigger, true)

	// --- Amplitude labels (left margin) ---
	topLabel, botLabel := "+1", "-1"
	if yGain != 1.0 {
		edge := 1.0 / yGain
		topLabel = fmt.Sprintf("+%.2g", edge)
		botLabel = fmt.Sprintf("-%.2g", edge)
	}
	DrawTextColorAtScale(dst, topLabel, fullRect.Min.X+2, waveRect.Min.Y+2, colTextPrimary, captionScale)
	DrawTextColorAtScale(dst, "0", fullRect.Min.X+4, midY-lh/2, colTextPrimary, captionScale)
	DrawTextColorAtScale(dst, botLabel, fullRect.Min.X+2, waveRect.Max.Y-lh-2, colTextPrimary, captionScale)

	// --- Draw traces ---
	if state == nil {
		drawScopeBorder(dst, waveRect)
		return
	}

	// Draw TapB (orange/"after") first, then TapA (blue/"before") on top
	// so the "before" comparison signal is always visible.
	if showB && state.TapB.Active && len(state.TapB.Samples) > 0 {
		drawWaveTrace(dst, state.TapB.Samples, waveRect, midY, w, colScopeB, yGain, colScopeBFill)
	}
	if showA && state.TapA.Active && len(state.TapA.Samples) > 0 {
		drawWaveTrace(dst, state.TapA.Samples, waveRect, midY, w, colScopeA, yGain, colScopeAFill)
	}

	// --- Legend (right-aligned, peak/RMS with color swatches) ---
	legendY := waveRect.Min.Y + 2
	legendX := waveRect.Max.X - 4

	if state.TapA.Active {
		textA := fmt.Sprintf("A: %s  Pk: %.1f dB  RMS: %.1f dB",
			scope.StageLabel(state.TapA.Stage),
			clampDBDisplay(state.TapA.PeakDB),
			clampDBDisplay(state.TapA.RMSDB))
		tw := int(float64(TextWidth(textA)) * captionScale)
		// Semi-transparent background behind legend.
		bgRect := image.Rect(legendX-tw-10, legendY-1, legendX+2, legendY+lh+1)
		drawRect(dst, bgRect, WithAlpha(genColorVizScopeBg, 180), true)
		// Color swatch.
		swatchY := legendY + lh/2 - 1
		drawRect(dst, image.Rect(legendX-tw-8, swatchY, legendX-tw-2, swatchY+2), colScopeA, true)
		textCol := colScopeA
		if !showA {
			textCol = WithAlpha(genColorVizScopeTraceA, 60) // dimmed when hidden
		}
		DrawTextColorAtScale(dst, textA, legendX-tw, legendY, textCol, captionScale)
		legendY += lh + 2
	}
	if state.TapB.Active {
		textB := fmt.Sprintf("B: %s  Pk: %.1f dB  RMS: %.1f dB",
			scope.StageLabel(state.TapB.Stage),
			clampDBDisplay(state.TapB.PeakDB),
			clampDBDisplay(state.TapB.RMSDB))
		tw := int(float64(TextWidth(textB)) * captionScale)
		bgRect := image.Rect(legendX-tw-10, legendY-1, legendX+2, legendY+lh+1)
		drawRect(dst, bgRect, WithAlpha(genColorVizScopeBg, 180), true)
		swatchY := legendY + lh/2 - 1
		drawRect(dst, image.Rect(legendX-tw-8, swatchY, legendX-tw-2, swatchY+2), colScopeB, true)
		textCol := colScopeB
		if !showB {
			textCol = WithAlpha(genColorVizScopeTraceB, 60) // dimmed when hidden
		}
		DrawTextColorAtScale(dst, textB, legendX-tw, legendY, textCol, captionScale)
	}

	drawScopeBorder(dst, waveRect)
}

// drawScopeHalf renders a single tap's waveform in a half-height area (split mode).
func drawScopeHalf(dst *ebiten.Image, fullRect, halfRect image.Rectangle, tap *scope.TapData, col color.Color, windowMs, captionScale float64, lh int, yGain float64) {
	midY := halfRect.Min.Y + halfRect.Dy()/2
	w := halfRect.Dx()
	h := halfRect.Dy()

	// Grid for this half.
	drawScopeGrid(dst, halfRect, w, h)

	// Trigger marker.
	trigX := halfRect.Min.X + w/10
	drawRect(dst, image.Rect(trigX, halfRect.Min.Y, trigX+1, halfRect.Max.Y), colScopeTrigger, true)

	// Amplitude labels.
	topLabel, botLabel := "+1", "-1"
	if yGain != 1.0 {
		edge := 1.0 / yGain
		topLabel = fmt.Sprintf("+%.2g", edge)
		botLabel = fmt.Sprintf("-%.2g", edge)
	}
	DrawTextColorAtScale(dst, topLabel, fullRect.Min.X+2, halfRect.Min.Y+1, colTextPrimary, captionScale)
	DrawTextColorAtScale(dst, "0", fullRect.Min.X+4, midY-lh/2, colTextPrimary, captionScale)
	DrawTextColorAtScale(dst, botLabel, fullRect.Min.X+2, halfRect.Max.Y-lh-1, colTextPrimary, captionScale)

	// Waveform trace with fill (each half is dedicated, no overlap concern).
	if tap.Active && len(tap.Samples) > 0 {
		var fillCol color.Color
		if nrgba, ok := col.(color.NRGBA); ok {
			// 18 matches the per-trace fill alpha used by colScopeAFill /
			// colScopeBFill at the top of this file (scope-internal pattern).
			fillCol = WithAlphaNRGBA(nrgba, 18)
		}
		drawWaveTrace(dst, tap.Samples, halfRect, midY, w, col, yGain, fillCol)
	}
}

// drawScopeDiff renders the A-B difference waveform.
func drawScopeDiff(dst *ebiten.Image, fullRect, waveRect image.Rectangle, state *scope.State, windowMs float64, captionScale float64, lh int, yGain float64) {
	midY := waveRect.Min.Y + waveRect.Dy()/2
	w := waveRect.Dx()
	h := waveRect.Dy()

	drawScopeGrid(dst, waveRect, w, h)

	// Trigger marker.
	trigX := waveRect.Min.X + w/10
	drawRect(dst, image.Rect(trigX, waveRect.Min.Y, trigX+1, waveRect.Max.Y), colScopeTrigger, true)

	// Amplitude labels.
	topLabel, botLabel := "+1", "-1"
	if yGain != 1.0 {
		edge := 1.0 / yGain
		topLabel = fmt.Sprintf("+%.2g", edge)
		botLabel = fmt.Sprintf("-%.2g", edge)
	}
	DrawTextColorAtScale(dst, topLabel, fullRect.Min.X+2, waveRect.Min.Y+2, colTextPrimary, captionScale)
	DrawTextColorAtScale(dst, "0", fullRect.Min.X+4, midY-lh/2, colTextPrimary, captionScale)
	DrawTextColorAtScale(dst, botLabel, fullRect.Min.X+2, waveRect.Max.Y-lh-2, colTextPrimary, captionScale)

	// Compute difference waveform.
	diff := chainDiffSamples(state.TapA.Samples, state.TapB.Samples)
	if len(diff) > 0 {
		drawWaveTrace(dst, diff, waveRect, midY, w, colScopeDiff, yGain, colScopeDiffFill)
	}

	// Legend.
	legendY := waveRect.Min.Y + 2
	legendX := waveRect.Max.X - 4
	text := fmt.Sprintf("A-B: %s - %s",
		scope.StageLabel(state.TapA.Stage),
		scope.StageLabel(state.TapB.Stage))
	// Compute diff peak/RMS.
	var peak, sumSq float64
	for _, s := range diff {
		abs := s
		if abs < 0 {
			abs = -abs
		}
		if abs > peak {
			peak = abs
		}
		sumSq += s * s
	}
	rms := 0.0
	if len(diff) > 0 {
		rms = math.Sqrt(sumSq / float64(len(diff)))
	}
	peakDB := clampDBDisplay(dBFromLinear(peak))
	rmsDB := clampDBDisplay(dBFromLinear(rms))
	text += fmt.Sprintf("  Pk: %.1f dB  RMS: %.1f dB", peakDB, rmsDB)

	tw := int(float64(TextWidth(text)) * captionScale)
	bgRect := image.Rect(legendX-tw-10, legendY-1, legendX+2, legendY+lh+1)
	drawRect(dst, bgRect, WithAlpha(genColorVizScopeBg, 180), true)
	swatchY := legendY + lh/2 - 1
	drawRect(dst, image.Rect(legendX-tw-8, swatchY, legendX-tw-2, swatchY+2), colScopeDiff, true)
	DrawTextColorAtScale(dst, text, legendX-tw, legendY, colScopeDiff, captionScale)

	drawScopeBorder(dst, waveRect)
}

// chainDiffSamples computes element-wise A-B from two sample slices.
func chainDiffSamples(a, b []float64) []float64 {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	if n == 0 {
		return nil
	}
	diff := make([]float64, n)
	for i := 0; i < n; i++ {
		diff[i] = a[i] - b[i]
	}
	return diff
}

// drawScopeTapLegend draws the A/B label with peak/RMS inside a half-rect (split mode).
func drawScopeTapLegend(dst *ebiten.Image, halfRect image.Rectangle, tap *scope.TapData, col color.Color, prefix string, captionScale float64, lh int) {
	if !tap.Active {
		return
	}
	text := fmt.Sprintf("%s: %s  Pk: %.1f dB  RMS: %.1f dB",
		prefix,
		scope.StageLabel(tap.Stage),
		clampDBDisplay(tap.PeakDB),
		clampDBDisplay(tap.RMSDB))
	tw := int(float64(TextWidth(text)) * captionScale)
	x := halfRect.Max.X - tw - 4
	y := halfRect.Min.Y + 2
	bgRect := image.Rect(x-4, y-1, halfRect.Max.X-2, y+lh+1)
	drawRect(dst, bgRect, WithAlpha(genColorVizScopeBg, 180), true)
	DrawTextColorAtScale(dst, text, x, y, col, captionScale)
}

// drawScopeGrid draws horizontal and vertical grid lines for the given area.
func drawScopeGrid(dst *ebiten.Image, r image.Rectangle, w, h int) {
	for _, frac := range []float64{0.25, 0.50, 0.75} {
		yy := r.Min.Y + int(frac*float64(h))
		if frac == 0.50 {
			drawRect(dst, image.Rect(r.Min.X, yy, r.Max.X, yy+1), colScopeGridMid, true)
		} else {
			for x := r.Min.X; x < r.Max.X; x += 6 {
				endX := x + 3
				if endX > r.Max.X {
					endX = r.Max.X
				}
				drawRect(dst, image.Rect(x, yy, endX, yy+1), colScopeGrid, true)
			}
		}
	}
	for _, frac := range []float64{0.20, 0.40, 0.60, 0.80} {
		xx := r.Min.X + int(frac*float64(w))
		for y := r.Min.Y; y < r.Max.Y; y += 6 {
			endY := y + 3
			if endY > r.Max.Y {
				endY = r.Max.Y
			}
			drawRect(dst, image.Rect(xx, y, xx+1, endY), colScopeGrid, true)
		}
	}
}

// drawScopeBorder draws a 1px outline around the waveform area.
func drawScopeBorder(dst *ebiten.Image, r image.Rectangle) {
	drawRect(dst, image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+1), colButtonBorder, true)
	drawRect(dst, image.Rect(r.Min.X, r.Max.Y-1, r.Max.X, r.Max.Y), colButtonBorder, true)
	drawRect(dst, image.Rect(r.Min.X, r.Min.Y, r.Min.X+1, r.Max.Y), colButtonBorder, true)
	drawRect(dst, image.Rect(r.Max.X-1, r.Min.Y, r.Max.X, r.Max.Y), colButtonBorder, true)
}

// chainPeakAmplitude returns the maximum absolute sample value across both taps.
func chainPeakAmplitude(state *scope.State) float64 {
	var peak float64
	for _, tap := range []*scope.TapData{&state.TapA, &state.TapB} {
		if !tap.Active {
			continue
		}
		for _, s := range tap.Samples {
			if s < 0 {
				s = -s
			}
			if s > peak {
				peak = s
			}
		}
	}
	return peak
}

