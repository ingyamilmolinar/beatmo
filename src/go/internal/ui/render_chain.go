package ui

import (
	"fmt"
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	scope "github.com/ingyamilmolinar/beatmo/internal/scope"
)

// The trace alphas are scope-internal and stay numeric; only the base hex is a token. Derived in
// rebuildThemeAliases so a runtime theme re-tints them.
var (
	colScopeA color.NRGBA
	colScopeB color.NRGBA
	// The fill under each trace is AlphaSubtle so the wash to the zero line reads while the trace stays a crisp
	// line; per lane, so split and overlay keep their tint.
	colScopeAFill color.NRGBA
	colScopeBFill color.NRGBA
)

var (
	colScopeDiff     color.NRGBA
	colScopeDiffFill color.NRGBA
)

var (
	colScopeBg      color.RGBA
	colScopeGrid    color.NRGBA
	colScopeGridMid color.NRGBA
)

// Accent, not red: freeze is neither destructive nor an error ("three reds, three roles").
var colScopeFrozenBorder color.NRGBA // derived in rebuildThemeAliases (theme.go)

// ONE precision for the whole axis, from the full scale, so every tick reads alike (never "0.00ms·4.0ms·12ms").
func chainAxisDecimals(windowMs float64) int {
	switch {
	case windowMs >= 10:
		return 0
	case windowMs >= 1:
		return 1
	default:
		return 2
	}
}

func chainAxisLabels(windowMs float64, numLabels int) []WaveAxisLabel {
	if numLabels < 2 {
		numLabels = 2
	}
	dec := chainAxisDecimals(windowMs)
	out := make([]WaveAxisLabel, numLabels)
	for i := 0; i < numLabels; i++ {
		frac := float64(i) / float64(numLabels-1)
		ms := frac * windowMs
		out[i] = WaveAxisLabel{Frac: frac, Text: fmt.Sprintf("%.*fms", dec, ms)}
	}
	return out
}

// Returns one X per label, < 0 = skip. The first label clamps to the left edge, the last right-anchors
// inside the plot, and a tick that would overprint the last kept one is dropped.
func chainPlaceAxisLabels(labels []WaveAxisLabel, waveRect image.Rectangle, w int) []int {
	const gap = 4
	xs := make([]int, len(labels))
	last := len(labels) - 1
	prevRight := waveRect.Min.X - gap - 1 // allow the first label flush-left
	lastKept := -1                        // index of the most recently placed label
	for i, lbl := range labels {
		tw := StyledTextWidth(lbl.Text, RoleMicro)
		center := waveRect.Min.X + int(lbl.Frac*float64(w))
		x := center - tw/2
		switch i {
		case 0:
			// First tick: clamp to the left border (4c).
			if x < waveRect.Min.X {
				x = waveRect.Min.X
			}
		case last:
			// Last tick: right-anchor inside the right border (4b).
			if x+tw > waveRect.Max.X {
				x = waveRect.Max.X - tw
			}
		default:
			if x < waveRect.Min.X {
				x = waveRect.Min.X
			}
			if x+tw > waveRect.Max.X {
				x = waveRect.Max.X - tw
			}
		}

		overlaps := x < prevRight+gap && lastKept >= 0
		if i == last {
			// The endpoint wins a collision, so the axis still ends with the window max.
			if overlaps && lastKept != 0 {
				xs[lastKept] = -1
			}
			xs[i] = x
			continue
		}
		// Middle/first ticks: decimate on overlap, but never drop the first.
		if i != 0 && overlaps {
			xs[i] = -1
			continue
		}
		xs[i] = x
		prevRight = x + tw
		lastKept = i
	}
	return xs
}

// Shared by the draw and the hit-area builder so the legend swatches land where the legend draws.
func chainTraceRects(rect image.Rectangle) (waveRect, legendStrip image.Rectangle) {
	dv := Profile().DensityValues()
	leftMargin := dv.ChainScopeLeftMargin
	bottomMargin := dv.ChainScopeBottomMargin
	legendStripH := dv.ChainLegendStripH
	waveBottom := rect.Max.Y - bottomMargin - legendStripH
	waveRect = image.Rect(rect.Min.X+leftMargin, rect.Min.Y, rect.Max.X, waveBottom)
	legendStrip = image.Rect(rect.Min.X+leftMargin, waveBottom, rect.Max.X, waveBottom+legendStripH)
	return waveRect, legendStrip
}

// The legend has its own strip below the waveform, never over it. yGain > 1 zooms in.
func drawChainTraces(dst *ebiten.Image, rect image.Rectangle, state *scope.State, windowMs float64, displayMode chainDisplayMode, frozen bool, yGain float64, showA, showB bool) {
	if rect.Dx() < 20 || rect.Dy() < 20 {
		return
	}

	// Dark background.
	drawRect(dst, rect, colScopeBg, true)

	waveRect, legendStrip := chainTraceRects(rect)
	if waveRect.Dx() <= 0 || waveRect.Dy() <= 0 {
		return
	}

	lh := StyledTextHeight(RoleMicro)
	w := waveRect.Dx()

	// --- Time axis labels (bottom margin) ---
	ly := legendStrip.Max.Y + 2 // below the legend strip, in the bottom margin
	labels := chainAxisLabels(windowMs, 6)
	xs := chainPlaceAxisLabels(labels, waveRect, w)
	for i, lbl := range labels {
		if xs[i] < 0 {
			continue // decimated to avoid overprinting the previous label
		}
		DrawTextStyled(dst, lbl.Text, xs[i], ly, RoleMicro, colTextSecondary)
	}

	switch {
	case displayMode == chainSplit && state != nil && state.TapA.Active && state.TapB.Active:
		// --- Split mode: A in top half, B in bottom half ---
		sepY := waveRect.Min.Y + waveRect.Dy()/2
		topRect := image.Rect(waveRect.Min.X, waveRect.Min.Y, waveRect.Max.X, sepY)
		botRect := image.Rect(waveRect.Min.X, sepY+1, waveRect.Max.X, waveRect.Max.Y)

		if showA {
			drawScopeHalf(dst, rect, topRect, &state.TapA, colScopeA, windowMs, yGain)
		}
		if showB {
			drawScopeHalf(dst, rect, botRect, &state.TapB, colScopeB, windowMs, yGain)
		}

		// Separator line.
		drawRect(dst, image.Rect(waveRect.Min.X, sepY, waveRect.Max.X, sepY+1), colScopeGridMid, true)

		drawScopeBorder(dst, waveRect)

	case displayMode == chainDiff && state != nil && state.TapA.Active && state.TapB.Active && showA && showB:
		// --- Difference mode: render A-B (requires both traces visible) ---
		drawScopeDiff(dst, rect, waveRect, state, windowMs, lh, yGain)

	default:
		// --- Overlay mode ---
		drawChainOverlay(dst, rect, waveRect, state, windowMs, lh, yGain, showA, showB)
	}

	// Legend in its reserved strip below the waveform (never over it).
	drawChainLegend(dst, legendStrip, state, displayMode, showA, showB)

	// --- Frozen indicator ---
	if frozen {
		frozenText := "FROZEN"
		ftw := StyledTextWidth(frozenText, RoleCaption)
		cx := waveRect.Min.X + (waveRect.Dx()-ftw)/2
		DrawTextStyled(dst, frozenText, cx, waveRect.Min.Y+2, RoleCaption, colAccentBright)
		// Accent, not red ("three reds, three roles").
		drawRect(dst, image.Rect(waveRect.Min.X, waveRect.Min.Y, waveRect.Max.X, waveRect.Min.Y+1), colScopeFrozenBorder, true)
		drawRect(dst, image.Rect(waveRect.Min.X, waveRect.Max.Y-1, waveRect.Max.X, waveRect.Max.Y), colScopeFrozenBorder, true)
		drawRect(dst, image.Rect(waveRect.Min.X, waveRect.Min.Y, waveRect.Min.X+1, waveRect.Max.Y), colScopeFrozenBorder, true)
		drawRect(dst, image.Rect(waveRect.Max.X-1, waveRect.Min.Y, waveRect.Max.X, waveRect.Max.Y), colScopeFrozenBorder, true)
	}
}

func drawChainOverlay(dst *ebiten.Image, fullRect, waveRect image.Rectangle, state *scope.State, windowMs float64, lh int, yGain float64, showA, showB bool) {
	midY := waveRect.Min.Y + waveRect.Dy()/2
	w := waveRect.Dx()
	h := waveRect.Dy()

	// --- Grid lines ---
	drawScopeGrid(dst, waveRect, w, h)

	// --- Amplitude labels, right-aligned in the (density-aware) left gutter ---
	topLabel, botLabel := "+1", "-1"
	if yGain != 1.0 {
		edge := 1.0 / yGain
		topLabel = fmt.Sprintf("+%.2g", edge)
		botLabel = fmt.Sprintf("-%.2g", edge)
	}
	drawChainAmplitudeLabels(dst, fullRect, waveRect, midY, lh, topLabel, botLabel)

	// --- Draw traces ---
	if state == nil {
		drawScopeBorder(dst, waveRect)
		return
	}

	// B first, then A on top, so the "before" signal is always visible.
	if showB && state.TapB.Active && len(state.TapB.Samples) > 0 {
		drawWaveTrace(dst, state.TapB.Samples, waveRect, midY, w, colScopeB, yGain, colScopeBFill)
	}
	if showA && state.TapA.Active && len(state.TapA.Samples) > 0 {
		drawWaveTrace(dst, state.TapA.Samples, waveRect, midY, w, colScopeA, yGain, colScopeAFill)
	}

	drawScopeBorder(dst, waveRect)
}

// Right-aligned in the left gutter so the text never butts against the waveform or the stage column.
func drawChainAmplitudeLabels(dst *ebiten.Image, fullRect, waveRect image.Rectangle, midY, lh int, topLabel, botLabel string) {
	rightAlign := func(label string, y int) {
		tw := StyledTextWidth(label, RoleMicro)
		x := waveRect.Min.X - tw - 2
		if x < fullRect.Min.X {
			x = fullRect.Min.X
		}
		DrawTextStyled(dst, label, x, y, RoleMicro, colTextPrimary)
	}
	rightAlign(topLabel, waveRect.Min.Y+2)
	rightAlign("0", midY-lh/2)
	rightAlign(botLabel, waveRect.Max.Y-lh-2)
}

// drawScopeHalf renders a single tap's waveform in a half-height area (split mode).
func drawScopeHalf(dst *ebiten.Image, fullRect, halfRect image.Rectangle, tap *scope.TapData, col color.Color, windowMs, yGain float64) {
	lh := StyledTextHeight(RoleMicro)
	midY := halfRect.Min.Y + halfRect.Dy()/2
	w := halfRect.Dx()
	h := halfRect.Dy()

	// Grid for this half.
	drawScopeGrid(dst, halfRect, w, h)

	// Amplitude labels.
	topLabel, botLabel := "+1", "-1"
	if yGain != 1.0 {
		edge := 1.0 / yGain
		topLabel = fmt.Sprintf("+%.2g", edge)
		botLabel = fmt.Sprintf("-%.2g", edge)
	}
	DrawTextStyled(dst, topLabel, fullRect.Min.X+2, halfRect.Min.Y+1, RoleMicro, colTextPrimary)
	DrawTextStyled(dst, "0", fullRect.Min.X+4, midY-lh/2, RoleMicro, colTextPrimary)
	DrawTextStyled(dst, botLabel, fullRect.Min.X+2, halfRect.Max.Y-lh-1, RoleMicro, colTextPrimary)

	// The per-lane fill color enables the two-band fade; each half is dedicated, so nothing overlaps.
	if tap.Active && len(tap.Samples) > 0 {
		var fillCol color.Color
		switch col {
		case color.Color(colScopeA):
			fillCol = colScopeAFill
		case color.Color(colScopeB):
			fillCol = colScopeBFill
		default:
			if nrgba, ok := col.(color.NRGBA); ok {
				fillCol = WithAlphaNRGBA(nrgba, AlphaSubtle)
			}
		}
		drawWaveTrace(dst, tap.Samples, halfRect, midY, w, col, yGain, fillCol)
	}
}

func drawScopeDiff(dst *ebiten.Image, fullRect, waveRect image.Rectangle, state *scope.State, windowMs float64, lh int, yGain float64) {
	midY := waveRect.Min.Y + waveRect.Dy()/2
	w := waveRect.Dx()
	h := waveRect.Dy()

	drawScopeGrid(dst, waveRect, w, h)

	// Amplitude labels (right-aligned in the gutter).
	topLabel, botLabel := "+1", "-1"
	if yGain != 1.0 {
		edge := 1.0 / yGain
		topLabel = fmt.Sprintf("+%.2g", edge)
		botLabel = fmt.Sprintf("-%.2g", edge)
	}
	drawChainAmplitudeLabels(dst, fullRect, waveRect, midY, lh, topLabel, botLabel)

	// Compute difference waveform.
	diff := chainDiffSamples(state.TapA.Samples, state.TapB.Samples)
	if len(diff) > 0 {
		drawWaveTrace(dst, diff, waveRect, midY, w, colScopeDiff, yGain, colScopeDiffFill)
	}

	drawScopeBorder(dst, waveRect)
}

// Without allocating.
func chainDiffStats(a, b []float64) (peak, rms float64) {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	if n == 0 {
		return 0, 0
	}
	var sumSq float64
	for i := 0; i < n; i++ {
		d := a[i] - b[i]
		abs := d
		if abs < 0 {
			abs = -abs
		}
		if abs > peak {
			peak = abs
		}
		sumSq += d * d
	}
	return peak, math.Sqrt(sumSq / float64(n))
}

// Diff mode has no per-trace toggle, so segB is empty. Shared with the swatch hit areas.
func chainLegendHalves(strip image.Rectangle, displayMode chainDisplayMode) (segA, segB image.Rectangle) {
	if strip.Dx() <= 0 || strip.Dy() <= 0 {
		return image.Rectangle{}, image.Rectangle{}
	}
	if displayMode == chainDiff {
		return strip, image.Rectangle{}
	}
	midX := strip.Min.X + strip.Dx()/2
	segA = image.Rect(strip.Min.X, strip.Min.Y, midX, strip.Max.Y)
	segB = image.Rect(midX, strip.Min.Y, strip.Max.X, strip.Max.Y)
	return segA, segB
}

// A hidden trace is dimmed; diff mode shows one A-B readout across the strip.
func drawChainLegend(dst *ebiten.Image, strip image.Rectangle, state *scope.State, displayMode chainDisplayMode, showA, showB bool) {
	if state == nil || strip.Dx() <= 0 || strip.Dy() <= 0 {
		return
	}
	textY := strip.Min.Y + (strip.Dy()-StyledTextHeight(RoleMicro))/2
	const swatchW, swatchH = 8, 3

	drawSeg := func(seg image.Rectangle, swatch color.Color, text string, dimCol color.Color, dimmed bool) {
		if seg.Dx() <= 0 || text == "" {
			return
		}
		sx := seg.Min.X + 2
		sy := strip.Min.Y + strip.Dy()/2 - swatchH/2
		drawRect(dst, image.Rect(sx, sy, sx+swatchW, sy+swatchH), swatch, true)
		col := swatch
		if dimmed {
			col = dimCol
		}
		DrawTextStyled(dst, text, sx+swatchW+4, textY, RoleMicro, col)
	}

	// A compact readout when the segment is narrow, so it never overruns its neighbour.
	fit := func(seg image.Rectangle, full, compact string) string {
		avail := seg.Dx() - swatchW - 8
		if StyledTextWidth(full, RoleMicro) <= avail {
			return full
		}
		return compact
	}

	segA, segB := chainLegendHalves(strip, displayMode)

	if displayMode == chainDiff && state.TapA.Active && state.TapB.Active {
		pk, rms := chainDiffStats(state.TapA.Samples, state.TapB.Samples)
		pkDB := clampDBDisplay(dBFromLinear(pk))
		rmsDB := clampDBDisplay(dBFromLinear(rms))
		full := fmt.Sprintf("A-B  %s - %s   Pk %.1f  RMS %.1f dB",
			scope.StageLabel(state.TapA.Stage), scope.StageLabel(state.TapB.Stage), pkDB, rmsDB)
		compact := fmt.Sprintf("A-B  Pk %.0f RMS %.0f", pkDB, rmsDB)
		drawSeg(segA, colScopeDiff, fit(segA, full, compact), colScopeDiff, false)
		return
	}

	if state.TapA.Active {
		full := fmt.Sprintf("A  %s  Pk %.1f  RMS %.1f dB",
			scope.StageLabel(state.TapA.Stage), clampDBDisplay(state.TapA.PeakDB), clampDBDisplay(state.TapA.RMSDB))
		compact := fmt.Sprintf("A %s %.0f", scope.StageLabel(state.TapA.Stage), clampDBDisplay(state.TapA.PeakDB))
		drawSeg(segA, colScopeA, fit(segA, full, compact), WithAlpha(genColorVizScopeTraceA, 60), !showA)
	}
	if state.TapB.Active {
		full := fmt.Sprintf("B  %s  Pk %.1f  RMS %.1f dB",
			scope.StageLabel(state.TapB.Stage), clampDBDisplay(state.TapB.PeakDB), clampDBDisplay(state.TapB.RMSDB))
		compact := fmt.Sprintf("B %s %.0f", scope.StageLabel(state.TapB.Stage), clampDBDisplay(state.TapB.PeakDB))
		drawSeg(segB, colScopeB, fit(segB, full, compact), WithAlpha(genColorVizScopeTraceB, 60), !showB)
	}
}

// Grow-only so diff mode does not allocate per frame; package-level is safe because the Chain panel draws
// on the UI goroutine only.
var chainDiffScratch []float64

// Valid until the next call.
func chainDiffSamples(a, b []float64) []float64 {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	if n == 0 {
		return nil
	}
	if cap(chainDiffScratch) < n {
		chainDiffScratch = make([]float64, n)
	}
	diff := chainDiffScratch[:n]
	for i := 0; i < n; i++ {
		diff[i] = a[i] - b[i]
	}
	return diff
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

// Padding keeps the transient off the edge of the framed span.
const (
	chainFitThreshold = 0.04
	chainFitPad       = 0.20
)

// (0,0) when no visible tap has signal, so the caller keeps the full buffer.
func chainFitSpan(state *scope.State, showA, showB bool) (start, end int) {
	start, end = -1, -1
	consider := func(tap *scope.TapData, show bool) {
		if !show || !tap.Active || len(tap.Samples) == 0 {
			return
		}
		s, e := scope.ActiveSpan(tap.Samples, chainFitThreshold, chainFitPad)
		if start < 0 || s < start {
			start = s
		}
		if end < 0 || e > end {
			end = e
		}
	}
	consider(&state.TapA, showA)
	consider(&state.TapB, showB)
	if start < 0 || end <= start {
		return 0, 0
	}
	return start, end
}

// Alloc-free; an out-of-range or inverted span is empty.
func chainSliceSpan(s []float64, start, end int) []float64 {
	n := len(s)
	if n == 0 {
		return s
	}
	if start < 0 {
		start = 0
	}
	if end > n {
		end = n
	}
	if start >= end {
		return s[n:n]
	}
	return s[start:end]
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
