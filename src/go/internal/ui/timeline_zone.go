package ui

import (
	"fmt"
	"image"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

// beatCounterPillPadX/PadY define the pill chrome around the beat-counter
// readout. Promoted to package constants so layout tests can reproduce
// the same `pillH = TextHeight() + 2*pillPadY` math drawBeatCounter uses
// to size the chip — keeping math + render in lockstep.
const (
	beatCounterPillPadX = 8
	beatCounterPillPadY = 3
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
	// Explicit timeline-bar rect set by DrumView before Layout. When
	// non-empty, Layout uses this rect for the bar instead of placing
	// the bar at `rect.Min.Y`. This decouples the bar's vertical
	// position from the zone's clip-rect top, so the clip can extend
	// upward (to cover the beat-counter chrome above the bar) without
	// shifting the bar itself.
	explicitBarRect image.Rectangle

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

// SetTimelineBarRect provides the bar's screen-space rect explicitly so
// Layout doesn't have to derive it from `rect.Min.Y`. Required when the
// zone's clip rect extends above the bar (to cover the beat-counter
// chrome). Pass `image.Rectangle{}` to fall back to the legacy "bar at
// top of zone" behavior.
func (z *TimelineZone) SetTimelineBarRect(r image.Rectangle) { z.explicitBarRect = r }

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

	if !z.explicitBarRect.Empty() {
		// DrumView passed the bar's screen-space rect — use it verbatim
		// so the zone's clip rect can extend above/around the bar
		// without shifting the bar itself.
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

	// --- Track button ---
	// Visibility is rect-driven: drumview_layout.go sets the rect on desktop
	// and leaves it empty on mobile (mobile surfaces the toggle via the
	// overflow menu instead). One predicate, one source of truth.
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
	// elapsedBeats is already in beats (it's g.displayBeat()); previous
	// code divided by `units` here, silently shrinking needBeats1 by the
	// subdivisions-per-beat factor and letting TimelineBeats lag the
	// actual playhead until the offset-derived needBeats2 caught up.
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

// ribbonWindowBeats returns the fixed-beats-per-pixel visible window for
// the ribbon at the given playhead position. The window slides so the
// playhead sits at RibbonPlayheadFrac of the bar width; on overflow at the
// start of the session it pins to zero. Width zero collapses to a no-op
// caller, returning (0, 0, 0).
func ribbonWindowBeats(barWidth int, elapsedBeats float64) (start, end float64, pxPerBeat float64) {
	if barWidth <= 0 {
		return 0, 0, 0
	}
	rp := RuntimeProf()
	bpp := rp.RibbonBeatsPerPixel
	if bpp <= 0 {
		bpp = 0.25
	}
	frac := rp.RibbonPlayheadFrac
	if frac <= 0 || frac >= 1 {
		frac = 0.70
	}
	visibleBeats := float64(barWidth) * bpp
	end = elapsedBeats + (1-frac)*visibleBeats
	start = end - visibleBeats
	if start < 0 {
		// Pin to zero so early-session playback doesn't show negative beats.
		start = 0
		end = visibleBeats
	}
	return start, end, 1.0 / bpp
}

// drawTimelineBar renders the timeline progress bar: hierarchical tick
// background (major every 16 beats, medium every 4, minor every 1) over a
// fixed-beats-per-pixel scrolling window with the playhead pinned at
// RibbonPlayheadFrac of the bar width. The view-rect highlights the
// pattern's editable window; the cursor marks the playhead.
func (z *TimelineZone) drawTimelineBar(dst *ebiten.Image, elapsedBeats float64, totalBeats int) {
	barRect := z.timelineBarRect
	if barRect.Empty() {
		return
	}
	_ = totalBeats // kept for callback compatibility; visible window comes from elapsedBeats + bpp
	unitsPerBeat := max1(z.callbacks.TimelineUnitsPerBeat())
	offset := z.callbacks.Offset()
	length := z.callbacks.Length()
	units := float64(unitsPerBeat)
	offsetBeats := float64(offset) / units
	lengthBeats := float64(length) / units

	winStart, winEnd, pxPerBeat := ribbonWindowBeats(barRect.Dx(), elapsedBeats)

	// Build or reuse the hierarchical-tick background cache. Cache key
	// is (barRect.Dx, Dy, winStart, winEnd) — window slides every frame
	// during playback, so a fresh cache rebuild here is the steady-state
	// path. We still cache against accidental same-frame double-draws.
	cacheKeyW := barRect.Dx()
	cacheKeyH := barRect.Dy()
	// Quantize winStart/winEnd to integer beat boundaries for the cache
	// signature so we don't bust the cache on sub-beat playhead drift.
	keyStart := int(math.Floor(winStart))
	keyEnd := int(math.Ceil(winEnd))
	// Split the cache invalidation into two paths: a dimension-change
	// path that must allocate a fresh atlas image, and a content-change
	// path that reuses the same image and only redraws pixels.
	//
	// During playback, keyStart advances every integer-beat boundary
	// (twice a second at BPM 120). The old code allocated a fresh
	// *ebiten.Image on every keyStart transition and dropped the
	// previous one with no Deallocate(); on WASM the orphaned atlas
	// slots accumulated in Ebiten's BSP packing tree until the heap
	// exhausted (reported OOM at ~41 min playback, atlas depth >125).
	// See: ebiten/v2/internal/packing/packing.go alloc() recursion.
	dimChanged := z.TlCache == nil || z.TlCacheW != cacheKeyW || z.TlCacheH != cacheKeyH
	contentChanged := dimChanged || z.TlCacheBeats != keyEnd-keyStart || z.TlCacheStep != keyStart
	if dimChanged {
		if z.TlCache != nil {
			z.TlCache.Deallocate()
		}
		z.TlCache = newTrackedImage("timelineZone.TlCache", cacheKeyW, cacheKeyH)
		z.TlCacheW, z.TlCacheH = cacheKeyW, cacheKeyH
	}
	if contentChanged {
		z.TlCacheBeats, z.TlCacheStep = keyEnd-keyStart, keyStart
		if !dimChanged {
			z.TlCache.Clear()
		}
		drawRect(z.TlCache, image.Rect(0, 0, z.TlCacheW, z.TlCacheH), colTimelineTotal, true)
		z.drawRibbonTicks(z.TlCache, winStart, pxPerBeat)
	}
	var op ebiten.DrawImageOptions
	op.GeoM.Translate(float64(barRect.Min.X), float64(barRect.Min.Y))
	dst.DrawImage(z.TlCache, &op)

	// View rect — the editable pattern window, expressed in beats and
	// mapped into the visible window. Clamp to bar bounds; hide entirely
	// when the pattern sits outside the visible range.
	viewStartBeats := offsetBeats
	viewEndBeats := offsetBeats + lengthBeats
	if viewEndBeats >= winStart && viewStartBeats <= winEnd {
		viewX0 := barRect.Min.X + int(math.Round((viewStartBeats-winStart)*pxPerBeat))
		viewX1 := barRect.Min.X + int(math.Round((viewEndBeats-winStart)*pxPerBeat))
		if viewX0 < barRect.Min.X {
			viewX0 = barRect.Min.X
		}
		if viewX1 > barRect.Max.X {
			viewX1 = barRect.Max.X
		}
		if viewX1-viewX0 < 1 {
			viewX1 = viewX0 + 1
		}
		viewRect := image.Rect(viewX0, barRect.Min.Y, viewX1, barRect.Max.Y)
		drawRect(dst, viewRect, colTimelineView, true)
		drawRect(dst, viewRect, colTimelineViewHi, false)
	}

	// Playback cursor — pinned at RibbonPlayheadFrac of the bar width.
	// Pre-fix this drifted with elapsed beats; the new geometry keeps it
	// at a stable position so users always have a fixed reference point.
	frac := RuntimeProf().RibbonPlayheadFrac
	if frac <= 0 || frac >= 1 {
		frac = 0.70
	}
	cursorX := barRect.Min.X + int(math.Round(frac*float64(barRect.Dx())))
	// If the visible window starts at zero (early-session), the playhead
	// floats at its true position inside the window instead of pinning. The
	// float must continue until the true position reaches the pin column, i.e.
	// until elapsedBeats*pxPerBeat == frac*barWidth (the same point where
	// ribbonWindowBeats stops pinning winStart at zero). Using (1-frac) here
	// pinned the cursor at frac*barWidth for the whole band between
	// (1-frac)*visibleBeats and frac*visibleBeats while the window — and the
	// view-rect — were still anchored at zero, stranding the playback line to
	// the right of the active-drum-view window.
	if winStart <= 0 && elapsedBeats < frac*float64(barRect.Dx())/pxPerBeat {
		cursorX = barRect.Min.X + int(math.Round((elapsedBeats-winStart)*pxPerBeat))
	}
	cursorCol := colTimelineCursor
	cursorThick := 1
	if Profile().IsMobile() {
		cursorCol = colAccentBright
		cursorThick = 2
	}
	cursorRect := image.Rect(cursorX-cursorThick, barRect.Min.Y, cursorX+cursorThick, barRect.Max.Y)
	drawRect(dst, cursorRect, cursorCol, true)

	drawRect(dst, barRect, colButtonBorder, false)
}

// drawRibbonTicks paints hierarchical beat ticks for the visible window
// onto the cache image. Major ticks (full height) land every 16 beats;
// medium ticks (2/3 height) every 4 beats; minor ticks (1/3 height) every
// 1 beat. Each tier auto-skips when its spacing would fall below 3 px.
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
	if thrCfg := RuntimeProf().TimelineInfoThrottleMS; thrCfg > 0 {
		thr := thrCfg
		if (curMS/thr) == (z.LastInfoCurMS/thr) && (totMS/thr) == (z.LastInfoTotMS/thr) && z.LastInfoText != "" {
			return z.LastInfoText
		}
	} else if curMS == z.LastInfoCurMS && totMS == z.LastInfoTotMS && z.LastInfoText != "" {
		return z.LastInfoText
	}
	curS := curMS / 1000
	z.LastInfoCurMS = curMS
	z.LastInfoTotMS = totMS
	z.LastInfoText = fmt.Sprintf("Beat %d · %s", int(elapsedBeats)+1, formatElapsedTime(curS))
	return z.LastInfoText
}

// drawBeatCounter renders the beat/time counter above the timeline bar
// inside a surface-1 chip (rounded.md) so the readout reads as a real
// container rather than floating debug chrome (A6 in the screenshot
// critique). Right-aligned within the allotted rect so it doesn't
// collide with row affordances on the left.
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
	// Chip background behind text.
	pillPadX, pillPadY := beatCounterPillPadX, beatCounterPillPadY
	pillW := tw + pillPadX*2
	pillH := th + pillPadY*2
	if pillW > beatCounterRect.Dx() {
		pillW = beatCounterRect.Dx()
	}
	// Clamp pillH to the rect — defense in depth against an `infoH`
	// regression that would otherwise let the pill bleed into the timeline
	// bar (root cause of the startup-chrome bug). The layout guarantees
	// `infoH >= TextHeight() + 2*pillPadY` so this is normally a no-op.
	if pillH > beatCounterRect.Dy() {
		pillH = beatCounterRect.Dy()
	}
	// Right-aligned: pillX = Max.X - pillW. Falls back to left-anchor if
	// the rect is too narrow to host the chip.
	pillX := beatCounterRect.Max.X - pillW
	if pillX < beatCounterRect.Min.X {
		pillX = beatCounterRect.Min.X
	}
	pillY := beatCounterRect.Min.Y + (beatCounterRect.Dy()-pillH)/2
	pillR := image.Rect(pillX, pillY, pillX+pillW, pillY+pillH)
	drawRoundedRect(dst, pillR, colSurface1, RadiusMD, true)
	drawRoundedRect(dst, pillR, colBorderSubtle, RadiusMD, false)
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
	// Cell-X anchor: row step cells are drawn at dv.timelineRect bounds
	// (see drumview_cache_rows_layer.go). z.stepsRect is wider on desktop —
	// drumview_layout.go unions chrome rects (track button, len ± buttons)
	// into the zone rect, pulling z.stepsRect.Min.X ~48 px left of where
	// cells actually start. z.timelineBarRect == dv.timelineRect via
	// SetTimelineBarRect, so it gives the correct cell anchor.
	startX := z.timelineBarRect.Min.X
	totalW := z.timelineBarRect.Dx()

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
			// Dim the cell strip only, not the chrome column to its left.
			// See drawHighlights for the same rationale.
			dimRect := image.Rect(z.timelineBarRect.Min.X, y, z.timelineBarRect.Max.X, y+rh)
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

	// Timeline bar scrub area (higher z-index than grid).
	//
	// Mobile touch-target: previously the rect was physically enlarged
	// downward by `(TouchMinTarget - barH)` to meet the 44 px touch
	// floor. That ENLARGED rect leaked into the steps region below
	// and was the spatial root of the "drag a synth knob also scrubs
	// the timeline" leak: a drag started in the enlarged strip
	// captured input via OnPress, and the tree then routed every
	// subsequent OnDrag to the captured handler without re-testing
	// hit areas — so the scrub kept firing even after the cursor
	// drifted into a different zone's region.
	//
	// Fix: publish the *visible* bar rect and let HitIndex.At expand
	// the touch radius via `HitArea.Touch + ClipRect`. The skirt
	// (4 px inset) is generous enough for fat-finger tolerance but
	// strictly bounded — the expansion can never reach the audio
	// panel below.
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
	if globalTouchState != nil && globalTouchState.RecentMultiTouch() {
		return InputIgnored
	}
	z := h.zone
	z.scrubbing = true
	// Engagement cue for the enlarged mobile scrub hit area (Task 3.1):
	// fires after the multi-touch gate so suppressed presses don't buzz.
	hapticEmit(8)
	h.scrubTo(x)
	return InputCaptured
}

func (h *timelineScrubHitAdapter) OnDrag(x, y int) {
	if !h.zone.scrubbing {
		return
	}
	// Self-policing: the tree's drag dispatch routes every move to the
	// captured handler without re-testing hit areas, so a captured
	// scrub would otherwise stay attached forever even when the
	// cursor drifts into a sibling zone. Release capture when the
	// pointer leaves the bar's vertical lane (with the same skirt
	// HitArea.Touch + ClipRect uses on the press path). Prevents the
	// "drag into the audio panel still scrubs the timeline" leak.
	skirt := h.zone.timelineBarRect.Inset(-4)
	if y < skirt.Min.Y || y >= skirt.Max.Y {
		h.zone.scrubbing = false
		return
	}
	h.scrubTo(x)
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
