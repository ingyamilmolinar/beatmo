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
	colScopeA = WithAlpha(genColorVizScopeTraceA, 180)
	colScopeB = WithAlpha(genColorVizScopeTraceB, 150)
	// Synthwave "outrun" fills beneath the A/B scope traces — raised from
	// the old 18/15 hints to AlphaSubtle so the wash from trace to the
	// zero-line reads as the defining sunset move, while the trace stays a
	// crisp line on top. Per-lane color so split/overlay keep their tint.
	colScopeAFill = WithAlpha(genColorVizScopeTraceA, AlphaSubtle)
	colScopeBFill = WithAlpha(genColorVizScopeTraceB, AlphaSubtle)
)

var (
	colScopeDiff     = WithAlpha(genColorVizScopeTraceDiff, 200)
	colScopeDiffFill = WithAlpha(genColorVizScopeTraceDiff, AlphaSubtle)
)

var (
	colScopeBg      = genColorVizScopeBg
	colScopeGrid    = WithAlpha(genColorBorder, 20)
	colScopeGridMid = WithAlpha(genColorBorder, 35)
	colScopeTrigger = genColorVizScopeTrigger
)

// colScopeFrozenBorder marks the frozen waveform with the azure accent at
// AlphaMedium. The old form reused destructive-confirm-border (#FF5050), a
// red — which violated "three reds, three roles" (error text / mute fill /
// destructive fill). Freeze is neither destructive nor an error, so it uses
// the chrome accent instead (bug #6). The FROZEN caption still names the state.
var colScopeFrozenBorder = WithAlpha(genColorPrimary, AlphaMedium)

// chainAxisDecimals picks ONE decimal precision for the entire time axis from
// the window's full scale, so every tick on the axis is formatted identically
// (bug #4a: a single row mixed "0.00ms·4.0ms·12ms"). The choice is driven by
// the largest value on the axis (windowMs), not per-tick magnitude.
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

// chainAxisLabels builds the numLabels evenly-spaced time-axis ticks for the
// given window, each with its [0,1] fraction along the plot width and a label
// formatted at the single axis-wide precision (see chainAxisDecimals).
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

// chainPlaceAxisLabels resolves the on-screen X for each axis label inside the
// plot and decimates overprinting ticks. Returns one X per label; a value < 0
// means "skip this label". Fixes:
//   - (4b) the rightmost label is right-anchored inside the plot's right edge
//     instead of running off it.
//   - (4c) the first label is clamped to the plot's left edge instead of
//     poking left of the border.
//   - (4d) a tick is skipped when the previously-kept label's right edge + a
//     small gap would overlap this label's left edge (narrow/mobile widths).
func chainPlaceAxisLabels(labels []WaveAxisLabel, waveRect image.Rectangle, w int, labelScale float64) []int {
	const gap = 4
	xs := make([]int, len(labels))
	last := len(labels) - 1
	prevRight := waveRect.Min.X - gap - 1 // allow the first label flush-left
	lastKept := -1                        // index of the most recently placed label
	for i, lbl := range labels {
		tw := int(float64(TextWidth(lbl.Text)) * labelScale)
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
			// The endpoint label always wins: if it collides with the most
			// recently kept middle tick, drop that middle tick rather than the
			// endpoint, so the axis still ends with the window max (4b/4d).
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

// chainTraceRects splits a content rect into the waveform area and the
// reserved legend strip below it, using density-aware margins. Layout
// top-to-bottom: waveform | legend strip | time-axis labels (bottom margin).
// Both drawChainTraces and the zone's hit-area builder call this so the
// legend swatches land exactly where the legend draws.
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

// drawChainTraces renders overlaid or split A/B waveform traces with grid,
// labels, trigger marker, and a peak/RMS legend in a reserved strip below
// the waveform (never over it).
// yGain scales the amplitude display (1.0 = normal, >1 = zoomed in).
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

	// Density-aware text scales replace the old hardcoded
	// FontSizeCaption/FontSizeBody (~0.71) so Chain text grows on mobile
	// (Spacious). labelScale = axis/amplitude/FROZEN; readoutScale = the
	// Pk/RMS legend (slightly smaller so the denser readout still fits).
	labelScale := chainLabelScale()
	readoutScale := chainReadoutScale()
	lh := int(float64(TextHeight()) * labelScale)
	readoutLh := int(float64(TextHeight()) * readoutScale)
	w := waveRect.Dx()

	// --- Time axis labels (bottom margin) ---
	ly := legendStrip.Max.Y + 2 // below the legend strip, in the bottom margin
	labels := chainAxisLabels(windowMs, 6)
	xs := chainPlaceAxisLabels(labels, waveRect, w, labelScale)
	for i, lbl := range labels {
		if xs[i] < 0 {
			continue // decimated to avoid overprinting the previous label
		}
		DrawTextColorAtScale(dst, lbl.Text, xs[i], ly, colTextSecondary, labelScale)
	}

	switch {
	case displayMode == chainSplit && state != nil && state.TapA.Active && state.TapB.Active:
		// --- Split mode: A in top half, B in bottom half ---
		sepY := waveRect.Min.Y + waveRect.Dy()/2
		topRect := image.Rect(waveRect.Min.X, waveRect.Min.Y, waveRect.Max.X, sepY)
		botRect := image.Rect(waveRect.Min.X, sepY+1, waveRect.Max.X, waveRect.Max.Y)

		if showA {
			drawScopeHalf(dst, rect, topRect, &state.TapA, colScopeA, windowMs, labelScale, lh, yGain)
		}
		if showB {
			drawScopeHalf(dst, rect, botRect, &state.TapB, colScopeB, windowMs, labelScale, lh, yGain)
		}

		// Separator line.
		drawRect(dst, image.Rect(waveRect.Min.X, sepY, waveRect.Max.X, sepY+1), colScopeGridMid, true)

		drawScopeBorder(dst, waveRect)

	case displayMode == chainDiff && state != nil && state.TapA.Active && state.TapB.Active && showA && showB:
		// --- Difference mode: render A-B (requires both traces visible) ---
		drawScopeDiff(dst, rect, waveRect, state, windowMs, labelScale, lh, readoutScale, readoutLh, yGain)

	default:
		// --- Overlay mode ---
		drawChainOverlay(dst, rect, waveRect, state, windowMs, labelScale, lh, yGain, showA, showB)
	}

	// Phase 3 audio-panel redesign: trigger marker at 10 % of the
	// trace area. Reads as a beat-anchor reference so the user can
	// correlate "the moment of the hit" with the rendered waveform.
	drawChainTriggerMarker(dst, waveRect)

	// Legend in its reserved strip below the waveform (never over it).
	drawChainLegend(dst, legendStrip, state, displayMode, showA, showB, readoutScale, readoutLh)

	// --- Frozen indicator ---
	if frozen {
		frozenText := "FROZEN"
		ftw := int(float64(TextWidth(frozenText)) * labelScale)
		cx := waveRect.Min.X + (waveRect.Dx()-ftw)/2
		DrawTextColorAtScale(dst, frozenText, cx, waveRect.Min.Y+2, colAccentBright, labelScale)
		// Azure (accent @ AlphaMedium) border to indicate frozen state — no
		// red, per "three reds, three roles" (bug #6).
		drawRect(dst, image.Rect(waveRect.Min.X, waveRect.Min.Y, waveRect.Max.X, waveRect.Min.Y+1), colScopeFrozenBorder, true)
		drawRect(dst, image.Rect(waveRect.Min.X, waveRect.Max.Y-1, waveRect.Max.X, waveRect.Max.Y), colScopeFrozenBorder, true)
		drawRect(dst, image.Rect(waveRect.Min.X, waveRect.Min.Y, waveRect.Min.X+1, waveRect.Max.Y), colScopeFrozenBorder, true)
		drawRect(dst, image.Rect(waveRect.Max.X-1, waveRect.Min.Y, waveRect.Max.X, waveRect.Max.Y), colScopeFrozenBorder, true)
	}
}

// drawChainOverlay renders both traces overlaid on the same waveRect. The
// Pk/RMS legend now lives in a reserved strip (drawChainLegend); this only
// paints the grid, trigger marker, amplitude labels, and traces.
func drawChainOverlay(dst *ebiten.Image, fullRect, waveRect image.Rectangle, state *scope.State, windowMs float64, labelScale float64, lh int, yGain float64, showA, showB bool) {
	midY := waveRect.Min.Y + waveRect.Dy()/2
	w := waveRect.Dx()
	h := waveRect.Dy()

	// --- Grid lines ---
	drawScopeGrid(dst, waveRect, w, h)

	// --- Trigger marker at 10% from left ---
	trigX := waveRect.Min.X + w/10
	drawRect(dst, image.Rect(trigX, waveRect.Min.Y, trigX+1, waveRect.Max.Y), colScopeTrigger, true)

	// --- Amplitude labels, right-aligned in the (density-aware) left gutter ---
	topLabel, botLabel := "+1", "-1"
	if yGain != 1.0 {
		edge := 1.0 / yGain
		topLabel = fmt.Sprintf("+%.2g", edge)
		botLabel = fmt.Sprintf("-%.2g", edge)
	}
	drawChainAmplitudeLabels(dst, fullRect, waveRect, midY, lh, labelScale, topLabel, botLabel)

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

	drawScopeBorder(dst, waveRect)
}

// drawChainAmplitudeLabels right-aligns the +1/0/-1 amplitude labels inside
// the left gutter (fullRect.Min.X .. waveRect.Min.X) so the enlarged
// density text never butts against the waveform or the stage column.
func drawChainAmplitudeLabels(dst *ebiten.Image, fullRect, waveRect image.Rectangle, midY, lh int, labelScale float64, topLabel, botLabel string) {
	rightAlign := func(label string, y int) {
		tw := int(float64(TextWidth(label)) * labelScale)
		x := waveRect.Min.X - tw - 2
		if x < fullRect.Min.X {
			x = fullRect.Min.X
		}
		DrawTextColorAtScale(dst, label, x, y, colTextPrimary, labelScale)
	}
	rightAlign(topLabel, waveRect.Min.Y+2)
	rightAlign("0", midY-lh/2)
	rightAlign(botLabel, waveRect.Max.Y-lh-2)
}

// drawScopeHalf renders a single tap's waveform in a half-height area (split mode).
func drawScopeHalf(dst *ebiten.Image, fullRect, halfRect image.Rectangle, tap *scope.TapData, col color.Color, windowMs, labelScale float64, lh int, yGain float64) {
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
	DrawTextColorAtScale(dst, topLabel, fullRect.Min.X+2, halfRect.Min.Y+1, colTextPrimary, labelScale)
	DrawTextColorAtScale(dst, "0", fullRect.Min.X+4, midY-lh/2, colTextPrimary, labelScale)
	DrawTextColorAtScale(dst, botLabel, fullRect.Min.X+2, halfRect.Max.Y-lh-1, colTextPrimary, labelScale)

	// Waveform trace with the synthwave "outrun" fill. Use the canonical
	// per-lane fill color (matched to col) so the two-band gradient fade
	// kicks in (drawWaveTrace → traceFillFaint); each half is dedicated, so
	// there's no overlap concern.
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

// drawScopeDiff renders the A-B difference waveform. The legend is drawn
// separately in the reserved strip (drawChainLegend).
func drawScopeDiff(dst *ebiten.Image, fullRect, waveRect image.Rectangle, state *scope.State, windowMs float64, labelScale float64, lh int, readoutScale float64, readoutLh int, yGain float64) {
	midY := waveRect.Min.Y + waveRect.Dy()/2
	w := waveRect.Dx()
	h := waveRect.Dy()

	drawScopeGrid(dst, waveRect, w, h)

	// Trigger marker.
	trigX := waveRect.Min.X + w/10
	drawRect(dst, image.Rect(trigX, waveRect.Min.Y, trigX+1, waveRect.Max.Y), colScopeTrigger, true)

	// Amplitude labels (right-aligned in the gutter).
	topLabel, botLabel := "+1", "-1"
	if yGain != 1.0 {
		edge := 1.0 / yGain
		topLabel = fmt.Sprintf("+%.2g", edge)
		botLabel = fmt.Sprintf("-%.2g", edge)
	}
	drawChainAmplitudeLabels(dst, fullRect, waveRect, midY, lh, labelScale, topLabel, botLabel)

	// Compute difference waveform.
	diff := chainDiffSamples(state.TapA.Samples, state.TapB.Samples)
	if len(diff) > 0 {
		drawWaveTrace(dst, diff, waveRect, midY, w, colScopeDiff, yGain, colScopeDiffFill)
	}

	drawScopeBorder(dst, waveRect)
}

// chainDiffStats returns the peak and RMS (linear) of the element-wise A-B
// difference without allocating. Used by the diff-mode legend.
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

// chainLegendHalves splits the reserved legend strip into the A (left) and B
// (right) clickable segments. In diff mode the whole strip is the A-B readout
// (no per-trace toggle) so segB is empty. Shared by drawChainLegend and the
// zone's swatch hit areas so clicks land on the right readout.
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

// drawChainLegend paints the Pk/RMS readout into the reserved strip below the
// waveform — never over it. Overlay/split: A in the left half, B in the right
// half, each prefixed by a color swatch; a hidden trace is dimmed. Diff: one
// A-B readout spanning the strip.
func drawChainLegend(dst *ebiten.Image, strip image.Rectangle, state *scope.State, displayMode chainDisplayMode, showA, showB bool, readoutScale float64, readoutLh int) {
	if state == nil || strip.Dx() <= 0 || strip.Dy() <= 0 {
		return
	}
	textY := strip.Min.Y + (strip.Dy()-readoutLh)/2
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
		DrawTextColorAtScale(dst, text, sx+swatchW+4, textY, col, readoutScale)
	}

	// fit picks a full or compact readout depending on the segment width so
	// the larger density text doesn't overrun into the neighbour segment.
	fit := func(seg image.Rectangle, full, compact string) string {
		avail := seg.Dx() - swatchW - 8
		if int(float64(TextWidth(full))*readoutScale) <= avail {
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

// chainDiffScratch is grow-only reusable storage for the A-B difference
// waveform so diff mode doesn't heap-allocate per frame (alloc budget gate:
// TabScope cap). The Chain panel renders on the single UI goroutine, so a
// package-level scratch is safe here.
var chainDiffScratch []float64

// chainDiffSamples computes element-wise A-B from two sample slices, reusing
// chainDiffScratch. The returned slice is valid until the next call.
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

// Auto-fit tuning: a sample counts as "active" once it reaches
// chainFitThreshold of the buffer peak; the framed span is padded by
// chainFitPad on each side so the transient isn't flush against the edge.
const (
	chainFitThreshold = 0.04
	chainFitPad       = 0.20
)

// chainFitSpan returns the union active-span [start,end) across the visible
// taps, in sample indices. Returns (0,0) when no visible tap has signal, so
// the caller leaves the full buffer untouched.
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

// chainSliceSpan returns s[start:end] clamped to s's bounds (alloc-free).
// Out-of-range or inverted spans yield an empty slice.
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
