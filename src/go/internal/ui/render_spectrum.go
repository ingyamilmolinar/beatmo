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
// Values are held in dBFS, NOT as normalised plot heights.
//
// A normalised height only means something against the window it was measured
// under, and the window moves (see audioDBWindow). Storing 0..1 therefore
// baked the old axis into every held mark, so after a re-anchor the ticks
// were drawn against a scale they had never been measured on — watermarks
// floated over bands holding nothing. dB is absolute: store it, and convert
// at draw time through the CURRENT window.
type SpectrumPeakState struct {
	// PeaksDB is the short peak-hold that keeps a bar up between transients.
	// spectrumNoPeakDB means "nothing held".
	PeaksDB [10]float64
	// lastObserved timestamps the previous observe PER BAND. One shared
	// timestamp does not work: observe is called once per band per frame, so
	// the first call stamped "now" and every later band saw dt=0 and never
	// decayed at all — bands 1-9 froze at their loudest reading.
	lastObserved [10]time.Time
	// MaxPeaksDB is the ALL-TIME watermark; it never decays on its own and is
	// cleared only by (*EQPanelZone).InvalidateSpectrumReadout.
	MaxPeaksDB [10]float64
}

// spectrumNoPeakDB marks a band with nothing held. The zero value of the
// struct must mean "empty", and 0 dB is full scale, so the sentinel cannot be
// 0 — every accessor normalises it away.
const spectrumNoPeakDB = 0.0

// spectrumHoldDecayDBPerSec is how fast a held bar falls once the band goes
// quiet. Slow enough that a kick on every beat at 120 bpm (one hit per 500 ms)
// never leaves the plot empty, fast enough that the bar still visibly
// answers the music rather than sitting at the last transient.
const spectrumHoldDecayDBPerSec = 24.0

// heldDB returns the short-hold level for band i, or spectrumMinDB when the
// band holds nothing.
func (s *SpectrumPeakState) heldDB(i int) float64 {
	if s.PeaksDB[i] == spectrumNoPeakDB {
		return spectrumMinDB
	}
	return s.PeaksDB[i]
}

// observe folds this frame's dB reading for band i into both holds. win is
// unused by the arithmetic — it is taken so callers cannot forget that what
// is stored is absolute and what they draw must be re-normalised.
//
// A louder reading wins at once; a quieter one lets the hold FALL at
// spectrumHoldDecayDBPerSec. The previous rule held the bar flat for 30
// frames and then dropped it to zero in one, which on a per-instrument
// channel — silent between transients — made the tab blink at the tempo of
// the music. A rate also makes the ballistics independent of frame rate.
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

// maxDB returns the all-time watermark for band i, or spectrumMinDB when the
// band has never been observed.
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

// ResetMax clears every band's MAX watermark. The watermark never decays, so
// this is the only way it comes down; InvalidateSpectrumReadout calls it at
// every discontinuity.
func (s *SpectrumPeakState) ResetMax() {
	for i := range s.MaxPeaksDB {
		s.MaxPeaksDB[i] = spectrumNoPeakDB
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

// spectrumBandDB aggregates an (FFTBins dB, FreqBins Hz) pair into one
// average dB per display band, and reports which bands were actually
// MEASURED.
//
// `measured[b]` is false when NO FFT bin falls inside the band. That is not
// the same thing as silence, and conflating the two is how a browser-only
// blank plot shipped: the WASM bridge used to deliver 64 linear bins over
// the whole spectrum (375 Hz apart at 48 kHz), so the four lowest ISO bands
// — 20-355 Hz, where a kick lives — contained no bins at all and every one
// of them reported the reference floor. Desktop, at 43 Hz per bin, was
// correct throughout and no screenshot of it could show the defect.
//
// The level returned for an unmeasured band is still the floor (changing
// what is DRAWN there would be inventing data the analyser never supplied).
// The flag exists so the invariant can be asserted where it belongs — on
// the resolution the bridge ships, not on the pixels downstream. Guard:
// TestSpectrumBinsCoverEveryDisplayBand.
//
// Pure; unit-tested.
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
		// A silent bin is -Inf, and averaging a few of them yields
		// -math.MaxFloat64. Those are not "very quiet", they are unusable:
		// left alone they flow into the axis anchor and the peak holds and
		// poison both. Clamp to the reference floor at the one place the
		// band levels are formed.
		if !(bandDB[b] > spectrumMinDB) { // also catches NaN
			bandDB[b] = spectrumMinDB
		}
	}
	return bandDB, measured
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

// spectrumDBWindow is the dB range the plot maps to its full height. The zero
// value means "the fixed reference", so every caller that has no adaptive
// window (legacy entry points, the idle path) keeps the old -80..0 behaviour.
type spectrumDBWindow struct {
	Floor, Ceil float64
}

// resolve returns a usable (floor, ceil), substituting the fixed reference
// for a zero or inverted window.
func (w spectrumDBWindow) resolve() (float64, float64) {
	if w.Ceil <= w.Floor {
		return spectrumMinDB, spectrumMaxDB
	}
	return w.Floor, w.Ceil
}

// spectrumFreqLabelMinH is the minimum height of the Hz-label row at the
// bottom of the spectrum panel (legacy 14px floor).
const spectrumFreqLabelMinH = 14

// captionTextH returns the pixel height of one axis-label text row (the
// styled micro role — smooth Inter, true 10px).
func captionTextH() int {
	return StyledTextHeight(RoleMicro)
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
	drawAnalyzerSpectrumImpl(dst, rect, ch, peaks, scale, spectrumChromeOpts{})
}

// spectrumCurveKey is the TraceSprite version key for the cached FFT curve
// underlay: the ChannelMetrics POINTER is the per-tick freshness signal (the
// wave/chain cache precedent), plus every display input the trace depends on.
type spectrumCurveKey struct {
	ch    *analyzer.ChannelMetrics
	scale freqScaleMode
	w, h  int
	// win must be in the key for the same reason `detail` is in the chrome
	// key: the adaptive dB window changes the BAKED pixels, so a cache keyed
	// without it keeps blitting the curve drawn against the previous window.
	win spectrumDBWindow
}

// drawAnalyzerSpectrumImpl is drawAnalyzerSpectrumWithScale plus an optional
// TraceSprite for the high-resolution curve underlay. Bars and peak-hold
// ballistics always render live (their decay animates per frame); only the
// per-column FFT trace — whose data changes per analyzer tick — is cached.
// spectrumChromeOpts bundles the Spectrum tab's cached-chrome sprites with the
// detail flag. A struct rather than four more positional parameters: the three
// sprites and the flag are always supplied together, and the flag has to reach
// BOTH the cached chrome (via the sprite key) and the live bar loop.
type spectrumChromeOpts struct {
	curve, grid, labels *TraceSprite
	// detail reveals the analyser marks the redesign moved off the default
	// read — dB numbers, note anchors, peak-hold and max-watermark ticks.
	detail bool
	// dbWin is the adaptive dB range the bars and curve map to the plot
	// height. Zero value = the fixed -80..0 reference.
	dbWin spectrumDBWindow
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

	// Idle state: baseline rule + quiet hint instead of a dead panel (the
	// bare-outline empty state read as broken chrome — 2026-07-04 critique).
	// NOT gated on ch.Active. On WASM that flag is derived per snapshot
	// (snapshotIsActive), and a single instrument's channel is SILENT
	// between its hits — so a percussive channel reported inactive on most
	// frames and this early return threw away the peak-holds that exist
	// precisely to bridge those gaps, replacing a decaying bar with the
	// idle hint at the tempo of the music. A quiet frame is a legitimate
	// reading; the idle path is for having no analyser data at all.
	// spectrumPeakDB drops the same gate, so the axis anchors on exactly
	// the readings the bars are drawn from.
	//
	// (This was NOT the cause of the 2026-09-18 browser-only empty plot —
	// that path draws a visible baseline rule and hint, not a blank panel.
	// See analyzer.SpectrumFFTSize for what was.)
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

	// Reserve a left margin for the dB scale, plus two stacked label rows
	// below the bars: the Bass/Mids/Treble bracket strip and the Hz tick
	// labels each get their own row so they can never collide.
	barRect, _, _ := spectrumPanelRows(rect)

	winFloor, winCeil := opts.dbWin.resolve()

	// Group FFT bins into 10 bands and compute average dB for each.
	bandDB, _ := spectrumBandDB(fftBins, freqBins, bands)

	// Static dB grid chrome (reference lines + dB labels). Depends only on
	// the panel geometry, the theme and the locale — never on audio data — so
	// it is cached in a sprite and blitted once per frame instead of being
	// repainted. The dashed lines alone issued ~832 one-per-dash blits EVERY
	// frame (4 dashed lines x ~208 dashes), 82% of the whole tab's per-frame
	// blit count, for pixels that had not changed since the previous frame.
	if opts.grid != nil && spectrumChromeCacheEnabled {
		opts.grid.tag = "spectrumGridSprite"
		opts.grid.Draw(dst, rect, spectrumChromeKey{w: rect.Dx(), h: rect.Dy(), scale: scale,
			theme: ThemeGeneration(), locale: i18n.ActiveLocale(), detail: opts.detail, win: opts.dbWin},
			func(img *ebiten.Image, r image.Rectangle) { drawSpectrumGridChrome(img, r, opts.detail, opts.dbWin) })
	} else {
		drawSpectrumGridChrome(dst, rect, opts.detail, opts.dbWin)
	}

	// High-resolution FFT curve underlay. Maps every FFT bin to its
	// pixel column on the log-frequency axis (linear scale uses the
	// same routine but with a linear x-axis) and paints a slope-tilted
	// magnitude trace. The 10 ISO bars layer over the top so the
	// musician-friendly aggregation stays the primary read while the
	// underlying detail tells the user where exactly the energy lives
	// — kicks at 60 Hz vs 90 Hz, snare body vs zing, etc.
	//
	// Cached when a sprite is provided: the trace re-renders per analyzer
	// tick (fresh ch pointer) instead of every frame.
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

		// Peak-hold first so the bar can fall back to the held level when
		// norm is zero between hits (typical between drum transients — the
		// analyser's 512-sample window captures silence and the bar would
		// otherwise disappear). Held in dB, re-read in the current window.
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
		if peaks != nil && opts.detail {
			peakHeight := int(peaks.holdNorm(b, opts.dbWin) * float64(barRect.Dy()))
			if peakHeight > 0 && peakHeight < barRect.Dy() {
				py := barRect.Max.Y - peakHeight
				drawRect(dst, image.Rect(x0, py, x1, py+1), WithAlpha(genColorVizSpectrumPeakMarker, 200), true)
			}
			// Persistent MAX watermark: raise the all-time max and paint
			// a 2px-tall solid tick at its position. Stays put until
			// ResetMax is called from the UI.
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

	// Static label chrome (Hz ticks, Bass/Mids/Treble bracket strip, note
	// anchors). Drawn AFTER the bars so the note ticks stay on top, hence a
	// second sprite rather than folding into the grid backdrop — the same
	// two-layer split the EQ tab uses (eqBackdropSprite / eqLabelsSprite).
	if opts.labels != nil && spectrumChromeCacheEnabled {
		opts.labels.tag = "spectrumLabelsSprite"
		opts.labels.Draw(dst, rect, spectrumChromeKey{w: rect.Dx(), h: rect.Dy(), scale: scale,
			theme: ThemeGeneration(), locale: i18n.ActiveLocale(), detail: opts.detail},
			func(img *ebiten.Image, r image.Rectangle) { drawSpectrumLabelChrome(img, r, scale, opts.detail) })
	} else {
		drawSpectrumLabelChrome(dst, rect, scale, opts.detail)
	}
}

// spectrumChromeCacheEnabled gates the Spectrum tab's static-chrome sprites
// (test seam, mirrors waveTraceCacheEnabled / chainTraceCacheEnabled). Tests
// that COUNT drawRect calls at on-screen coordinates disable it so every
// primitive reaches the interceptor directly, exactly as the Wave and Chain
// caches are disabled in audio_panel_render_pixels_test.go.
var spectrumChromeCacheEnabled = true

// SetSpectrumChromeCacheForTest toggles the chrome cache, returning a restore func.
func SetSpectrumChromeCacheForTest(on bool) func() {
	prev := spectrumChromeCacheEnabled
	spectrumChromeCacheEnabled = on
	return func() { spectrumChromeCacheEnabled = prev }
}

// spectrumChromeKey versions the two static-chrome sprites. The chrome is a
// pure function of the panel rect and the band table (chosen by scale), so the
// only other inputs are the palette generation and the locale — both of which
// change the baked pixels without changing the geometry.
type spectrumChromeKey struct {
	w, h   int
	scale  freqScaleMode
	theme  int
	locale i18n.Locale
	// detail must be in the key: the flag changes the BAKED pixels (dB numbers,
	// note anchors), so a cache keyed without it would keep blitting the old
	// chrome after the DTL pill is pressed. The window moves the reference
	// lines and their labels, so it belongs here for the same reason.
	detail bool
	win    spectrumDBWindow
}

// drawSpectrumGridChrome paints the dB reference lines and their left-margin
// labels. Pure in rect: it re-derives the row geometry via spectrumPanelRows,
// so it renders identically into an origin-based sprite and straight to the
// screen (the envRenderSafe pixel-test path).
func drawSpectrumGridChrome(dst *ebiten.Image, rect image.Rectangle, detail bool, win spectrumDBWindow) {
	barRect, _, _ := spectrumPanelRows(rect)
	winFloor, winCeil := win.resolve()
	dbRefs := []float64{0, -12, -24, -36, -48, -60}
	// Inset the reference lines from the panel edges so they never read
	// as a border artifact, and skip any line pinned at the top of the
	// range — the 0 dB ref at norm=1 landed flush under the sticky bar
	// and read as an error underline across the whole tab.
	lineX0 := barRect.Min.X + SpaceXS
	lineX1 := barRect.Max.X - SpaceXS
	for _, db := range dbRefs {
		// A reference outside the active window has no position on this
		// plot; drawing it clamped would stack several lines on one edge.
		if db > winCeil || db < winFloor {
			continue
		}
		norm := audioDBNorm(db, winFloor, winCeil)
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
		// The numbers are detail: the grid LINES already carry the shape, and
		// six figures down the left margin competed with the headline sentence.
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

// drawSpectrumLabelChrome paints the Hz tick labels, the Bass/Mids/Treble
// bracket strip and the note anchors. Pure in (rect, scale) — it re-derives
// the band table and bar geometry exactly as the bar loop does.
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
	// Draw frequency labels in their dedicated Hz row (below the bracket
	// strip row, never overlapping it).
	freqLabelY := hzRow.Min.Y + 1
	for b := 0; b < numBands; b++ {
		x0 := barRect.Min.X + b*(barWidth+1)
		label := labels[b]
		lw := StyledTextWidth(label, RoleMicro)
		lx := x0 + (barWidth-lw)/2
		DrawTextStyled(dst, label, lx, freqLabelY, RoleMicro, colTextSecondary)
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
			lw := StyledTextWidth(gLabel, RoleMicro)
			lx := groupX0 + (groupX1-groupX0-lw)/2
			ly := bracketY + 3
			DrawTextStyled(dst, gLabel, lx, ly, RoleMicro, colTextSecondary)
		}

		// Note-name overlay: tiny C3/C4/C5/C6/C7 markers along the top of the
		// bar area (low alpha so they don't compete with the bars). Each marker
		// is a 1-px vertical tick + a 2-char label rooted at the center of the
		// ISO band the note falls into. Pedagogical aid — maps Hz to note.
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

// Synthwave spectrum fill palette. colSpectrumCurveFill is the faint area
// wash painted beneath the hi-res FFT curve down to the baseline. The three
// bar-gradient bands give each ISO bar a bright peak fading to a deeper base
// (top → bottom) for the "outrun" read; all are fixed package-level colors so
// drawRect serves them from pixelCache without per-bar/per-frame allocation.
var (
	// Derived in rebuildThemeAliases (theme.go).
	colSpectrumCurveFill    color.NRGBA
	colSpectrumBandEmphasis color.NRGBA
	colSpectrumBarTop       color.NRGBA
	colSpectrumBarMid       color.NRGBA
	colSpectrumBarBase      color.NRGBA
)

// drawSpectrumBarGradient fills one ISO bar with a fixed three-band vertical
// gradient (bright peak → deep base). Three drawRects per bar (10 bars =
// 30 rects/frame) keeps the cost flat regardless of bar height, unlike a
// per-pixel gradient. Degrades gracefully to a single fill on tiny bars.
// drawSpectrumBarGradient is a func VAR so tests can observe the exact bar
// rects the renderer paints (the drawMenuRow precedent). Measuring bar height
// off intercepted drawRect calls would mean guessing which rects are bars.
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
		// The dB/oct slope tilt was removed with its pill: it is a
		// mastering-desk convention that reads as "the analyser is broken"
		// to anyone who has not met it, and nothing set it but that control.
		dbVal := ch.FFTBins[i]
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
