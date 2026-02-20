package ui

import (
	"image"
	"image/color"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

const (
	// timelineHeight reserves vertical space for the transport controls and
	// the thin timeline bar above the instrument rows. Keep this as small as
	// possible so the bottom panel wastes no vertical space. Two rows of
	// controls (2×rowHeight) + a 10px bar + a small margin is sufficient.
	timelineHeight = 36
	// desktopHeaderH is the target header height for the two-row desktop transport bar.
	desktopHeaderH = 72
	// mobileHeaderH is the target header height for the two-row mobile transport bar.
	mobileHeaderH               = 72
	mobileHeaderMaxH            = 72
	timelineBarHeightDesktop    = 12
	timelineBarHeightMobile     = 14
	buttonPad                   = 2
	defaultRowCachePadPx        = 32
	defaultRowsLayerPadPx       = 96
	wasmStripeTargetPx          = 440
	wasmStripeMaxCount          = 12
	instMenuMaxVisibleRows      = 8
	instMenuScrollBarWidth      = 10
	eqChannelMenuMaxVisibleRows = 8
	eqChannelMenuScrollBarWidth = 10
	fallbackInstCategory        = "Registered"
)

// tlBarHeight returns the timeline bar height, larger on mobile for touch targets.
func tlBarHeight() int { return Profile().TimelineBarH }

// instMenuMode is an enum describing the instrument dropdown view.
type instMenuMode string

const (
	instMenuModeUnset       instMenuMode = ""
	instMenuModeCategories  instMenuMode = "categories"
	instMenuModeInstruments instMenuMode = "instruments"
)

// viewMode describes which pane is visible in the mobile drum view.
type viewMode int

const (
	viewModeRows  viewMode = iota // drum rows visible
	viewModeAudio                 // EQ/Wave panel visible (has its own EQ↔Wave toggle)
)

var eqPanelHeight = 190

type eqBandDef struct {
	loHz float64
	hiHz float64
}

// 10-band ISO standard 1-octave center frequencies (31–16k Hz).
// Edges: lo = center/sqrt(2), hi = center*sqrt(2), last band capped at 20 kHz.
var eqBandDefs = []eqBandDef{
	{loHz: 22, hiHz: 44},       // 31 Hz
	{loHz: 44, hiHz: 88},       // 62 Hz
	{loHz: 88, hiHz: 177},      // 125 Hz
	{loHz: 177, hiHz: 354},     // 250 Hz
	{loHz: 354, hiHz: 707},     // 500 Hz
	{loHz: 707, hiHz: 1414},    // 1 kHz
	{loHz: 1414, hiHz: 2828},   // 2 kHz
	{loHz: 2828, hiHz: 5657},   // 4 kHz
	{loHz: 5657, hiHz: 11314},  // 8 kHz
	{loHz: 11314, hiHz: 20000}, // 16 kHz
}

/* ───────────────────────────────────────────────────────────── */

type DrumRow struct {
	Name        string
	Instrument  string
	Steps       []bool
	CellTypes   []model.NodeType
	Color       color.Color
	Origin      model.NodeID
	Node        *uiNode
	Volume      float64
	Muted       bool
	Solo        bool
	EQGainsDB   []float64          // Per-instrument EQ gains (10 bands, range ±24dB)
	EQBandMuted []bool             // Per-band mute state (10 bands)
	HPFEnabled  bool               // Per-row high-pass filter on/off
	HPFCutoffHz float64            // Per-row HPF cutoff frequency (20–2000 Hz)
	LPFEnabled  bool               // Per-row low-pass filter on/off
	LPFCutoffHz float64            // Per-row LPF cutoff frequency (1000–20000 Hz)
	Effects     []audio.EffectSlot // Ordered insert effect chain
	Pan         float64            // Stereo pan: -1 (left) to +1 (right), 0 = center
	DelaySend   float64            // Delay send amount (0-1)
	ReverbSend  float64            // Reverb send amount (0-1)
}

func instColor(id string) color.Color {
	if c, ok := instColors[id]; ok {
		return c
	}
	if c, ok := customColors[id]; ok {
		return c
	}
	if len(customPalette) == 0 {
		// Graceful fallback when palette is empty (e.g., in tests).
		return color.RGBA{200, 200, 200, 255}
	}
	c := customPalette[nextCustomColor%len(customPalette)]
	nextCustomColor++
	customColors[id] = c
	return c
}

/* ───────────────────────────────────────────────────────────── */

type uploadResult struct {
	path string
	err  error
}

type importResult struct {
	data []byte
	err  error
}

type DrumView struct {
	Rows             []*DrumRow
	Bounds           image.Rectangle
	Graph            *model.Graph
	logger           *game_log.Logger
	tree             *DrumViewTree     // zone-based component tree (Phase 1+)
	eqPanelZone      *EQPanelZone      // Phase 2: EQ panel zone (owns EQ buttons/sliders/state)
	transportZone    *TransportZone    // Phase 3: transport zone (owns transport buttons/state)
	rowRackZone      *RowRackZone      // Phase 4: row rack zone (owns per-row buttons/sliders/scroll)
	timelineZone     *TimelineZone     // Phase 5: timeline zone (owns drag/scrub)
	layoutResizeZone *layoutResizeZone // layout resize zone (column/row divider pills)

	// Overlay components (Phase 5 integration)
	subdivMenuComp *SubdivMenuComponent
	renameComp     *RenameComponent
	colorWheelComp *ColorWheelComponent
	instMenuComp   *InstrumentMenuComponent

	// widget layout (bottom pane is divided into movable widgets)
	widgets         *WidgetBoard
	widgetRects     map[WidgetKind]image.Rectangle
	headerH         int // dynamic header height (replaces timelineHeight)
	eqH             int // dynamic EQ panel height
	layoutHoverAxis string
	layoutHoverIdx  int
	layoutDragAxis  string
	layoutDragIdx   int
	layoutDragPrev  int
	layoutHandler   *LayoutResizeHandler // widget-span-aware layout resize

	cell      int // px per step
	labelW    int
	controlsW int // width reserved for control buttons

	// control-panel components (non-transport)
	lenDecBtn       *Button // decrease length
	lenIncBtn       *Button // increase length
	saveBtn         *Button
	mainVolRect     image.Rectangle
	mainVolIconRect image.Rectangle

	// per-row components: owned by rowRackZone, accessed via accessor methods.
	selRow int

	// instrument selection dropdown
	instMenuRow                int
	instMenuBtns               []*Button
	instMenuScroll             VerticalScroller
	instMenuLastAdded          string
	instMenuUserScrolled       bool
	instCategories             []string
	instCategoryBtns           []*Button
	instMenuFullRect           image.Rectangle
	instMenuMode               instMenuMode
	instMenuActiveCat          string
	instMenuForceCategories    bool
	instMenuCameFromCategories bool
	instMenuActiveByRow        map[int]string // remembers last category per row
	instCatByID                map[string]string
	instMeta                   map[string]audio.SoundMeta
	instSearch                 string
	instSearchBox              *TextInput
	instSearchRect             image.Rectangle
	instOrderIndex             map[string]int // optional custom ordering index
	instCatalogVersion         uint64
	instRegistryVersion        uint64
	instRefreshDirty           bool

	// color picker: wheel popup
	colorMenuRow   int
	colorWheelRect image.Rectangle
	colorWheelImg  *ebiten.Image
	wheelCacheW    int
	wheelCacheH    int

	// FX panel
	fxPanelRow       int
	fxPanelRect      image.Rectangle
	fxPanelBtns      []*Button         // add/remove/toggle/reorder buttons within the panel
	fxPanelSliders   []*Slider         // param sliders within the panel
	fxSliderBindings []fxSliderBinding // maps slider index to effect param
	fxSliderDragging bool              // active slider drag in FX panel
	fxSliderDragIdx  int               // index into fxPanelSliders being dragged
	fxAddMenuOpen    bool
	fxExpandedSlots  map[int]bool    // which effect slots are expanded (mobile only)
	fxScrollOffsetPx int             // pixel scroll offset for FX panel content area
	fxScrollTS       TouchScroller   // touch scroll tracking for FX panel
	fxScrollMaxPx    int             // max scroll offset (contentH - viewportH), 0 = no scroll
	fxSliderLeft     int             // computed label area width for FX param rows (desktop)
	fxViewportRect   image.Rectangle // scrollable content area (between header and footer)

	// subdiv dropdown
	subdivMenuBtns []*Button

	deleted            []deletedRow
	added              []int
	originReq          []int
	deleteConfirmRow   int   // row pending delete confirmation, -1 = none
	deleteConfirmFrame int64 // frame when first click happened
	renameRow          int
	renameBox          *TextInput

	// import handler
	onImport            func([]byte) error
	onImportDialogStart func()
	onImportDialogEnd   func()

	bgDirty          bool
	layoutSuppressed bool
	bgCache          []*ebiten.Image

	instOptions []string
	instAvail   map[string]bool
	missingInst map[string]bool
	instMu      sync.RWMutex

	uploading  bool
	uploadCh   chan uploadResult
	pendingWAV string
	nameInput  string     // legacy helper, not used for typing anymore
	nameBox    *TextInput // active while naming

	// JSON import
	importing      bool
	importCh       chan importResult
	lengthChanging bool // Set true during length change to suppress parity

	// callback to change subdivisions per beat
	onChangeSubdiv func(int) error

	// sample loading status (WASM): show a transient message while embedded
	// samples are being registered and another once finished.
	samplesTotal  int
	samplesLoaded int
	showLoading   bool
	doneMsgTimer  int // frames to show "Finished loading samples"

	// per-row cached sprites for the steps area (no highlights). Each sprite
	// covers the full timeline width and one row height. Rebuilt when length,
	// row color, or timeline width/height change.
	rowCache        []*ebiten.Image
	rowDirty        []bool
	rowFullDirty    []bool
	rowCacheW       int
	rowCacheH       int
	rowCacheLen     int
	rowCacheOff     []int
	rowCacheGen     []int
	rowCacheSig     []uint64
	rowCacheSteps   [][]bool
	rowCacheTypes   [][]model.NodeType
	rowCachePadPx   int
	rowCacheShift   int
	rowCachePatch   int
	rowCacheFull    int
	rowCacheScratch []*ebiten.Image // double-buffer scratch for row sprite shifts

	// Segmented timeline slices populated each refresh; mirrors TimelineSegments.
	timelineOffset    []int
	timelinePast      [][]bool
	timelinePastTypes [][]model.NodeType
	timelinePresent   [][]bool
	timelineFuture    [][]bool

	// rowsLayer caches the composition of all visible row sprites (rowCache)
	// for the current offset and scroll. Highlights are drawn on top separately.
	rowsLayer        *ebiten.Image
	rowsLayerW       int
	rowsLayerH       int
	rowsLayerOffset  int
	rowsLayerRowOff  int
	rowsLayerBaseX   int
	rowsLayerGen     int
	rowsLayerDirty   bool
	rowsLayerPadPx   int
	rowsLayerScratch *ebiten.Image // double-buffer scratch for layer shifts
	rowsLayerBytes   int64
	rowsLayerFrame   int64
	// WASM-only adaptive pad helpers: expand pad when frequent full rebuilds
	// happen due to horizontal pans; decay toward default when stationary.
	rowsPadFullRebuilds int
	rowsPadLastDecay    int64

	// Prototype: stripe-based rows layer (WASM-only gated via JS export).
	rowsStripingEnabled bool
	rowsStripeCount     int
	rowsStripeAuto      bool
	rowsStripeAutoLarge int
	rowsStripeAutoCalm  int
	rowsStripes         []*ebiten.Image
	rowsStripeStarts    []int // local X starts inside timelineRect
	rowsStripeWidths    []int // widths per stripe
	rowsStripeRowOff    int
	rowsStripeOffset    int
	rowsStripeGen       int
	rowsStripeScratch   []*ebiten.Image
	rowsStripeCachedW   []int // cached stripe widths to avoid img.Size() calls
	rowsStripeCachedH   []int // cached stripe heights to avoid img.Size() calls
	rowsStripeLastFrame int64 // frame counter for skip optimization
	// Last-frame row draw mask for visibility assertions (set during Draw)
	rowsDrawnMask   []bool
	rowsRepaints    int
	directDrawCount int64 // frames where drawRowsDirect was used (mobile fallback)
	directDrawCells int   // cells drawn in last directDraw call
	rowFrame        []int64
	rowRepaint      []int
	panelMaskRect   image.Rectangle

	// Mobile EQ collapse: hides the Wave/EQ panel by default on mobile.
	mobileEQCollapsed bool
	mobileEQInited    bool // true once the mobile-default has been applied
	mobileEQMode      bool // true = EQ panel replaces rows on mobile

	// userAdjustedLength is set when the user manually changes the timeline
	// length via +/- buttons. When false, the mobile default cap is applied
	// in SetBounds and updateBeatInfos to keep the initial view at 8 beats.
	userAdjustedLength bool

	// Mobile view switching (Rows → EQ → Wave cycle)
	currentViewMode viewMode

	// Mobile overflow menu for Upload/Import/Export
	overflowScroll *ScrollBehavior // scroll when items overflow

	// Mobile beat counter rect (inside transport row, 7th column on mobile)
	beatCounterRect image.Rectangle

	// Mobile per-row volume popup
	volPopup    *SliderPopup
	volPopupRow int // which row's volume is being edited

	// Master volume popup (desktop icon-click opens vertical slider)
	masterVolPopup *SliderPopup

	// Mobile context menu for row controls (long-press on row label)
	contextMenuRow        int
	contextMenuRect       image.Rectangle
	contextMenuBtns       []*Button
	contextMenuIcons      []string         // icon name per button (parallel to contextMenuBtns, excluding close btn)
	contextMenuHeaderRect image.Rectangle // mobile bottom sheet header area
	contextMenuScroll     *ScrollBehavior // scroll when items overflow

	// EQ visualization (supports master and per-instrument channels)
	eqRect          image.Rectangle
	eqWaveformMode  bool
	eqBandVals      []float64
	eqLastBands     []float64
	eqApplied       []audio.EQBand
	masterGainsDB   []float64       // master channel EQ gains (separate from zone working copy)
	masterMuted     []bool          // master channel EQ mute state
	eqActiveChannel string          // "main" or instrument ID; empty defaults to "main"
	eqChannelScroll *ScrollBehavior // Scroll state for EQ channel dropdown
	// Deferred taps: two-phase mobile tap pattern (position stored on press,
	// fired on release if no scroll committed). Replaces 15 individual fields.
	eqChDeferredTap        DeferredTap
	instMenuDeferredTap    DeferredTap
	contextMenuDeferredTap DeferredTap
	overflowDeferredTap    DeferredTap
	fxPanelDeferredTap     DeferredTap

	// EQ frequency response curve
	eqCurveDragBand   int                       // -1 when not dragging, else band index
	eqCurveDragFilter string                    // "" when not dragging, "hpf" or "lpf" when dragging a filter handle
	eqCurveDirty      bool                      // true when EQ settings changed
	eqCurveCache      []audio.FreqResponsePoint // cached response curve

	// High-pass and low-pass filter state (master channel)
	hpfEnabled  bool    // high-pass filter on/off
	hpfCutoffHz float64 // cutoff frequency (20–2000 Hz range)
	lpfEnabled  bool    // low-pass filter on/off
	lpfCutoffHz float64 // cutoff frequency (1000–20000 Hz range)

	// eqTestSnapshot lets tests inject a deterministic analyzer reading.
	eqTestSnapshot      *audio.AnalyzerSnapshot
	eqTestPreEQSnapshot *audio.AnalyzerSnapshot

	frame int64

	timelineRect  image.Rectangle // progress bar for fast seek
	timelineBeats int             // total beats represented by timeline
	// Units per beat used by the timeline header. When running under Game,
	// this is set to the grid's MaxDiv so offsets/length (which are tracked
	// in subdivision steps) are mapped to beats consistently in the header.
	timelineUnitsPerBeat int

	// internal ui state
	bpm           int
	secPerBeat    float64
	playPressed   bool
	stopPressed   bool
	Length        int  // Length of the drum view, independent of graph
	lenIncPressed bool // State for length increase button
	lenDecPressed bool // State for length decrease button
	follow        bool // auto-scroll with playback
	bpmPrev       int  // previous BPM before editing
	bpmDelta      int  // accumulated BPM adjustments from +/- buttons

	// button animations
	playAnim     float64
	stopAnim     float64
	bpmDecAnim   float64
	bpmIncAnim   float64
	lenDecAnim   float64
	lenIncAnim   float64
	uploadAnim   float64
	bpmErrorAnim float64
	saveAnim     float64
	namePhase    float64
	savePressed  bool

	// window scrolling
	Offset        int // index of first visible beat
	dragging      bool
	offsetChanged bool

	rowOffset         int
	rowScrollFromZone bool // set by zone callbacks to signal zone→dv sync needed

	scrubbing bool

	// mouseDownInBounds is true while mouse is pressed and press started in drum view.
	// Prevents splitter (or other handlers) from stealing mid-drag.
	mouseDownInBounds bool
	// inputCapturedExternally is set by Game when another handler holds dispatcher capture.
	// Prevents drum view from starting new interactions when another component owns input.
	inputCapturedExternally bool

	// Local cache of custom instrument WAV paths by instrument ID. Used for
	// export so imported projects can auto-register custom samples.
	samplePath map[string]string

	// Playing flag (copied from Game) so DrumView can freeze header scale
	// and avoid cursor jumps during active playback.
	isPlaying bool

	// simple draw hint propagated from Game; enables low-overhead highlight path
	simpleDraw bool
	// perfDrawLite skips expensive, non-critical chrome when perf fast path is on.
	perfDrawLite bool

	// notifications: small popups in the top-right of the drum view panel
	notifs []notification
	// import
	importAttemptFrame  int
	importAttemptUpdate int

	// update sequence (increments each Update call)
	updateSeq int

	// Label caching for performance: avoids O(N) string ops every frame.
	// instLabelCache caches computed display labels for instrument IDs.
	instLabelCache map[string]string
	// cachedLabelW stores the computed labelW; labelWidthDirty triggers recompute.
	cachedLabelW    int
	labelWidthDirty bool
	// Track data signature to detect stale cache even when labelWidthDirty is false.
	labelCacheRowCount int
	labelCacheInstLen  int
	labelCacheRowNames []string // cached row names to detect direct modifications

	// Toolbar caching: renders transport buttons/sliders to a cached image
	// to avoid ~130 DrawImage calls every frame (1 blit on cache hit).
	toolbarCache     *ebiten.Image
	toolbarCacheHash uint64

	// Row controls caching: renders per-row buttons/sliders to a cached image
	// to avoid excessive DrawImage calls every frame.
	rowControlsCacheDirty bool
	rowControlsCacheRect  image.Rectangle // cached bounds
}

// --- EQ zone accessor methods (delegate to eqPanelZone) ---

func (dv *DrumView) eqMuteBtns() []*Button    { return dv.eqPanelZone.eqMuteBtns }
func (dv *DrumView) eqBandGainsDB() []float64 { return dv.eqPanelZone.bandGainsDB }
func (dv *DrumView) eqBandMuted() []bool      { return dv.eqPanelZone.bandMuted }
func (dv *DrumView) eqToggleBtn() *Button     { return dv.eqPanelZone.eqToggleBtn }
func (dv *DrumView) eqChannelBtn() *Button    { return dv.eqPanelZone.eqChannelBtn }
func (dv *DrumView) hpfBtn() *Button          { return dv.eqPanelZone.hpfBtn }
func (dv *DrumView) lpfBtn() *Button          { return dv.eqPanelZone.lpfBtn }

// --- RowRack zone accessor methods (delegate to rowRackZone) ---

func (dv *DrumView) addRowBtn() *Button {
	if dv.rowRackZone == nil {
		return nil
	}
	return dv.rowRackZone.AddRowButton()
}
func (dv *DrumView) rowEntryCount() int {
	if dv.rowRackZone == nil {
		return 0
	}
	return dv.rowRackZone.EntryCount()
}
func (dv *DrumView) rowLabels() []*Button {
	if dv.rowRackZone == nil {
		return nil
	}
	return dv.rowRackZone.RowLabels()
}
func (dv *DrumView) rowEditBtns() []*Button {
	if dv.rowRackZone == nil {
		return nil
	}
	return dv.rowRackZone.RowEditBtns()
}
func (dv *DrumView) rowSaveBtns() []*Button {
	if dv.rowRackZone == nil {
		return nil
	}
	return dv.rowRackZone.RowSaveBtns()
}
func (dv *DrumView) rowColorBtns() []*Button {
	if dv.rowRackZone == nil {
		return nil
	}
	return dv.rowRackZone.RowColorBtns()
}
func (dv *DrumView) rowDeleteBtns() []*Button {
	if dv.rowRackZone == nil {
		return nil
	}
	return dv.rowRackZone.RowDeleteBtns()
}
func (dv *DrumView) rowVolSliders() []*Slider {
	if dv.rowRackZone == nil {
		return nil
	}
	return dv.rowRackZone.RowVolSliders()
}
func (dv *DrumView) rowOriginBtns() []*Button {
	if dv.rowRackZone == nil {
		return nil
	}
	return dv.rowRackZone.RowOriginBtns()
}
func (dv *DrumView) rowMuteBtns() []*Button {
	if dv.rowRackZone == nil {
		return nil
	}
	return dv.rowRackZone.RowMuteBtns()
}
func (dv *DrumView) rowSoloBtns() []*Button {
	if dv.rowRackZone == nil {
		return nil
	}
	return dv.rowRackZone.RowSoloBtns()
}
func (dv *DrumView) rowMenuBtns() []*Button {
	if dv.rowRackZone == nil {
		return nil
	}
	return dv.rowRackZone.RowMenuBtns()
}
func (dv *DrumView) rowFXBtns() []*Button {
	if dv.rowRackZone == nil {
		return nil
	}
	return dv.rowRackZone.RowFXBtns()
}
func (dv *DrumView) rowGroups() []RowButtonGroup {
	if dv.rowRackZone == nil {
		return nil
	}
	return dv.rowRackZone.RowGroups()
}
func (dv *DrumView) rowVolGroup() *SliderGroup {
	if dv.rowRackZone == nil {
		return nil
	}
	return dv.rowRackZone.RowVolGroup()
}
func (dv *DrumView) rowScroll() *ScrollBehavior {
	if dv.rowRackZone == nil {
		return nil
	}
	return dv.rowRackZone.RowScroll()
}

// --- Transport zone accessor methods (delegate to transportZone) ---

func (dv *DrumView) playBtn() *Button             { return dv.transportZone.playBtn }
func (dv *DrumView) stopBtn() *Button             { return dv.transportZone.stopBtn }
func (dv *DrumView) bpmDecBtn() *Button           { return dv.transportZone.bpmDecBtn }
func (dv *DrumView) bpmBox() *TextInput           { return dv.transportZone.bpmBox }
func (dv *DrumView) bpmIncBtn() *Button           { return dv.transportZone.bpmIncBtn }
func (dv *DrumView) subdivBtn() *Button           { return dv.transportZone.subdivBtn }
func (dv *DrumView) trackBtn() *Button            { return dv.transportZone.trackBtn }
func (dv *DrumView) uploadBtn() *Button           { return dv.transportZone.uploadBtn }
func (dv *DrumView) importBtn() *Button           { return dv.transportZone.importBtn }
func (dv *DrumView) exportBtn() *Button           { return dv.transportZone.exportBtn }
func (dv *DrumView) eqToggleMobile() *Button      { return dv.transportZone.eqToggleMobile }
func (dv *DrumView) viewSwitchBtn() *Button       { return dv.transportZone.viewSwitchBtn }
func (dv *DrumView) overflowBtn() *Button         { return dv.transportZone.overflowBtn }
func (dv *DrumView) mainVolSlider() *Slider       { return dv.transportZone.mainVolSlider }
func (dv *DrumView) mainVolGroup() *SliderGroup   { return dv.transportZone.mainVolGroup }
func (dv *DrumView) transportGroup() *LayoutGroup { return dv.transportZone.transportGroup }
