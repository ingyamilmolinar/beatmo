package ui

import (
	"image"
	"image/color"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/tunkul/core/model"
	"github.com/ingyamilmolinar/tunkul/internal/audio"
	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
)

const (
	asciiPrintableMin = 32
	asciiPrintableMax = 126
	// timelineHeight reserves vertical space for the transport controls and
	// the thin timeline bar above the instrument rows. Keep this as small as
	// possible so the bottom panel wastes no vertical space. Two rows of
	// controls (2×rowHeight) + a 10px bar + a small margin is sufficient.
	timelineHeight              = 64
	timelineBarHeight           = 10
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

// instMenuMode is an enum describing the instrument dropdown view.
type instMenuMode string

const (
	instMenuModeUnset       instMenuMode = ""
	instMenuModeCategories  instMenuMode = "categories"
	instMenuModeInstruments instMenuMode = "instruments"
)

var eqPanelHeight = 180

type eqBandDef struct {
	loHz float64
	hiHz float64
}

// 10-band layout spanning 20 Hz–20 kHz (log-ish steps).
var eqBandDefs = []eqBandDef{
	{loHz: 20, hiHz: 40},
	{loHz: 40, hiHz: 80},
	{loHz: 80, hiHz: 160},
	{loHz: 160, hiHz: 315},
	{loHz: 315, hiHz: 630},
	{loHz: 630, hiHz: 1250},
	{loHz: 1250, hiHz: 2500},
	{loHz: 2500, hiHz: 5000},
	{loHz: 5000, hiHz: 10000},
	{loHz: 10000, hiHz: 20000},
}

// Number of auto-generated color swatches offered in the color menu.
// Reduced to keep selection concise.
const generatedColorCount = 6

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
	EQGainsDB   []float64 // Per-instrument EQ gains (10 bands, range ±24dB)
	EQBandMuted []bool    // Per-band mute state (10 bands)
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
	Rows       []*DrumRow
	Bounds     image.Rectangle
	Graph      *model.Graph
	logger     *game_log.Logger
	components *ComponentRegistry
	overlays   *OverlayStack

	// Overlay components (Phase 5 integration)
	subdivMenuComp  *SubdivMenuComponent
	renameComp      *RenameComponent
	colorWheelComp  *ColorWheelComponent
	instMenuComp    *InstrumentMenuComponent

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

	// control-panel components
	playBtn       *Button
	stopBtn       *Button
	bpmDecBtn     *Button // decrease BPM
	bpmBox        *TextInput
	bpmIncBtn     *Button // increase BPM
	subdivBtn     *Button // subdivisions-per-beat dropdown
	lenDecBtn     *Button // decrease length
	lenIncBtn     *Button // increase length
	trackBtn      *Button // toggle follow playback
	uploadBtn     *Button
	importBtn     *Button
	exportBtn     *Button
	saveBtn       *Button
	mainVolSlider *Slider
	mainVolRect   image.Rectangle

	// per-row components
	addRowBtn     *Button
	rowLabels     []*Button
	rowEditBtns   []*Button
	rowSaveBtns   []*Button
	rowColorBtns  []*Button
	rowDeleteBtns []*Button
	rowVolSliders []*Slider
	rowOriginBtns []*Button
	rowMuteBtns   []*Button
	rowSoloBtns   []*Button
	selRow        int
	activeSlider  int // index of slider capturing mouse events, -1 if none

	// instrument selection dropdown
	instMenuOpen               bool
	instMenuRow                int
	instMenuBtns               []*Button
	instHold                   bool
	instMenuScroll             VerticalScroller
	instMenuLastAdded          string
	instMenuUserScrolled       bool
	instCategories             []string
	instFilter                 string
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
	colorMenuOpen  bool
	colorMenuRow   int
	colorMenuBtns  []*Button  // deprecated (kept for compatibility; unused)
	colorHexBox    *TextInput // deprecated; no longer used
	colorWheelRect image.Rectangle
	colorWheelImg  *ebiten.Image
	wheelCacheW    int
	wheelCacheH    int
	colorHold      bool

	// subdiv dropdown
	subdivMenuOpen bool
	subdivMenuBtns []*Button

	deleted    []deletedRow
	added      []int
	originReq  []int
	renameRow  int
	renameBox  *TextInput
	renameHold bool

	// import handler
	onImport            func([]byte) error
	onImportDialogStart func()
	onImportDialogEnd   func()

	bgDirty bool
	bgCache []*ebiten.Image

	instOptions []string
	instAvail   map[string]bool
	missingInst map[string]bool
	instMu      sync.RWMutex

	uploading  bool
	uploadCh   chan uploadResult
	pendingWAV string
	naming     bool
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

	// timeline base cache (background + beat markers)
	tlCache      *ebiten.Image
	tlCacheW     int
	tlCacheH     int
	tlCacheBeats int
	tlCacheStep  int
	// cached timeline info label to reduce fmt/string allocs
	lastInfoCurMS int
	lastInfoTotMS int
	lastInfoText  string

	// per-row cached sprites for the steps area (no highlights). Each sprite
	// covers the full timeline width and one row height. Rebuilt when length,
	// row color, or timeline width/height change.
	rowCache      []*ebiten.Image
	rowDirty      []bool
	rowFullDirty  []bool
	rowCacheW     int
	rowCacheH     int
	rowCacheLen   int
	rowCacheOff   []int
	rowCacheGen   []int
	rowCacheSig   []uint64
	rowCacheSteps [][]bool
	rowCacheTypes [][]model.NodeType
	rowCachePadPx int
	rowCacheShift int
	rowCachePatch int
	rowCacheFull  int

	// Segmented timeline slices populated each refresh; mirrors TimelineSegments.
	timelineOffset    []int
	timelinePast      [][]bool
	timelinePastTypes [][]model.NodeType
	timelinePresent   [][]bool
	timelineFuture    [][]bool

	// rowsLayer caches the composition of all visible row sprites (rowCache)
	// for the current offset and scroll. Highlights are drawn on top separately.
	rowsLayer       *ebiten.Image
	rowsLayerW      int
	rowsLayerH      int
	rowsLayerOffset int
	rowsLayerRowOff int
	rowsLayerBaseX  int
	rowsLayerGen    int
	rowsLayerDirty  bool
	rowsLayerPadPx  int
	rowsLayerBytes  int64
	rowsLayerFrame  int64
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
	rowsDrawnMask []bool
	rowsRepaints  int
	rowFrame      []int64
	rowRepaint    []int
	panelMaskRect image.Rectangle

	// EQ visualization (supports master and per-instrument channels)
	eqRect          image.Rectangle
	eqBars          []float64
	eqBinsCount     int
	eqWaveformMode  bool
	eqToggleBtn     *Button
	eqBandVals      []float64
	eqLastBands     []float64
	eqSliders       []*Slider
	eqMuteBtns      []*Button   // Per-band mute buttons
	eqBandGainsDB   []float64
	eqBandMuted     []bool      // Per-band mute state for master channel
	eqApplied       []audio.EQBand
	eqActiveChannel string           // "main" or instrument ID; empty defaults to "main"
	eqChannelBtn    *Button          // Button showing current channel selection
	eqChannelOpen   bool             // Dropdown open state
	eqChannelBtns   []*Button        // Dropdown menu buttons
	eqChannelScroll VerticalScroller // Scroll state for EQ channel dropdown
	// eqTestSnapshot lets tests inject a deterministic analyzer reading.
	eqTestSnapshot *audio.AnalyzerSnapshot

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
	// smooth wheel zoom accumulator (in beats). Each notch contributes a
	// fraction; when |accum| >= 1, we apply whole-beat length changes.
	zoomAccum float64

	bpmPrev  int // previous BPM before editing
	bpmDelta int // accumulated BPM adjustments from +/- buttons

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
	dragStartX    int
	startOffset   int
	offsetChanged bool

	rowOffset      int
	scrollDrag     bool
	scrollStartY   int
	scrollStartOff int

	scrubbing bool
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

	// cached sprites for simple highlight rendering
	hlSpriteReg  *ebiten.Image
	hlSpriteMute *ebiten.Image
	hlSpriteH    int

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

	// Row controls caching: renders per-row buttons/sliders to a cached image
	// to avoid excessive DrawImage calls every frame.
	rowControlsCache      *ebiten.Image
	rowControlsCacheDirty bool
	rowControlsCacheRowOff int   // cached rowOffset for invalidation
	rowControlsCacheVis   int    // cached visible rows count
	rowControlsCacheRect  image.Rectangle // cached bounds
	// Per-row hover tracking to invalidate cache on hover changes
	rowControlsHoverRow   int
	rowControlsHoverBtn   int // index of hovered button within row (or -1)
}
