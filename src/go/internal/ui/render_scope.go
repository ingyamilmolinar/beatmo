package ui

import (
	"fmt"
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	scope "github.com/ingyamilmolinar/beatmo/internal/scope"
)

// Scope trace colors for A/B comparison.
var (
	colScopeA = color.RGBA{68, 136, 255, 255}  // blue
	colScopeB = color.RGBA{255, 136, 68, 255}  // orange
)

// Scope background and grid colors.
var (
	colScopeBg      = color.RGBA{10, 10, 22, 255}
	colScopeGrid    = color.NRGBA{255, 255, 255, 20}
	colScopeGridMid = color.NRGBA{255, 255, 255, 35}
	colScopeTrigger = color.RGBA{200, 200, 80, 255}
)

const (
	scopeLeftMargin   = 32
	scopeBottomMargin = 14
)

// drawScopeTraces renders overlaid A/B waveform traces with grid, labels,
// trigger marker, and peak/RMS legend.
func drawScopeTraces(dst *ebiten.Image, rect image.Rectangle, state *scope.State, windowMs float64) {
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

	midY := waveRect.Min.Y + waveRect.Dy()/2
	w := waveRect.Dx()
	h := waveRect.Dy()

	captionScale := FontSizeCaption / FontSizeBody
	lh := int(float64(TextHeight()) * captionScale)

	// --- Grid lines ---

	// Horizontal: 25%, 50%, 75%.
	for _, frac := range []float64{0.25, 0.50, 0.75} {
		yy := waveRect.Min.Y + int(frac*float64(h))
		if frac == 0.50 {
			// Solid midline.
			drawRect(dst, image.Rect(waveRect.Min.X, yy, waveRect.Max.X, yy+1), colScopeGridMid, true)
		} else {
			// Dashed.
			for x := waveRect.Min.X; x < waveRect.Max.X; x += 6 {
				endX := x + 3
				if endX > waveRect.Max.X {
					endX = waveRect.Max.X
				}
				drawRect(dst, image.Rect(x, yy, endX, yy+1), colScopeGrid, true)
			}
		}
	}

	// Vertical: 20%, 40%, 60%, 80% (dashed).
	for _, frac := range []float64{0.20, 0.40, 0.60, 0.80} {
		xx := waveRect.Min.X + int(frac*float64(w))
		for y := waveRect.Min.Y; y < waveRect.Max.Y; y += 6 {
			endY := y + 3
			if endY > waveRect.Max.Y {
				endY = waveRect.Max.Y
			}
			drawRect(dst, image.Rect(xx, y, xx+1, endY), colScopeGrid, true)
		}
	}

	// --- Trigger marker at 10% from left ---
	trigX := waveRect.Min.X + w/10
	drawRect(dst, image.Rect(trigX, waveRect.Min.Y, trigX+1, waveRect.Max.Y), colScopeTrigger, true)

	// --- Amplitude labels (left margin) ---
	DrawTextColorAtScale(dst, "+1", rect.Min.X+2, waveRect.Min.Y+2, colTextSecondary, captionScale)
	DrawTextColorAtScale(dst, "0", rect.Min.X+4, midY-lh/2, colTextSecondary, captionScale)
	DrawTextColorAtScale(dst, "-1", rect.Min.X+2, waveRect.Max.Y-lh-2, colTextSecondary, captionScale)

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

	// --- Draw traces ---
	if state == nil {
		// No data; draw border and return.
		drawScopeBorder(dst, waveRect)
		return
	}

	if state.TapA.Active && len(state.TapA.Samples) > 0 {
		drawWaveTrace(dst, state.TapA.Samples, waveRect, midY, w, colScopeA)
	}
	if state.TapB.Active && len(state.TapB.Samples) > 0 {
		drawWaveTrace(dst, state.TapB.Samples, waveRect, midY, w, colScopeB)
	}

	// --- Legend (right-aligned, peak/RMS) ---
	legendY := waveRect.Min.Y + 2
	legendX := waveRect.Max.X - 4

	if state.TapA.Active {
		textA := fmt.Sprintf("A:%s Pk:%.1fdB RMS:%.1fdB",
			scope.StageLabel(state.TapA.Stage),
			clampDBDisplay(state.TapA.PeakDB),
			clampDBDisplay(state.TapA.RMSDB))
		tw := int(float64(TextWidth(textA)) * captionScale)
		DrawTextColorAtScale(dst, textA, legendX-tw, legendY, colScopeA, captionScale)
		legendY += lh + 1
	}
	if state.TapB.Active {
		textB := fmt.Sprintf("B:%s Pk:%.1fdB RMS:%.1fdB",
			scope.StageLabel(state.TapB.Stage),
			clampDBDisplay(state.TapB.PeakDB),
			clampDBDisplay(state.TapB.RMSDB))
		tw := int(float64(TextWidth(textB)) * captionScale)
		DrawTextColorAtScale(dst, textB, legendX-tw, legendY, colScopeB, captionScale)
	}

	// --- Border ---
	drawScopeBorder(dst, waveRect)
}

// drawScopeBorder draws a 1px outline around the waveform area.
func drawScopeBorder(dst *ebiten.Image, r image.Rectangle) {
	drawRect(dst, image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+1), colButtonBorder, true)
	drawRect(dst, image.Rect(r.Min.X, r.Max.Y-1, r.Max.X, r.Max.Y), colButtonBorder, true)
	drawRect(dst, image.Rect(r.Min.X, r.Min.Y, r.Min.X+1, r.Max.Y), colButtonBorder, true)
	drawRect(dst, image.Rect(r.Max.X-1, r.Min.Y, r.Max.X, r.Max.Y), colButtonBorder, true)
}

// clampDBDisplay clamps -Inf dB to -96 for display.
func clampDBDisplay(db float64) float64 {
	if math.IsInf(db, -1) {
		return -96.0
	}
	return db
}

// formatWindowMs formats a time value for scope axis labels.
func formatWindowMs(ms float64) string {
	if ms >= 100 {
		return fmt.Sprintf("%.0fms", ms)
	}
	if ms >= 10 {
		return fmt.Sprintf("%.0fms", ms)
	}
	if ms >= 1 {
		return fmt.Sprintf("%.1fms", ms)
	}
	return fmt.Sprintf("%.2fms", ms)
}
