package ui

import (
	"fmt"
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// Meter bridge colors — sourced from DESIGN.md `viz-meter-*` tokens.
// To change a hue, edit DESIGN.md and run `make gen-design-tokens`.
var (
	meterGreen  = genColorVizMeterGreen
	meterYellow = genColorVizMeterYellow
	meterRed    = genColorVizMeterRed
	meterBg     = genColorVizMeterBg
	meterClip   = genColorVizMeterClip
)

const (
	meterDBFloor    = -60.0
	meterDBCeil     = 0.0
	meterYellowDB   = -6.0
	meterRedDB      = -1.0
	meterRMSOpacity = 128 // 50% of 255
	// clipLatchFrames keeps the clip readout latched in error color for ~1s
	// after a new clip event (assumes ~60 fps).
	clipLatchFrames = 60
	// peakHoldStickyFrames keeps PeakHoldDB pinned for ~250ms after a new
	// peak before it starts decaying (DAW-standard behavior).
	peakHoldStickyFrames = 15
	// peakHoldDecayDB sets the per-frame decay rate once the sticky window
	// expires (0.5 dB/frame ≈ 30 dB/s at 60 fps).
	peakHoldDecayDB = 0.5
)

// MultiLevelsLatch holds per-channel LevelsLatch values keyed by channel
// ID so the multi-channel Levels view can maintain independent peak-
// hold + clip-latch state for every instrument + master. Phase 2 of the
// audio-panel redesign.
type MultiLevelsLatch struct {
	byID map[string]*LevelsLatch
}

// NewMultiLevelsLatch returns an initialised map.
func NewMultiLevelsLatch() *MultiLevelsLatch {
	return &MultiLevelsLatch{byID: map[string]*LevelsLatch{}}
}

// Clear resets every per-channel latch so the persistent "!" markers
// disappear immediately. Used by the Levels-tab Clear Clips pill —
// without this users had no way to acknowledge clip events; the latch
// would auto-expire after 60 frames regardless.
func (m *MultiLevelsLatch) Clear() {
	if m == nil {
		return
	}
	for _, l := range m.byID {
		if l == nil {
			continue
		}
		l.LatchFramesLeft = 0
		l.LastClipCount = 0
	}
}

// Reset clears the peak/RMS hold ballistics and clip latch of every
// per-channel latch so the next Update re-seeds directly from the live
// analyzer state. Used when the Levels tab is re-entered after being
// hidden: the held values are frozen at whatever they were when the tab
// was last drawn, so without this the meters decay down from a stale
// (often loud) value instead of reflecting the current audio immediately.
func (m *MultiLevelsLatch) Reset() {
	if m == nil {
		return
	}
	for _, l := range m.byID {
		if l != nil {
			l.Reset()
		}
	}
}

// Get returns the latch for id, creating a fresh one on first reference.
func (m *MultiLevelsLatch) Get(id string) *LevelsLatch {
	if m.byID == nil {
		m.byID = map[string]*LevelsLatch{}
	}
	if l, ok := m.byID[id]; ok {
		return l
	}
	l := &LevelsLatch{}
	m.byID[id] = l
	return l
}

// LevelsLatch tracks transient clip-event + peak/RMS hold state for the
// Levels tab. A new clip (ClipCount higher than the last seen value)
// latches the readout in error color for clipLatchFrames frames. The
// peak-hold and RMS-hold fields rise instantly when the live values
// exceed the hold, stay pinned for peakHoldStickyFrames frames, then
// decay slowly so the bars remain visible between drum transients
// (a snapshot caught between hits otherwise reads -∞ dB and the bar
// disappears entirely — see Phase 0a of the audio-panel redesign).
type LevelsLatch struct {
	LastClipCount   int
	LatchFramesLeft int
	PeakHoldDB      float64
	RMSHoldDB       float64
	peakHoldSticky  int
	rmsHoldSticky   int
	peakHoldSeeded  bool
	rmsHoldSeeded   bool
}

// Update advances the latch by one frame. Returns true while the clip
// latch is active. PeakHoldDB and RMSHoldDB are updated as side effects.
//
// rmsDB carries the held-value semantics for the RMS bar so the bottom
// half of the Levels strip survives between hits the same way the Peak
// bar does. Callers passing only a clip count + peak (legacy two-arg
// shape) should route through UpdateLegacy below — newer renderers
// always pass RMS too.
func (l *LevelsLatch) Update(clipCount int, peakDB, rmsDB float64) bool {
	if clipCount > l.LastClipCount {
		l.LatchFramesLeft = clipLatchFrames
	} else if l.LatchFramesLeft > 0 {
		l.LatchFramesLeft--
	}
	l.LastClipCount = clipCount

	switch {
	case !l.peakHoldSeeded:
		l.PeakHoldDB = peakDB
		l.peakHoldSticky = peakHoldStickyFrames
		l.peakHoldSeeded = true
	case peakDB > l.PeakHoldDB:
		l.PeakHoldDB = peakDB
		l.peakHoldSticky = peakHoldStickyFrames
	case l.peakHoldSticky > 0:
		l.peakHoldSticky--
	default:
		l.PeakHoldDB -= peakHoldDecayDB
		if l.PeakHoldDB < peakDB {
			l.PeakHoldDB = peakDB
		}
	}

	switch {
	case !l.rmsHoldSeeded:
		l.RMSHoldDB = rmsDB
		l.rmsHoldSticky = peakHoldStickyFrames
		l.rmsHoldSeeded = true
	case rmsDB > l.RMSHoldDB:
		l.RMSHoldDB = rmsDB
		l.rmsHoldSticky = peakHoldStickyFrames
	case l.rmsHoldSticky > 0:
		l.rmsHoldSticky--
	default:
		l.RMSHoldDB -= peakHoldDecayDB
		if l.RMSHoldDB < rmsDB {
			l.RMSHoldDB = rmsDB
		}
	}
	return l.LatchFramesLeft > 0
}

// Reset returns the latch to its unseeded zero state so the next Update
// seeds PeakHoldDB/RMSHoldDB directly from the live values (no decay) and
// clears any active clip latch. See MultiLevelsLatch.Reset for why this is
// needed on Levels-tab re-entry.
func (l *LevelsLatch) Reset() { *l = LevelsLatch{} }

// Latched reports the current latch state without advancing it. Useful when
// the latch's owner ticked it elsewhere (e.g., in Update) and the renderer
// only wants to read the visual state.
func (l *LevelsLatch) Latched() bool { return l.LatchFramesLeft > 0 }

// drawLevelsDetail renders the single-channel Levels view: big Peak bar,
// big RMS bar, dB scale, and a footer readout (Pk / RMS / Hdr / Clips).
// Replaces the legacy multi-channel meter bridge — overview meters now live
// inline on each drum row via layer_eq_peek.go.
//
// If ch is nil, draws only the panel border (empty state). The latch may
// be nil; nil latch means "no clip hold tracking" (readout color follows
// the instantaneous clip count).
func drawLevelsDetail(dst *ebiten.Image, rect image.Rectangle, ch *analyzer.ChannelMetrics, latch *LevelsLatch) {
	drawRect(dst, rect, colButtonBorder, false)
	if ch == nil {
		return
	}

	const headerH = 14                                     // px: dB scale ticks on top
	const footerH = 18                                     // px: readout row (Pk / RMS / Hdr / Clips)
	gutterX := Profile().DensityValues().AudioLabelMarginW // left margin for dB labels

	contentH := rect.Dy() - headerH - footerH
	if contentH < 8 {
		// Too short for two bars — just render the border + readout if room.
		if rect.Dy() > footerH+2 {
			drawLevelsReadout(dst, image.Rect(rect.Min.X, rect.Max.Y-footerH, rect.Max.X, rect.Max.Y), ch, latch)
		}
		return
	}
	peakH := contentH * 60 / 100
	rmsH := contentH - peakH

	barX0 := rect.Min.X + gutterX
	barX1 := rect.Max.X - 4
	if barX1 <= barX0 {
		return
	}

	// Header strip with dB scale ticks: 0, -6, -12, -24, -48.
	headerRect := image.Rect(barX0, rect.Min.Y, barX1, rect.Min.Y+headerH)
	drawLevelsScale(dst, headerRect)
	// Clip LED (8×8) at the left of the header strip; lit while a clip
	// event is latched. Stays visible for ~1s after each new clip event
	// independent of the live peak — turns the cryptic "Clips N" footer
	// into a glanceable warning.
	if latch != nil && latch.Latched() {
		ledX := rect.Min.X + 2
		ledY := headerRect.Min.Y + 3
		drawRect(dst, image.Rect(ledX, ledY, ledX+8, ledY+8), meterClip, true)
	}

	// Peak bar (top, larger). Phase 5: when stereo data is present,
	// split horizontally into L (top half) + R (bottom half) so the
	// user sees per-channel headroom at a glance. Mono signals keep
	// the legacy single-bar render.
	//
	// Phase 0a (audio-panel redesign): mono bar fills to the held
	// peak rather than the instantaneous frame value. Between drum
	// transients the analyser captures silence, which makes the bar
	// disappear; the latch holds the most-recent peak for ~250 ms
	// sticky + 0.5 dB/frame decay so the bar remains legible. The
	// instantaneous live peak is still drawn as a thin overlay so
	// the user sees the moment-to-moment signal.
	peakY0 := rect.Min.Y + headerH
	peakY1 := peakY0 + peakH
	peakBar := image.Rect(barX0, peakY0, barX1, peakY1)
	if ch.HasStereo() {
		mid := peakY0 + (peakY1-peakY0)/2
		drawLevelsBar(dst, image.Rect(barX0, peakY0, barX1, mid), ch.PeakL(), "L", rect.Min.X+2)
		drawLevelsBar(dst, image.Rect(barX0, mid, barX1, peakY1), ch.PeakR(), "R", rect.Min.X+2)
		if latch != nil {
			drawLevelsPeakHoldMarker(dst, image.Rect(barX0, peakY0, barX1, mid), latch.PeakHoldDB)
		}
	} else {
		barDB := ch.PeakDB
		if latch != nil && latch.peakHoldSeeded && latch.PeakHoldDB > barDB {
			barDB = latch.PeakHoldDB
		}
		drawLevelsBar(dst, peakBar, barDB, "PEAK", rect.Min.X+2)
		if latch != nil {
			drawLevelsPeakHoldMarker(dst, peakBar, latch.PeakHoldDB)
		}
	}

	// RMS bar (bottom). Same stereo split treatment. The mono bar
	// likewise uses the held RMS value so the bar persists between
	// hits — see the peak-bar comment above for the rationale.
	rmsY0 := peakY1
	rmsY1 := rmsY0 + rmsH
	if ch.HasStereo() {
		mid := rmsY0 + (rmsY1-rmsY0)/2
		drawLevelsBar(dst, image.Rect(barX0, rmsY0, barX1, mid), ch.RMSL(), "L", rect.Min.X+2)
		drawLevelsBar(dst, image.Rect(barX0, mid, barX1, rmsY1), ch.RMSR(), "R", rect.Min.X+2)
	} else {
		barRMS := ch.RMSDB
		if latch != nil && latch.rmsHoldSeeded && latch.RMSHoldDB > barRMS {
			barRMS = latch.RMSHoldDB
		}
		drawLevelsBar(dst, image.Rect(barX0, rmsY0, barX1, rmsY1), barRMS, "RMS", rect.Min.X+2)
	}

	// Footer readout strip.
	drawLevelsReadout(dst, image.Rect(rect.Min.X, rect.Max.Y-footerH, rect.Max.X, rect.Max.Y), ch, latch)
}

// drawLevelsBar renders one large horizontal level bar with a side label.
func drawLevelsBar(dst *ebiten.Image, rect image.Rectangle, db float64, label string, labelX int) {
	if rect.Dy() < 4 {
		return
	}
	// Background.
	drawRect(dst, image.Rect(rect.Min.X, rect.Min.Y+1, rect.Max.X, rect.Max.Y-1), meterBg, true)

	// Side label (caption font).
	captionScale := FontSizeCaption / FontSizeBody
	lh := int(float64(TextHeight()) * captionScale)
	labelY := rect.Min.Y + (rect.Dy()-lh)/2
	DrawTextColorAtScale(dst, label, labelX, labelY, colTextSecondary, captionScale)

	// Level fill.
	frac := dbToFrac(db)
	if frac > 0 {
		fillX1 := rect.Min.X + int(frac*float64(rect.Dx()))
		fillCol := meterColor(db)
		drawRect(dst, image.Rect(rect.Min.X, rect.Min.Y+1, fillX1, rect.Max.Y-1), fillCol, true)
	}
}

// drawLevelsPeakHoldMarker paints a 2px-wide vertical bar at the peak-
// hold dB position on the Peak bar. Uses colTextPrimary so it reads
// clearly against the meter green/yellow/red fills.
func drawLevelsPeakHoldMarker(dst *ebiten.Image, peakBar image.Rectangle, peakHoldDB float64) {
	if peakBar.Dy() < 4 || peakBar.Dx() < 4 {
		return
	}
	if peakHoldDB <= meterDBFloor {
		return
	}
	frac := dbToFrac(peakHoldDB)
	x := peakBar.Min.X + int(frac*float64(peakBar.Dx()))
	if x >= peakBar.Max.X {
		x = peakBar.Max.X - 1
	}
	if x < peakBar.Min.X {
		x = peakBar.Min.X
	}
	drawRect(dst, image.Rect(x-1, peakBar.Min.Y+1, x+1, peakBar.Max.Y-1), colTextPrimary, true)
}

// drawLevelsScale renders a thin tick row across the bar area at the
// standard meter dB references: 0, -6, -12, -24, -48.
func drawLevelsScale(dst *ebiten.Image, rect image.Rectangle) {
	captionScale := FontSizeCaption / FontSizeBody
	tickCol := WithAlpha(genColorBorder, AlphaPanelBorder)
	for _, db := range []float64{-48, -24, -12, -6, 0} {
		frac := dbToFrac(db)
		x := rect.Min.X + int(frac*float64(rect.Dx()))
		drawRect(dst, image.Rect(x, rect.Min.Y+rect.Dy()/2, x+1, rect.Max.Y), tickCol, true)
		label := fmt.Sprintf("%.0f", db)
		lw := int(float64(TextWidth(label)) * captionScale)
		DrawTextColorAtScale(dst, label, x-lw/2, rect.Min.Y, colTextSecondary, captionScale)
	}
}

// drawLevelsReadout renders the footer strip showing Pk / RMS / Hdr / Clips
// as a single line of text. When the latch is active (recent clip event),
// the Clips number is rendered in error color; otherwise it follows the
// instantaneous ClipCount.
func drawLevelsReadout(dst *ebiten.Image, rect image.Rectangle, ch *analyzer.ChannelMetrics, latch *LevelsLatch) {
	captionScale := FontSizeCaption / FontSizeBody
	lh := int(float64(TextHeight()) * captionScale)
	y := rect.Min.Y + (rect.Dy()-lh)/2

	// Phase 0a: footer reads the held peak / RMS so the numeric
	// remains meaningful between drum transients. The instantaneous
	// fields (ch.PeakDB / ch.RMSDB) are -∞ for most frames because
	// the analyser's 512-sample window catches silence between hits.
	// Headroom still derives from the held peak.
	peakReadout := ch.PeakDB
	rmsReadout := ch.RMSDB
	if latch != nil && latch.peakHoldSeeded && latch.PeakHoldDB > peakReadout {
		peakReadout = latch.PeakHoldDB
	}
	if latch != nil && latch.rmsHoldSeeded && latch.RMSHoldDB > rmsReadout {
		rmsReadout = latch.RMSHoldDB
	}
	headroom := ch.HeadroomDB
	if headroom == 0 && peakReadout != 0 {
		headroom = -peakReadout
	}

	// Static prefix.
	prefix := fmt.Sprintf("Pk %s · RMS %s · Hdr %s · Clips ",
		formatMeterDB(peakReadout),
		formatMeterDB(rmsReadout),
		formatMeterDB(headroom),
	)
	DrawTextColorAtScale(dst, prefix, rect.Min.X+4, y, colTextSecondary, captionScale)
	prefixW := int(float64(TextWidth(prefix)) * captionScale)

	// Clip number with optional latch coloring.
	clipsText := fmt.Sprintf("%d", ch.ClipCount)
	clipsCol := colTextSecondary
	if latch != nil && latch.Latched() {
		clipsCol = colError
	} else if ch.ClipCount > 0 {
		clipsCol = colError
	}
	DrawTextColorAtScale(dst, clipsText, rect.Min.X+4+prefixW, y, clipsCol, captionScale)
}

// formatDB renders a dB value compactly for the readout strip.
// Below the meter floor it renders as "-∞" (caller sets context, but the
// font may not have ∞; fall back to "-inf").
func formatMeterDB(db float64) string {
	if db <= meterDBFloor {
		return "-inf"
	}
	return fmt.Sprintf("%.0f", db)
}

// meterColor returns the meter color for a given peak dB level.
func meterColor(db float64) color.RGBA {
	if db > meterRedDB {
		return meterRed
	}
	if db > meterYellowDB {
		return meterYellow
	}
	return meterGreen
}

// meterColorAlpha returns the meter color with a custom alpha for overlays.
func meterColorAlpha(db float64, alpha uint8) color.NRGBA {
	return WithAlpha(meterColor(db), alpha)
}

// drawLevelsMultiChannel renders one vertical Peak/RMS strip per
// instrument plus a wider Master strip on the right, plus an aggregate
// readout column on the far right. Phase 2 of the audio-panel redesign:
// matches the consumer DAW vocabulary (one strip per channel, segmented
// LED-style fill, per-strip peak-hold marker, clip LED + readout).
//
// latches is per-channel keyed by InstrumentMetrics.ID (and the literal
// "main" for the master). The caller is responsible for advancing each
// latch via .Update(...) before this function runs.
//
// When state has no instruments (only master), falls back to the
// single-channel drawLevelsDetail so the layout stays meaningful for
// boot-time idle states.
func drawLevelsMultiChannel(dst *ebiten.Image, rect image.Rectangle, state *analyzer.State, latches *MultiLevelsLatch) {
	drawRect(dst, rect, colButtonBorder, false)
	if state == nil {
		return
	}

	if len(state.Instruments) == 0 {
		drawLevelsDetail(dst, rect, &state.Master, latchOrNil(latches, "main"))
		return
	}

	const (
		footerH        = 18 // dB scale + numeric readouts
		headerH        = 12 // channel-name label
		stripGap       = 4
		masterStripPad = 8
	)
	// Phase 4 audio-panel redesign: levels readout column width is
	// density-driven. Three modes:
	//   1. rect.Dx() ≥ 2 × WFull → full-text column (today's path).
	//   2. WFull + WIcons ≤ rect.Dx() < 2 × WFull → icon-row collapse
	//      (Headroom / Clips / Loudest as three stacked icons).
	//   3. rect.Dx() < WFull + WIcons → footer-chevron only (the
	//      aggregate is one tap away in the chevron menu).
	// Pre-Phase-4 a single binary cascade hid the column completely
	// below 360 px, leaving mobile users with no Headroom readout at
	// all.
	ldv := Profile().DensityValues()
	readoutWFull := ldv.LevelsReadoutWFull
	readoutWIcons := ldv.LevelsReadoutWIcons

	contentH := rect.Dy() - headerH - footerH
	if contentH < 24 {
		drawLevelsDetail(dst, rect, &state.Master, latchOrNil(latches, "main"))
		return
	}

	readoutMode := readoutColModeFull
	readoutW := readoutWFull
	switch {
	case rect.Dx() >= 2*readoutWFull:
		readoutMode = readoutColModeFull
		readoutW = readoutWFull
	case rect.Dx() >= readoutWFull+readoutWIcons:
		readoutMode = readoutColModeIcons
		readoutW = readoutWIcons
	default:
		readoutMode = readoutColModeChevron
		readoutW = 0
	}
	stripsRect := image.Rect(rect.Min.X+4, rect.Min.Y, rect.Max.X-readoutW, rect.Max.Y)

	// Slots: every instrument + master last. Master gets ~2× the
	// width of a regular strip.
	nInst := len(state.Instruments)
	totalWeights := nInst + 2 // master = 2 weights
	stripSpace := stripsRect.Dx() - (nInst)*stripGap - masterStripPad
	if stripSpace < totalWeights {
		drawLevelsDetail(dst, rect, &state.Master, latchOrNil(latches, "main"))
		return
	}
	weightPx := stripSpace / totalWeights
	if weightPx < 14 {
		weightPx = 14
	}

	x := stripsRect.Min.X
	for i := range state.Instruments {
		inst := &state.Instruments[i]
		stripW := weightPx
		drawLevelsChannelStrip(dst,
			image.Rect(x, stripsRect.Min.Y, x+stripW, stripsRect.Max.Y),
			inst.Name, inst.PeakDB, inst.RMSDB, inst.ClipCount,
			latchOrNil(latches, inst.ID))
		x += stripW + stripGap
	}
	// Separator before master.
	sepX := x + masterStripPad/2
	drawRect(dst, image.Rect(sepX, stripsRect.Min.Y+headerH, sepX+1, stripsRect.Max.Y-footerH), WithAlpha(genColorBorder, AlphaSubtle), true)
	x += masterStripPad
	masterW := weightPx * 2
	if x+masterW > stripsRect.Max.X {
		masterW = stripsRect.Max.X - x
	}
	if masterW > 14 {
		drawLevelsChannelStrip(dst,
			image.Rect(x, stripsRect.Min.Y, x+masterW, stripsRect.Max.Y),
			"Master", state.Master.PeakDB, state.Master.RMSDB, state.Master.ClipCount,
			latchOrNil(latches, "main"))
	}

	// Phase 4 audio-panel redesign: three readout modes.
	switch readoutMode {
	case readoutColModeFull:
		drawLevelsAggregates(dst,
			image.Rect(rect.Max.X-readoutW+4, rect.Min.Y+4, rect.Max.X-4, rect.Max.Y-4),
			state, latches)
	case readoutColModeIcons:
		drawLevelsAggregatesIconRow(dst,
			image.Rect(rect.Max.X-readoutW+2, rect.Min.Y+4, rect.Max.X-2, rect.Max.Y-4),
			state, latches)
	case readoutColModeChevron:
		// Footer chevron — drawn in the footer strip's right edge. The
		// tap target opens a bottom-sheet (UseBottomSheet) carrying the
		// full aggregates. The chevron rect itself is tiny but the
		// sheet expands to fill the viewport.
		chevW := ldv.LevelsReadoutWIcons
		if chevW < 16 {
			chevW = 16
		}
		drawLevelsAggregatesChevron(dst,
			image.Rect(rect.Max.X-chevW-2, rect.Max.Y-footerH, rect.Max.X-2, rect.Max.Y-2))
	}
}

// readoutColMode picks how the Levels-tab aggregate readouts (Headroom
// / Clips / Loudest) collapse when horizontal space shrinks. Phase 4
// audio-panel redesign: replaces the pre-Phase-4 binary "180 px or
// nothing" cascade so mobile users never lose the Headroom number.
type readoutColMode int

const (
	readoutColModeFull readoutColMode = iota
	readoutColModeIcons
	readoutColModeChevron
)

// LevelsAggregate captures the formatted readout values for the Levels
// icon-row. Used by EQPanelZone to surface long-press tooltips with the
// unabbreviated value (e.g., "HEADROOM: 7.3 dB"). The icon row itself
// renders the icon + numeric text without the prose label.
type LevelsAggregate struct {
	HeadroomDB  float64
	HeadroomTxt string
	ClipsTotal  int
	ClipsTxt    string
	LoudestName string
}

// levelsAggregatesValues computes the headroom / clips / loudest readouts
// for the icon-row. Pure function — used by both the renderer and the
// tooltip layer to keep their formatted strings in sync.
func levelsAggregatesValues(state *analyzer.State, latches *MultiLevelsLatch) LevelsAggregate {
	masterPeak := state.Master.PeakDB
	if latches != nil {
		if l, ok := latches.byID["main"]; ok && l != nil && l.peakHoldSeeded && l.PeakHoldDB > masterPeak {
			masterPeak = l.PeakHoldDB
		}
	}
	headroom := -masterPeak
	hdrTxt := "—"
	if headroom > 99 {
		hdrTxt = "99+"
	} else if headroom > 0 {
		hdrTxt = formatMeterDB(headroom)
	}
	totalClips := state.Master.ClipCount
	for i := range state.Instruments {
		totalClips += state.Instruments[i].ClipCount
	}
	clipsTxt := "0"
	if totalClips > 0 {
		clipsTxt = formatClipShort(totalClips)
	}
	loudestName := ""
	loudestPeak := -1e9
	for i := range state.Instruments {
		p := state.Instruments[i].PeakDB
		if latches != nil {
			if l, ok := latches.byID[state.Instruments[i].ID]; ok && l != nil && l.peakHoldSeeded && l.PeakHoldDB > p {
				p = l.PeakHoldDB
			}
		}
		if p > loudestPeak {
			loudestPeak = p
			loudestName = state.Instruments[i].Name
		}
	}
	return LevelsAggregate{
		HeadroomDB:  headroom,
		HeadroomTxt: hdrTxt,
		ClipsTotal:  totalClips,
		ClipsTxt:    clipsTxt,
		LoudestName: loudestName,
	}
}

// levelsAggregateIconRects returns the three sub-rects (Headroom, Clips,
// Loudest) within the icon-row container. Pure positioning — used by both
// the renderer and EQPanelZone for hit-area registration / hover dwell.
func levelsAggregateIconRects(rect image.Rectangle) (hdr, clp, lou image.Rectangle) {
	if rect.Dx() < 20 || rect.Dy() < 40 {
		return
	}
	rowH := rect.Dy() / 3
	if rowH < 14 {
		rowH = 14
	}
	hdr = image.Rect(rect.Min.X, rect.Min.Y, rect.Max.X, rect.Min.Y+rowH)
	clp = image.Rect(rect.Min.X, rect.Min.Y+rowH, rect.Max.X, rect.Min.Y+rowH*2)
	lou = image.Rect(rect.Min.X, rect.Min.Y+rowH*2, rect.Max.X, rect.Min.Y+rowH*3)
	return
}

// drawLevelsAggregatesIconRow paints the three aggregate readouts as
// a vertical icon stack with abbreviated text. Used when the panel
// is too narrow for the full-text column but wide enough to surface
// the per-icon values inline. Each icon's tap target meets the
// density's MinTarget at Spacious.
func drawLevelsAggregatesIconRow(dst *ebiten.Image, rect image.Rectangle, state *analyzer.State, latches *MultiLevelsLatch) {
	if rect.Dx() < 20 || rect.Dy() < 40 {
		return
	}
	agg := levelsAggregatesValues(state, latches)
	headroomCol := meterGreen
	if agg.HeadroomDB < 6 {
		headroomCol = meterYellow
	}
	if agg.HeadroomDB < 1 {
		headroomCol = meterRed
	}
	clipsCol := colTextSecondary
	if agg.ClipsTotal > 0 {
		clipsCol = colError
	}
	captionScale := FontSizeCaption / FontSizeBody
	hdrR, clpR, louR := levelsAggregateIconRects(rect)

	// Icon size: square, vertically centered in the row, leaving the
	// remaining width for the numeric text.
	iconBox := func(r image.Rectangle) (icon image.Rectangle, textX int) {
		side := r.Dy() - 2
		if side < 10 {
			side = 10
		}
		if side > 18 {
			side = 18
		}
		icon = image.Rect(r.Min.X, r.Min.Y+(r.Dy()-side)/2, r.Min.X+side, r.Min.Y+(r.Dy()+side)/2)
		textX = icon.Max.X + 2
		return
	}

	// HEADROOM
	iconR, textX := iconBox(hdrR)
	DrawIcon(dst, IconHeadroom, iconR, colTextSecondary)
	DrawTextColorAtScale(dst, agg.HeadroomTxt, textX, hdrR.Min.Y, headroomCol, captionScale)

	// CLIPS
	iconR, textX = iconBox(clpR)
	DrawIcon(dst, IconClipCount, iconR, clipsCol)
	DrawTextColorAtScale(dst, agg.ClipsTxt, textX, clpR.Min.Y, clipsCol, captionScale)

	// LOUDEST — initials only (e.g. "K" for kick).
	if agg.LoudestName != "" {
		iconR, textX = iconBox(louR)
		DrawIcon(dst, IconLoudest, iconR, colTextSecondary)
		initial := string([]rune(agg.LoudestName)[0])
		DrawTextColorAtScale(dst, initial, textX, louR.Min.Y, colTextPrimary, captionScale)
	}
}

// levelsAggregateTooltipText returns the long-press tooltip text for
// the given aggregate icon slot ("headroom", "clips", or "loudest").
// Returns "" for unknown slots.
func levelsAggregateTooltipText(slot string, agg LevelsAggregate) string {
	switch slot {
	case "headroom":
		if agg.HeadroomDB <= 0 {
			return "Headroom: 0 dB (clipping)"
		}
		return "Headroom: " + agg.HeadroomTxt + " dB"
	case "clips":
		return "Clips (10s): " + agg.ClipsTxt
	case "loudest":
		if agg.LoudestName == "" {
			return "Loudest: —"
		}
		return "Loudest: " + agg.LoudestName
	}
	return ""
}

// drawLevelsAggregatesChevron paints a tiny chevron-down glyph in the
// footer right edge when the panel is too narrow for both the icon-
// row and the full-text column. Tapping it opens a bottom-sheet
// (handled by the EQ-panel zone's existing overlay portal). This
// keeps the aggregates one-tap reachable instead of disappearing
// entirely. Pre-Phase-4 the readouts were silently dropped below
// 360 px; the chevron preserves discoverability.
func drawLevelsAggregatesChevron(dst *ebiten.Image, r image.Rectangle) {
	if r.Empty() {
		return
	}
	drawRoundedRect(dst, r, WithAlpha(colSurface2, AlphaStrong), RadiusXXS, true)
	drawRoundedRect(dst, r, TokenBorderSubtle(), RadiusXXS, false)
	// Chevron-down icon centered in the rect.
	iconW := r.Dx() - 4
	if iconW > r.Dy()-4 {
		iconW = r.Dy() - 4
	}
	iconR := image.Rect(
		r.Min.X+(r.Dx()-iconW)/2,
		r.Min.Y+(r.Dy()-iconW)/2,
		r.Min.X+(r.Dx()+iconW)/2,
		r.Min.Y+(r.Dy()+iconW)/2,
	)
	DrawIcon(dst, IconChevronDown, iconR, colTextSecondary)
}

// formatClipShort renders a clip count compactly: "12", "1k", "9k+"
// for values too large for the 2-character cell.
func formatClipShort(n int) string {
	switch {
	case n >= 9999:
		return "9k+"
	case n >= 1000:
		return "1k+"
	case n >= 100:
		return "99+"
	default:
		return fmt.Sprintf("%d", n)
	}
}

// latchOrNil returns the latch for id, or nil if latches is nil.
func latchOrNil(m *MultiLevelsLatch, id string) *LevelsLatch {
	if m == nil {
		return nil
	}
	return m.Get(id)
}

// drawLevelsChannelStrip renders ONE vertical channel strip: header
// label on top, segmented Peak bar (wide) + RMS bar (narrow) below,
// optional peak-hold marker, optional clip LED, numeric peak readout at
// the bottom. Used by both the per-channel multi-meter view and (with a
// nil latch) by simple chrome contexts that just need a strip.
func drawLevelsChannelStrip(dst *ebiten.Image, rect image.Rectangle, label string, peakDB, rmsDB float64, clipCount int, latch *LevelsLatch) {
	if rect.Dx() < 6 || rect.Dy() < 32 {
		return
	}
	captionScale := FontSizeCaption / FontSizeBody
	const headerH = 12
	const footerH = 18

	// Header label centered — truncated to the strip width so a long
	// instrument name (e.g. "fm-epiano-1") never bleeds into the
	// neighboring strip's label.
	label = truncateName(label, rect.Dx()-2, captionScale)
	lw := int(float64(TextWidth(label)) * captionScale)
	lx := rect.Min.X + (rect.Dx()-lw)/2
	if lx < rect.Min.X+1 {
		lx = rect.Min.X + 1
	}
	DrawTextColorAtScale(dst, label, lx, rect.Min.Y+1, colTextPrimary, captionScale)

	// Clip LED above bars (right side of header band).
	if latch != nil && latch.Latched() {
		ledX := rect.Max.X - 6
		ledY := rect.Min.Y + 2
		drawRect(dst, image.Rect(ledX, ledY, ledX+4, ledY+4), meterClip, true)
	}

	barTop := rect.Min.Y + headerH
	barBot := rect.Max.Y - footerH
	barH := barBot - barTop
	if barH < 4 {
		return
	}
	peakW := rect.Dx() * 60 / 100
	if peakW < 4 {
		peakW = rect.Dx() - 2
	}
	rmsW := rect.Dx() - peakW - 2
	if rmsW < 2 {
		rmsW = 2
	}

	// Bar fills use the held values where higher than instantaneous;
	// see Phase 0a discussion in drawLevelsDetail.
	barPeakDB := peakDB
	barRMSDB := rmsDB
	if latch != nil && latch.peakHoldSeeded && latch.PeakHoldDB > barPeakDB {
		barPeakDB = latch.PeakHoldDB
	}
	if latch != nil && latch.rmsHoldSeeded && latch.RMSHoldDB > barRMSDB {
		barRMSDB = latch.RMSHoldDB
	}

	peakRect := image.Rect(rect.Min.X+1, barTop, rect.Min.X+1+peakW, barBot)
	rmsRect := image.Rect(rect.Min.X+2+peakW, barTop, rect.Min.X+2+peakW+rmsW, barBot)
	drawSegmentedLevelBar(dst, peakRect, barPeakDB, true)
	drawSegmentedLevelBar(dst, rmsRect, barRMSDB, false)

	// Peak-hold marker: a thin horizontal tick at PeakHoldDB across
	// the Peak bar (not the RMS bar — too narrow).
	if latch != nil && latch.peakHoldSeeded && latch.PeakHoldDB > meterDBFloor {
		frac := dbToFrac(latch.PeakHoldDB)
		if frac > 0 {
			markerY := barBot - int(frac*float64(peakRect.Dy()))
			if markerY < barTop {
				markerY = barTop
			}
			drawRect(dst, image.Rect(peakRect.Min.X, markerY, peakRect.Max.X, markerY+1), colTextPrimary, true)
		}
	}

	// Footer: numeric peak readout + clip count.
	footerY := barBot + 2
	readPeak := peakDB
	if latch != nil && latch.peakHoldSeeded && latch.PeakHoldDB > readPeak {
		readPeak = latch.PeakHoldDB
	}
	peakTxt := formatMeterDB(readPeak)
	pw := int(float64(TextWidth(peakTxt)) * captionScale)
	px := rect.Min.X + (rect.Dx()-pw)/2
	DrawTextColorAtScale(dst, peakTxt, px, footerY, colTextSecondary, captionScale)
	if clipCount > 0 || (latch != nil && latch.Latched()) {
		clipTxt := fmt.Sprintf("!%d", clipCount)
		cw := int(float64(TextWidth(clipTxt)) * captionScale)
		cx := rect.Min.X + (rect.Dx()-cw)/2
		DrawTextColorAtScale(dst, clipTxt, cx, footerY+8, colError, captionScale)
	}
}

// drawSegmentedLevelBar paints a vertical level meter with 1.5 dB LED
// segments: green below -6 dB, yellow between -6 and -1 dB, red above.
// `isPeak` controls the segment opacity — RMS uses a softer fill so
// peak transients stand out visually against the body of the signal.
func drawSegmentedLevelBar(dst *ebiten.Image, r image.Rectangle, db float64, isPeak bool) {
	if r.Dx() < 2 || r.Dy() < 4 {
		return
	}
	// Background.
	drawRect(dst, r, meterBg, true)

	if db <= meterDBFloor {
		return
	}
	segDB := 1.5
	totalSegs := int((meterDBCeil - meterDBFloor) / segDB)
	if totalSegs < 4 {
		totalSegs = 4
	}
	// Height-robust LED ladder: in a short meter the fixed 1.5 dB/segment
	// ladder makes every LED sub-2px and nothing draws. Cap the segment count
	// so each LED is ≥2px tall (fewer, taller LEDs when short) and widen segDB
	// to match so the lit-proportion stays correct. Tall meters (≥ ~80px) keep
	// the full 40-segment ladder unchanged.
	if maxSegs := r.Dy() / 2; maxSegs >= 4 && totalSegs > maxSegs {
		totalSegs = maxSegs
		segDB = (meterDBCeil - meterDBFloor) / float64(totalSegs)
	}
	litSegs := int((db - meterDBFloor) / segDB)
	if litSegs > totalSegs {
		litSegs = totalSegs
	}
	if litSegs <= 0 {
		return
	}
	segH := float64(r.Dy()) / float64(totalSegs)
	for s := 0; s < litSegs; s++ {
		// Segment dB midpoint determines its colour.
		segDBVal := meterDBFloor + float64(s)*segDB + segDB/2
		c := meterColor(segDBVal)
		if !isPeak {
			c = withColorAlpha(c, 170)
		}
		y1 := r.Max.Y - int(float64(s)*segH)
		y0 := r.Max.Y - int(float64(s+1)*segH)
		if y0 < r.Min.Y {
			y0 = r.Min.Y
		}
		if y1-y0 < 2 {
			continue
		}
		// Leave a 1-pixel gap between segments to emphasise the LED
		// look — skipped for the bottom-most segment so the bar's
		// foot sits flush on the rect baseline.
		drawRect(dst, image.Rect(r.Min.X, y0, r.Max.X, y1-1), c, true)
	}
}

func withColorAlpha(c color.RGBA, a uint8) color.RGBA {
	c.A = a
	return c
}

// Aggregate-column row indices, in render order top-to-bottom. Used to
// index the slice returned by levelsAggregateColumnRows.
const (
	aggRowHeadroomLabel = iota
	aggRowHeadroomValue
	aggRowClipsLabel
	aggRowClipsValue
	aggRowLUFSLabel
	aggRowLUFSValue
	aggRowLoudestLabel
	aggRowLoudestValue
	aggRowCount
)

// levelsAggRow describes one text row in the Levels aggregate column:
// its top y position and the text scale it renders at.
type levelsAggRow struct {
	Y     int
	Scale float64
}

// levelsAggRowGap is the consistent vertical gap between consecutive
// rows of the aggregate column.
const levelsAggRowGap = 4

// levelsAggregateColumnRows lays out the aggregate column as a
// sequential flow: each row's y is a running cursor advanced by the
// actual rendered text height (textH × the row's scale) plus a
// consistent gap. Rows that would extend past rect.Max.Y are omitted —
// the column truncates rather than overlapping. (The pre-fix layout
// positioned rows with hardcoded absolute offsets from the CLIPS row,
// which made the LUFS-S value row and the LOUDEST label physically
// collide.) Pure function so the layout is unit-testable without an
// Ebiten surface.
func levelsAggregateColumnRows(rect image.Rectangle, textH int) []levelsAggRow {
	captionScale := FontSizeCaption / FontSizeBody
	const (
		bodyScale     = 1.0
		headlineScale = 2.0
	)
	scales := [aggRowCount]float64{
		aggRowHeadroomLabel: captionScale,
		aggRowHeadroomValue: headlineScale,
		aggRowClipsLabel:    captionScale,
		aggRowClipsValue:    bodyScale,
		aggRowLUFSLabel:     captionScale,
		aggRowLUFSValue:     bodyScale,
		aggRowLoudestLabel:  captionScale,
		aggRowLoudestValue:  bodyScale,
	}
	rows := make([]levelsAggRow, 0, aggRowCount)
	y := rect.Min.Y
	for _, s := range scales {
		h := int(math.Ceil(float64(textH) * s))
		if y+h > rect.Max.Y {
			break
		}
		rows = append(rows, levelsAggRow{Y: y, Scale: s})
		y += h + levelsAggRowGap
	}
	return rows
}

// drawLevelsAggregates renders the right-side readout column showing
// Headroom (big, prominent), Clips-last-10s, LUFS-S, and the
// currently-loudest channel name. Pedagogical metadata that consumer
// DAWs put in a side panel; kid-friendly because the headroom number
// answers "how much room do I have left before it gets ugly?" in one
// glance. Row positions come from levelsAggregateColumnRows — a
// sequential flow that never overlaps; rows that don't fit are dropped.
func drawLevelsAggregates(dst *ebiten.Image, rect image.Rectangle, state *analyzer.State, latches *MultiLevelsLatch) {
	if rect.Dx() < 60 || rect.Dy() < 60 {
		return
	}
	rows := levelsAggregateColumnRows(rect, TextHeight())
	rowAt := func(i int) (levelsAggRow, bool) {
		if i < len(rows) {
			return rows[i], true
		}
		return levelsAggRow{}, false
	}

	// Compute headroom: -peak of master (use held value if available).
	masterPeak := state.Master.PeakDB
	if latches != nil {
		if l, ok := latches.byID["main"]; ok && l != nil && l.peakHoldSeeded && l.PeakHoldDB > masterPeak {
			masterPeak = l.PeakHoldDB
		}
	}
	headroom := -masterPeak
	headroomCol := meterGreen
	if headroom < 6 {
		headroomCol = meterYellow
	}
	if headroom < 1 {
		headroomCol = meterRed
	}

	// "Headroom" label.
	if r, ok := rowAt(aggRowHeadroomLabel); ok {
		DrawTextColorAtScale(dst, i18n.T(i18n.KeyCapHeadroom), rect.Min.X, r.Y, colTextSecondary, r.Scale)
	}
	// Big number — render "CLIP!" when peak has exceeded 0 dBFS so
	// the kid-friendly answer ("you're too loud right now") replaces
	// the confusing "negative headroom" math. Phase 5 polish.
	var bigNum string
	switch {
	case headroom < 0:
		bigNum = "CLIP!"
		headroomCol = meterRed
	case headroom > 99:
		bigNum = "+99 dB"
	default:
		bigNum = fmt.Sprintf("%.0f dB", headroom)
	}
	if r, ok := rowAt(aggRowHeadroomValue); ok {
		DrawTextColorAtScale(dst, bigNum, rect.Min.X, r.Y, headroomCol, r.Scale)
	}

	// Rolling 10-second clip count — Phase 2 audio-panel redesign:
	// state.ClipsLastWindow is owned by the audio package's clip-window
	// tracker (audio/loudness.go) and reflects the last 10 s only, so
	// a long session doesn't keep showing an ever-climbing counter.
	// Fall back to the session-total sum when the rolling window has
	// never been populated (zero) AND the session-total is non-zero
	// (stub / pre-wiring builds).
	clipsLast := state.ClipsLastWindow
	totalClips := state.Master.ClipCount
	for i := range state.Instruments {
		totalClips += state.Instruments[i].ClipCount
	}
	clipsDisplay := clipsLast
	clipsLabel := "CLIPS (10s)"
	if clipsLast == 0 && totalClips > 0 {
		clipsDisplay = totalClips
		clipsLabel = "CLIPS"
	}
	if r, ok := rowAt(aggRowClipsLabel); ok {
		DrawTextColorAtScale(dst, clipsLabel, rect.Min.X, r.Y, colTextSecondary, r.Scale)
	}
	clipsTxt := fmt.Sprintf("%d", clipsDisplay)
	clipsCol := colTextPrimary
	if clipsDisplay > 0 {
		clipsCol = colError
	}
	if r, ok := rowAt(aggRowClipsValue); ok {
		DrawTextColorAtScale(dst, clipsTxt, rect.Min.X, r.Y, clipsCol, r.Scale)
	}

	// LUFS-S row — Phase 2: K-weighted short-term loudness from the
	// audio package's integrator. Rendered just below CLIPS so the
	// kid-readable "headroom" + the broadcast "LUFS-S" sit next to
	// each other. Skipped (rendered as "—") when the integrator is
	// silent.
	if r, ok := rowAt(aggRowLUFSLabel); ok {
		DrawTextColorAtScale(dst, "LUFS-S", rect.Min.X, r.Y, colTextSecondary, r.Scale)
	}
	lufsTxt := "—"
	if state.Master.LUFSShortTermDB > -120 {
		lufsTxt = fmt.Sprintf("%.1f", state.Master.LUFSShortTermDB)
	}
	if r, ok := rowAt(aggRowLUFSValue); ok {
		DrawTextColorAtScale(dst, lufsTxt, rect.Min.X, r.Y, colTextPrimary, r.Scale)
	}

	// Loudest channel (by peak).
	loudestName := ""
	loudestPeak := -math.Inf(1)
	for i := range state.Instruments {
		p := state.Instruments[i].PeakDB
		if latches != nil {
			if l, ok := latches.byID[state.Instruments[i].ID]; ok && l != nil && l.peakHoldSeeded && l.PeakHoldDB > p {
				p = l.PeakHoldDB
			}
		}
		if p > loudestPeak {
			loudestPeak = p
			loudestName = state.Instruments[i].Name
		}
	}
	if loudestName == "" {
		return
	}
	if r, ok := rowAt(aggRowLoudestLabel); ok {
		DrawTextColorAtScale(dst, i18n.T(i18n.KeyCapLoudest), rect.Min.X, r.Y, colTextSecondary, r.Scale)
	}
	if r, ok := rowAt(aggRowLoudestValue); ok {
		// "kick-1 · -3 dB" — name plus its peak in dB. The pre-fix
		// "kick-1 (-3)" suffix was cryptic (an unlabeled number).
		loudTxt := fmt.Sprintf("%s · %s dB", loudestName, formatMeterDB(loudestPeak))
		loudTxt = truncateName(loudTxt, rect.Dx(), r.Scale)
		DrawTextColorAtScale(dst, loudTxt, rect.Min.X, r.Y, colTextPrimary, r.Scale)
	}
}
