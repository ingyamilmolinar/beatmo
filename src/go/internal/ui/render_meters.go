package ui

import (
	"fmt"
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

var (
	// Derived in rebuildThemeAliases (theme.go).
	meterLow  color.RGBA
	meterMid  color.RGBA
	meterHigh color.RGBA
	meterBg   color.RGBA
	meterClip color.RGBA
)

const (
	meterDBFloor    = -60.0
	meterDBCeil     = 0.0
	meterYellowDB   = -6.0
	meterRedDB      = -1.0
	meterRMSOpacity = 128 // 50% of 255
	// ~1 s at 60 fps.
	clipLatchFrames = 60
	// ~250 ms at 60 fps before decay starts, the DAW convention.
	peakHoldStickyFrames = 15
	// ≈ 30 dB/s at 60 fps.
	peakHoldDecayDB = 0.5
)

type MultiLevelsLatch struct {
	byID map[channel.ID]*LevelsLatch
}

// NewMultiLevelsLatch returns an initialised map.
func NewMultiLevelsLatch() *MultiLevelsLatch {
	return &MultiLevelsLatch{byID: map[channel.ID]*LevelsLatch{}}
}

// Lets the user acknowledge clips (the Clear Clips pill) instead of waiting for the latch to expire.
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

// For Levels-tab re-entry: the held values froze when the tab was last drawn, so without a reset the
// meters decay from a stale value instead of showing the current audio.
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
func (m *MultiLevelsLatch) Get(id channel.ID) *LevelsLatch {
	if m.byID == nil {
		m.byID = map[channel.ID]*LevelsLatch{}
	}
	if l, ok := m.byID[id]; ok {
		return l
	}
	l := &LevelsLatch{}
	m.byID[id] = l
	return l
}

// The holds rise instantly, stay for peakHoldStickyFrames, then decay slowly: a snapshot between drum hits
// reads -∞ dB and the bar would vanish.
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

// Returns true while the clip latch is active.
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

// Unseeded, so the next Update seeds the holds straight from the live values.
func (l *LevelsLatch) Reset() { *l = LevelsLatch{} }

// Reads without advancing.
func (l *LevelsLatch) Latched() bool { return l.LatchFramesLeft > 0 }

// A nil ch draws only the border; a nil latch means the readout follows the instantaneous clip count.
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
	// The LED stays lit ~1 s after each clip, independent of the live peak.
	if latch != nil && latch.Latched() {
		ledX := rect.Min.X + 2
		ledY := headerRect.Min.Y + 3
		drawRect(dst, image.Rect(ledX, ledY, ledX+8, ledY+8), meterClip, true)
	}

	// Stereo splits the bar into L (top) and R (bottom). Mono fills to the HELD peak, since the analyser
	// sees silence between hits; the live peak is a thin overlay on top.
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
		drawLevelsBar(dst, peakBar, barDB, i18n.T(i18n.KeyLevelsPeak), rect.Min.X+2)
		if latch != nil {
			drawLevelsPeakHoldMarker(dst, peakBar, latch.PeakHoldDB)
		}
	}

	// Same treatment: stereo split, mono fills to the held RMS.
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

	// Side label (caption role — styled Inter, never the scaled bitmap font).
	lh := StyledTextHeight(RoleCaption)
	labelY := rect.Min.Y + (rect.Dy()-lh)/2
	DrawTextStyled(dst, label, labelX, labelY, RoleCaption, colTextSecondary)

	// Level fill.
	frac := dbToFrac(db)
	if frac > 0 {
		fillX1 := rect.Min.X + int(frac*float64(rect.Dx()))
		fillCol := meterColor(db)
		drawRect(dst, image.Rect(rect.Min.X, rect.Min.Y+1, fillX1, rect.Max.Y-1), fillCol, true)
	}
}

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

func drawLevelsScale(dst *ebiten.Image, rect image.Rectangle) {
	tickCol := WithAlpha(genColorBorder, AlphaPanelBorder)
	for _, db := range []float64{-48, -24, -12, -6, 0} {
		frac := dbToFrac(db)
		x := rect.Min.X + int(frac*float64(rect.Dx()))
		drawRect(dst, image.Rect(x, rect.Min.Y+rect.Dy()/2, x+1, rect.Max.Y), tickCol, true)
		label := fmt.Sprintf("%.0f", db)
		lw := StyledTextWidth(label, RoleMicro)
		DrawTextStyled(dst, label, x-lw/2, rect.Min.Y, RoleMicro, colTextSecondary)
	}
}

func drawLevelsReadout(dst *ebiten.Image, rect image.Rectangle, ch *analyzer.ChannelMetrics, latch *LevelsLatch) {
	lh := StyledTextHeight(RoleMicro)
	y := rect.Min.Y + (rect.Dy()-lh)/2

	// Reads the held values: the live ones are -∞ on most frames, as the 512-sample window catches the
	// silence between hits.
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

	// Static prefix (localized; the clip COUNT is drawn separately, colored).
	prefix := i18n.Tf(i18n.KeyLevelsReadoutFmt,
		formatMeterDB(peakReadout),
		formatMeterDB(rmsReadout),
		formatMeterDB(headroom),
	)
	// Clip number with optional latch coloring.
	clipsText := fmt.Sprintf("%d", ch.ClipCount)
	clipsCol := colTextSecondary
	if latch != nil && latch.Latched() {
		clipsCol = colError
	} else if ch.ClipCount > 0 {
		clipsCol = colError
	}

	// Roles render at a fixed size, so a long locale truncates with an ellipsis instead of shrinking.
	microW := func(s string) int { return StyledTextWidth(s, RoleMicro) }
	maxPrefixW := rect.Dx() - 8 - microW(clipsText)
	prefix = clipTextToWidthMeasure(prefix, maxPrefixW, microW)
	DrawTextStyled(dst, prefix, rect.Min.X+4, y, RoleMicro, colTextSecondary)
	DrawTextStyled(dst, clipsText, rect.Min.X+4+microW(prefix), y, RoleMicro, clipsCol)
}

func fitScaleToWidth(naturalW int, base float64, maxW int) float64 {
	if naturalW <= 0 || maxW <= 0 {
		return base
	}
	if int(float64(naturalW)*base) <= maxW {
		return base
	}
	return float64(maxW) / float64(naturalW)
}

// "-inf" because the font may lack ∞.
func formatMeterDB(db float64) string {
	if db <= meterDBFloor {
		return "-inf"
	}
	return fmt.Sprintf("%.0f", db)
}

// meterColor returns the meter color for a given peak dB level.
func meterColor(db float64) color.RGBA {
	if db > meterRedDB {
		return meterHigh
	}
	if db > meterYellowDB {
		return meterMid
	}
	return meterLow
}

// meterColorAlpha returns the meter color with a custom alpha for overlays.
func meterColorAlpha(db float64, alpha uint8) color.NRGBA {
	return WithAlpha(meterColor(db), alpha)
}

// latches are keyed by channel ("main" for master) and the caller advances them first. With no
// instruments it falls back to drawLevelsDetail.
func drawLevelsMultiChannel(dst *ebiten.Image, rect image.Rectangle, state *analyzer.State, latches *MultiLevelsLatch, visibleIDs map[channel.ID]bool) {
	drawRect(dst, rect, colButtonBorder, false)
	if state == nil {
		return
	}

	if len(state.Instruments) == 0 {
		drawLevelsDetail(dst, rect, &state.Master, latchOrNil(latches, channel.Master))
		return
	}

	const (
		footerH        = 18 // dB scale + numeric readouts
		headerH        = 12 // channel-name label
		stripGap       = 4
		masterStripPad = 8
	)
	// Three readout modes by width: the full column, an icon row, or a footer chevron one tap away, so a narrow
	// panel never loses Headroom.
	ldv := Profile().DensityValues()

	contentH := rect.Dy() - headerH - footerH
	if contentH < 24 {
		drawLevelsDetail(dst, rect, &state.Master, latchOrNil(latches, channel.Master))
		return
	}

	readoutMode, readoutW := levelsReadoutLayout(rect.Dx())
	stripsRect := image.Rect(rect.Min.X+4, rect.Min.Y, rect.Max.X-readoutW, rect.Max.Y)

	nInst := 0
	for i := range state.Instruments {
		if visibleIDs == nil {
			nInst++
			continue
		}
		if _, present := visibleIDs[state.Instruments[i].ID]; present {
			nInst++
		}
	}
	if nInst == 0 {
		// Nothing audible among instruments — fall back to master-only detail.
		drawLevelsDetail(dst, rect, &state.Master, latchOrNil(latches, channel.Master))
		return
	}
	totalWeights := nInst + 2 // master = 2 weights
	stripSpace := stripsRect.Dx() - (nInst)*stripGap - masterStripPad
	if stripSpace < totalWeights {
		drawLevelsDetail(dst, rect, &state.Master, latchOrNil(latches, channel.Master))
		return
	}
	weightPx := stripSpace / totalWeights
	if weightPx < 14 {
		weightPx = 14
	}

	x := stripsRect.Min.X
	for i := range state.Instruments {
		inst := &state.Instruments[i]
		if visibleIDs != nil {
			if _, present := visibleIDs[inst.ID]; !present {
				// Not audible (solo/mute): skipped so the others reclaim its width.
				continue
			}
		}
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
			i18n.T(i18n.KeyMaster), state.Master.PeakDB, state.Master.RMSDB, state.Master.ClipCount,
			latchOrNil(latches, channel.Master))
	}

	switch readoutMode {
	case readoutColModeFull:
		drawLevelsAggregates(dst,
			image.Rect(rect.Max.X-readoutW+4, rect.Min.Y+4, rect.Max.X-4, rect.Max.Y-4),
			state, latches, visibleIDs)
	case readoutColModeIcons:
		drawLevelsAggregatesIconRow(dst,
			image.Rect(rect.Max.X-readoutW+2, rect.Min.Y+4, rect.Max.X-2, rect.Max.Y-4),
			state, latches, visibleIDs)
	case readoutColModeChevron:
		// The chevron is tiny, but it opens a bottom sheet with the full aggregates.
		chevW := ldv.LevelsReadoutWIcons
		if chevW < 16 {
			chevW = 16
		}
		drawLevelsAggregatesChevron(dst,
			image.Rect(rect.Max.X-chevW-2, rect.Max.Y-footerH, rect.Max.X-2, rect.Max.Y-2))
	}
}

type readoutColMode int

const (
	readoutColModeFull readoutColMode = iota
	readoutColModeIcons
	readoutColModeChevron
)

// Holds the unabbreviated values for the long-press tooltip; the icon row shows only icon + number.
type LevelsAggregate struct {
	HeadroomDB  float64
	HeadroomTxt string
	ClipsTotal  int
	ClipsTxt    string
	LoudestName string
}

// nil visibleIDs includes all.
func instrumentVisibleInAgg(visibleIDs map[channel.ID]bool, id channel.ID) bool {
	return visibleIDs == nil || visibleIDs[id]
}

// The 4 px inset per side drawLevelsAggregates draws into, plus slack.
const levelsReadoutFullPadX = 12

const levelsReadoutFullMinW = 68

// Independent of instrument count and of the per-frame loudest name (which truncates), so the summary
// stays a fixed width and the bars take the rest. Never wider than the density's budget.
func levelsReadoutFullW() int {
	w := 0
	cands := []struct {
		s    string
		role TextRole
	}{
		{i18n.T(i18n.KeyCapHeadroom), RoleCaption},
		{i18n.T(i18n.KeyLevelsClips) + " (10s)", RoleCaption},
		{"LUFS-S", RoleCaption},
		{i18n.T(i18n.KeyCapLoudest), RoleCaption},
		{"+99 dB", RolePanelTitle}, // widest big number
		{"CLIP!", RolePanelTitle},
	}
	for _, c := range cands {
		if tw := StyledTextWidth(c.s, c.role); tw > w {
			w = tw
		}
	}
	w += levelsReadoutFullPadX
	if w < levelsReadoutFullMinW {
		w = levelsReadoutFullMinW
	}
	if budget := Profile().DensityValues().LevelsReadoutWFull; w > budget {
		w = budget
	}
	return w
}

// The mode thresholds stay on the density tokens (so a phone lands in icon-row mode), but the full column
// renders at its content-fit width.
func levelsReadoutLayout(panelDx int) (readoutColMode, int) {
	ldv := Profile().DensityValues()
	full := ldv.LevelsReadoutWFull
	icons := ldv.LevelsReadoutWIcons
	switch {
	case panelDx >= 2*full:
		return readoutColModeFull, levelsReadoutFullW()
	case panelDx >= full+icons:
		return readoutColModeIcons, icons
	default:
		return readoutColModeChevron, 0
	}
}

// Shared by the renderer and the tooltip so their strings agree.
func levelsAggregatesValues(state *analyzer.State, latches *MultiLevelsLatch, visibleIDs map[channel.ID]bool) LevelsAggregate {
	masterPeak := state.Master.PeakDB
	if latches != nil {
		if l, ok := latches.byID[channel.Master]; ok && l != nil && l.peakHoldSeeded && l.PeakHoldDB > masterPeak {
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
		if !instrumentVisibleInAgg(visibleIDs, state.Instruments[i].ID) {
			continue
		}
		totalClips += state.Instruments[i].ClipCount
	}
	clipsTxt := "0"
	if totalClips > 0 {
		clipsTxt = formatClipShort(totalClips)
	}
	loudestName := ""
	loudestPeak := -1e9
	for i := range state.Instruments {
		if !instrumentVisibleInAgg(visibleIDs, state.Instruments[i].ID) {
			continue
		}
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

// Shared by the renderer and the panel's hit areas.
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

// Each icon's tap target meets MinTarget at Spacious.
func drawLevelsAggregatesIconRow(dst *ebiten.Image, rect image.Rectangle, state *analyzer.State, latches *MultiLevelsLatch, visibleIDs map[channel.ID]bool) {
	if rect.Dx() < 20 || rect.Dy() < 40 {
		return
	}
	agg := levelsAggregatesValues(state, latches, visibleIDs)
	headroomCol := meterLow
	if agg.HeadroomDB < 6 {
		headroomCol = meterMid
	}
	if agg.HeadroomDB < 1 {
		headroomCol = meterHigh
	}
	clipsCol := colTextSecondary
	if agg.ClipsTotal > 0 {
		clipsCol = colError
	}
	hdrR, clpR, louR := levelsAggregateIconRects(rect)

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
	DrawTextStyled(dst, agg.HeadroomTxt, textX, hdrR.Min.Y, RoleCaption, headroomCol)

	// CLIPS
	iconR, textX = iconBox(clpR)
	DrawIcon(dst, IconClipCount, iconR, clipsCol)
	DrawTextStyled(dst, agg.ClipsTxt, textX, clpR.Min.Y, RoleCaption, clipsCol)

	// LOUDEST — initials only (e.g. "K" for kick).
	if agg.LoudestName != "" {
		iconR, textX = iconBox(louR)
		DrawIcon(dst, IconLoudest, iconR, colTextSecondary)
		initial := string([]rune(agg.LoudestName)[0])
		DrawTextStyled(dst, initial, textX, louR.Min.Y, RoleCaption, colTextPrimary)
	}
}

// "" for an unknown slot.
func levelsAggregateTooltipText(slot string, agg LevelsAggregate) string {
	switch slot {
	case "headroom":
		if agg.HeadroomDB <= 0 {
			return i18n.T(i18n.KeyLevelsTipHeadroomClip)
		}
		return i18n.Tf(i18n.KeyLevelsTipHeadroom, agg.HeadroomTxt)
	case "clips":
		return i18n.Tf(i18n.KeyLevelsTipClips, agg.ClipsTxt)
	case "loudest":
		if agg.LoudestName == "" {
			return i18n.T(i18n.KeyLevelsTipLoudestNone)
		}
		return i18n.Tf(i18n.KeyLevelsTipLoudest, agg.LoudestName)
	}
	return ""
}

// Keeps the aggregates reachable when the panel is too narrow for both other modes.
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

// Fits the 2-character cell.
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
func latchOrNil(m *MultiLevelsLatch, id channel.ID) *LevelsLatch {
	if m == nil {
		return nil
	}
	return m.Get(id)
}

// A nil latch serves chrome contexts that need a strip and nothing else.
func drawLevelsChannelStrip(dst *ebiten.Image, rect image.Rectangle, label string, peakDB, rmsDB float64, clipCount int, latch *LevelsLatch) {
	if rect.Dx() < 6 || rect.Dy() < 32 {
		return
	}
	// Both bands derive from the styled line heights so the bars always start below the text.
	microH := StyledTextHeight(RoleMicro)
	headerH := StyledTextHeight(RoleCaption) + 2
	footerH := microH*2 + 2

	// Truncated so a long name never bleeds into the next strip.
	capW := func(s string) int { return StyledTextWidth(s, RoleCaption) }
	label = clipTextToWidthMeasure(label, rect.Dx()-2, capW)
	lw := capW(label)
	lx := rect.Min.X + (rect.Dx()-lw)/2
	if lx < rect.Min.X+1 {
		lx = rect.Min.X + 1
	}
	DrawTextStyled(dst, label, lx, rect.Min.Y+1, RoleCaption, colTextPrimary)

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

	// The held value when higher, so the bar survives between hits.
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

	// Not on the RMS bar: too narrow.
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
	pw := StyledTextWidth(peakTxt, RoleMicro)
	px := rect.Min.X + (rect.Dx()-pw)/2
	DrawTextStyled(dst, peakTxt, px, footerY, RoleMicro, colTextSecondary)
	if clipCount > 0 || (latch != nil && latch.Latched()) {
		clipTxt := fmt.Sprintf("!%d", clipCount)
		cw := StyledTextWidth(clipTxt, RoleMicro)
		cx := rect.Min.X + (rect.Dx()-cw)/2
		DrawTextStyled(dst, clipTxt, cx, footerY+microH, RoleMicro, colError)
	}
}

// The RMS bar is softer so peak transients stand out against the body of the signal.
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
	// A short meter would make every 1.5 dB LED sub-2 px and draw nothing: cap the segment count and widen
	// segDB so the lit proportion stays right.
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
		// No gap under the bottom segment, so the bar's foot sits on the baseline.
		drawRect(dst, image.Rect(r.Min.X, y0, r.Max.X, y1-1), c, true)
	}
}

func withColorAlpha(c color.RGBA, a uint8) color.RGBA {
	c.A = a
	return c
}

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

type levelsAggRow struct {
	Y    int
	Role TextRole
}

const levelsAggRowGap = 4

// A running cursor: rows that would pass rect.Max.Y are dropped, so the column truncates instead of
// overlapping.
func levelsAggregateColumnRows(rect image.Rectangle) []levelsAggRow {
	roles := [aggRowCount]TextRole{
		aggRowHeadroomLabel: RoleCaption,
		aggRowHeadroomValue: RolePanelTitle,
		aggRowClipsLabel:    RoleCaption,
		aggRowClipsValue:    RoleBody,
		aggRowLUFSLabel:     RoleCaption,
		aggRowLUFSValue:     RoleBody,
		aggRowLoudestLabel:  RoleCaption,
		aggRowLoudestValue:  RoleBody,
	}
	rows := make([]levelsAggRow, 0, aggRowCount)
	y := rect.Min.Y
	for _, role := range roles {
		h := StyledTextHeight(role)
		if y+h > rect.Max.Y {
			break
		}
		rows = append(rows, levelsAggRow{Y: y, Role: role})
		y += h + levelsAggRowGap
	}
	return rows
}

// The headroom number answers "how much room before it gets ugly?" at a glance.
func drawLevelsAggregates(dst *ebiten.Image, rect image.Rectangle, state *analyzer.State, latches *MultiLevelsLatch, visibleIDs map[channel.ID]bool) {
	if rect.Dx() < 60 || rect.Dy() < 60 {
		return
	}
	rows := levelsAggregateColumnRows(rect)
	rowAt := func(i int) (levelsAggRow, bool) {
		if i < len(rows) {
			return rows[i], true
		}
		return levelsAggRow{}, false
	}

	// Compute headroom: -peak of master (use held value if available).
	masterPeak := state.Master.PeakDB
	if latches != nil {
		if l, ok := latches.byID[channel.Master]; ok && l != nil && l.peakHoldSeeded && l.PeakHoldDB > masterPeak {
			masterPeak = l.PeakHoldDB
		}
	}
	headroom := -masterPeak
	headroomCol := meterLow
	if headroom < 6 {
		headroomCol = meterMid
	}
	if headroom < 1 {
		headroomCol = meterHigh
	}

	// "Headroom" label.
	if r, ok := rowAt(aggRowHeadroomLabel); ok {
		DrawTextStyled(dst, i18n.T(i18n.KeyCapHeadroom), rect.Min.X, r.Y, r.Role, colTextSecondary)
	}
	// "CLIP!" above 0 dBFS instead of a negative headroom nobody can read.
	var bigNum string
	switch {
	case headroom < 0:
		bigNum = "CLIP!"
		headroomCol = meterHigh
	case headroom > 99:
		bigNum = "+99 dB"
	default:
		bigNum = fmt.Sprintf("%.0f dB", headroom)
	}
	if r, ok := rowAt(aggRowHeadroomValue); ok {
		DrawTextStyled(dst, bigNum, rect.Min.X, r.Y, r.Role, headroomCol)
	}

	// The audio package's rolling 10 s count, so a long session does not show an ever-climbing total; the
	// session total stands in only when the window was never populated.
	clipsLast := state.ClipsLastWindow
	totalClips := state.Master.ClipCount
	for i := range state.Instruments {
		if !instrumentVisibleInAgg(visibleIDs, state.Instruments[i].ID) {
			continue
		}
		totalClips += state.Instruments[i].ClipCount
	}
	clipsDisplay := clipsLast
	clipsLabel := i18n.T(i18n.KeyLevelsClips) + " (10s)"
	if clipsLast == 0 && totalClips > 0 {
		clipsDisplay = totalClips
		clipsLabel = i18n.T(i18n.KeyLevelsClips)
	}
	if r, ok := rowAt(aggRowClipsLabel); ok {
		DrawTextStyled(dst, clipsLabel, rect.Min.X, r.Y, r.Role, colTextSecondary)
	}
	clipsTxt := fmt.Sprintf("%d", clipsDisplay)
	clipsCol := colTextPrimary
	if clipsDisplay > 0 {
		clipsCol = colError
	}
	if r, ok := rowAt(aggRowClipsValue); ok {
		DrawTextStyled(dst, clipsTxt, rect.Min.X, r.Y, r.Role, clipsCol)
	}

	// "—" while the integrator is silent.
	if r, ok := rowAt(aggRowLUFSLabel); ok {
		DrawTextStyled(dst, "LUFS-S", rect.Min.X, r.Y, r.Role, colTextSecondary)
	}
	lufsTxt := "—"
	if state.Master.LUFSShortTermDB > -120 {
		lufsTxt = fmt.Sprintf("%.1f", state.Master.LUFSShortTermDB)
	}
	if r, ok := rowAt(aggRowLUFSValue); ok {
		DrawTextStyled(dst, lufsTxt, rect.Min.X, r.Y, r.Role, colTextPrimary)
	}

	// Loudest channel (by peak).
	loudestName := ""
	loudestPeak := -math.Inf(1)
	for i := range state.Instruments {
		if !instrumentVisibleInAgg(visibleIDs, state.Instruments[i].ID) {
			continue
		}
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
		DrawTextStyled(dst, i18n.T(i18n.KeyCapLoudest), rect.Min.X, r.Y, r.Role, colTextSecondary)
	}
	if r, ok := rowAt(aggRowLoudestValue); ok {
		// The unit makes the number readable; a bare "(-3)" was not.
		loudTxt := fmt.Sprintf("%s · %s dB", loudestName, formatMeterDB(loudestPeak))
		loudTxt = clipTextToWidthMeasure(loudTxt, rect.Dx(),
			func(s string) int { return StyledTextWidth(s, r.Role) })
		DrawTextStyled(dst, loudTxt, rect.Min.X, r.Y, r.Role, colTextPrimary)
	}
}
