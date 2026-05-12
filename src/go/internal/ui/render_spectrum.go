package ui

import (
	"fmt"
	"image"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
)

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
	label      string
	startBand  int
	endBand    int // exclusive
}{
	{"Bass", 0, 4},
	{"Mids", 4, 7},
	{"Treble", 7, 10},
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
type SpectrumPeakState struct {
	Peaks  [10]float64 // normalized 0-1 peak values
	Ages   [10]int     // frames since peak was set
	MaxAge int         // frames before peak decays (default 30 ≈ 500ms)
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

	// Empty state: draw border only.
	if ch == nil || !ch.Active || len(ch.FFTBins) == 0 {
		drawRect(dst, rect, colButtonBorder, false)
		return
	}

	fftBins := ch.FFTBins
	freqBins := ch.FreqBins

	// FreqBins and FFTBins must be the same length for pairing.
	if len(freqBins) != len(fftBins) {
		drawRect(dst, rect, colButtonBorder, false)
		return
	}

	// Reserve 28px left for dB scale, 14px bottom for freq labels, plus
	// 8px above that for the Bass/Mids/Treble bracket strip.
	const freqLabelH = 14
	const bracketStripH = 8
	barRect := image.Rect(rect.Min.X+28, rect.Min.Y, rect.Max.X, rect.Max.Y-freqLabelH-bracketStripH)

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
	dbRefs := []float64{0, -20, -40, -60}
	for _, db := range dbRefs {
		norm := (db - spectrumMinDB) / (spectrumMaxDB - spectrumMinDB)
		y := barRect.Max.Y - int(norm*float64(barRect.Dy()))
		// Dashed line (3px on, 3px off).
		for x := barRect.Min.X; x < barRect.Max.X; x += 6 {
			endX := x + 3
			if endX > barRect.Max.X {
				endX = barRect.Max.X
			}
			drawRect(dst, image.Rect(x, y, endX, y+1), WithAlpha(genColorBorder, genAlphaWhiteDecoration), true)
		}
		// Label on left margin.
		label := fmt.Sprintf("%.0f", db)
		lh := int(float64(TextHeight()) * captionScale)
		DrawTextColorAtScale(dst, label, rect.Min.X+2, y-lh/2, colTextSecondary, captionScale)
	}

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

		barHeight := int(norm * float64(totalHeight))
		if barHeight < 1 && norm > 0 {
			barHeight = 1
		}

		x0 := barRect.Min.X + b*(barWidth+1)
		x1 := x0 + barWidth
		y1 := barRect.Max.Y
		y0 := y1 - barHeight

		if barHeight > 0 {
			drawRect(dst, image.Rect(x0, y0, x1, y1), colWaveTrace, true)
		}

		// Peak hold.
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
			// Draw peak marker.
			peakHeight := int(peaks.Peaks[b] * float64(barRect.Dy()))
			if peakHeight > 0 {
				py := barRect.Max.Y - peakHeight
				drawRect(dst, image.Rect(x0, py, x1, py+1), WithAlpha(genColorVizSpectrumPeakMarker, 200), true)
			}
		}
	}

	// Draw frequency labels below bars (below bracket strip).
	freqLabelY := barRect.Max.Y + bracketStripH + 1
	for b := 0; b < numBands; b++ {
		x0 := barRect.Min.X + b*(barWidth+1)
		label := labels[b]
		lw := int(float64(TextWidth(label)) * captionScale)
		lx := x0 + (barWidth-lw)/2
		DrawTextColorAtScale(dst, label, lx, freqLabelY, colTextSecondary, captionScale)
	}

	// Draw Bass/Mids/Treble bracket strip above the frequency labels.
	// The Bass/Mids/Treble grouping is tied to the ISO band layout, so the
	// strip is only drawn in log mode — linear mode skips it (the equal-Hz
	// bands don't map cleanly to musical bass/mid/treble ranges).
	if scale == freqScaleLog {
		bracketCol := WithAlpha(genColorBorder, AlphaSubtle)
		bracketY := barRect.Max.Y + 2 // 2 px gap above bar baseline
		for _, g := range bandGroups {
			if g.endBand <= g.startBand || g.endBand > numBands {
				continue
			}
			// Start X = left edge of first band in group; end X = right edge of last.
			groupX0 := barRect.Min.X + g.startBand*(barWidth+1)
			groupX1 := barRect.Min.X + (g.endBand-1)*(barWidth+1) + barWidth
			if groupX1 <= groupX0 {
				continue
			}
			// Horizontal bracket line.
			drawRect(dst, image.Rect(groupX0, bracketY, groupX1, bracketY+1), bracketCol, true)
			// Tiny vertical end-caps (2 px tall).
			drawRect(dst, image.Rect(groupX0, bracketY, groupX0+1, bracketY+2), bracketCol, true)
			drawRect(dst, image.Rect(groupX1-1, bracketY, groupX1, bracketY+2), bracketCol, true)
			// Centered group label.
			lw := int(float64(TextWidth(g.label)) * captionScale)
			lx := groupX0 + (groupX1-groupX0-lw)/2
			ly := bracketY + 3
			DrawTextColorAtScale(dst, g.label, lx, ly, colTextSecondary, captionScale)
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

// drawSpectrumCursor renders a vertical crosshair at cursorX inside the
// spectrum bar area, plus a "Hz · dB" readout label anchored above. cursorX
// is in screen pixels; nothing renders if cursorX falls outside the bar rect.
//
// The function recomputes the same bar geometry as drawAnalyzerSpectrum so the
// crosshair lands exactly on a band boundary regardless of the data path.
// Only the log-scale layout is supported here — the cursor on the linear
// layout falls back to "Hz · dB" computed off linearBands().
func drawSpectrumCursor(dst *ebiten.Image, rect image.Rectangle, ch *analyzer.ChannelMetrics, cursorX int) {
	const freqLabelH = 14
	const bracketStripH = 8
	barRect := image.Rect(rect.Min.X+28, rect.Min.Y, rect.Max.X, rect.Max.Y-freqLabelH-bracketStripH)
	if cursorX < barRect.Min.X || cursorX >= barRect.Max.X {
		return
	}
	if barRect.Dx() <= 0 || barRect.Dy() <= 0 {
		return
	}

	// Vertical line.
	drawRect(dst, image.Rect(cursorX, barRect.Min.Y, cursorX+1, barRect.Max.Y), colTextSecondary, true)

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

	// Label: "1.2 kHz · -32 dB"
	label := formatHzShort(centerHz) + " · " + formatMeterDB(db) + " dB"
	captionScale := FontSizeCaption / FontSizeBody
	tw := int(float64(TextWidth(label)) * captionScale)
	th := int(float64(TextHeight()) * captionScale)
	lx := cursorX + 4
	if lx+tw+4 > barRect.Max.X {
		lx = cursorX - tw - 4
	}
	if lx < barRect.Min.X {
		lx = barRect.Min.X
	}
	ly := barRect.Min.Y + 2
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
