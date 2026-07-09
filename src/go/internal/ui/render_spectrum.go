package ui

import (
	"fmt"
	"image"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// spectrumSlope expresses the post-display tilt applied to the spectrum
// curve so different reference signals (pink vs white noise) look
// horizontal. 3 dB/oct is the broadcast / mastering convention — pink
// noise plots flat. 4.5 dB/oct biases the eye toward treble (matches
// some FabFilter Pro-Q presets); 0 dB/oct shows raw magnitude.
var spectrumSlopeDBPerOct float64

// SetSpectrumSlope updates the slope tilt; safe to call from the UI
// thread. The renderer reads the value on the next Draw.
func SetSpectrumSlope(dbPerOct float64) { spectrumSlopeDBPerOct = dbPerOct }

// SpectrumSlope returns the current slope tilt in dB/octave.
func SpectrumSlope() float64 { return spectrumSlopeDBPerOct }

// isoLabels are the center-frequency labels for the 10 ISO bands.
var isoLabels = [10]string{"31", "62", "125", "250", "500", "1k", "2k", "4k", "8k", "16k"}

// bandGroups partition the 10 ISO bands into Bass / Mids / Treble groups
// that span the bottom-of-spectrum bracket strip. Indices are half-open
// [start, end) over isoBands.
//
// Bass = bands 0-3   (31 Hz – 250 Hz)
// Mids = bands 4-6   (500 Hz – 2 kHz)
// Treble = bands 7-9 (4 kHz – 16 kHz)
//
// These groupings match how musicians (not engineers) think about
// frequency content — kicks live in Bass, snares straddle Mids, hats
// live in Treble. The brackets are a kid-friendly orientation aid;
// the underlying ISO band data is unchanged.
var bandGroups = [3]struct {
	label     string   // canonical English (reference / fallback)
	key       i18n.Key // localized display label
	startBand int
	endBand   int // exclusive
}{
	{"Bass", i18n.KeySpectrumBandBass, 0, 4},
	{"Mids", i18n.KeySpectrumBandMids, 4, 7},
	{"Treble", i18n.KeySpectrumBandTreble, 7, 10},
}

// bandGroupLabelAt returns the localized display label for ISO band group i.
func bandGroupLabelAt(i int) string {
	if i < 0 || i >= len(bandGroups) {
		return ""
	}
	return i18n.T(bandGroups[i].key)
}

// noteAnchors are octave-C reference frequencies overlaid on the spectrum
// so kids can map Hz to note name (e.g., a kick living in band 0-1 is
// "below C3"). Five C-octave anchors, each in a different ISO band so the
// labels don't collide:
//
//	C3 = 130.81 Hz  (ISO band 2: 88-177 Hz)
//	C4 = 261.63 Hz  (ISO band 3: 177-355 Hz) — middle C
//	C5 = 523.25 Hz  (ISO band 4: 355-710 Hz)
//	C6 = 1046.50 Hz (ISO band 5: 710-1420 Hz)
//	C7 = 2093.00 Hz (ISO band 6: 1420-2840 Hz)
var noteAnchors = [5]struct {
	label string
	hz    float64
}{
	{"C3", 130.81},
	{"C4", 261.63},
	{"C5", 523.25},
	{"C6", 1046.50},
	{"C7", 2093.00},
}

// findISOBand returns the index of the ISO band whose [lo, hi) range
// contains hz, or -1 if outside any band.
func findISOBand(hz float64) int {
	for i, b := range isoBands {
		if hz >= b[0] && hz < b[1] {
			return i
		}
	}
	return -1
}

// SpectrumPeakState tracks peak-hold values for the 10-band spectrum display.
// Two separate tracks: Peaks/Ages is the transient ~500ms decay marker (live
// "hot spot" feedback), MaxPeaks is the never-decay watermark (call ResetMax
// to clear). DAW convention: the transient gives you motion, the watermark
// gives you "what did this signal actually peak at over the whole session."
type SpectrumPeakState struct {
	Peaks    [10]float64 // normalized 0-1 peak values (decays via Ages)
	Ages     [10]int     // frames since peak was set
	MaxAge   int         // frames before peak decays (default 30 ≈ 500ms)
	MaxPeaks [10]float64 // normalized 0-1 ALL-TIME peak values (never decay)
}

// UpdateMax raises each MaxPeak[i] to the higher of (live[i], MaxPeak[i]).
// Once raised, MaxPeak stays at that value until ResetMax is called.
func (s *SpectrumPeakState) UpdateMax(live [10]float64) {
	for i, v := range live {
		if v > s.MaxPeaks[i] {
			s.MaxPeaks[i] = v
		}
	}
}

// ResetMax clears every band's MAX watermark back to zero. Wired to a UI
// click handler so the user can ask "what's the peak from this point
// forward?" at any time.
func (s *SpectrumPeakState) ResetMax() {
	for i := range s.MaxPeaks {
		s.MaxPeaks[i] = 0
	}
}

// isoBands defines the 10 ISO frequency bands used for the spectrum display.
// Each pair is [lowHz, highHz).
var isoBands = [10][2]float64{
	{20, 44},
	{44, 88},
	{88, 177},
	{177, 355},
	{355, 710},
	{710, 1420},
	{1420, 2840},
	{2840, 5680},
	{5680, 11360},
	{11360, 22000},
}

// freqScaleMode selects the frequency-axis bucketing used by
// drawAnalyzerSpectrumWithScale. The default freqScaleLog keeps the legacy
// 10-ISO-band layout; freqScaleLinear partitions [20, 22000] Hz into 10
// equal-Hz bins so high-frequency content gets the same visual weight as
// lows. The toggle is per-session and exposed in the sticky bar.
type freqScaleMode int

const (
	freqScaleLog    freqScaleMode = iota // ISO 1/3-octave bands (default)
	freqScaleLinear                      // 10 equal-Hz bands across [20, 22000]
)

// linearBands returns the 10 equal-Hz bands used by freqScaleLinear. Each
// band spans (22000-20)/10 = 2198 Hz. Inlined as a function so the band
// boundaries stay co-located with isoBands and never drift apart.
func linearBands() [10][2]float64 {
	var out [10][2]float64
	const lo, hi = 20.0, 22000.0
	step := (hi - lo) / 10
	for i := 0; i < 10; i++ {
		out[i] = [2]float64{lo + float64(i)*step, lo + float64(i+1)*step}
	}
	return out
}

// linearLabels returns frequency labels for the linear band layout. Each
// label sits at the band center expressed in kHz to one decimal place (kept
// short so it fits inside a single bar width on mobile).
func linearLabels() [10]string {
	bands := linearBands()
	var out [10]string
	for i, b := range bands {
		center := (b[0] + b[1]) / 2
		if center >= 1000 {
			out[i] = fmt.Sprintf("%.1fk", center/1000)
		} else {
			out[i] = fmt.Sprintf("%.0f", center)
		}
	}
	return out
}

const (
	spectrumMinDB = -80.0
	spectrumMaxDB = 0.0
)

// spectrumFreqLabelMinH is the minimum height of the Hz-label row at the
// bottom of the spectrum panel (legacy 14px floor).
const spectrumFreqLabelMinH = 14

// captionTextH returns the pixel height of one caption-scale text row.
func captionTextH() int {
	return int(float64(TextHeight()) * (FontSizeCaption / FontSizeBody))
}

// spectrumLabelRows splits a spectrum panel rect into three vertically
// stacked, non-overlapping regions:
//
//	barRect    — the bar/curve plot area (top)
//	bracketRow — the Bass/Mids/Treble bracket strip (line + group labels)
//	hzRow      — the Hz tick-label row (bottom)
//
// Pure function so layout invariants are unit-testable. bracketStripH is
// the density token (SpectrumBracketH); it is raised to fit a full caption
// text row under the bracket line so the group labels can never bleed into
// the Hz row below — the pre-fix bug stamped "Mids" over "1k" and "Treble"
// over "8k" because both label sets shared one undersized strip.
func spectrumLabelRows(rect image.Rectangle, bracketStripH, captionH int) (barRect, bracketRow, hzRow image.Rectangle) {
	// Bracket row contents: 2px gap + 1px line (caps reach 2px below the
	// line top) + caption text row starting 3px below the line top.
	minBracket := captionH + 5
	if bracketStripH < minBracket {
		bracketStripH = minBracket
	}
	hzRowH := captionH + 2
	if hzRowH < spectrumFreqLabelMinH {
		hzRowH = spectrumFreqLabelMinH
	}
	barBottom := rect.Max.Y - hzRowH - bracketStripH
	barRect = image.Rect(rect.Min.X+Profile().DensityValues().AudioLabelMarginW, rect.Min.Y, rect.Max.X, barBottom)
	bracketRow = image.Rect(barRect.Min.X, barBottom, rect.Max.X, barBottom+bracketStripH)
	hzRow = image.Rect(barRect.Min.X, bracketRow.Max.Y, rect.Max.X, rect.Max.Y)
	return barRect, bracketRow, hzRow
}

// spectrumPanelRows resolves spectrumLabelRows from the live density
// profile + font metrics. Every spectrum renderer (bars, pre-EQ overlay,
// cursor) derives its plot geometry through this single helper so the
// layers always align.
func spectrumPanelRows(rect image.Rectangle) (barRect, bracketRow, hzRow image.Rectangle) {
	return spectrumLabelRows(rect, Profile().DensityValues().SpectrumBracketH, captionTextH())
}

// drawAnalyzerSpectrum renders a 10-band ISO spectrum analyzer into the given
// rectangle using the channel's FFT data. It draws frequency labels along the
// bottom, a dB scale on the left, and optional peak-hold indicators when peaks
// is non-nil.
//
// This is the legacy entry point — it forwards to drawAnalyzerSpectrumWithScale
// with freqScaleLog so older call sites keep their log-axis behavior.
func drawAnalyzerSpectrum(dst *ebiten.Image, rect image.Rectangle, ch *analyzer.ChannelMetrics, peaks *SpectrumPeakState) {
	drawAnalyzerSpectrumWithScale(dst, rect, ch, peaks, freqScaleLog)
}

// drawAnalyzerSpectrumWithScale is the scale-aware spectrum renderer. The
// scale argument picks the band table: freqScaleLog uses ISO 1/3-octave bands
// (isoBands), freqScaleLinear uses 10 equal-Hz bands. The rest of the chrome
// (dB grid, frequency labels, bracket strip, note anchors, peak hold) is
// identical between modes, scaled to the active band table.
func drawAnalyzerSpectrumWithScale(dst *ebiten.Image, rect image.Rectangle, ch *analyzer.ChannelMetrics, peaks *SpectrumPeakState, scale freqScaleMode) {
	// Resolve band table + labels for the chosen scale.
	var bands [10][2]float64
	var labels [10]string
	switch scale {
	case freqScaleLinear:
		bands = linearBands()
		labels = linearLabels()
	default:
		bands = isoBands
		labels = isoLabels
	}

	// Idle state: baseline rule + quiet hint instead of a dead panel (the
	// bare-outline empty state read as broken chrome — 2026-07-04 critique).
	if ch == nil || !ch.Active || len(ch.FFTBins) == 0 {
		drawRect(dst, rect, colButtonBorder, false)
		baseY := rect.Max.Y - SpaceLG
		drawRect(dst, image.Rect(rect.Min.X+SpaceSM, baseY, rect.Max.X-SpaceSM, baseY+1),
			WithAlpha(genColorBorder, genAlphaBorderPanel), true)
		hint := i18n.T(i18n.KeyAnalyzerIdle)
		hx := rect.Min.X + (rect.Dx()-StyledTextWidth(hint, RoleCaption))/2
		hy := rect.Min.Y + (rect.Dy()-StyledTextHeight(RoleCaption))/2
		DrawTextStyled(dst, hint, hx, hy, RoleCaption, colTextSecondary)
		return
	}

	fftBins := ch.FFTBins
	freqBins := ch.FreqBins

	// FreqBins and FFTBins must be the same length for pairing.
	if len(freqBins) != len(fftBins) {
		drawRect(dst, rect, colButtonBorder, false)
		return
	}

	// Reserve a left margin for the dB scale, plus two stacked label rows
	// below the bars: the Bass/Mids/Treble bracket strip and the Hz tick
	// labels each get their own row so they can never collide.
	barRect, bracketRow, hzRow := spectrumPanelRows(rect)

	// Group FFT bins into 10 bands and compute average dB for each.
	var bandDB [10]float64
	for b := range bands {
		lo, hi := bands[b][0], bands[b][1]
		sum := 0.0
		count := 0
		for i, hz := range freqBins {
			if hz >= lo && hz < hi {
				sum += fftBins[i]
				count++
			}
		}
		if count > 0 {
			bandDB[b] = sum / float64(count)
		} else {
			bandDB[b] = spectrumMinDB
		}
	}

	// Draw dB reference lines and labels.
	captionScale := FontSizeCaption / FontSizeBody
	dbRefs := []float64{0, -12, -24, -36, -48, -60}
	// Inset the reference lines from the panel edges so they never read
	// as a border artifact, and skip any line pinned at the top of the
	// range — the 0 dB ref at norm=1 landed flush under the sticky bar
	// and read as an error underline across the whole tab.
	lineX0 := barRect.Min.X + SpaceXS
	lineX1 := barRect.Max.X - SpaceXS
	for _, db := range dbRefs {
		norm := (db - spectrumMinDB) / (spectrumMaxDB - spectrumMinDB)
		y := barRect.Max.Y - int(norm*float64(barRect.Dy()))
		// Dashed line (3px on, 3px off). The 0 dB and -12 dB lines
		// render solid + brighter so the headroom envelope is
		// immediately visible. Solid lines are a single full-width
		// rect — the old step=1 loop issued one 1-px drawRect per
		// pixel column (~2×panel-width calls/frame), the largest
		// avoidable share of the Spectrum tab's alloc budget.
		gridCol := WithAlpha(genColorBorder, genAlphaWhiteDecoration)
		solid := false
		if db == 0 {
			gridCol = WithAlpha(genColorError, genAlphaMedium)
			solid = true
		} else if db == -12 {
			gridCol = WithAlpha(genColorBorder, genAlphaMedium)
			solid = true
		}
		pinnedAtTop := y <= barRect.Min.Y
		if !pinnedAtTop && lineX1 > lineX0 {
			if solid {
				drawRect(dst, image.Rect(lineX0, y, lineX1, y+1), gridCol, true)
			} else {
				for x := lineX0; x < lineX1; x += 6 {
					endX := x + 3
					if endX > lineX1 {
						endX = lineX1
					}
					drawRect(dst, image.Rect(x, y, endX, y+1), gridCol, true)
				}
			}
		}
		// Label on left margin (SpaceXS inset so it never sits flush at
		// x=0; clamped so the topmost label stays inside the panel).
		label := fmt.Sprintf("%.0f", db)
		lh := int(float64(TextHeight()) * captionScale)
		ly := y - lh/2
		if ly < rect.Min.Y {
			ly = rect.Min.Y
		}
		DrawTextColorAtScale(dst, label, rect.Min.X+SpaceXS, ly, colTextSecondary, captionScale)
	}

	// High-resolution FFT curve underlay. Maps every FFT bin to its
	// pixel column on the log-frequency axis (linear scale uses the
	// same routine but with a linear x-axis) and paints a slope-tilted
	// magnitude trace. The 10 ISO bars layer over the top so the
	// musician-friendly aggregation stays the primary read while the
	// underlying detail tells the user where exactly the energy lives
	// — kicks at 60 Hz vs 90 Hz, snare body vs zing, etc.
	drawSpectrumCurve(dst, barRect, ch, scale)

	// Draw bars.
	numBands := len(bands)
	totalWidth := barRect.Dx()
	totalHeight := barRect.Dy()
	if totalWidth <= 0 || totalHeight <= 0 {
		return
	}

	// Each bar gets an equal share of the width, minus 1px gap between bars.
	gaps := numBands - 1
	barSpace := totalWidth - gaps
	if barSpace < numBands {
		// Not enough room for even 1px per bar.
		return
	}
	barWidth := barSpace / numBands

	for b := 0; b < numBands; b++ {
		// Normalize dB to 0-1 range.
		db := bandDB[b]
		if db < spectrumMinDB {
			db = spectrumMinDB
		}
		if db > spectrumMaxDB {
			db = spectrumMaxDB
		}
		norm := (db - spectrumMinDB) / (spectrumMaxDB - spectrumMinDB)

		// Phase 0a: peak-hold tick first so the bar can fall back to
		// peaks.Peaks[b] when norm is zero between hits (typical between
		// drum transients — the analyser's 512-sample window captures
		// silence and the bar would otherwise disappear).
		if peaks != nil {
			if peaks.MaxAge == 0 {
				peaks.MaxAge = 30
			}
			if norm > peaks.Peaks[b] {
				peaks.Peaks[b] = norm
				peaks.Ages[b] = 0
			} else {
				peaks.Ages[b]++
				if peaks.Ages[b] > peaks.MaxAge {
					peaks.Peaks[b] = norm
					peaks.Ages[b] = 0
				}
			}
		}

		drawNorm := norm
		if peaks != nil && peaks.Peaks[b] > drawNorm {
			drawNorm = peaks.Peaks[b]
		}
		barHeight := int(drawNorm * float64(totalHeight))
		if barHeight < 1 && drawNorm > 0 {
			barHeight = 1
		}

		x0 := barRect.Min.X + b*(barWidth+1)
		x1 := x0 + barWidth
		// Clamp the band to the plot rect: bars (and their hold markers)
		// must never run past the panel's right edge. A band that ends up
		// with zero width or fully outside the plot draws nothing — the
		// pre-fix renderer painted an orphan peak-hold line floating past
		// the last bar, off the right edge of the panel.
		if x1 > barRect.Max.X {
			x1 = barRect.Max.X
		}
		if x0 >= barRect.Max.X || x1 <= x0 {
			continue
		}
		y1 := barRect.Max.Y
		y0 := y1 - barHeight

		if barHeight > 0 {
			drawSpectrumBarGradient(dst, image.Rect(x0, y0, x1, y1))
		}

		// Peak-hold marker overlay (transient + watermark). Markers pinned
		// at the top extreme of the range are suppressed — a hold line
		// flush under the sticky bar reads as chrome, not data.
		if peaks != nil {
			peakHeight := int(peaks.Peaks[b] * float64(barRect.Dy()))
			if peakHeight > 0 && peakHeight < barRect.Dy() {
				py := barRect.Max.Y - peakHeight
				drawRect(dst, image.Rect(x0, py, x1, py+1), WithAlpha(genColorVizSpectrumPeakMarker, 200), true)
			}
			// Persistent MAX watermark: raise the all-time max and paint
			// a 2px-tall solid tick at its position. Stays put until
			// ResetMax is called from the UI.
			if norm > peaks.MaxPeaks[b] {
				peaks.MaxPeaks[b] = norm
			}
			maxHeight := int(peaks.MaxPeaks[b] * float64(barRect.Dy()))
			if maxHeight > 0 && maxHeight < barRect.Dy() {
				my := barRect.Max.Y - maxHeight
				myTop := my - 1
				if myTop < barRect.Min.Y {
					myTop = barRect.Min.Y
				}
				drawRect(dst, image.Rect(x0, myTop, x1, my+1), genColorVizSpectrumPeakMarker, true)
			}
		}
	}

	// Draw frequency labels in their dedicated Hz row (below the bracket
	// strip row, never overlapping it).
	freqLabelY := hzRow.Min.Y + 1
	for b := 0; b < numBands; b++ {
		x0 := barRect.Min.X + b*(barWidth+1)
		label := labels[b]
		lw := int(float64(TextWidth(label)) * captionScale)
		lx := x0 + (barWidth-lw)/2
		DrawTextColorAtScale(dst, label, lx, freqLabelY, colTextSecondary, captionScale)
	}

	// Draw Bass/Mids/Treble bracket strip in its own row above the Hz
	// labels. The bracket line AND its group labels both live inside
	// bracketRow (spectrumLabelRows sizes the row to fit a caption text
	// line under the bracket), so they can never collide with the Hz row.
	// The Bass/Mids/Treble grouping is tied to the ISO band layout, so the
	// strip is only drawn in log mode — linear mode skips it (the equal-Hz
	// bands don't map cleanly to musical bass/mid/treble ranges).
	if scale == freqScaleLog {
		bracketCol := WithAlpha(genColorBorder, AlphaSubtle)
		bracketY := bracketRow.Min.Y + 2 // 2 px gap below bar baseline
		for gi, g := range bandGroups {
			if g.endBand <= g.startBand || g.endBand > numBands {
				continue
			}
			gLabel := bandGroupLabelAt(gi)
			// Start X = left edge of first band in group; end X = right edge of last.
			groupX0 := barRect.Min.X + g.startBand*(barWidth+1)
			groupX1 := barRect.Min.X + (g.endBand-1)*(barWidth+1) + barWidth
			if groupX1 > barRect.Max.X {
				groupX1 = barRect.Max.X
			}
			if groupX1 <= groupX0 {
				continue
			}
			// Horizontal bracket line.
			drawRect(dst, image.Rect(groupX0, bracketY, groupX1, bracketY+1), bracketCol, true)
			// Tiny vertical end-caps (2 px tall).
			drawRect(dst, image.Rect(groupX0, bracketY, groupX0+1, bracketY+2), bracketCol, true)
			drawRect(dst, image.Rect(groupX1-1, bracketY, groupX1, bracketY+2), bracketCol, true)
			// Centered group label, inside the bracket row.
			lw := int(float64(TextWidth(gLabel)) * captionScale)
			lx := groupX0 + (groupX1-groupX0-lw)/2
			ly := bracketY + 3
			DrawTextColorAtScale(dst, gLabel, lx, ly, colTextSecondary, captionScale)
		}

		// Note-name overlay: tiny C3/C4/C5/C6/C7 markers along the top of the
		// bar area (low alpha so they don't compete with the bars). Each marker
		// is a 1-px vertical tick + a 2-char label rooted at the center of the
		// ISO band the note falls into. Pedagogical aid — maps Hz to note.
		noteCol := WithAlpha(genColorBorder, AlphaSubtle)
		noteLabelY := barRect.Min.Y + 1
		for _, n := range noteAnchors {
			bandIdx := findISOBand(n.hz)
			if bandIdx < 0 || bandIdx >= numBands {
				continue
			}
			bandX := barRect.Min.X + bandIdx*(barWidth+1) + barWidth/2
			// Short vertical tick down from top of bar area (4 px).
			drawRect(dst, image.Rect(bandX, barRect.Min.Y, bandX+1, barRect.Min.Y+4), noteCol, true)
			// Label centered on the tick.
			lw := int(float64(TextWidth(n.label)) * captionScale)
			lx := bandX - lw/2
			DrawTextColorAtScale(dst, n.label, lx, noteLabelY, colTextSecondary, captionScale)
		}
	}
}

// Synthwave spectrum fill palette. colSpectrumCurveFill is the faint area
// wash painted beneath the hi-res FFT curve down to the baseline. The three
// bar-gradient bands give each ISO bar a bright peak fading to a deeper base
// (top → bottom) for the "outrun" read; all are fixed package-level colors so
// drawRect serves them from pixelCache without per-bar/per-frame allocation.
var (
	colSpectrumCurveFill = WithAlpha(colWaveTrace, AlphaFaint)
	colSpectrumBarTop    = WithAlpha(colWaveTrace, AlphaOverlay)
	colSpectrumBarMid    = WithAlpha(colWaveTrace, AlphaStrong)
	colSpectrumBarBase   = WithAlpha(colWaveTrace, AlphaMedium)
)

// drawSpectrumBarGradient fills one ISO bar with a fixed three-band vertical
// gradient (bright peak → deep base). Three drawRects per bar (10 bars =
// 30 rects/frame) keeps the cost flat regardless of bar height, unlike a
// per-pixel gradient. Degrades gracefully to a single fill on tiny bars.
func drawSpectrumBarGradient(dst *ebiten.Image, bar image.Rectangle) {
	if bar.Empty() {
		return
	}
	h := bar.Dy()
	if h < 3 {
		drawRect(dst, bar, colSpectrumBarTop, true)
		return
	}
	y1 := bar.Min.Y + h/3
	y2 := bar.Min.Y + 2*h/3
	drawRect(dst, image.Rect(bar.Min.X, bar.Min.Y, bar.Max.X, y1), colSpectrumBarTop, true)
	drawRect(dst, image.Rect(bar.Min.X, y1, bar.Max.X, y2), colSpectrumBarMid, true)
	drawRect(dst, image.Rect(bar.Min.X, y2, bar.Max.X, bar.Max.Y), colSpectrumBarBase, true)
}

// spectrumCurveColScratch is the reusable per-column buffer used by
// drawSpectrumCurve. Single-threaded — the renderer runs on the UI
// goroutine and the slice is never aliased across calls. Pre-sized at
// 2048 (covers any plausible monitor width including 2× DPI tablets);
// drawSpectrumCurve clamps to whatever the active bar rect needs.
var spectrumCurveColScratch [2048]float64

// drawSpectrumCurve paints a high-resolution magnitude curve through the
// supplied FFT bins. Each pixel column is rendered as a thin vertical
// strip whose top edge sits at the dB-mapped y for that column's
// frequency, providing far more detail than the 10 ISO band bars on
// their own. Frequencies are mapped to x via:
//   - log axis  → `log10(hz / minHz) / log10(maxHz / minHz)`  (default)
//   - linear axis → `(hz - minHz) / (maxHz - minHz)`
//
// The optional slope tilt (spectrumSlopeDBPerOct) is added to each
// bin's dB before the y-mapping so pink-noise displays flat at 3 dB/oct
// (the broadcast convention). The curve uses the spectrum trace token
// at reduced alpha so it reads as a secondary layer beneath the 10
// musician-friendly band bars.
func drawSpectrumCurve(dst *ebiten.Image, barRect image.Rectangle, ch *analyzer.ChannelMetrics, scale freqScaleMode) {
	if ch == nil || len(ch.FFTBins) == 0 || len(ch.FreqBins) != len(ch.FFTBins) {
		return
	}
	if barRect.Dx() <= 1 || barRect.Dy() <= 1 {
		return
	}

	const minHz = 20.0
	const maxHz = 22000.0
	width := barRect.Dx()
	if width > len(spectrumCurveColScratch) {
		width = len(spectrumCurveColScratch)
	}
	height := float64(barRect.Dy())
	// Per-column maximum dB so the curve aggregates multiple bins
	// falling into the same column (high-frequency bins compress
	// densely under the log map). Re-use a package-level scratch
	// buffer so the renderer doesn't allocate a new slice every
	// frame — the per-tab alloc budget is tight.
	colDB := spectrumCurveColScratch[:width]
	for i := range colDB {
		colDB[i] = spectrumMinDB
	}

	logMin := math.Log10(minHz)
	logMax := math.Log10(maxHz)
	for i, hz := range ch.FreqBins {
		if hz < minHz || hz > maxHz {
			continue
		}
		var frac float64
		if scale == freqScaleLinear {
			frac = (hz - minHz) / (maxHz - minHz)
		} else {
			frac = (math.Log10(hz) - logMin) / (logMax - logMin)
		}
		col := int(frac * float64(width))
		if col < 0 {
			col = 0
		} else if col >= width {
			col = width - 1
		}
		dbVal := ch.FFTBins[i]
		// Apply slope tilt: pink noise (-3 dB/oct natural roll-off)
		// becomes flat at 3 dB/oct so the eye reads tonal balance
		// without compensating mentally.
		if spectrumSlopeDBPerOct != 0 && hz > 0 {
			dbVal += spectrumSlopeDBPerOct * math.Log2(hz/1000.0)
		}
		if dbVal > colDB[col] {
			colDB[col] = dbVal
		}
	}

	curveCol := WithAlpha(colWaveTrace, AlphaSubtle)
	// Batch adjacent columns that share the same y so the renderer
	// produces a small number of wide rects rather than thousands of
	// 1-pixel rects (the per-tab alloc budget is tight — see the
	// discipline test). Each batch emits a synthwave "outrun" area-fill
	// (faint) from the curve crown down to the baseline, then the 2px
	// crown line on top. Because columns are batched, the fill is one
	// extra wide rect per batch — not per pixel — so it stays alloc-cheap.
	startC := -1
	startY := 0
	flush := func(endC int) {
		if startC < 0 {
			return
		}
		x0 := barRect.Min.X + startC
		x1 := barRect.Min.X + endC
		if startY+2 < barRect.Max.Y {
			drawRect(dst, image.Rect(x0, startY+2, x1, barRect.Max.Y), colSpectrumCurveFill, true)
		}
		drawRect(dst, image.Rect(x0, startY, x1, startY+2), curveCol, true)
		startC = -1
	}
	for c := 0; c < width; c++ {
		db := colDB[c]
		if db <= spectrumMinDB {
			flush(c)
			continue
		}
		if db > spectrumMaxDB {
			db = spectrumMaxDB
		}
		norm := (db - spectrumMinDB) / (spectrumMaxDB - spectrumMinDB)
		h := int(norm * height)
		if h < 1 {
			h = 1
		}
		y := barRect.Max.Y - h
		if startC < 0 || y != startY {
			flush(c)
			startC = c
			startY = y
		}
	}
	flush(width)
}

// drawPreEQOverlayFromSnapshot paints a thin pre-EQ magnitude trace
// across the spectrum bar area at reduced alpha. Source is the raw
// FFT bin magnitudes returned by audio.PreEQAnalyzerSnapshot(...);
// since they're linear-magnitude (not dB), we apply 20·log10 inside
// the loop. The curve uses TokenVizCurve so it reads as a distinct
// "pre" layer beneath the post-EQ bars without competing for the
// musician-friendly bar fill (which uses TokenVizBar's cyan family).
//
// `spec` must have length >= 4 and is interpreted as bins [0, len)
// covering [0, sampleRate/2). When spec is empty the overlay is a
// no-op. scale picks the same log/linear mapping the bar renderer
// uses so the pre/post traces align.
func drawPreEQOverlayFromSnapshot(dst *ebiten.Image, rect image.Rectangle, spec []float64, sampleRate float64, scale freqScaleMode) {
	if len(spec) < 4 || sampleRate <= 0 {
		return
	}
	barRect, _, _ := spectrumPanelRows(rect)
	if barRect.Dx() <= 1 || barRect.Dy() <= 1 {
		return
	}

	const minHz = 20.0
	const maxHz = 22000.0
	width := barRect.Dx()
	if width > len(spectrumCurveColScratch) {
		width = len(spectrumCurveColScratch)
	}
	colDB := spectrumCurveColScratch[:width]
	for i := range colDB {
		colDB[i] = spectrumMinDB
	}

	logMin := math.Log10(minHz)
	logMax := math.Log10(maxHz)
	binHz := sampleRate / float64(2*len(spec))
	for i, m := range spec {
		hz := float64(i) * binHz
		if hz < minHz || hz > maxHz {
			continue
		}
		if m <= 0 {
			continue
		}
		// Convert linear magnitude → dB. The +20 offset compensates for
		// the FFT scaling so a full-scale sine reads near 0 dBFS, in
		// line with the post-EQ bin dB values produced by the FFT
		// observer.
		db := 20*math.Log10(m) + 20
		if spectrumSlopeDBPerOct != 0 && hz > 0 {
			db += spectrumSlopeDBPerOct * math.Log2(hz/1000.0)
		}
		var frac float64
		if scale == freqScaleLinear {
			frac = (hz - minHz) / (maxHz - minHz)
		} else {
			frac = (math.Log10(hz) - logMin) / (logMax - logMin)
		}
		col := int(frac * float64(width))
		if col < 0 {
			col = 0
		} else if col >= width {
			col = width - 1
		}
		if db > colDB[col] {
			colDB[col] = db
		}
	}

	overlayCol := WithAlpha(genColorVizCurve, AlphaFaint)
	startC := -1
	startY := 0
	flush := func(endC int) {
		if startC < 0 {
			return
		}
		x0 := barRect.Min.X + startC
		x1 := barRect.Min.X + endC
		drawRect(dst, image.Rect(x0, startY, x1, startY+1), overlayCol, true)
		startC = -1
	}
	height := float64(barRect.Dy())
	for c := 0; c < width; c++ {
		db := colDB[c]
		if db <= spectrumMinDB {
			flush(c)
			continue
		}
		if db > spectrumMaxDB {
			db = spectrumMaxDB
		}
		norm := (db - spectrumMinDB) / (spectrumMaxDB - spectrumMinDB)
		h := int(norm * height)
		if h < 1 {
			h = 1
		}
		y := barRect.Max.Y - h
		if startC < 0 || y != startY {
			flush(c)
			startC = c
			startY = y
		}
	}
	flush(width)
}

// drawSpectrumCursor renders a vertical crosshair at cursorX inside the
// spectrum bar area, plus a "Hz · dB" readout label anchored above. cursorX
// is in screen pixels; nothing renders if cursorX falls outside the bar rect.
//
// The function recomputes the same bar geometry as drawAnalyzerSpectrum so the
// crosshair lands exactly on a band boundary regardless of the data path.
// Only the log-scale layout is supported here — the cursor on the linear
// layout falls back to "Hz · dB" computed off linearBands().
func drawSpectrumCursor(dst *ebiten.Image, rect image.Rectangle, ch *analyzer.ChannelMetrics, cursorX int) {
	barRect, _, _ := spectrumPanelRows(rect)
	if cursorX < barRect.Min.X || cursorX >= barRect.Max.X {
		return
	}
	if barRect.Dx() <= 0 || barRect.Dy() <= 0 {
		return
	}

	// Vertical line.
	// Phase 6 audio-panel redesign: density-driven stroke so the
	// cursor stays visible at mobile portrait sizes.
	cursorW := Profile().DensityValues().SpectrumCursorStroke
	if cursorW < 1 {
		cursorW = 1
	}
	drawRect(dst, image.Rect(cursorX, barRect.Min.Y, cursorX+cursorW, barRect.Max.Y), colTextSecondary, true)

	// Map cursorX → ISO band (same arithmetic as the bar renderer).
	bands := isoBands
	numBands := len(bands)
	gaps := numBands - 1
	barSpace := barRect.Dx() - gaps
	if barSpace < numBands {
		return
	}
	barWidth := barSpace / numBands
	bandIdx := (cursorX - barRect.Min.X) / (barWidth + 1)
	if bandIdx < 0 || bandIdx >= numBands {
		return
	}
	centerHz := (bands[bandIdx][0] + bands[bandIdx][1]) / 2

	// dB at this band: same averaging the renderer uses.
	db := spectrumMinDB
	if ch != nil && len(ch.FFTBins) > 0 && len(ch.FreqBins) == len(ch.FFTBins) {
		sum := 0.0
		count := 0
		for i, hz := range ch.FreqBins {
			if hz >= bands[bandIdx][0] && hz < bands[bandIdx][1] {
				sum += ch.FFTBins[i]
				count++
			}
		}
		if count > 0 {
			db = sum / float64(count)
		}
	}

	// Label: "1.2 kHz · C5 · -32 dB" — the note name is the pedagogical
	// add (Phase 1): kids can correlate a spectral peak with a key on a
	// piano without doing the conversion in their head. Skip the note
	// segment when hzToNote returns "" (sub-audible cursor positions).
	parts := []string{formatHzShort(centerHz)}
	if note := hzToNote(centerHz); note != "" {
		parts = append(parts, note)
	}
	parts = append(parts, formatMeterDB(db)+" dB")
	label := parts[0]
	for i := 1; i < len(parts); i++ {
		label += " · " + parts[i]
	}
	captionScale := FontSizeCaption / FontSizeBody
	tw := int(float64(TextWidth(label)) * captionScale)
	th := int(float64(TextHeight()) * captionScale)
	lx := cursorX + 4
	if lx+tw+4 > barRect.Max.X-SpaceXS {
		lx = cursorX - tw - 4
	}
	if lx < barRect.Min.X+SpaceXS {
		lx = barRect.Min.X + SpaceXS
	}
	ly := barRect.Min.Y + SpaceXS
	bg := image.Rect(lx-2, ly-1, lx+tw+2, ly+th+2)
	drawRect(dst, bg, WithAlpha(colSurface2, AlphaStrong), true)
	DrawTextColorAtScale(dst, label, lx, ly, colTextSecondary, captionScale)
}

// formatHzShort renders a frequency in compact form: 250 Hz, 1.2 kHz.
func formatHzShort(hz float64) string {
	if hz >= 1000 {
		return fmt.Sprintf("%.1f kHz", hz/1000)
	}
	return fmt.Sprintf("%.0f Hz", hz)
}

// noteNames maps the 12 semitones of an octave to their (sharp-free)
// name. We pick the natural name when ambiguous; sharps are rendered
// with a trailing "#". Kid-readable mapping — no flat notation.
var noteNames = [12]string{
	"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B",
}

// hzToNote returns the closest equal-temperament note name + octave
// for the given frequency (A4 = 440 Hz reference). Returns "" for
// frequencies below the audible piano range (< 16 Hz) so cursor chips
// don't render garbage at the lower bound of the spectrum.
//
// Used by drawSpectrumCursor to compose the "Hz · note · dB" readout
// chip; the original cursor only displayed Hz + dB which made it hard
// for kids to correlate a peak in the spectrum with a musical note.
func hzToNote(hz float64) string {
	if hz < 16 {
		return ""
	}
	// MIDI note number n: hz = 440 * 2^((n-69)/12)
	// → n = 12 * log2(hz/440) + 69
	n := 12*math.Log2(hz/440.0) + 69
	// Round to nearest integer; clamp to a plausible piano range so
	// rounding edges don't produce note names below A0 (MIDI 21) or
	// above C8 (MIDI 108).
	ni := int(math.Round(n))
	if ni < 21 {
		ni = 21
	} else if ni > 108 {
		ni = 108
	}
	octave := (ni / 12) - 1
	idx := ni % 12
	return fmt.Sprintf("%s%d", noteNames[idx], octave)
}
