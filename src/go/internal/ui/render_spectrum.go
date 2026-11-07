package ui

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// isoLabels are the center-frequency labels for the 10 ISO bands.
var isoLabels = [10]string{"31", "62", "125", "250", "500", "1k", "2k", "4k", "8k", "16k"}

// Half-open ranges over isoBands, grouped the way musicians think (kick in Bass, snare across Mids, hats
// in Treble); an orientation aid only.
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

// One C per ISO band (C3 band 2 … C7 band 6) so the labels never collide.
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

func findISOBand(hz float64) int {
	for i, b := range isoBands {
		if hz >= b[0] && hz < b[1] {
			return i
		}
	}
	return -1
}

// A ~500 ms transient hold plus a never-decaying watermark, both in dBFS: a normalised height is only
// meaningful against the window it was measured under, and the window moves; convert at draw time.
type SpectrumPeakState struct {
	PeaksDB [10]float64
	// Per band: observe runs once per band per frame, so a shared timestamp gave every band after the first
	// dt=0 and they never decayed.
	lastObserved [10]time.Time
	// Never decays; cleared only by InvalidateSpectrumReadout.
	MaxPeaksDB [10]float64
}

// The zero value must mean "empty", and 0 dB is full scale, so every accessor normalises the sentinel away.
const spectrumNoPeakDB = 0.0

// Slow enough that a kick every 500 ms never empties the plot, fast enough that the bar still answers the music.
const spectrumHoldDecayDBPerSec = 24.0

func (s *SpectrumPeakState) heldDB(i int) float64 {
	if s.PeaksDB[i] == spectrumNoPeakDB {
		return spectrumMinDB
	}
	return s.PeaksDB[i]
}

// win is unused by the arithmetic; taking it reminds callers that the store is absolute and the draw must
// re-normalise. Louder wins at once; quieter falls at a fixed rate, so the tab never blinks at the tempo.
func (s *SpectrumPeakState) observe(i int, db float64, win spectrumDBWindow, now time.Time) {
	if db > s.heldDB(i) {
		s.PeaksDB[i] = db
	} else if !s.lastObserved[i].IsZero() {
		if dt := now.Sub(s.lastObserved[i]).Seconds(); dt > 0 {
			fallen := s.heldDB(i) - spectrumHoldDecayDBPerSec*dt
			if fallen < db {
				fallen = db
			}
			s.PeaksDB[i] = fallen
		}
	}
	if db > s.maxDB(i) {
		s.MaxPeaksDB[i] = db
	}
	s.lastObserved[i] = now
}

func (s *SpectrumPeakState) maxDB(i int) float64 {
	if s.MaxPeaksDB[i] == spectrumNoPeakDB {
		return spectrumMinDB
	}
	return s.MaxPeaksDB[i]
}

// holdNorm and maxNorm place the two holds in the CURRENT window.
func (s *SpectrumPeakState) holdNorm(i int, win spectrumDBWindow) float64 {
	floor, ceil := win.resolve()
	return audioDBNorm(s.heldDB(i), floor, ceil)
}

func (s *SpectrumPeakState) maxNorm(i int, win spectrumDBWindow) float64 {
	floor, ceil := win.resolve()
	return audioDBNorm(s.maxDB(i), floor, ceil)
}

// UpdateMax raises each band's watermark to the higher of (liveDB[i], held).
func (s *SpectrumPeakState) UpdateMax(liveDB [10]float64) {
	for i, v := range liveDB {
		if v > s.maxDB(i) {
			s.MaxPeaksDB[i] = v
		}
	}
}

// The only way the watermark comes down.
func (s *SpectrumPeakState) ResetMax() {
	for i := range s.MaxPeaksDB {
		s.MaxPeaksDB[i] = spectrumNoPeakDB
	}
}

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

// measured[b] is false when no FFT bin lies in the band - which is not silence: a 64-bin bridge left the
// four lowest bands empty and the plot blank. The level stays the floor; the flag lets the resolution be asserted.
func spectrumBandDB(fftBins, freqBins []float64, bands [10][2]float64) (bandDB [10]float64, measured [10]bool) {
	for b := range bands {
		lo, hi := bands[b][0], bands[b][1]
		sum := 0.0
		count := 0
		for i, hz := range freqBins {
			if hz >= lo && hz < hi && i < len(fftBins) {
				sum += fftBins[i]
				count++
			}
		}
		if count > 0 {
			bandDB[b] = sum / float64(count)
			measured[b] = true
		} else {
			bandDB[b] = spectrumMinDB
		}
		// A silent bin is -Inf and the average can reach -MaxFloat64, which would poison the axis anchor and the
		// holds: clamp here, where band levels are formed.
		if !(bandDB[b] > spectrumMinDB) { // also catches NaN
			bandDB[b] = spectrumMinDB
		}
	}
	return bandDB, measured
}

type freqScaleMode int

const (
	freqScaleLog    freqScaleMode = iota // ISO 1/3-octave bands (default)
	freqScaleLinear                      // 10 equal-Hz bands across [20, 22000]
)

func linearBands() [10][2]float64 {
	var out [10][2]float64
	const lo, hi = 20.0, 22000.0
	step := (hi - lo) / 10
	for i := 0; i < 10; i++ {
		out[i] = [2]float64{lo + float64(i)*step, lo + float64(i+1)*step}
	}
	return out
}

// kHz to one decimal so a label fits one bar on mobile.
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

// The zero value means the fixed -80..0 reference.
type spectrumDBWindow struct {
	Floor, Ceil float64
}

func (w spectrumDBWindow) resolve() (float64, float64) {
	if w.Ceil <= w.Floor {
		return spectrumMinDB, spectrumMaxDB
	}
	return w.Floor, w.Ceil
}

const spectrumFreqLabelMinH = 14

func captionTextH() int {
	return StyledTextHeight(RoleMicro)
}

// Three non-overlapping rows: plot, bracket strip, Hz labels. The bracket row is raised to fit a caption
// under its line, so the group labels can never stamp over the Hz labels.
func spectrumLabelRows(rect image.Rectangle, bracketStripH, captionH int) (barRect, bracketRow, hzRow image.Rectangle) {
	// 2 px gap + 1 px line, caption text 3 px below the line top.
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

// Every spectrum layer derives its geometry here, so the layers always align.
func spectrumPanelRows(rect image.Rectangle) (barRect, bracketRow, hzRow image.Rectangle) {
	return spectrumLabelRows(rect, Profile().DensityValues().SpectrumBracketH, captionTextH())
}

func drawAnalyzerSpectrum(dst *ebiten.Image, rect image.Rectangle, ch *analyzer.ChannelMetrics, peaks *SpectrumPeakState) {
	drawAnalyzerSpectrumWithScale(dst, rect, ch, peaks, freqScaleLog)
}

func drawAnalyzerSpectrumWithScale(dst *ebiten.Image, rect image.Rectangle, ch *analyzer.ChannelMetrics, peaks *SpectrumPeakState, scale freqScaleMode) {
	drawAnalyzerSpectrumImpl(dst, rect, ch, peaks, scale, spectrumChromeOpts{})
}

// The ChannelMetrics pointer is the per-tick freshness signal.
type spectrumCurveKey struct {
	ch    *analyzer.ChannelMetrics
	scale freqScaleMode
	w, h  int
	// The window changes the baked pixels, so the key must carry it.
	win spectrumDBWindow
}

// The sprites and the flag always travel together, and the flag must reach both the cached chrome (the key)
// and the live bars.
type spectrumChromeOpts struct {
	curve, grid, labels *TraceSprite
	// detail reveals the analyser marks kept off the default read: dB numbers, note anchors, hold ticks.
	detail bool
	dbWin  spectrumDBWindow
}

func drawAnalyzerSpectrumImpl(dst *ebiten.Image, rect image.Rectangle, ch *analyzer.ChannelMetrics, peaks *SpectrumPeakState, scale freqScaleMode, opts spectrumChromeOpts) {
	// Resolve band table + labels for the chosen scale.
	var bands [10][2]float64
	switch scale {
	case freqScaleLinear:
		bands = linearBands()
	default:
		bands = isoBands
	}

	// Not gated on ch.Active: on WASM a percussive channel reads inactive between hits, and returning here would
	// replace the decaying holds with the idle hint at the tempo. The idle path is for no analyser data at all.
	if ch == nil || len(ch.FFTBins) == 0 {
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

	barRect, _, _ := spectrumPanelRows(rect)

	winFloor, winCeil := opts.dbWin.resolve()

	// Group FFT bins into 10 bands and compute average dB for each.
	bandDB, _ := spectrumBandDB(fftBins, freqBins, bands)

	// Static grid chrome is cached: its dashed lines were most of the tab's per-frame blits for pixels that
	// never change.
	if opts.grid != nil && spectrumChromeCacheEnabled {
		opts.grid.tag = "spectrumGridSprite"
		opts.grid.Draw(dst, rect, spectrumChromeKey{w: rect.Dx(), h: rect.Dy(), scale: scale,
			theme: ThemeGeneration(), locale: i18n.ActiveLocale(), detail: opts.detail, win: opts.dbWin},
			func(img *ebiten.Image, r image.Rectangle) { drawSpectrumGridChrome(img, r, opts.detail, opts.dbWin) })
	} else {
		drawSpectrumGridChrome(dst, rect, opts.detail, opts.dbWin)
	}

	// The fine curve shows where the energy actually lives under the ten bars; cached per analyzer tick.
	if opts.curve != nil && ch != nil {
		opts.curve.tag = "spectrumCurveSprite"
		opts.curve.Draw(dst, barRect,
			spectrumCurveKey{ch: ch, scale: scale,
				w: barRect.Dx(), h: barRect.Dy(), win: opts.dbWin},
			func(img *ebiten.Image, r image.Rectangle) {
				drawSpectrumCurve(img, r, ch, scale, opts.dbWin)
			})
	} else {
		drawSpectrumCurve(dst, barRect, ch, scale, opts.dbWin)
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
		// Normalize dB to 0-1 range within the active window.
		norm := audioDBNorm(bandDB[b], winFloor, winCeil)

		// Holds first, so between hits (silent analyser window) the bar falls back to the held level.
		drawNorm := norm
		if peaks != nil {
			peaks.observe(b, bandDB[b], opts.dbWin, time.Now())
			if held := peaks.holdNorm(b, opts.dbWin); held > drawNorm {
				drawNorm = held
			}
		}
		barHeight := int(drawNorm * float64(totalHeight))
		if barHeight < 1 && drawNorm > 0 {
			barHeight = 1
		}

		x0 := barRect.Min.X + b*(barWidth+1)
		x1 := x0 + barWidth
		// Clamped to the plot so no bar or hold marker runs past the right edge.
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

		// A hold pinned at the top would sit flush under the sticky bar and read as chrome, so it is suppressed.
		if peaks != nil && opts.detail {
			peakHeight := int(peaks.holdNorm(b, opts.dbWin) * float64(barRect.Dy()))
			if peakHeight > 0 && peakHeight < barRect.Dy() {
				py := barRect.Max.Y - peakHeight
				drawRect(dst, image.Rect(x0, py, x1, py+1), WithAlpha(genColorVizSpectrumPeakMarker, 200), true)
			}
			maxHeight := int(peaks.maxNorm(b, opts.dbWin) * float64(barRect.Dy()))
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

	// Drawn after the bars so the note ticks stay on top, hence a second sprite.
	if opts.labels != nil && spectrumChromeCacheEnabled {
		opts.labels.tag = "spectrumLabelsSprite"
		opts.labels.Draw(dst, rect, spectrumChromeKey{w: rect.Dx(), h: rect.Dy(), scale: scale,
			theme: ThemeGeneration(), locale: i18n.ActiveLocale(), detail: opts.detail},
			func(img *ebiten.Image, r image.Rectangle) { drawSpectrumLabelChrome(img, r, scale, opts.detail) })
	} else {
		drawSpectrumLabelChrome(dst, rect, scale, opts.detail)
	}
}

// Test seam: tests that count drawRect calls turn the sprites off.
var spectrumChromeCacheEnabled = true

// SetSpectrumChromeCacheForTest toggles the chrome cache, returning a restore func.
func SetSpectrumChromeCacheForTest(on bool) func() {
	prev := spectrumChromeCacheEnabled
	spectrumChromeCacheEnabled = on
	return func() { spectrumChromeCacheEnabled = prev }
}

// Palette generation and locale change the baked pixels without changing geometry.
type spectrumChromeKey struct {
	w, h   int
	scale  freqScaleMode
	theme  int
	locale i18n.Locale
	// detail and the window change the baked pixels, so both are in the key.
	detail bool
	win    spectrumDBWindow
}

// Pure in rect, so it renders the same into an origin sprite and straight to the screen.
func drawSpectrumGridChrome(dst *ebiten.Image, rect image.Rectangle, detail bool, win spectrumDBWindow) {
	barRect, _, _ := spectrumPanelRows(rect)
	winFloor, winCeil := win.resolve()
	dbRefs := []float64{0, -12, -24, -36, -48, -60}
	// Inset from the edges, and the 0 dB line at the very top is skipped: flush under the sticky bar it read as
	// an error underline.
	lineX0 := barRect.Min.X + SpaceXS
	lineX1 := barRect.Max.X - SpaceXS
	for _, db := range dbRefs {
		// Clamping a reference outside the window would stack several lines on one edge.
		if db > winCeil || db < winFloor {
			continue
		}
		norm := audioDBNorm(db, winFloor, winCeil)
		y := barRect.Max.Y - int(norm*float64(barRect.Dy()))
		// 0 and -12 dB render solid and brighter so the headroom envelope is obvious; a solid line is one rect,
		// never one per pixel column.
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
		// The numbers are detail: the lines carry the shape, and six figures competed with the headline.
		if detail {
			label := fmt.Sprintf("%.0f", db)
			lh := StyledTextHeight(RoleMicro)
			ly := y - lh/2
			if ly < rect.Min.Y {
				ly = rect.Min.Y
			}
			DrawTextStyled(dst, label, rect.Min.X+SpaceXS, ly, RoleMicro, colTextSecondary)
		}
	}

}

func drawSpectrumLabelChrome(dst *ebiten.Image, rect image.Rectangle, scale freqScaleMode, detail bool) {
	var labels [10]string
	if scale == freqScaleLinear {
		labels = linearLabels()
	} else {
		labels = isoLabels
	}
	barRect, bracketRow, hzRow := spectrumPanelRows(rect)
	numBands := len(labels)
	totalWidth := barRect.Dx()
	if totalWidth <= 0 || barRect.Dy() <= 0 {
		return
	}
	barSpace := totalWidth - (numBands - 1)
	if barSpace < numBands {
		return
	}
	barWidth := barSpace / numBands
	freqLabelY := hzRow.Min.Y + 1
	for b := 0; b < numBands; b++ {
		x0 := barRect.Min.X + b*(barWidth+1)
		label := labels[b]
		lw := StyledTextWidth(label, RoleMicro)
		lx := x0 + (barWidth-lw)/2
		DrawTextStyled(dst, label, lx, freqLabelY, RoleMicro, colTextSecondary)
	}

	// Log mode only: equal-Hz bands do not map onto bass/mid/treble.
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
			lw := StyledTextWidth(gLabel, RoleMicro)
			lx := groupX0 + (groupX1-groupX0-lw)/2
			ly := bracketY + 3
			DrawTextStyled(dst, gLabel, lx, ly, RoleMicro, colTextSecondary)
		}

		// Low alpha so the note markers never compete with the bars.
		noteCol := WithAlpha(genColorBorder, AlphaSubtle)
		noteLabelY := barRect.Min.Y + 1
		if !detail {
			return
		}
		for _, n := range noteAnchors {
			bandIdx := findISOBand(n.hz)
			if bandIdx < 0 || bandIdx >= numBands {
				continue
			}
			bandX := barRect.Min.X + bandIdx*(barWidth+1) + barWidth/2
			// Short vertical tick down from top of bar area (4 px).
			drawRect(dst, image.Rect(bandX, barRect.Min.Y, bandX+1, barRect.Min.Y+4), noteCol, true)
			// Label centered on the tick.
			lw := StyledTextWidth(n.label, RoleMicro)
			lx := bandX - lw/2
			DrawTextStyled(dst, n.label, lx, noteLabelY, RoleMicro, colTextSecondary)
		}
	}
}

// Fixed package-level colors so drawRect serves them from pixelCache without allocating.
var (
	// Derived in rebuildThemeAliases (theme.go).
	colSpectrumCurveFill    color.NRGBA
	colSpectrumBandEmphasis color.NRGBA
	colSpectrumBarTop       color.NRGBA
	colSpectrumBarMid       color.NRGBA
	colSpectrumBarBase      color.NRGBA
)

// Three rects per bar keeps the cost flat whatever its height. A var so tests observe the exact bar rects
// instead of guessing them from drawRect calls.
var drawSpectrumBarGradient = func(dst *ebiten.Image, bar image.Rectangle) {
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

// UI goroutine only, never aliased. 2048 covers any plausible width.
var spectrumCurveColScratch [2048]float64

// The slope tilt (if any) is added before the y-mapping; reduced alpha keeps the curve a secondary layer
// under the bars.
func drawSpectrumCurve(dst *ebiten.Image, barRect image.Rectangle, ch *analyzer.ChannelMetrics, scale freqScaleMode, win spectrumDBWindow) {
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
	// Per-column max because high bins pile into one column under the log map; a reused scratch, because the
	// tab's alloc budget is tight.
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
		// No slope tilt: a mastering-desk convention that reads as "the analyser is broken" to everyone else.
		dbVal := ch.FFTBins[i]
		if dbVal > colDB[col] {
			colDB[col] = dbVal
		}
	}

	curveCol := WithAlpha(colWaveTrace, AlphaSubtle)
	// Adjacent columns with the same y are batched into one wide rect (plus one fill rect), never one per pixel.
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
	winFloor, winCeil := win.resolve()
	for c := 0; c < width; c++ {
		db := colDB[c]
		if db <= winFloor {
			flush(c)
			continue
		}
		norm := audioDBNorm(db, winFloor, winCeil)
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

func drawSpectrumCursor(dst *ebiten.Image, rect image.Rectangle, ch *analyzer.ChannelMetrics, cursorX int) {
	barRect, _, _ := spectrumPanelRows(rect)
	if cursorX < barRect.Min.X || cursorX >= barRect.Max.X {
		return
	}
	if barRect.Dx() <= 0 || barRect.Dy() <= 0 {
		return
	}

	// Density-driven so the cursor stays visible at phone sizes.
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

	// The note name lets a peak be found on a keyboard without conversion; skipped below the audible range.
	parts := []string{formatHzShort(centerHz)}
	if note := hzToNote(centerHz); note != "" {
		parts = append(parts, note)
	}
	parts = append(parts, formatMeterDB(db)+" dB")
	label := parts[0]
	for i := 1; i < len(parts); i++ {
		label += " · " + parts[i]
	}
	tw := StyledTextWidth(label, RoleCaption)
	th := StyledTextHeight(RoleCaption)
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
	DrawTextStyled(dst, label, lx, ly, RoleCaption, colTextSecondary)
}

// formatHzShort renders a frequency in compact form: 250 Hz, 1.2 kHz.
func formatHzShort(hz float64) string {
	if hz >= 1000 {
		return fmt.Sprintf("%.1f kHz", hz/1000)
	}
	return fmt.Sprintf("%.0f Hz", hz)
}

// Sharps only, never flats.
var noteNames = [12]string{
	"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B",
}

// "" below 16 Hz so the cursor chip shows nothing at the bottom edge.
func hzToNote(hz float64) string {
	if hz < 16 {
		return ""
	}
	n := 12*math.Log2(hz/440.0) + 69
	// Clamped to the piano (MIDI 21..108); node note names must not use this.
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
