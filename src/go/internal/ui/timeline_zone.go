package ui

import (
	"image"
	"math"
	"strconv"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

// Package constants so layout tests reproduce the exact pill math drawBeatCounter uses.
const (
	beatCounterPillPadX = 8
	beatCounterPillPadY = 3
)

type TimelineCallbacks struct {
	// Read-only accessors
	Rows                 func() []*DrumRow
	IsPlaying            func() bool
	Follow               func() bool
	BPM                  func() int
	SecPerBeat           func() float64
	TimelineUnitsPerBeat func() int
	Length               func() int
	Offset               func() int
	RowOffset            func() int
	VisibleRows          func() int
	RowHeight            func() int
	Cell                 func() int
	TimelineBeats        func() int
	Frame                func() int64

	// Per-frame draw data
	BeatLength      func() int             // Graph.BeatLength()
	SimpleDraw      func() bool            // low-overhead highlight path
	PerfDrawLite    func() bool            // skip expensive chrome
	MobileEQActive  func() bool            // mobile EQ mode active
	BeatCounterRect func() image.Rectangle // beat counter display rect
	RowsTopY        func() int             // Y coordinate where rows start (Bounds.Min.Y + headerH)

	// Action callbacks
	OnOffsetChange   func(newOffset int)
	OnSeekToStep     func(step int, origin hooks.SeekOrigin) // ribbon seek commit
	PlayheadStep     func() int                              // current absolute subdivision
	OnRowsLayerDirty func()                                  // signal that the rows layer composite needs rebuild
	SetTimelineBeats func(int)
	OnOpenNavigation func() // beat-counter tap: open the navigation menu (dv.OpenNavigation)

	// Row scroll forwarding (vertical scroll over grid)
	OnRowScrollWheel func(steps int) bool
	OnRowScrollDrag  func(targetRowOffset int)

	// Stays on DrumView: deeply coupled to its offset caching and geometry.
	DrawRowComposite func(dst *ebiten.Image)
}

type TimelineZone struct {
	rect       image.Rectangle
	needLayout bool
	// layouts counts Layout calls (recalcButtons' layout gate is judged by it).
	layouts   int
	callbacks TimelineCallbacks
	portal    *OverlayPortal

	// Computed sub-rects (set during Layout)
	stepsRect       image.Rectangle // grid area for drag scrolling
	timelineBarRect image.Rectangle // progress bar for scrub

	// Drag state (grid horizontal scroll)
	dragging    bool
	dragStartX  int
	startOffset int

	// Scrub state (timeline bar seek)
	scrubbing      bool
	scrubGhostBeat int // beat under the pointer during a scrub drag
	// A preview only: nothing seeks until Go. Absolute subdivision step.
	gotoGhost      bool
	gotoGhostStep  int
	scrubPressBeat int // beat committed on press (so a tap re-seeks at most once)

	// Timeline bar height (set by DrumView before Layout)
	timelineBarH int
	// Decouples the bar's position from the clip top, so the clip can grow upward over the counter
	// chrome without moving the bar.
	explicitBarRect image.Rectangle
	cellGridRect    image.Rectangle // X anchor for row step cells (see SetCellGridRect)

	// DrumView owns these buttons; the zone only registers their hit areas.
	trackBtn  *Button
	lenDecBtn *Button
	lenIncBtn *Button

	// Hit areas cache (rebuilt on Layout)
	hitAreas []HitArea

	// Per-frame draw state (set by DrumView before Draw)
	elapsedBeats    float64
	highlightsByRow [][]highlightEntry

	// Highlight sprite cache
	hlSpriteReg  *ebiten.Image
	hlSpriteMute *ebiten.Image
	hlSpriteH    int
	// --- Per-row sprite cache (owned by zone, aliased to DrumView) ---

	// Timeline base cache (background + beat markers)
	TlCache      *ebiten.Image
	TlCacheW     int
	TlCacheH     int
	TlCacheBeats int
	// TlCacheThemeGen is the ThemeGeneration the ribbon was baked under.
	TlCacheThemeGen int
	TlCacheStep     int
	LastInfoCurMS   int
	LastInfoTotMS   int
	LastInfoText    string

	// Per-row cached sprites
	RowCache        []*ebiten.Image
	RowDirty        []bool
	RowFullDirty    []bool
	RowCacheOff     []int
	RowCacheGen     []int
	RowCacheSig     []uint64
	RowCacheSteps   [][]bool
	RowCacheTypes   [][]model.NodeType
	RowCacheScratch []*ebiten.Image

	// Segmented timeline slices
	TimelineOffset    []int
	TimelinePast      [][]bool
	TimelinePastTypes [][]model.NodeType
	TimelinePresent   [][]bool
	TimelineFuture    [][]bool

	// Row frame/repaint tracking
	RowFrame   []int64
	RowRepaint []int

	// Row draw mask for visibility assertions
	RowsDrawnMask []bool
}

// NewTimelineZone creates a TimelineZone with the provided callbacks.
func NewTimelineZone(cb TimelineCallbacks) *TimelineZone {
	return &TimelineZone{
		needLayout: true,
		callbacks:  cb,
	}
}

// SetPortal sets the overlay portal reference (for consistency with other zones).
func (z *TimelineZone) SetPortal(p *OverlayPortal) { z.portal = p }

// Keeps the ribbon a slim whole-song bar; a clamp over the density token, not a token change.
const timelineBarHMax = 8

// Call before Layout.
func (z *TimelineZone) SetTimelineBarHeight(h int) {
	if h > timelineBarHMax {
		h = timelineBarHMax
	}
	z.timelineBarH = h
}

// Needed when the clip extends above the bar; the zero rect puts the bar at the top of the zone.
func (z *TimelineZone) SetTimelineBarRect(r image.Rectangle) { z.explicitBarRect = r }

// Separate from the bar: the bar reserves header chrome and maps the whole song, the cells map the
// window and fill the full grid width.
func (z *TimelineZone) SetCellGridRect(r image.Rectangle) { z.cellGridRect = r }

// Falls back to the bar rect when no grid rect was supplied (standalone tests).
func (z *TimelineZone) cellGridAnchor() image.Rectangle {
	if z.cellGridRect.Empty() {
		return z.timelineBarRect
	}
	return z.cellGridRect
}

// --- Zone interface ---

func (z *TimelineZone) ID() string          { return "timeline" }
func (z *TimelineZone) NeedsLayout() bool   { return z.needLayout }
func (z *TimelineZone) Invalidate()         { z.needLayout = true }
func (z *TimelineZone) HitAreas() []HitArea { return z.hitAreas }

func (z *TimelineZone) Layout(rect image.Rectangle) {
	z.rect = rect
	z.needLayout = false
	z.layouts++

	barH := z.timelineBarH
	if barH <= 0 {
		barH = tlBarHeight()
		if barH > timelineBarHMax {
			barH = timelineBarHMax
		}
	}

	if !z.explicitBarRect.Empty() {
		// Verbatim, so the clip can extend around the bar without shifting it.
		z.timelineBarRect = z.explicitBarRect
	} else {
		// Legacy: bar lives at the TOP of the zone rect.
		z.timelineBarRect = image.Rect(rect.Min.X, rect.Min.Y, rect.Max.X, rect.Min.Y+barH)
		if z.timelineBarRect.Max.Y > rect.Max.Y {
			z.timelineBarRect.Max.Y = rect.Max.Y
		}
	}

	// stepsRect: the grid area where rows are drawn (below the timeline bar).
	z.stepsRect = image.Rect(rect.Min.X, z.timelineBarRect.Max.Y, rect.Max.X, rect.Max.Y)
	if z.stepsRect.Dy() < 0 {
		z.stepsRect.Max.Y = z.stepsRect.Min.Y
	}

	z.rebuildHitAreas()
}

func (z *TimelineZone) Update() {
}

// SetDrawParams sets per-frame draw data before Draw() is called.
func (z *TimelineZone) SetDrawParams(elapsedBeats float64, highlights [][]highlightEntry) {
	z.elapsedBeats = elapsedBeats
	z.highlightsByRow = highlights
}

func (z *TimelineZone) Draw(screen *ebiten.Image) {
	mobileEQActive := z.callbacks.MobileEQActive != nil && z.callbacks.MobileEQActive()
	simpleDraw := z.callbacks.SimpleDraw != nil && z.callbacks.SimpleDraw()
	perfDrawLite := z.callbacks.PerfDrawLite != nil && z.callbacks.PerfDrawLite()
	elapsedBeats := z.elapsedBeats

	// --- Timeline beats computation (high-water mark) ---
	z.computeTimelineBeats(elapsedBeats)
	totalBeats := z.callbacks.TimelineBeats()

	// --- Timeline bar rendering ---
	z.drawTimelineBar(screen, elapsedBeats, totalBeats)

	// --- Beat/time counter ---
	if !simpleDraw && !perfDrawLite {
		z.drawBeatCounter(screen, elapsedBeats)
	}

	// --- Len +/- buttons ---
	if z.lenIncBtn != nil {
		z.lenIncBtn.Draw(screen)
	}
	if z.lenDecBtn != nil {
		z.lenDecBtn.Draw(screen)
	}

	// Rect-driven: empty on mobile, where the toggle lives in the overflow menu.
	if z.trackBtn != nil && !z.trackBtn.Rect().Empty() {
		z.trackBtn.Draw(screen)
	}

	// --- Row composite rendering (delegated to DrumView) ---
	if !mobileEQActive {
		if z.callbacks.DrawRowComposite != nil {
			z.callbacks.DrawRowComposite(screen)
		}

		// --- Gap fill below visible rows ---
		z.drawGapFill(screen)

		// --- Row highlights ---
		z.drawHighlights(screen, simpleDraw)

		// --- Mute/Solo dimming ---
		z.drawMuteSoloDimming(screen)
	}
}

// Long enough for the graph, the playhead and the visible window.
func (z *TimelineZone) computeTimelineBeats(elapsedBeats float64) {
	isPlaying := z.callbacks.IsPlaying != nil && z.callbacks.IsPlaying()
	timelineBeats := z.callbacks.TimelineBeats()
	unitsPerBeat := max1(z.callbacks.TimelineUnitsPerBeat())
	length := z.callbacks.Length()
	offset := z.callbacks.Offset()

	if !isPlaying && z.callbacks.BeatLength != nil {
		units := z.callbacks.BeatLength()
		beats := int(math.Ceil(float64(units) / float64(unitsPerBeat)))
		if timelineBeats < beats {
			timelineBeats = beats
		}
	}
	units := float64(unitsPerBeat)
	lengthBeats := float64(length) / units
	// elapsedBeats is already in beats.
	needBeats1 := int(math.Ceil(elapsedBeats + lengthBeats))
	offsetBeats := float64(offset) / units
	needBeats2 := int(math.Ceil(offsetBeats + lengthBeats))
	if needBeats1 > timelineBeats {
		timelineBeats = needBeats1
	}
	if needBeats2 > timelineBeats {
		timelineBeats = needBeats2
	}
	if z.callbacks.SetTimelineBeats != nil {
		z.callbacks.SetTimelineBeats(timelineBeats)
	}
}

// Ticks span the whole song [0, totalBeats], independent of the drum window, with the cursor at
// elapsed/total; the ribbon is drag-only, so nothing implies click-to-seek.
func (z *TimelineZone) drawTimelineBar(dst *ebiten.Image, elapsedBeats float64, totalBeats int) {
	barRect := z.timelineBarRect
	if barRect.Empty() {
		return
	}
	beats := max1(totalBeats)
	pxPerBeat := float64(barRect.Dx()) / float64(beats)

	// The whole-song span only grows, so this rarely rebuilds.
	cacheKeyW := barRect.Dx()
	cacheKeyH := barRect.Dy()
	// A size change allocates; a content change reuses the image. Dropping images without Deallocate
	// filled WASM's atlas packing tree until the heap ran out.
	dimChanged := z.TlCache == nil || z.TlCacheW != cacheKeyW || z.TlCacheH != cacheKeyH
	// The ribbon bakes theme colors, so a theme change is a content change.
	contentChanged := dimChanged || z.TlCacheBeats != beats || z.TlCacheThemeGen != ThemeGeneration()
	if dimChanged {
		if z.TlCache != nil {
			z.TlCache.Deallocate()
		}
		z.TlCache = newTrackedImage("timelineZone.TlCache", cacheKeyW, cacheKeyH)
		z.TlCacheW, z.TlCacheH = cacheKeyW, cacheKeyH
	}
	if contentChanged {
		z.TlCacheBeats, z.TlCacheStep = beats, 0
		z.TlCacheThemeGen = ThemeGeneration()
		if !dimChanged {
			z.TlCache.Clear()
		}
		drawRect(z.TlCache, image.Rect(0, 0, z.TlCacheW, z.TlCacheH), colTimelineTotal, true)
		// Baked into the cache, alloc-free per frame.
		drawRect(z.TlCache, image.Rect(0, 0, z.TlCacheW, z.TlCacheH), WithAlpha(genColorTimelineView, AlphaFaint), true)
		z.drawRibbonTicks(z.TlCache, 0, pxPerBeat)
	}
	var op ebiten.DrawImageOptions
	op.GeoM.Translate(float64(barRect.Min.X), float64(barRect.Min.Y))
	dst.DrawImage(z.TlCache, &op)

	// Clamped to the bar.
	frac := elapsedBeats / float64(beats)
	if frac < 0 {
		frac = 0
	} else if frac > 1 {
		frac = 1
	}
	cursorX := barRect.Min.X + int(math.Round(frac*float64(barRect.Dx())))
	cursorCol := colTimelineCursor
	cursorThick := 1
	if Profile().IsMobile() {
		cursorCol = colAccentBright
		cursorThick = 2
	}
	cursorRect := image.Rect(cursorX-cursorThick, barRect.Min.Y, cursorX+cursorThick, barRect.Max.Y)
	drawRect(dst, cursorRect, cursorCol, true)
	// 1px brighter core so the playhead is findable against the warm ticks.
	coreRect := image.Rect(cursorX, barRect.Min.Y, cursorX+1, barRect.Max.Y)
	drawRect(dst, coreRect, colAccentBright, true)

	drawRect(dst, barRect, colButtonBorder, false)

	if z.scrubbing {
		z.drawScrubGhost(dst, z.scrubGhostBeat)
	} else if z.gotoGhost {
		z.drawScrubGhost(dst, z.gotoGhostStep)
	}
}

// A preview only: nothing seeks.
func (z *TimelineZone) SetGotoGhost(step int, on bool) {
	z.gotoGhost = on
	z.gotoGhostStep = step
}

// GotoGhost reports the pending go-to ghost (step, shown).
func (z *TimelineZone) GotoGhost() (int, bool) { return z.gotoGhostStep, z.gotoGhost }

// OnDrag moves the ghost without seeking; the seek commits on release, only if the beat changed.
func (z *TimelineZone) drawScrubGhost(dst *ebiten.Image, step int) {
	bar := z.timelineBarRect
	if bar.Empty() {
		return
	}
	beats := 1
	if z.callbacks.TimelineBeats != nil {
		if b := z.callbacks.TimelineBeats(); b > 0 {
			beats = b
		}
	}
	div := 1
	if z.callbacks.TimelineUnitsPerBeat != nil {
		if d := z.callbacks.TimelineUnitsPerBeat(); d > 0 {
			div = d
		}
	}
	ghostBeat := step / div
	frac := float64(ghostBeat) / float64(beats)
	gx := bar.Min.X + int(math.Round(frac*float64(bar.Dx())))
	if gx < bar.Min.X {
		gx = bar.Min.X
	} else if gx >= bar.Max.X {
		gx = bar.Max.X - 1
	}
	drawRect(dst, image.Rect(gx, bar.Min.Y, gx+1, bar.Max.Y), colAccentBright, true)
	// 1-based, like the beat counter.
	DrawTextAt(dst, strconv.Itoa(ghostBeat+1), gx+3, bar.Min.Y+2)
}

// Major every 16 beats, medium every 4, minor every 1; a tier spaced under 3 px is skipped.
func (z *TimelineZone) drawRibbonTicks(dst *ebiten.Image, winStart, pxPerBeat float64) {
	const minTierPx = 3.0
	startBeat := int(math.Floor(winStart))
	endBeat := startBeat + int(math.Ceil(float64(z.TlCacheW)/pxPerBeat)) + 1
	h := z.TlCacheH
	majorH := h
	mediumH := h * 2 / 3
	if mediumH < 1 {
		mediumH = 1
	}
	minorH := h / 3
	if minorH < 1 {
		minorH = 1
	}
	mediumCol := WithAlpha(colTimelineBeat, AlphaSubtle)
	minorCol := WithAlpha(colTimelineBeat, AlphaFaint)
	majorSpacingPx := 16 * pxPerBeat
	mediumSpacingPx := 4 * pxPerBeat
	for beat := startBeat; beat <= endBeat; beat++ {
		x := int(math.Round((float64(beat) - winStart) * pxPerBeat))
		if x < 0 || x >= z.TlCacheW {
			continue
		}
		switch {
		case beat%16 == 0 && majorSpacingPx >= minTierPx:
			drawRect(dst, image.Rect(x, 0, x+1, majorH), colTimelineBeat, true)
		case beat%4 == 0 && mediumSpacingPx >= minTierPx:
			drawRect(dst, image.Rect(x, h-mediumH, x+1, h), mediumCol, true)
		case pxPerBeat >= minTierPx:
			drawRect(dst, image.Rect(x, h-minorH, x+1, h), minorCol, true)
		}
	}
}

// Re-rendered only when the milliseconds change.
func (z *TimelineZone) timelineInfoCached(elapsedBeats float64) string {
	unitsPerBeat := max1(z.callbacks.TimelineUnitsPerBeat())
	length := z.callbacks.Length()
	secPerBeat := z.callbacks.SecPerBeat()
	isPlaying := z.callbacks.IsPlaying != nil && z.callbacks.IsPlaying()

	units := float64(unitsPerBeat)
	patternBeats := math.Ceil(float64(length) / units)
	totalBeats := patternBeats
	if isPlaying {
		totalBeats = math.Max(patternBeats, elapsedBeats)
	}
	curMS := int(math.Round(elapsedBeats * secPerBeat * 1000.0))
	totMS := int(math.Round(totalBeats * secPerBeat * 1000.0))
	if thrCfg := RuntimeProf().TimelineInfoThrottleMS; thrCfg > 0 {
		thr := thrCfg
		if (curMS/thr) == (z.LastInfoCurMS/thr) && (totMS/thr) == (z.LastInfoTotMS/thr) && z.LastInfoText != "" {
			return z.LastInfoText
		}
	} else if curMS == z.LastInfoCurMS && totMS == z.LastInfoTotMS && z.LastInfoText != "" {
		return z.LastInfoText
	}
	z.LastInfoCurMS = curMS
	z.LastInfoTotMS = totMS
	// Through playheadPosAt, like the navigation menu's title, so they never disagree.
	p := playheadPosAt(elapsedBeats, secPerBeat, unitsPerBeat)
	z.LastInfoText = formatBeatReadout(p.DisplayBeat, p.Seconds)
	return z.LastInfoText
}

// Icon, gap, text and padding: the single width source for the cap, the slot ceiling and layout, so
// the counter never crops.
func beatCounterContentWidth(readout string) int {
	iconSz := TextHeight()
	iconGap := beatCounterPillPadX
	return iconSz + iconGap + TextWidth(readout) + 2*beatCounterPillPadX
}

// A stable ceiling so the counter does not reflow as the text changes; mobile uses its compact bound.
func beatCounterSlotWidth() int {
	if Profile().IsMobile() {
		return beatCounterContentWidth("888 · 8:88")
	}
	return beatCounterContentWidth("Beat 888 · 88:88")
}

// A keycap like every Button, so it reads as pressable; a tap opens the navigation menu.
func (z *TimelineZone) drawBeatCounter(dst *ebiten.Image, elapsedBeats float64) {
	beatCounterRect := image.Rectangle{}
	if z.callbacks.BeatCounterRect != nil {
		beatCounterRect = z.callbacks.BeatCounterRect()
	}
	if beatCounterRect.Empty() {
		return
	}
	info := z.timelineInfoCached(elapsedBeats)
	th := TextHeight()
	iconSz := th // square target icon, aligned to the text cap height
	iconGap := beatCounterPillPadX
	contentW := beatCounterContentWidth(info)
	contentH := th + 2*beatCounterPillPadY + genGeomKeycapWallDepth
	pillW := contentW
	if pillW > beatCounterRect.Dx() {
		pillW = beatCounterRect.Dx()
	}
	pillH := contentH
	if pillH > beatCounterRect.Dy() {
		pillH = beatCounterRect.Dy()
	}
	pillX := beatCounterRect.Min.X
	pillY := beatCounterRect.Min.Y + (beatCounterRect.Dy()-pillH)/2
	pillR := image.Rect(pillX, pillY, pillX+pillW, pillY+pillH)
	// Hover only: the press opens the menu at once.
	cx, cy := cursorPosition()
	hovered := image.Pt(cx, cy).In(pillR)
	cap := drawFlatPanelButton(dst, pillR, TransportMiscStyle.Fill, hovered)
	// Leading target icon, vertically centered on the cap face.
	iconY := cap.Min.Y + (cap.Dy()-iconSz)/2
	iconR := image.Rect(cap.Min.X+beatCounterPillPadX, iconY, cap.Min.X+beatCounterPillPadX+iconSz, iconY+iconSz)
	DrawIcon(dst, IconTarget, iconR, colTextSecondary)
	// Readout text after the icon, vertically centered on the cap face.
	infoY := cap.Min.Y + (cap.Dy()-th)/2
	DrawTextAt(dst, info, cap.Min.X+beatCounterPillPadX+iconSz+iconGap, infoY)
}

func (z *TimelineZone) rowsTopY() int {
	if z.callbacks.RowsTopY != nil {
		return z.callbacks.RowsTopY()
	}
	return z.stepsRect.Min.Y
}

// The rack bed, so the area under the last row reads as one surface with the rack.
func (z *TimelineZone) drawGapFill(dst *ebiten.Image) {
	rows := z.callbacks.Rows()
	vis := z.callbacks.VisibleRows()
	rowOffset := z.callbacks.RowOffset()
	rh := z.callbacks.RowHeight()
	rowsTop := z.rowsTopY()
	nBelow := len(rows) - rowOffset
	if nBelow > vis {
		nBelow = vis
	}
	bottomOfRows := rowsTop + nBelow*rh
	rackBottom := z.stepsRect.Max.Y
	tlLeft := z.stepsRect.Min.X
	tlRight := z.stepsRect.Max.X
	if bottomOfRows < rackBottom && tlRight > tlLeft {
		gapRect := image.Rect(tlLeft, bottomOfRows, tlRight, rackBottom)
		drawRect(dst, gapRect, colRackSurface, true)
	}
}

func (z *TimelineZone) ensureHighlightSprites() {
	h := z.callbacks.RowHeight()
	if h <= 0 {
		h = 1
	}
	if z.hlSpriteReg != nil && z.hlSpriteH == h {
		return
	}
	releaseImage(z.hlSpriteReg)
	releaseImage(z.hlSpriteMute)
	reg := newTrackedImage("timelineZone.hlSpriteReg", 1, h)
	drawRect(reg, image.Rect(0, 0, 1, h), fadeColor(colHighlight, float64(genAnimPlayheadColumnFade)), true)
	mute := newTrackedImage("timelineZone.hlSpriteMute", 1, h)
	drawRect(mute, image.Rect(0, 0, 1, h), colMuteHighlight, true)
	z.hlSpriteReg = reg
	z.hlSpriteMute = mute
	z.hlSpriteH = h
}

// They get a subdued highlight instead of the bright flash.
func isNonAudibleCellType(t model.NodeType) bool {
	return !t.Fires()
}

// One scaled 1-px sprite per highlight.
func (z *TimelineZone) drawHighlights(dst *ebiten.Image, simpleDraw bool) {
	rows := z.callbacks.Rows()
	vis := z.callbacks.VisibleRows()
	rowOffset := z.callbacks.RowOffset()
	rh := z.callbacks.RowHeight()
	offset := z.callbacks.Offset()
	length := z.callbacks.Length()
	rowsTop := z.rowsTopY()
	// Highlights follow the cell grid, never the ribbon, which header chrome narrows.
	anchor := z.cellGridAnchor()
	startX := anchor.Min.X
	totalW := anchor.Dx()

	z.ensureHighlightSprites()

	for i := range rows {
		if i < rowOffset || i >= rowOffset+vis {
			continue
		}
		r := rows[i]
		// Ensure Steps slice matches current length.
		if len(r.Steps) != length {
			r.Steps = make([]bool, length)
			r.CellTypes = make([]model.NodeType, length)
		}
		y := rowsTop + (i-rowOffset)*rh
		n := len(r.Steps)
		if n > 0 && i < len(z.highlightsByRow) && len(z.highlightsByRow[i]) > 0 {
			for _, hl := range z.highlightsByRow[i] {
				val := hl.val
				j := hl.idx - offset
				if j < 0 || j >= n {
					continue
				}
				x0 := startX + (j*totalW)/n
				x1 := startX + ((j+1)*totalW)/n
				if x1 <= x0 {
					x1 = x0 + 1
				}

				// Tests intercept drawRect to assert highlights.
				if runningUnderGoTest() && !simpleDraw {
					rect := image.Rect(x0, y, x1, y+rh)
					if isMuteHighlight(val) || isNonAudibleCellType(r.CellTypes[j]) {
						drawRect(dst, rect, colMuteHighlight, true)
						drawRect(dst, rect, DrumCellUI.Border, false)
					} else {
						DrumCellUI.Draw(dst, rect, r.Steps[j], true, r.Color)
					}
				} else {
					var spr *ebiten.Image
					if isMuteHighlight(val) || isNonAudibleCellType(r.CellTypes[j]) {
						spr = z.hlSpriteMute
					} else {
						spr = z.hlSpriteReg
					}
					if spr != nil {
						var hop ebiten.DrawImageOptions
						w := float64(x1 - x0)
						hop.GeoM.Scale(w/float64(spr.Bounds().Dx()), 1)
						hop.GeoM.Translate(float64(x0), float64(y))
						dst.DrawImage(spr, &hop)
					}
				}
			}
		}
	}
}

// drawMuteSoloDimming renders semi-transparent overlays on muted/non-soloed rows.
func (z *TimelineZone) drawMuteSoloDimming(dst *ebiten.Image) {
	rows := z.callbacks.Rows()
	anySolo := false
	anyMuted := false
	for _, r := range rows {
		if r.Solo {
			anySolo = true
		}
		if r.Muted {
			anyMuted = true
		}
	}
	if !anySolo && !anyMuted {
		return
	}
	vis := z.callbacks.VisibleRows()
	rowOffset := z.callbacks.RowOffset()
	rh := z.callbacks.RowHeight()
	rowsTop := z.rowsTopY()
	// The cell strip only, anchored to the grid.
	anchor := z.cellGridAnchor()
	for i := rowOffset; i < rowOffset+vis && i < len(rows); i++ {
		shouldDim := rows[i].Muted || (anySolo && !rows[i].Solo)
		if shouldDim {
			y := rowsTop + (i-rowOffset)*rh
			dimRect := image.Rect(anchor.Min.X, y, anchor.Max.X, y+rh)
			drawRect(dst, dimRect, WithAlpha(genColorDimBlack, genAlphaSidebarSection), true)
		}
	}
}

func (z *TimelineZone) HandleKey(key ebiten.Key) InputResult {
	return InputIgnored
}

func (z *TimelineZone) HandleChars(chars []rune) InputResult {
	return InputIgnored
}

// --- Public accessors ---

// IsDragging returns true while a grid drag is in progress.
func (z *TimelineZone) IsDragging() bool { return z.dragging }

// IsScrubbing returns true while a timeline scrub is in progress.
func (z *TimelineZone) IsScrubbing() bool { return z.scrubbing }

// StepsRect returns the computed grid area rectangle.
func (z *TimelineZone) StepsRect() image.Rectangle { return z.stepsRect }

// TimelineBarRect returns the computed timeline bar rectangle.
func (z *TimelineZone) TimelineBarRect() image.Rectangle { return z.timelineBarRect }

// --- Cache management public API ---

// true when reallocated: the caller must re-alias.
func (z *TimelineZone) EnsureRowCache() bool {
	rows := z.callbacks.Rows()
	n := len(rows)
	reallocated := false

	if len(z.RowCache) != n {
		z.RowCache = make([]*ebiten.Image, n)
		z.RowCacheScratch = make([]*ebiten.Image, n)
		z.RowDirty = make([]bool, n)
		z.RowFullDirty = make([]bool, n)
		for i := range z.RowDirty {
			z.RowDirty[i] = true
		}
		for i := range z.RowFullDirty {
			z.RowFullDirty[i] = true
		}
		z.RowCacheOff = make([]int, n)
		z.RowCacheGen = make([]int, n)
		z.RowCacheSig = make([]uint64, n)
		z.RowCacheSteps = make([][]bool, n)
		z.RowCacheTypes = make([][]model.NodeType, n)
		offset := z.callbacks.Offset()
		for i := range z.RowCacheOff {
			z.RowCacheOff[i] = offset
		}
		reallocated = true
	}
	if len(z.RowCacheScratch) != n {
		scratch := make([]*ebiten.Image, n)
		copy(scratch, z.RowCacheScratch)
		z.RowCacheScratch = scratch
		reallocated = true
	}
	if len(z.RowCacheSig) != n {
		sig := make([]uint64, n)
		copy(sig, z.RowCacheSig)
		z.RowCacheSig = sig
		reallocated = true
	}
	if len(z.RowCacheSteps) != n {
		steps := make([][]bool, n)
		copy(steps, z.RowCacheSteps)
		z.RowCacheSteps = steps
		reallocated = true
	}
	if len(z.RowCacheTypes) != n {
		types := make([][]model.NodeType, n)
		copy(types, z.RowCacheTypes)
		z.RowCacheTypes = types
		reallocated = true
	}
	if len(z.RowFrame) != n {
		rf := make([]int64, n)
		copy(rf, z.RowFrame)
		z.RowFrame = rf
		reallocated = true
	}
	if len(z.RowRepaint) != n {
		rr := make([]int, n)
		copy(rr, z.RowRepaint)
		z.RowRepaint = rr
		reallocated = true
	}
	if len(z.TimelineOffset) != n {
		offsets := make([]int, n)
		copy(offsets, z.TimelineOffset)
		z.TimelineOffset = offsets
		reallocated = true
	}
	if len(z.TimelinePast) != n {
		past := make([][]bool, n)
		copy(past, z.TimelinePast)
		z.TimelinePast = past
		reallocated = true
	}
	if len(z.TimelinePastTypes) != n {
		types := make([][]model.NodeType, n)
		copy(types, z.TimelinePastTypes)
		z.TimelinePastTypes = types
		reallocated = true
	}
	if len(z.TimelinePresent) != n {
		present := make([][]bool, n)
		copy(present, z.TimelinePresent)
		z.TimelinePresent = present
		reallocated = true
	}
	if len(z.TimelineFuture) != n {
		future := make([][]bool, n)
		copy(future, z.TimelineFuture)
		z.TimelineFuture = future
		reallocated = true
	}
	return reallocated
}

// CacheRowSteps snapshots a row's Steps and CellTypes into the cache.
func (z *TimelineZone) CacheRowSteps(row int) {
	rows := z.callbacks.Rows()
	if row < 0 || row >= len(rows) {
		return
	}
	n := len(rows)
	if len(z.RowCacheSteps) != n {
		steps := make([][]bool, n)
		copy(steps, z.RowCacheSteps)
		z.RowCacheSteps = steps
	}
	if len(z.RowCacheTypes) != n {
		types := make([][]model.NodeType, n)
		copy(types, z.RowCacheTypes)
		z.RowCacheTypes = types
	}
	copyBoolSliceInto(&z.RowCacheSteps[row], rows[row].Steps)
	copyNodeTypeSliceInto(&z.RowCacheTypes[row], rows[row].CellTypes)
}

// MarkRowDirty invalidates the cached sprite for a single row.
func (z *TimelineZone) MarkRowDirty(i int) {
	z.EnsureRowCache()
	if i >= 0 && i < len(z.RowDirty) {
		z.RowDirty[i] = true
		if i < len(z.RowFullDirty) {
			z.RowFullDirty[i] = true
		}
	}
	if z.callbacks.OnRowsLayerDirty != nil {
		z.callbacks.OnRowsLayerDirty()
	}
}

// MarkAllRowsDirty invalidates all row sprite caches.
func (z *TimelineZone) MarkAllRowsDirty() {
	z.EnsureRowCache()
	for i := range z.RowDirty {
		z.RowDirty[i] = true
	}
	for i := range z.RowFullDirty {
		z.RowFullDirty[i] = true
	}
	if z.callbacks.OnRowsLayerDirty != nil {
		z.callbacks.OnRowsLayerDirty()
	}
}

// Leaves RowFullDirty alone, so the cheap patch path still applies.
func (z *TimelineZone) MarkRowCellsDirty(i int) {
	z.EnsureRowCache()
	if i >= 0 && i < len(z.RowDirty) {
		z.RowDirty[i] = true
	}
	if z.callbacks.OnRowsLayerDirty != nil {
		z.callbacks.OnRowsLayerDirty()
	}
}

// MarkRowShiftDirty invalidates a row but allows incremental reuse.
func (z *TimelineZone) MarkRowShiftDirty(i int) {
	z.EnsureRowCache()
	if i >= 0 && i < len(z.RowDirty) {
		z.RowDirty[i] = true
		if i < len(z.RowFullDirty) {
			z.RowFullDirty[i] = false
		}
	}
	if z.callbacks.OnRowsLayerDirty != nil {
		z.callbacks.OnRowsLayerDirty()
	}
}

func (z *TimelineZone) MarkRowsShiftDirty() {
	z.EnsureRowCache()
	for i := range z.RowDirty {
		z.RowDirty[i] = true
	}
	if z.callbacks.OnRowsLayerDirty != nil {
		z.callbacks.OnRowsLayerDirty()
	}
}

// InvalidateRowCaches resets row cache dimensions and marks all dirty.
func (z *TimelineZone) InvalidateRowCaches() {
	z.MarkAllRowsDirty()
	if z.callbacks.OnRowsLayerDirty != nil {
		z.callbacks.OnRowsLayerDirty()
	}
}

// SetTimelineSegments stores segmented timeline data for a row.
func (z *TimelineZone) SetTimelineSegments(row, offset int, past []bool, pastTypes []model.NodeType, present, future []bool) {
	rows := z.callbacks.Rows()
	if row < 0 || row >= len(rows) {
		return
	}
	z.EnsureRowCache()
	z.TimelineOffset[row] = offset
	copyBoolSliceInto(&z.TimelinePast[row], past)
	copyNodeTypeSliceInto(&z.TimelinePastTypes[row], pastTypes)
	copyBoolSliceInto(&z.TimelinePresent[row], present)
	copyBoolSliceInto(&z.TimelineFuture[row], future)
}

// NeedsRowRebuild reports whether row i needs its cached sprite rebuilt.
func (z *TimelineZone) NeedsRowRebuild(i int) bool {
	rows := z.callbacks.Rows()
	if i < 0 || i >= len(rows) {
		return false
	}
	if len(z.RowCache) != len(rows) || len(z.RowDirty) != len(rows) {
		return true
	}
	if len(z.RowFullDirty) != len(rows) {
		return true
	}
	if z.RowDirty[i] {
		return true
	}
	if i < len(z.RowFullDirty) && z.RowFullDirty[i] {
		return true
	}
	if z.RowCache[i] == nil {
		return true
	}
	return false
}

// ResetAfterDelete clears cache slices so indexes realign after row deletion.
func (z *TimelineZone) ResetAfterDelete() {
	z.RowCache = nil
	z.RowDirty = nil
	z.RowFullDirty = nil
	z.RowFrame = nil
	z.RowRepaint = nil
	z.RowsDrawnMask = nil
}

// SetButtons sets the timeline-area buttons that will be registered as hit areas.
func (z *TimelineZone) SetButtons(track, lenDec, lenInc *Button) {
	z.trackBtn = track
	z.lenDecBtn = lenDec
	z.lenIncBtn = lenInc
}

// After calcLayout repositions the buttons.
func (z *TimelineZone) RefreshButtonHitAreas() {
	z.rebuildHitAreas()
}

// --- Hit area construction ---

func (z *TimelineZone) rebuildHitAreas() {
	z.hitAreas = z.hitAreas[:0]

	// Grid drag area (lower z-index so timeline bar wins on overlap).
	if !z.stepsRect.Empty() {
		z.hitAreas = append(z.hitAreas, HitArea{
			Rect:    z.stepsRect,
			ZIndex:  110,
			Handler: &gridDragHitAdapter{zone: z},
			Tag:     "timeline-grid-drag",
		})
	}

	// The VISIBLE bar rect, expanded by Touch + ClipRect: physically enlarging it leaked into the steps
	// below, and a captured scrub kept firing in another zone. The 4 px skirt can never reach the panel.
	scrubRect := z.timelineBarRect
	if !scrubRect.Empty() {
		clip := scrubRect.Inset(-4)
		if !z.rect.Empty() {
			clip = clip.Intersect(z.rect)
		}
		z.hitAreas = append(z.hitAreas, HitArea{
			Rect:     scrubRect, // strict visible bar — no downward growth
			ZIndex:   111,
			Handler:  &timelineScrubHitAdapter{zone: z},
			Tag:      "timeline-scrub",
			Touch:    true,
			ClipRect: clip,
		})
	}

	// Timeline-area buttons (track, len+/-).
	const zBtn = 112 // higher than scrub bar

	// Track button: simple click (not repeat).
	if z.trackBtn != nil {
		if r := z.trackBtn.Rect(); !r.Empty() {
			z.hitAreas = append(z.hitAreas, HitArea{
				Rect:     r,
				ZIndex:   zBtn,
				Handler:  &buttonHitAdapter{btn: z.trackBtn},
				Tag:      "timeline-track",
				Touch:    true,
				ClipRect: z.rect,
			})
		}
	}

	// Length +/- buttons: hold-to-repeat.
	for _, sb := range []struct {
		btn *Button
		tag string
	}{
		{z.lenDecBtn, "timeline-len-dec"},
		{z.lenIncBtn, "timeline-len-inc"},
	} {
		if sb.btn == nil {
			continue
		}
		r := sb.btn.Rect()
		if r.Empty() {
			continue
		}
		z.hitAreas = append(z.hitAreas, HitArea{
			Rect:    r,
			ZIndex:  zBtn,
			Handler: &repeatButtonHitAdapter{btn: sb.btn},
			Tag:     sb.tag,
			Touch:   true,
			// No ClipRect: recalcButtons places these outside the zone rect, and a clip would block touch hits.
		})
	}

	// An exact-rect, non-Touch catch-all: the band-mates' touch-expanded areas reach over the counter,
	// and exact hits sort first, so a tap here can never open their menus.
	if z.callbacks.BeatCounterRect != nil {
		if bc := z.callbacks.BeatCounterRect(); !bc.Empty() {
			z.hitAreas = append(z.hitAreas, HitArea{
				Rect:     bc,
				ZIndex:   zBtn,
				Handler:  &beatCounterHitAdapter{zone: z},
				Tag:      "timeline-beat-counter",
				ClipRect: z.rect,
			})
		}
	}
}

// Absorbs presses so a band-mate's expanded hit area cannot steal them; opens the navigation menu.
type beatCounterHitAdapter struct{ zone *TimelineZone }

func (h *beatCounterHitAdapter) OnPress(x, y int) InputResult {
	if h.zone != nil && h.zone.callbacks.OnOpenNavigation != nil {
		h.zone.callbacks.OnOpenNavigation()
	}
	return InputConsumed
}
func (h *beatCounterHitAdapter) OnDrag(x, y int)    {}
func (h *beatCounterHitAdapter) OnRelease(x, y int) {}
func (h *beatCounterHitAdapter) OnWheel(x, y, steps int) InputResult {
	return InputIgnored
}

// --- Grid drag hit handler ---

type gridDragHitAdapter struct {
	zone        *TimelineZone
	dragStartY  int
	startRowOff int  // row offset when drag began
	locked      bool // true once direction is determined
	vertical    bool // true = vertical (row scroll), false = horizontal (offset scroll)
}

func (h *gridDragHitAdapter) OnPress(x, y int) InputResult {
	z := h.zone
	z.dragging = true
	z.dragStartX = x
	h.dragStartY = y
	z.startOffset = z.callbacks.Offset()
	h.startRowOff = z.callbacks.RowOffset()
	h.locked = false
	h.vertical = false
	return InputCaptured
}

func (h *gridDragHitAdapter) OnDrag(x, y int) {
	z := h.zone
	if !z.dragging {
		return
	}
	if !h.locked {
		dx := x - z.dragStartX
		dy := y - h.dragStartY
		if dx < 0 {
			dx = -dx
		}
		if dy < 0 {
			dy = -dy
		}
		const deadZone = 8
		if dx < deadZone && dy < deadZone {
			return // still in dead zone
		}
		h.locked = true
		h.vertical = dy > dx
	}
	if h.vertical {
		// Dragging up scrolls down.
		rowH := z.callbacks.RowHeight()
		if rowH > 0 && z.callbacks.OnRowScrollDrag != nil {
			targetOff := h.startRowOff + (h.dragStartY-y)/rowH
			z.callbacks.OnRowScrollDrag(targetOff)
		}
	} else {
		// Existing horizontal offset logic
		cell := z.callbacks.Cell()
		if cell < 1 {
			cell = 1
		}
		delta := (z.dragStartX - x) / cell
		newOffset := z.startOffset + delta
		if newOffset < 0 {
			newOffset = 0
		}
		if z.callbacks.OnOffsetChange != nil {
			z.callbacks.OnOffsetChange(newOffset)
		}
	}
}

func (h *gridDragHitAdapter) OnRelease(x, y int) {
	h.zone.dragging = false
	h.locked = false
	h.vertical = false
}

func (h *gridDragHitAdapter) OnWheel(x, y, steps int) InputResult {
	z := h.zone
	// Route only the dominant axis of a two-finger scroll, so a diagonal gesture never moves both the
	// timeline and the rows. Detents, never raw units.
	wx, wy := wheelNotches()
	ax, ay := wx, wy
	if ax < 0 {
		ax = -ax
	}
	if ay < 0 {
		ay = -ay
	}
	if ax > ay {
		sx := wheelRowsFromNotches(wx)
		if sx == 0 || z.callbacks.OnOffsetChange == nil || z.callbacks.Offset == nil {
			return InputIgnored
		}
		// Clamped at 0 like the horizontal grid drag.
		newOffset := z.callbacks.Offset() + sx
		if newOffset < 0 {
			newOffset = 0
		}
		z.callbacks.OnOffsetChange(newOffset)
		return InputConsumed
	}

	if steps == 0 {
		return InputIgnored
	}
	if z.callbacks.OnRowScrollWheel != nil && z.callbacks.OnRowScrollWheel(steps) {
		return InputConsumed
	}
	return InputIgnored
}

func (z *TimelineZone) ribbonStepAt(x int) int {
	bar := z.timelineBarRect
	if bar.Dx() <= 0 {
		return 0
	}
	if x < bar.Min.X {
		x = bar.Min.X
	} else if x >= bar.Max.X {
		x = bar.Max.X - 1
	}
	frac := float64(x-bar.Min.X) / float64(bar.Dx())
	beats := 0
	if z.callbacks.TimelineBeats != nil {
		beats = z.callbacks.TimelineBeats()
	}
	beat := int(math.Round(frac * float64(beats)))
	if beat < 0 {
		beat = 0
	} else if beat > beats {
		beat = beats
	}
	div := 1
	if z.callbacks.TimelineUnitsPerBeat != nil {
		if d := z.callbacks.TimelineUnitsPerBeat(); d > 0 {
			div = d
		}
	}
	return beat * div
}

// Drag-only: press arms, drag moves the ghost without seeking, release commits one seek only if
// the beat changed. A pure tap commits nothing.
type timelineScrubHitAdapter struct {
	zone *TimelineZone
}

func (h *timelineScrubHitAdapter) OnPress(x, y int) InputResult {
	if globalTouchState != nil && globalTouchState.RecentMultiTouch() {
		return InputIgnored
	}
	z := h.zone
	z.scrubbing = true
	// After the multi-touch gate, so suppressed presses do not buzz.
	hapticEmit(8)
	z.scrubGhostBeat = z.ribbonStepAt(x)
	z.scrubPressBeat = z.scrubGhostBeat // remember where the gesture started; no seek on press
	return InputCaptured
}

func (h *timelineScrubHitAdapter) OnDrag(x, y int) {
	z := h.zone
	if !z.scrubbing {
		return
	}
	// Captured drags skip hit re-testing, so release capture when the pointer leaves the bar's lane,
	// or a drag into the audio panel keeps scrubbing.
	skirt := z.timelineBarRect.Inset(-4)
	if y < skirt.Min.Y || y >= skirt.Max.Y {
		z.scrubbing = false
		return
	}
	z.scrubGhostBeat = z.ribbonStepAt(x) // ghost only — no seek during drag
}

func (h *timelineScrubHitAdapter) OnRelease(x, y int) {
	z := h.zone
	if z.scrubbing && z.callbacks.OnSeekToStep != nil {
		final := z.ribbonStepAt(x)
		// Only if the beat changed: a tap emits no seek, a drag exactly one.
		if final != z.scrubPressBeat {
			z.callbacks.OnSeekToStep(final, hooks.SeekRibbon)
		}
	}
	z.scrubbing = false
}

func (h *timelineScrubHitAdapter) OnWheel(x, y, steps int) InputResult {
	return InputIgnored
}
