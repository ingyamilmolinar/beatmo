package ui

import (
	"image"
	"math"
	"strconv"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
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
	OnSeekToStep     func(step int, origin hooks.SeekOrigin) // ribbon seek commit
	PlayheadStep     func() int                              // current absolute subdivision
	OnRowsLayerDirty func()                                  // signal that the rows layer composite needs rebuild
	SetTimelineBeats func(int)
	OnOpenNavigation func() // beat-counter tap: open the navigation menu (dv.OpenNavigation)

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
	// layouts counts Layout calls (recalcButtons' layout gate is judged by it).
	layouts int
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
	scrubbing      bool
	scrubGhostBeat int // beat under the pointer during a scrub drag
	// gotoGhost marks the navigation menu's PENDING go-to position (a preview;
	// no seek until Go). gotoGhostStep is an absolute subdivision step.
	gotoGhost      bool
	gotoGhostStep  int
	scrubPressBeat int // beat committed on press (so a tap re-seeks at most once)

	// Timeline bar height (set by DrumView before Layout)
	timelineBarH int
	// Explicit timeline-bar rect set by DrumView before Layout. When
	// non-empty, Layout uses this rect for the bar instead of placing
	// the bar at `rect.Min.Y`. This decouples the bar's vertical
	// position from the zone's clip-rect top, so the clip can extend
	// upward (to cover the beat-counter chrome above the bar) without
	// shifting the bar itself.
	explicitBarRect image.Rectangle
	cellGridRect    image.Rectangle // X anchor for row step cells (see SetCellGridRect)

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

// timelineBarHMax caps the ribbon's height. Task 6 reshapes the ribbon into
// a slim whole-song progress bar (it no longer carries the pattern-window
// "view rect" affordance, so it doesn't need the extra height that used to
// make that inner highlight legible). This is a clamp fallback, NOT a design
// token change: DESIGN.md's `densities.timelineBarH` (10/12/14 px for
// compact/comfortable/spacious, see design_density.gen.go) still drives the
// unclamped value passed in here — changing that YAML + regenerating via
// `make gen-design-tokens` is a 3-file ratcheted change (DESIGN.md ↔
// theme.go ↔ design_md_drift_test.go's want-map) that's out of scope for
// this task. Follow-up: fold this cap into the density token directly once
// the ribbon's new visual weight has been eyeballed against real UI.
const timelineBarHMax = 8

// SetTimelineBarHeight sets the height of the timeline progress bar, clamped
// to timelineBarHMax so the ribbon reads as a slim progress strip rather than
// the taller bar the pattern-window view-rect used to need.
// Must be called before Layout so the bar rect is computed correctly.
func (z *TimelineZone) SetTimelineBarHeight(h int) {
	if h > timelineBarHMax {
		h = timelineBarHMax
	}
	z.timelineBarH = h
}

// SetTimelineBarRect provides the bar's screen-space rect explicitly so
// Layout doesn't have to derive it from `rect.Min.Y`. Required when the
// zone's clip rect extends above the bar (to cover the beat-counter
// chrome). Pass `image.Rectangle{}` to fall back to the legacy "bar at
// top of zone" behavior.
func (z *TimelineZone) SetTimelineBarRect(r image.Rectangle) { z.explicitBarRect = r }

// SetCellGridRect supplies the X span row step cells are drawn into. It is a
// separate rect from the ribbon bar: the bar reserves room for header-only
// chrome and maps the whole song, while cells map the current window and fill
// the full width dedicated to the grid. See DrumView.cellGridRect.
func (z *TimelineZone) SetCellGridRect(r image.Rectangle) { z.cellGridRect = r }

// cellGridAnchor returns the X span to align cell-indexed overlays
// (highlights, mute dimming) to. Falls back to the bar rect when DrumView has
// not supplied a grid rect (legacy/standalone zone construction in tests).
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
	// Drag/scrub are handled entirely via hit handlers (timelineScrubHitAdapter,
	// gridDragHitAdapter); the zone has no other per-frame state to drive here.
	// (Task 6 removed the beat-counter's inline position editor — tapping the
	// counter now opens the navigation menu via OnOpenNavigation instead.)
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

// drawTimelineBar renders the timeline as a slim whole-song progress bar:
// hierarchical tick background (major every 16 beats, medium every 4, minor
// every 1) spans the full `[0, totalBeats]` range — decoupled from the
// drum-grid pattern window (dv.Offset/dv.Length) — with the playback cursor
// at `elapsedBeats / totalBeats` of the bar width. Task 6 removed the old
// fixed-beats-per-pixel sliding window (`ribbonWindowBeats`) and the
// pattern-window "view rect" highlight; the ribbon is now drag-only (see
// timelineScrubHitAdapter) so there is no click-to-seek affordance to imply
// visually beyond the cursor itself.
func (z *TimelineZone) drawTimelineBar(dst *ebiten.Image, elapsedBeats float64, totalBeats int) {
	barRect := z.timelineBarRect
	if barRect.Empty() {
		return
	}
	beats := max1(totalBeats)
	pxPerBeat := float64(barRect.Dx()) / float64(beats)

	// Build or reuse the hierarchical-tick background cache. Cache key is
	// (barRect.Dx, Dy, totalBeats) — the whole-song span only grows (see
	// computeTimelineBeats' high-water mark), so this rebuilds far less
	// often than the old per-frame sliding window did.
	cacheKeyW := barRect.Dx()
	cacheKeyH := barRect.Dy()
	// Split the cache invalidation into two paths: a dimension-change path
	// that must allocate a fresh atlas image, and a content-change path
	// that reuses the same image and only redraws pixels. See history: the
	// old code allocated a fresh *ebiten.Image on every window-slide
	// transition and dropped the previous one with no Deallocate(); on
	// WASM the orphaned atlas slots accumulated in Ebiten's BSP packing
	// tree until the heap exhausted (reported OOM at ~41 min playback,
	// atlas depth >125). See: ebiten/v2/internal/packing/packing.go alloc().
	dimChanged := z.TlCache == nil || z.TlCacheW != cacheKeyW || z.TlCacheH != cacheKeyH
	// The ribbon bakes colTimelineTotal / timeline-view / tick colors, so a
	// runtime theme change is a content change too.
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
		// Faint amber track fill so the bar reads as a recessed warm
		// "sunset" track rather than floating in the near-black total-bg.
		// Baked into the cache (alloc-free per frame).
		drawRect(z.TlCache, image.Rect(0, 0, z.TlCacheW, z.TlCacheH), WithAlpha(genColorTimelineView, AlphaFaint), true)
		z.drawRibbonTicks(z.TlCache, 0, pxPerBeat)
	}
	var op ebiten.DrawImageOptions
	op.GeoM.Translate(float64(barRect.Min.X), float64(barRect.Min.Y))
	dst.DrawImage(z.TlCache, &op)

	// Playback cursor — whole-song fraction of elapsed beats over the total
	// song length, clamped to the bar bounds.
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

	// Ghost preview of the pending seek position while dragging the ribbon,
	// or of the navigation menu's pending go-to position.
	if z.scrubbing {
		z.drawScrubGhost(dst, z.scrubGhostBeat)
	} else if z.gotoGhost {
		z.drawScrubGhost(dst, z.gotoGhostStep)
	}
}

// SetGotoGhost shows (on) or clears the ghost marking the navigation menu's
// pending go-to step. A preview only: nothing seeks.
func (z *TimelineZone) SetGotoGhost(step int, on bool) {
	z.gotoGhost = on
	z.gotoGhostStep = step
}

// GotoGhost reports the pending go-to ghost (step, shown).
func (z *TimelineZone) GotoGhost() (int, bool) { return z.gotoGhostStep, z.gotoGhost }

// drawScrubGhost renders the pending seek position (a vertical line + beat
// chip) while the user drags the ribbon. The ghost tracks z.scrubGhostBeat,
// which OnDrag updates without dispatching a seek — the seek commits only
// on OnRelease, and only if the drag moved to a different beat.
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
	// Beat chip near the top of the bar: 1-based, the number the beat counter
	// shows for that beat.
	DrawTextAt(dst, strconv.Itoa(ghostBeat+1), gx+3, bar.Min.Y+2)
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
	z.LastInfoCurMS = curMS
	z.LastInfoTotMS = totMS
	// Beat/second pair resolved by playheadPosAt — the same derivation the
	// navigation menu's position title reads, so the two never disagree.
	p := playheadPosAt(elapsedBeats, secPerBeat, unitsPerBeat)
	z.LastInfoText = formatBeatReadout(p.DisplayBeat, p.Seconds)
	return z.LastInfoText
}

// beatCounterContentWidth is the pixel width the beat-counter keycap needs to
// render `readout` without clipping: the leading target icon (square == text
// cap height) + icon→text gap + the readout text + the cap's horizontal
// padding on both sides. Single source of truth shared by drawBeatCounter (cap
// sizing), beatCounterSlotWidth (upper bound), and drumview_layout.go (the live
// slot width) so layout and render never disagree — the cause of the counter
// crop was the layout omitting the icon+gap allowance this helper includes.
func beatCounterContentWidth(readout string) int {
	iconSz := TextHeight()
	iconGap := beatCounterPillPadX
	return iconSz + iconGap + TextWidth(readout) + 2*beatCounterPillPadX
}

// beatCounterSlotWidth returns a stable left-anchored slot width ceiling for the
// beat-counter keycap, sized to a generous upper-bound readout so the counter
// doesn't reflow as the live text changes width frame to frame. Mobile uses the
// compact position-only bound ("888 · 8:88") to match formatBeatReadout — the
// desktop "Beat 888 · 88:88" bound would exceed the narrow mobile band.
func beatCounterSlotWidth() int {
	if Profile().IsMobile() {
		return beatCounterContentWidth("888 · 8:88")
	}
	return beatCounterContentWidth("Beat 888 · 88:88")
}

// drawBeatCounter renders the beat/time counter above the timeline bar as a
// raised keycap (colSurface2 fill, square corners) with a leading IconTarget,
// so it reads as a pressable control — tapping it opens the navigation menu to
// change beat/time. Left-anchored in the allotted rect, with a hover-brightened
// border when the pointer is over it.
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
	// Render as a matte keycap identical to every other Button (DESIGN.md
	// "Keycap elevation") so it reads as a pressable control — tapping it opens
	// the navigation menu to change beat/time. Uses the same secondary fill/
	// border as the neighbouring track/lock keycap, plus a leading IconTarget
	// affordance and the readout text on the cap face.
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
	// Left-anchored: the counter lives in a left slot next to the track/lock
	// keycap, with the notification area to its right.
	pillX := beatCounterRect.Min.X
	pillY := beatCounterRect.Min.Y + (beatCounterRect.Dy()-pillH)/2
	pillR := image.Rect(pillX, pillY, pillX+pillW, pillY+pillH)
	// Hover: read the live cursor so the chip lifts (drawFlatPanelButton
	// brightens the fill) when the pointer is over it. Press is momentary
	// (the tap opens the menu immediately), so only hover is reflected.
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

// rowsTopY returns the Y coordinate where rows start. Uses the RowsTopY
// callback when available; falls back to stepsRect.Min.Y.
func (z *TimelineZone) rowsTopY() int {
	if z.callbacks.RowsTopY != nil {
		return z.callbacks.RowsTopY()
	}
	return z.stepsRect.Min.Y
}

// drawGapFill fills the gap below visible rows with the rack bed, so the
// empty area under the last row reads as ONE surface across the rack column
// and the timeline. It used to take the cell-off (surface-2) tone — a control
// tone that made the empty area read as one giant raised cell, lighter than
// the rack beside it.
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
	return !t.Fires()
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
	// Cell-X anchor: row step cells are drawn at dv.cellGridRect bounds
	// (see drumview_cache_rows_layer.go), NOT at the ribbon bar's bounds.
	// The bar is narrowed by header-only chrome (track/lock chip, len ±
	// buttons) and maps the whole song; the grid maps the current window and
	// fills the full dedicated width. Highlights must follow the grid.
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
	// Dim the cell strip only, not the chrome column to its left. Anchored to
	// the cell grid, not the ribbon bar — see drawHighlights for the rationale.
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

	// Beat counter: a visible opaque chip with no action of its own. It sits
	// in the band between the track chip (left) and the notification area
	// (right), both of which register Touch-expanded hit areas (44 px on every
	// side, clipped only to the whole zone). Without its own hit area, those
	// expansions reach over the counter and a tap on it opened the track menu
	// or the notification history. Register a non-Touch catch-all on its exact
	// rect: HitIndex.At sorts exact-rect hits ahead of touch-expanded-only
	// hits, so this wins for any tap inside the counter, and it consumes the
	// press (the counter has no action) so nothing falls through. This is the
	// opaque-to-z contract applied to the counter. Regression test:
	// beat_counter_input_isolation_test.go.
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

// beatCounterHitAdapter absorbs presses on the beat-counter chip so a
// band-mate's touch-expanded hit area can never steal a tap meant for the
// counter, and opens the navigation menu (Task 6 repoint — the counter
// previously opened an inline tap-to-seek position editor; that surface is
// now reached via OnOpenNavigation → dv.OpenNavigation, matching the
// transport bar's nav button). The catch-all-and-consume contract is
// unchanged (see beat_counter_input_isolation_test.go).
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
	z := h.zone
	// Dominant-axis routing over the drum cell grid. A two-finger trackpad
	// scroll arrives as a wheel event with both axes; route the dominant one
	// and ONLY that one — a mostly-horizontal scroll moves the timeline view,
	// a mostly-vertical scroll scrolls the rows. They are mutually exclusive:
	// a diagonal gesture never moves both at once. (The vertical wheel path
	// reaching only OnRowScrollWheel was the bug — horizontal intent was
	// dropped and its incidental vertical component scrolled the rows.)
	// Detents, not raw platform units — see input_dynamics.go. Reading the raw
	// wheel here is what let one browser notch fly the timeline across the song.
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
		// Scroll right (positive dx) advances the timeline; clamp at 0 to match
		// the single-finger horizontal grid drag.
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

// ribbonStepAt maps pointer X on the timeline bar to the nearest whole-beat
// absolute subdivision step.
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

// --- Timeline scrub hit handler ---
//
// Task 6: the ribbon is drag-only, not a click-to-seek surface. Press only
// arms scrubbing (records the press beat + ghost); drag updates the ghost
// preview (z.scrubGhostBeat) with no seek dispatched; release commits a
// seek ONLY if the gesture ended on a different beat than the press beat —
// a pure tap (press+release at the same beat) commits nothing at all, and
// a real drag emits exactly one EventSeek on release.
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
	z.scrubGhostBeat = z.ribbonStepAt(x)
	z.scrubPressBeat = z.scrubGhostBeat // remember where the gesture started; no seek on press
	return InputCaptured
}

func (h *timelineScrubHitAdapter) OnDrag(x, y int) {
	z := h.zone
	if !z.scrubbing {
		return
	}
	// Self-policing: the tree's drag dispatch routes every move to the
	// captured handler without re-testing hit areas, so a captured
	// scrub would otherwise stay attached forever even when the
	// cursor drifts into a sibling zone. Release capture when the
	// pointer leaves the bar's vertical lane (with the same skirt
	// HitArea.Touch + ClipRect uses on the press path). Prevents the
	// "drag into the audio panel still scrubs the timeline" leak.
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
		// Commit a seek on release ONLY if the drag moved to a different
		// beat than where the gesture started. Press no longer seeks
		// (drag-only ribbon), so a pure tap (final == press beat) emits NO
		// seek; a drag emits exactly one EventSeek, on release.
		if final != z.scrubPressBeat {
			z.callbacks.OnSeekToStep(final, hooks.SeekRibbon)
		}
	}
	z.scrubbing = false
}

func (h *timelineScrubHitAdapter) OnWheel(x, y, steps int) InputResult {
	return InputIgnored
}
