package ui

import (
	"fmt"
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

// TimelineCallbacks contains callbacks for TimelineZone to communicate with
// DrumView. Read-only accessors return current state; action callbacks push
// changes back to the owner.
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
	OnScrubPosition  func(newOffset int)
	OnRowsLayerDirty func() // signal that the rows layer composite needs rebuild
	SetTimelineBeats func(int)

	// Row scroll forwarding (vertical scroll over grid)
	OnRowScrollWheel func(steps int) bool
	OnRowScrollDrag  func(targetRowOffset int)

	// Draw delegate: row composite rendering (stripes/layer/direct) stays
	// on DrumView due to deep coupling with offset caching and geometry.
	DrawRowComposite func(dst *ebiten.Image)
}

// TimelineZone implements the Zone interface for the grid drag/scrub area.
// It handles horizontal drag-to-scroll in the steps grid and click-to-scrub
// in the timeline progress bar. It also owns per-row sprite cache state.
type TimelineZone struct {
	rect       image.Rectangle
	needLayout bool
	callbacks  TimelineCallbacks
	portal     *OverlayPortal

	// Computed sub-rects (set during Layout)
	stepsRect       image.Rectangle // grid area for drag scrolling
	timelineBarRect image.Rectangle // progress bar for scrub

	// Drag state (grid horizontal scroll)
	dragging    bool
	dragStartX  int
	startOffset int

	// Scrub state (timeline bar seek)
	scrubbing bool

	// Timeline bar height (set by DrumView before Layout)
	timelineBarH int

	// Buttons positioned in the timeline area by DrumView.calcLayout().
	// TimelineZone registers them as hit areas; DrumView owns the buttons.
	trackBtn  *Button
	lenDecBtn *Button
	lenIncBtn *Button

	// Hit areas cache (rebuilt on Layout)
	hitAreas []HitArea

	// Per-frame draw state (set by DrumView before Draw)
	elapsedBeats    float64
	highlightsByRow [][]highlightEntry

	// Highlight sprite cache
	hlSpriteReg    *ebiten.Image
	hlSpriteMute   *ebiten.Image
	hlSpriteH      int
	// --- Per-row sprite cache (owned by zone, aliased to DrumView) ---

	// Timeline base cache (background + beat markers)
	TlCache       *ebiten.Image
	TlCacheW      int
	TlCacheH      int
	TlCacheBeats  int
	TlCacheStep   int
	LastInfoCurMS int
	LastInfoTotMS int
	LastInfoText  string

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

// SetTimelineBarHeight sets the height of the timeline progress bar.
// Must be called before Layout so the bar rect is computed correctly.
func (z *TimelineZone) SetTimelineBarHeight(h int) { z.timelineBarH = h }

// --- Zone interface ---

func (z *TimelineZone) ID() string          { return "timeline" }
func (z *TimelineZone) NeedsLayout() bool   { return z.needLayout }
func (z *TimelineZone) Invalidate()         { z.needLayout = true }
func (z *TimelineZone) HitAreas() []HitArea { return z.hitAreas }

func (z *TimelineZone) Layout(rect image.Rectangle) {
	z.rect = rect
	z.needLayout = false

	barH := z.timelineBarH
	if barH <= 0 {
		barH = tlBarHeight()
	}

	// timelineBarRect: thin progress/seek bar at the TOP of the zone rect
	// (matches dv.timelineRect which is positioned at the bottom of the header).
	z.timelineBarRect = image.Rect(rect.Min.X, rect.Min.Y, rect.Max.X, rect.Min.Y+barH)
	if z.timelineBarRect.Max.Y > rect.Max.Y {
		z.timelineBarRect.Max.Y = rect.Max.Y
	}

	// stepsRect: the grid area where rows are drawn (below the timeline bar).
	z.stepsRect = image.Rect(rect.Min.X, z.timelineBarRect.Max.Y, rect.Max.X, rect.Max.Y)
	if z.stepsRect.Dy() < 0 {
		z.stepsRect.Max.Y = z.stepsRect.Min.Y
	}

	z.rebuildHitAreas()
}

func (z *TimelineZone) Update() {
	// No per-frame state to update; drag/scrub are handled via hit handlers.
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

	// --- Track button (mobile) ---
	if Profile().IsMobile() && z.trackBtn != nil && !z.trackBtn.Rect().Empty() {
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

// computeTimelineBeats ensures the timeline is long enough for the graph,
// the current playhead, and the visible window.
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
	needBeats1 := int(math.Ceil(elapsedBeats/units + lengthBeats))
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

// drawTimelineBar renders the timeline progress bar: background cache,
// view rectangle, playback cursor, and border.
func (z *TimelineZone) drawTimelineBar(dst *ebiten.Image, elapsedBeats float64, totalBeats int) {
	barRect := z.timelineBarRect
	if barRect.Empty() {
		return
	}
	unitsPerBeat := max1(z.callbacks.TimelineUnitsPerBeat())
	offset := z.callbacks.Offset()
	length := z.callbacks.Length()
	units := float64(unitsPerBeat)
	offsetBeats := float64(offset) / units
	lengthBeats := float64(length) / units

	// Build or reuse timeline base cache (background + beat markers)
	step := 1
	width := barRect.Dx()
	if totalBeats > width {
		step = int(math.Ceil(float64(totalBeats) / float64(width)))
	}
	if z.TlCache == nil || z.TlCacheW != barRect.Dx() || z.TlCacheH != barRect.Dy() || z.TlCacheBeats != totalBeats || z.TlCacheStep != step {
		z.TlCache = ebiten.NewImage(barRect.Dx(), barRect.Dy())
		z.TlCacheW, z.TlCacheH = barRect.Dx(), barRect.Dy()
		z.TlCacheBeats, z.TlCacheStep = totalBeats, step
		drawRect(z.TlCache, image.Rect(0, 0, z.TlCacheW, z.TlCacheH), colTimelineTotal, true)
		prevX := -1
		for i := 0; i <= totalBeats; i += step {
			x := int(float64(i) / float64(totalBeats) * float64(z.TlCacheW))
			if x != prevX {
				drawRect(z.TlCache, image.Rect(x, 0, x+1, z.TlCacheH), colTimelineBeat, true)
				prevX = x
			}
		}
	}
	if z.TlCache != nil {
		var op ebiten.DrawImageOptions
		op.GeoM.Translate(float64(barRect.Min.X), float64(barRect.Min.Y))
		dst.DrawImage(z.TlCache, &op)
	} else {
		drawRect(dst, barRect, colTimelineTotal, true)
	}

	// Current view rectangle
	viewStart := barRect.Min.X + int((offsetBeats/float64(totalBeats))*float64(barRect.Dx()))
	viewWidth := int((lengthBeats / float64(totalBeats)) * float64(barRect.Dx()))
	if viewWidth < 1 {
		viewWidth = 1
	}
	viewRect := image.Rect(viewStart, barRect.Min.Y, viewStart+viewWidth, barRect.Max.Y)
	drawRect(dst, viewRect, colTimelineView, true)
	drawRect(dst, viewRect, colTimelineViewHi, false)

	// Current playback cursor
	cursorX := barRect.Min.X + int((elapsedBeats/float64(totalBeats))*float64(barRect.Dx()))
	cursorRect := image.Rect(cursorX-1, barRect.Min.Y, cursorX+1, barRect.Max.Y)
	drawRect(dst, cursorRect, colTimelineCursor, true)

	drawRect(dst, barRect, colButtonBorder, false)
}

// timelineInfoCached returns a cached timeline info string, only re-rendering
// when milliseconds change.
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
	if timelineInfoThrottleMS > 0 {
		thr := timelineInfoThrottleMS
		if (curMS/thr) == (z.LastInfoCurMS/thr) && (totMS/thr) == (z.LastInfoTotMS/thr) && z.LastInfoText != "" {
			return z.LastInfoText
		}
	} else if curMS == z.LastInfoCurMS && totMS == z.LastInfoTotMS && z.LastInfoText != "" {
		return z.LastInfoText
	}
	curS := curMS / 1000
	z.LastInfoCurMS = curMS
	z.LastInfoTotMS = totMS
	z.LastInfoText = fmt.Sprintf("Beat %d · %d:%02d", int(elapsedBeats)+1, curS/60, curS%60)
	return z.LastInfoText
}

// drawBeatCounter renders the beat/time counter above the timeline bar
// inside a subtle pill-shaped container.
func (z *TimelineZone) drawBeatCounter(dst *ebiten.Image, elapsedBeats float64) {
	beatCounterRect := image.Rectangle{}
	if z.callbacks.BeatCounterRect != nil {
		beatCounterRect = z.callbacks.BeatCounterRect()
	}
	if beatCounterRect.Empty() {
		return
	}
	info := z.timelineInfoCached(elapsedBeats)
	tw := TextWidth(info)
	th := TextHeight()
	// Pill background behind text.
	pillPadX, pillPadY := 6, 2
	pillW := tw + pillPadX*2
	pillH := th + pillPadY*2
	pillX := beatCounterRect.Min.X
	pillY := beatCounterRect.Min.Y + (beatCounterRect.Dy()-pillH)/2
	pillR := image.Rect(pillX, pillY, pillX+pillW, pillY+pillH)
	drawRoundedRect(dst, pillR, color.NRGBA{0, 185, 235, 25}, 4, true)
	infoY := pillY + pillPadY
	DrawTextAt(dst, info, pillX+pillPadX, infoY)
}

// rowsTopY returns the Y coordinate where rows start. Uses the RowsTopY
// callback when available; falls back to stepsRect.Min.Y.
func (z *TimelineZone) rowsTopY() int {
	if z.callbacks.RowsTopY != nil {
		return z.callbacks.RowsTopY()
	}
	return z.stepsRect.Min.Y
}

// drawGapFill fills the gap below visible rows with the step-off color.
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
		drawRect(dst, gapRect, colStepOff, true)
	}
}

// ensureHighlightSprites builds 1px-wide highlight sprites for the current
// row height, caching them for reuse.
func (z *TimelineZone) ensureHighlightSprites() {
	h := z.callbacks.RowHeight()
	if h <= 0 {
		h = 1
	}
	if z.hlSpriteReg != nil && z.hlSpriteH == h {
		return
	}
	reg := ebiten.NewImage(1, h)
	drawRect(reg, image.Rect(0, 0, 1, h), fadeColor(colHighlight, 0.85), true)
	mute := ebiten.NewImage(1, h)
	drawRect(mute, image.Rect(0, 0, 1, h), colMuteHighlight, true)
	z.hlSpriteReg = reg
	z.hlSpriteMute = mute
	z.hlSpriteH = h
}

// isNonAudibleCellType returns true for node types that don't produce sound.
// These get a subdued grey highlight instead of the bright white flash.
func isNonAudibleCellType(t model.NodeType) bool {
	return t != model.NodeTypeRegular
}

// drawHighlights renders per-row highlight overlays for active beats.
// Uses 1px-wide cached sprites scaled to cell width for all paths
// (1 DrawImage per highlight instead of 6 via DrumCellUI.Draw).
func (z *TimelineZone) drawHighlights(dst *ebiten.Image, simpleDraw bool) {
	rows := z.callbacks.Rows()
	vis := z.callbacks.VisibleRows()
	rowOffset := z.callbacks.RowOffset()
	rh := z.callbacks.RowHeight()
	offset := z.callbacks.Offset()
	length := z.callbacks.Length()
	rowsTop := z.rowsTopY()
	startX := z.stepsRect.Min.X
	totalW := z.stepsRect.Dx()

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

				// Tests intercept drawRect to assert highlight draws,
				// so fall back to the drawRect path under test.
				if runningUnderGoTest() && !simpleDraw {
					rect := image.Rect(x0, y, x1, y+rh)
					if isMuteHighlight(val) || isNonAudibleCellType(r.CellTypes[j]) {
						drawRect(dst, rect, colMuteHighlight, true)
						drawRect(dst, rect, DrumCellUI.Border, false)
					} else {
						DrumCellUI.Draw(dst, rect, r.Steps[j], true, r.Color)
					}
				} else {
					// Sprite-based path: 1 DrawImage per highlight
					// instead of 6 via DrumCellUI.Draw.
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
	for i := rowOffset; i < rowOffset+vis && i < len(rows); i++ {
		shouldDim := rows[i].Muted || (anySolo && !rows[i].Solo)
		if shouldDim {
			y := rowsTop + (i-rowOffset)*rh
			dimRect := image.Rect(z.stepsRect.Min.X, y, z.stepsRect.Max.X, y+rh)
			drawRect(dst, dimRect, color.NRGBA{0, 0, 0, 100}, true)
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

// EnsureRowCache ensures cache slices match the current row count.
// Returns true if slices were reallocated (caller should re-alias).
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

// MarkRowCellsDirty marks a row as needing rebuild but allows the cheap
// cell-patch path (does NOT set RowFullDirty).
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

// MarkRowsShiftDirty invalidates all rows for offset shifts while
// allowing incremental reuse.
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

// RefreshButtonHitAreas rebuilds hit areas including buttons.
// Called by DrumView after calcLayout() repositions the buttons.
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

	// Timeline bar scrub area (higher z-index).
	if !z.timelineBarRect.Empty() {
		z.hitAreas = append(z.hitAreas, HitArea{
			Rect:    z.timelineBarRect,
			ZIndex:  111,
			Handler: &timelineScrubHitAdapter{zone: z},
			Tag:     "timeline-scrub",
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
			// No ClipRect: buttons are positioned outside the shrunk zone
			// rect by recalcButtons(), so clipping prevents mobile touch hits.
		})
	}
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
		// Compute target row offset from total drag delta.
		// Dragging up (negative dy) = scroll down (increase offset).
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
	if steps == 0 {
		return InputIgnored
	}
	if h.zone.callbacks.OnRowScrollWheel != nil {
		if h.zone.callbacks.OnRowScrollWheel(steps) {
			return InputConsumed
		}
	}
	return InputIgnored
}

// --- Timeline scrub hit handler ---

type timelineScrubHitAdapter struct {
	zone *TimelineZone
}

func (h *timelineScrubHitAdapter) OnPress(x, y int) InputResult {
	z := h.zone
	z.scrubbing = true
	h.scrubTo(x)
	return InputCaptured
}

func (h *timelineScrubHitAdapter) OnDrag(x, y int) {
	if h.zone.scrubbing {
		h.scrubTo(x)
	}
}

func (h *timelineScrubHitAdapter) OnRelease(x, y int) {
	h.zone.scrubbing = false
}

func (h *timelineScrubHitAdapter) OnWheel(x, y, steps int) InputResult {
	return InputIgnored
}

func (h *timelineScrubHitAdapter) scrubTo(x int) {
	z := h.zone
	barRect := z.timelineBarRect

	pos := x
	if pos < barRect.Min.X {
		pos = barRect.Min.X
	}
	if pos > barRect.Max.X {
		pos = barRect.Max.X
	}
	frac := float64(pos-barRect.Min.X) / float64(barRect.Dx())

	unitsPerBeat := z.callbacks.TimelineUnitsPerBeat()
	if unitsPerBeat < 1 {
		unitsPerBeat = 1
	}
	length := z.callbacks.Length()
	timelineBeats := z.callbacks.TimelineBeats()

	lengthBeats := float64(length) / float64(unitsPerBeat)
	maxOffBeats := float64(timelineBeats) - lengthBeats
	if maxOffBeats < 0 {
		maxOffBeats = 0
	}
	desiredBeats := frac * maxOffBeats
	desiredSteps := int(math.Round(desiredBeats * float64(unitsPerBeat)))
	maxOffSteps := int(math.Round(maxOffBeats * float64(unitsPerBeat)))
	if desiredSteps < 0 {
		desiredSteps = 0
	}
	if desiredSteps > maxOffSteps {
		desiredSteps = maxOffSteps
	}

	if z.callbacks.OnScrubPosition != nil {
		z.callbacks.OnScrubPosition(desiredSteps)
	}
}
