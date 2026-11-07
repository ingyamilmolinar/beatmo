package ui

import (
	"image"
	"image/color"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

const (
	// As small as fits two control rows and the bar: every extra pixel comes out of the bottom panel.
	timelineHeight = 36
	// desktopHeaderH is the target header height for the two-row desktop transport bar.
	desktopHeaderH = 72
	// mobileHeaderH is the target header height for the two-row mobile transport bar.
	mobileHeaderH               = 72
	defaultRowCachePadPx        = 32
	defaultRowsLayerPadPx       = 96
	instMenuMaxVisibleRows      = 8
	instMenuScrollBarWidth      = 10
	eqChannelMenuMaxVisibleRows = 8
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
	viewModeRows     viewMode = iota // drum rows visible
	viewModeEQ                       // mobile audio panel showing EQ tab
	viewModeWave                     // mobile audio panel showing Wave tab
	viewModeSpectrum                 // mobile audio panel showing Spectrum tab
	viewModeMeters                   // mobile audio panel showing Meters tab
	viewModeChain                    // mobile audio panel showing Scope tab
	viewModeSynth
	viewModeSampler // mobile audio panel showing Sampler tab
)

func viewModeSlug(m viewMode) string {
	switch m {
	case viewModeRows:
		return "pads"
	case viewModeEQ:
		return "eq"
	case viewModeWave:
		return "wave"
	case viewModeSpectrum:
		return "spectrum"
	case viewModeMeters:
		return "levels"
	case viewModeChain:
		return "chain"
	case viewModeSynth:
		return "synth"
	case viewModeSampler:
		return "sampler"
	default:
		return ""
	}
}

// The bottom nav's highlight is derived from this mapping, never stored, so every
// programmatic tab switch keeps it in sync.
func viewModeForPanelTab(tab PanelTab) (viewMode, bool) {
	switch tab {
	case TabEQ:
		return viewModeEQ, true
	case TabWave:
		return viewModeWave, true
	case TabSpectrum:
		return viewModeSpectrum, true
	case TabMeters:
		return viewModeMeters, true
	case TabScope:
		return viewModeChain, true
	case TabSynth:
		return viewModeSynth, true
	case TabSampler:
		return viewModeSampler, true
	default:
		return viewModeRows, false
	}
}

// Exactly one slug per view: no aliases, no case folding.
func viewModeFromSlug(slug string) (viewMode, bool) {
	for m := viewModeRows; m <= viewModeSampler; m++ {
		if viewModeSlug(m) == slug {
			return m, true
		}
	}
	return viewModeRows, false
}

var eqPanelHeight = 190

// Test-only: > 0 overrides the test-mode panel collapse so tests can drive the real layout.
var eqPanelHeightForTest int

type eqBandDef struct {
	loHz float64
	hiHz float64
}

var eqBandDefs = func() []eqBandDef {
	out := make([]eqBandDef, len(audio.EQBandEdges))
	for i, e := range audio.EQBandEdges {
		out[i] = eqBandDef{loHz: e[0], hiHz: e[1]}
	}
	return out
}()

// Edited in place; a change reaches the audio only through applyRowEQ.
func rowEQ(r *DrumRow) *audio.EQSettings { return audio.EQSettingsOf(r.Instrument) }

/* ───────────────────────────────────────────────────────────── */

type DrumRow struct {
	Name       string
	Instrument instrument.ID
	Steps      []bool
	CellTypes  []model.NodeType
	Color      color.Color
	Origin     model.NodeID
	Node       *uiNode
	Volume     float64
	Muted      bool
	Solo       bool
	// No EQ/FX/Sampler fields here: they belong to the row's instrument (rowEQ, audio.GetInsertEffects).
	Role instrument.Role // explicit kit role; empty = the instrument's own (audio.RoleForInstrument)
}

func instColor(id instrument.ID) color.Color {
	if c, ok := instColors[id]; ok {
		return c
	}
	if c, ok := customColors[id]; ok {
		return c
	}
	if len(customPalette) == 0 {
		// Graceful fallback when palette is empty (e.g., in tests).
		return genColorRowRackColorFallback
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
	name string // picked filename / path; "" when unknown (basenamed for the notification)
	err  error
}

func (dv *DrumView) ProjectPins() map[instrument.ID]struct{} {
	if dv == nil || len(dv.projectPins) == 0 {
		return nil
	}
	out := make(map[instrument.ID]struct{}, len(dv.projectPins))
	for k := range dv.projectPins {
		out[k] = struct{}{}
	}
	return out
}

func (dv *DrumView) SetProjectPins(ids []instrument.ID) {
	if dv == nil {
		return
	}
	if len(ids) == 0 {
		dv.projectPins = nil
		return
	}
	m := make(map[instrument.ID]struct{}, len(ids))
	for _, id := range ids {
		if id == "" {
			continue
		}
		m[id] = struct{}{}
	}
	if len(m) == 0 {
		dv.projectPins = nil
		return
	}
	dv.projectPins = m
}

// Sorted so the export is deterministic; empty omits the field.
func (dv *DrumView) exportPinnedInstrumentIDs() []instrument.ID {
	if dv == nil || len(dv.projectPins) == 0 {
		return nil
	}
	out := make([]instrument.ID, 0, len(dv.projectPins))
	for k := range dv.projectPins {
		out = append(out, k)
	}
	sortStrings(out)
	return out
}

type DrumView struct {
	Rows   []*DrumRow
	Bounds image.Rectangle
	Graph  *model.Graph
	// Round-trips the project's pinned_instruments for the instrument menu's pin tier; no UI edits it.
	projectPins                   map[instrument.ID]struct{}
	instMenuShowFavoritesCategory bool
	logger                        *game_log.Logger
	tree                          *DrumViewTree
	audioTree                     *DrumViewTree // audio-panel subtree (eq-panel + divider + view-switch)
	overlayTree                   *DrumViewTree // top-of-z overlay subtree: owns the SINGLE global portal
	rootTree                      *RootTree     // composes [tree, audioTree, overlayTree] for isolated input/draw
	// Shared by every drum-area mobile control so one bounced touch cannot double-fire any of them.
	tapGate tapDebounce
	// Last frame's synced rects, so syncNativeGestures arms/clears only what changed.
	lastNativeRects     []NativeRect
	eqPanelZone         *EQPanelZone
	scopeVisible        bool
	transportZone       *TransportZone
	viewSwitchSegmented *SegmentedControl
	rowRackZone         *RowRackZone
	timelineZone        *TimelineZone
	layoutResizeZone    *layoutResizeZone // layout resize zone (column/row divider pills)

	subdivMenuComp *SubdivMenuComponent
	renameComp     *RenameComponent
	colorWheelComp *ColorWheelComponent
	instMenuComp   *InstrumentMenuComponent
	jumpMenuComp   *SubdivMenuComponent
	jumpBeats      int
	// true: jump jumpBeats whole beats (beat-snapped); false: jumpBeats raw steps.
	jumpUnitBeats bool
	navPanel      *NavigationPanel
	// Owns the go-to editor, so it draws and takes input inside the tree, never floating outside it.
	navMenuComp *NavigationMenuComponent

	// widget layout (bottom pane is divided into movable widgets)
	widgets         *WidgetBoard
	widgetRects     map[WidgetKind]image.Rectangle
	headerH         int // dynamic header height (replaces timelineHeight)
	eqH             int // dynamic EQ panel height
	userEqH         int // explicit audio-panel height set by dragging the EQ divider (0 = auto/floored)
	layoutHoverAxis string
	layoutHoverIdx  int
	layoutDragAxis  string
	layoutDragIdx   int
	layoutDragPrev  int
	layoutHandler   *LayoutResizeHandler // widget-span-aware layout resize
	rowEQDivider    *rowEQDividerLayer   // animated EQ-boundary divider (shared SplitterHandle)

	cell      int // px per step
	labelW    int
	controlsW int // width reserved for control buttons

	// control-panel components (non-transport)
	lenDecBtn       *Button // decrease length
	lenIncBtn       *Button // increase length
	saveBtn         *Button
	mainVolRect     image.Rectangle
	mainVolIconRect image.Rectangle
	// Mobile bottom-sheet host; empty on desktop.
	bottomActionBarRect image.Rectangle
	eqPeekRect          image.Rectangle

	// per-row components: owned by rowRackZone, accessed via accessor methods.
	selRow int

	// instrument selection dropdown
	instMenuRow                int
	instMenuScroll             VerticalScroller
	instMenuLastAdded          instrument.ID
	instMenuUserScrolled       bool
	instCategories             []audio.Category
	instCategoryBtns           []*Button
	instMenuFullRect           image.Rectangle
	instMenuMode               instMenuMode
	instMenuActiveCat          audio.Category
	instMenuForceCategories    bool
	instMenuCameFromCategories bool
	instMenuActiveByRow        map[int]audio.Category // remembers last category per row
	instCatByID                map[instrument.ID]audio.Category
	instMeta                   map[instrument.ID]audio.SoundMeta
	instSearch                 string
	instSearchBox              *TextInput
	instSearchRect             image.Rectangle
	instCatalogVersion         uint64
	instRegistryVersion        uint64
	instRefreshDirty           bool

	// color picker: swatch-grid popup
	colorMenuRow int

	// FX panel
	fxPanelRow         int
	fxPanelRect        image.Rectangle
	fxPanelBtns        []*Button         // add/remove/toggle/reorder buttons within the panel
	fxPanelCloseButton *Button           // the close (×) button, tracked by identity (never the slice tail)
	fxPanelSliders     []*Slider         // param sliders within the panel
	fxSliderBindings   []fxSliderBinding // maps slider index to effect param
	fxSliderDragging   bool              // active slider drag in FX panel
	fxSliderDragIdx    int               // index into fxPanelSliders being dragged
	fxAddMenuOpen      bool
	fxListScrollPx     int                // effect-list scroll offset saved when the add picker opens, restored by Back/Esc
	fxAddRowEffect     []audio.EffectType // effect type picker rows, aligned with fxAddRowBtn
	fxAddRowBtn        []*Button          // picker-row buttons, aligned with fxAddRowEffect (drawn via drawMenuRow, not btn.Draw)
	fxAddEffectBtn     *Button            // the list's "+ Add Effect" footer (nil while the picker shows or it is scrolled out)
	fxAddMenuBackBtn   *Button            // the add-menu "Back" row, tracked by identity (drawn via drawMenuRow)
	fxAddHeaderRows    []fxAddHeaderRow   // idle-only category group-header rows interleaved into the picker layout
	fxExpandedSlots    map[int]bool       // which effect slots are expanded (mobile only)
	fxScroll           *MenuScroll        // FX panel content scroll (pixel mode, ItemHeight=1): the shared list-menu engine, so wheel/momentum/thumb match every menu
	fxSliderLeft       int                // computed label area width for FX param rows (desktop)
	fxViewportRect     image.Rectangle    // scrollable content area (between header and footer)

	fxAddSearch           *TextInput
	fxAddSearchRect       image.Rectangle
	fxAddSearchQuery      string
	fxAddSearchHighlights map[audio.EffectType][]int

	// Each ParamDef has a Knob (drawn, interactive) and a Slider (the value the hit-adapter and
	// coalesce path read); drag handlers keep the two in sync.
	instEditorSliders    []*Slider
	instEditorControls   []*Knob
	instEditorStepBadges []*KnobStepBadge   // index-aligned with instEditorControls; nil for discrete params
	instEditorBindings   []instParamBinding // maps widget index → ParamDef
	synthWheelNudge      wheelNudger        // desktop scroll-wheel value nudge -> debounced single-undo commit
	samplerWheelPopup    *MobileWheelPopup  // mobile vertical scroll-wheel for sampler knobs
	samplerWheelNudge    wheelNudger        // desktop scroll-wheel value nudge -> debounced single-undo commit
	eqWheelPopup         *MobileWheelPopup  // desktop+mobile precision wheel for EQ band dB
	valueWheelPopup      *MobileWheelPopup  // shared value wheel over a NumberModel (BPM, node values)
	eqWheelStepBadge     *KnobStepBadge     // shared step-resolution ladder for all EQ dB bands
	eqWheelStepRestored  bool               // true once eqWheelStepBadge's persisted rung has been restored (lazy, on first popup open)
	instEditorBtns       []*Button          // footer buttons: Reset, Save, Save-As (close lives on the sticky bar)
	instEditorSections   []synthSection     // PITCH | ENVELOPE | TONE | DRIVE cards
	// Empty when the panel is too narrow to split (mobile).
	instEditorPreviewRect image.Rectangle
	// Mobile only (no side pane); empty on desktop.
	instEditorMobileFocusRect image.Rectangle
	// Stored only for tests to inspect.
	instEditorSectionsRect image.Rectangle
	instEditorNoSynth      bool
	instEditorBannerRect   image.Rectangle
	instEditorHeader       synthHeaderLayout // header strip rects
	instEditorHeaderBtns   []*Button         // header retrigger + vol buttons (shared *Button chrome)
	// One instance serves the Synth and Sampler tabs; props are swapped on each open.
	actionMenu            *actionMenuComponent
	saveAsDialog          *synthSaveAsDialog // active Save-As name dialog (nil = closed)
	instEditorScrollPx    int
	instEditorScrollMax   int
	instEditorScrollTS    TouchScroller
	instEditorDeferredTap DeferredTap
	// Per instrument: the drilled category or stage; the zero value is the root.
	instEditorMacroOpen  map[instrument.ID]synthDrillTarget
	instEditorMoreOpen   map[string]bool
	instEditorPipePills  []synthPipePill
	instEditorPipeAddBtn *Button
	instEditorPipeGrid   *ControlGrid
	instEditorPipeRect   image.Rectangle
	// The only dial state: a dial's position is a pure function of the instrument document.
	synthDialActive *synthDialGesture

	instEditorMacros []macroGroup // built concept groups for the active instrument
	// synthFocusState is the Synth tab's focused dial (synth_focus.go).
	synthFocusState synthFocus
	graphPipe       *synthGraphPipeline
	dialRelevance   dialRelevanceState     // which dials the DSP vouches for (synth_dial_relevance.go)
	graphDetails    synthGraphDetailsState // the graph readout's Details toggle (synth_graph_details.go)
	// graphBefore is the graph's BEFORE ghost (synth_graph_pane.go).
	graphBefore graphBeforeSnap
	// graphViewOverride is the developer view picker's per-control choice.
	graphViewOverride         synthGraphViewOverride
	instEditorMacrosKey       synthMacroCacheKey // rebuild gate for instEditorMacros
	instEditorMacroCards      []macroCardSlot    // root: laid-out concept card rects
	instEditorMacroBackBtn    *Button            // drilled: the Back button (in instEditorBtns)
	instEditorMoreBtn         *Button            // drilled: the More divider's input button (in instEditorBtns)
	instEditorMacroPillRect   image.Rectangle    // drilled: the stage enable pill rect (empty = no pill)
	instEditorMacroPillSec    int                // drilled: section index the pill toggles (-1 = none)
	instEditorMacroTitle      string             // drilled: title text (category label / stage label)
	instEditorDrilledOrder    []int              // drilled: knob indices in display order (essentials then More)
	glow                      *synthGlow         // the Synth graph's trigger glow (lazy)
	instEditorDrillEss        []int              // scratch for drilledKnobIdxs (reused across frames)
	instEditorDrillAdv        []int              // scratch for drilledKnobIdxs (reused across frames)
	instEditorBindingIdx      map[string]int
	instEditorBindingIdxGen   int
	instEditorBindingsGen     int
	instEditorMacroGloss      string          // drilled: one-liner under the title
	instEditorMacroGrid       *ControlGrid    // grid for root cards / drilled knobs
	instEditorMacroDetailR    image.Rectangle // the macro layer's detail region
	instEditorSelectedControl map[instrument.ID]int
	// At most one armed control per instrument; absent = none.
	instEditorArmedControl map[instrument.ID]int
	// instEditorArmedEditor owns the two barrel wheels for the armed control.
	instEditorArmedEditor *ArmedControlEditor
	// Separate from the synth's maps on purpose: each tab indexes its own widget slice, so the
	// same instrument id names different controls.
	samplerArmedControl map[string]int
	samplerArmedEditor  *ArmedControlEditor
	// armGrowIdx stays set through the shrink so it can finish easing; armGrowScope exists because
	// the Synth and Sampler tabs index their controls independently.
	armGrowIdx           int
	armGrowInst          instrument.ID
	armGrowScope         armedScope
	armGrowT             float64
	armGrowTarget        float64
	synthAdvDividerRect  image.Rectangle
	synthAdvDividerCount int

	samplerPanel   *samplerPanel
	synthPanel     *synthPanel
	sampler        samplerState
	samplerButtons []samplerButton // header + control-row buttons, rebuilt each Layout

	// subdiv dropdown
	subdivMenuBtns []*Button

	deleted            []deletedRow
	added              []int
	originReq          []int
	deleteConfirmRow   int   // row pending delete confirmation, -1 = none
	deleteConfirmFrame int64 // frame when first click happened
	renameRow          int
	renameBox          *TextInput
	textFields         *textFieldRegistry

	// source names the success toast (file or template); "" shows the generic one.
	onImport            func(data []byte, source string) error
	onImportDialogStart func()
	onImportDialogEnd   func()

	// Called for every parity-visible mutation. Must not re-acquire seqMu: DrumView runs under it
	// from Game.Update.
	onStructuralMutation func(reason string)
	// Synchronous, so every index-keyed per-row store shifts before anything reads a row by its new index.
	onRowRemoved func(i int, origin model.NodeID)

	// Nil until Game wires it, so callback closures must guard.
	game *Game

	bgDirty          bool
	layoutSuppressed bool
	bgCache          []*ebiten.Image
	// bgCacheThemeGen is the ThemeGeneration bgCache was filled under.
	bgCacheThemeGen int

	instOptions []instrument.ID
	instAvail   map[instrument.ID]bool
	missingInst map[instrument.ID]bool
	instMu      sync.RWMutex

	uploading  bool
	uploadCh   chan uploadResult
	pendingWAV string
	nameInput  string     // legacy helper, not used for typing anymore
	nameBox    *TextInput // active while naming

	// JSON import
	importing      bool
	importCh       chan importResult
	instImporting  bool
	instImportCh   chan importResult
	lengthChanging bool // Set true during length change to suppress parity

	// callback to change subdivisions per beat
	onChangeSubdiv func(int) error

	// Embedded so its fields resolve on dv by promotion.
	rowSpriteCache

	// Segmented timeline slices populated each refresh; mirrors TimelineSegments.
	timelineOffset    []int
	timelinePast      [][]bool
	timelinePastTypes [][]model.NodeType
	timelinePresent   [][]bool
	timelineFuture    [][]bool

	// Embedded so its fields resolve on dv by promotion.
	rowsLayerCache

	// Embedded so its fields resolve on dv by promotion.
	rowsWinCache

	// Mirrors TransportZone.followPlayback for DrumViews built without one (tests).
	follow              bool
	rowsPadFullRebuilds int
	rowsPadLastDecay    int64

	// Last-frame row draw mask for visibility assertions (set during Draw)
	rowsDrawnMask   []bool
	rowsRepaints    int
	directDrawCount int64 // frames where drawRowsDirect was used (mobile fallback)
	directDrawCells int   // cells drawn in last directDraw call
	rowFrame        []int64
	rowRepaint      []int
	panelMaskRect   image.Rectangle

	rowFireDecay []float64

	// Mobile EQ collapse: hides the Wave/EQ panel by default on mobile.
	mobileEQCollapsed bool
	mobileEQInited    bool // true once the mobile-default has been applied

	// While false, the mobile default length cap still applies.
	userAdjustedLength bool

	// Mobile view switching (Rows → EQ → Wave cycle)
	currentViewMode viewMode

	// The chips resize the whole drum-view pane; row heights are fixed.
	rowZoomChipRect image.Rectangle
	rowZoomIncBtn   *Button
	rowZoomDecBtn   *Button

	// Mobile overflow menu for Upload/Import/Export
	overflowMenuScroll *MenuScroll // shared scroll component for the overflow menu
	overflowBtns       []*Button   // persisted overflow/template rows (rebuilt on open/page switch); rects updated per frame for scroll
	overflowCard       *MenuCard   // shared frame for the File/template menu (title band from the pinned header item, IconFolder, close ×)
	overflowBtnsPage   int         // page the persisted overflowBtns were built for; rebuild on page change

	// 0 = File actions, 1 = templates; reset to 0 on close so a reopen starts at File.
	overflowPage int

	templateSearchBox        *TextInput
	templateSearchRect       image.Rectangle
	templateSearchQuery      string
	templateSearchCache      []MenuSearchResult
	templateSearchCacheQuery string
	templateSearchCacheValid bool
	templateLoadedStem       string

	// overflowItems runs several times a frame, so its model is cached on overflowItemsKey and
	// callers must treat the slice as read-only; overflowNavSig resets the cursors when the list shape changes.
	overflowNav    *MenuKeyNav
	overflowNavSig string
	contextNav     *MenuKeyNav

	overflowItemsCache      []overflowItem
	overflowItemsCacheKey   overflowItemsKey
	overflowItemsCacheValid bool
	// Also keyed on the font generation: a locale or font switch changes every width.
	overflowLabelWidth      int
	overflowLabelWidthKey   overflowItemsKey
	overflowLabelWidthFont  int64
	overflowLabelWidthValid bool

	// Mobile beat counter rect (inside transport row, 7th column on mobile)
	beatCounterRect image.Rectangle

	// Mobile per-row volume popup
	volPopup    *SliderPopup
	volPopupRow int // which row's volume is being edited

	// Master volume popup (desktop icon-click opens vertical slider)
	masterVolPopup *SliderPopup

	// Mobile context menu for row controls (long-press on row label)
	contextMenuRow         int
	contextMenuRect        image.Rectangle
	contextMenuBtns        []*Button
	contextMenuCloseButton *Button            // the close (×) button, tracked by identity so retrieval never relies on append position
	contextMenuCard        *MenuCard          // shared frame (panel / sheet, title band, close ×)
	fxPanelCard            *MenuCard          // shared title band for the docked FX panel
	contextMenuItemIcons   map[*Button]string // per-button leading icon, keyed by button identity (not a parallel slice — can't drift out of lockstep)
	contextMenuHeaderRect  image.Rectangle    // mobile bottom sheet header area
	contextMenuScroll      *ScrollBehavior    // scroll when items overflow

	// EQ visualization (supports master and per-instrument channels)
	eqRect                 image.Rectangle
	eqWaveformMode         bool
	eqBandVals             []float64
	eqLastBands            []float64
	eqApplied              []audio.EQBand
	masterGainsDB          []float64       // master channel EQ gains (separate from zone working copy)
	masterMuted            []bool          // master channel EQ mute state
	eqActiveChannel        channel.ID      // the master bus or an instrument's channel; empty = master
	eqChannelScroll        *ScrollBehavior // Scroll state for EQ channel dropdown
	eqChDeferredTap        DeferredTap
	instMenuDeferredTap    DeferredTap
	contextMenuDeferredTap DeferredTap
	fxPanelDeferredTap     DeferredTap

	// Audio is written live per change; these hold what the release commit (one emit + undo step) needs.
	eqPendingChannel    channel.ID
	eqPendingBand       int
	eqPendingGainDB     float64
	eqPendingDirty      bool
	mainVolPending      float64
	mainVolPendingDirty bool

	// EQ frequency response curve
	eqCurveDragBand   int                       // -1 when not dragging, else band index
	eqCurveDragFilter hooks.EQFilter            // FilterNone when not dragging a filter handle
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

	// At most one frame stale: recalcButtons runs before Draw refreshes it.
	lastElapsedBeats float64
	// Memoized: recalcButtons reads it every frame, but the text changes once a second.
	readout readoutMemo
	// Gates for recalcButtons' forced zone layouts (drumview_layout_gate.go).
	timelineLayoutKey  timelineLayoutKey
	timelineLayoutHave bool
	eqLayoutGate       eqLayoutGate

	timelineRect image.Rectangle // progress bar for fast seek
	// Deliberately not timelineRect: the ribbon reserves room for header chrome the cells must not inherit.
	cellGridRect         image.Rectangle
	timelineBeats        int // total beats represented by timeline
	timelineUnitsPerBeat int

	// internal ui state
	bpm         int
	secPerBeat  float64
	playPressed bool
	stopPressed bool
	// Drained once per frame by drainSeekRequest, under seqMu.
	pendingSeek   *seekRequest
	Length        int  // Length of the drum view, independent of graph
	lenIncPressed bool // State for length increase button
	lenDecPressed bool // State for length decrease button
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

	twoFingerPan twoFingerPanState

	scrubbing bool

	// Held while a press that began in the drum view is down, so the splitter cannot steal the drag.
	mouseDownInBounds       bool
	inputCapturedExternally bool

	// Kept for export, so an imported project can re-register its custom samples.
	samplePath map[instrument.ID]string

	wavReg *audio.WavRegistry

	// Copied from Game to freeze the header scale during playback.
	isPlaying bool

	// simple draw hint propagated from Game; enables low-overhead highlight path
	simpleDraw bool
	// perfDrawLite skips expensive, non-critical chrome when perf fast path is on.
	perfDrawLite bool

	notifStore *notificationStore
	logsComp   *LogsComponent
	logsToast  logsToast
	// import
	importAttemptFrame  int
	importAttemptUpdate int

	// update sequence (increments each Update call)
	updateSeq int

	instLabelCache map[instrument.ID]string
	// cachedLabelW stores the computed labelW; labelWidthDirty triggers recompute.
	cachedLabelW    int
	labelWidthDirty bool
	// Track data signature to detect stale cache even when labelWidthDirty is false.
	labelCacheRowCount int
	labelCacheInstLen  int
	labelCacheRowNames []string // cached row names to detect direct modifications

	toolbarCache     *ebiten.Image
	toolbarCacheHash uint64

	rowControlsCacheDirty bool
	rowControlsCacheRect  image.Rectangle // cached bounds
}

// --- EQ zone accessor methods (delegate to eqPanelZone) ---

func (dv *DrumView) eqMuteBtns() []*Button    { return dv.eqPanelZone.eqMuteBtns }
func (dv *DrumView) eqBandGainsDB() []float64 { return dv.eqPanelZone.bandGainsDB }
func (dv *DrumView) eqBandMuted() []bool      { return dv.eqPanelZone.bandMuted }
func (dv *DrumView) eqToggleBtn() *Button     { return dv.eqPanelZone.stickyBar.TabBtn(0) }
func (dv *DrumView) eqChannelBtn() *Button    { return dv.eqPanelZone.stickyBar.ChannelBtn() }
func (dv *DrumView) hpfBtn() *Button          { return dv.eqPanelZone.hpfBtn }
func (dv *DrumView) lpfBtn() *Button          { return dv.eqPanelZone.lpfBtn }

// --- Scope panel accessors ---

func (dv *DrumView) ScopeVisible() bool {
	if dv.eqPanelZone != nil {
		return dv.eqPanelZone.tabState.ActiveTab() == TabScope
	}
	return false
}
func (dv *DrumView) SetChainVisible(v bool) {
	if v && !panelTabAvailable(TabScope) {
		return
	}
	dv.scopeVisible = v
	if dv.eqPanelZone != nil && v {
		dv.eqPanelZone.tabState.SetActiveTab(TabScope)
	}
}

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
func (dv *DrumView) recordBtn() *Button           { return dv.transportZone.recordBtn }
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
func (dv *DrumView) mainVolSlider() *Slider       { return dv.transportZone.mainVolSlider }
func (dv *DrumView) mainVolGroup() *SliderGroup   { return dv.transportZone.mainVolGroup }
func (dv *DrumView) transportGroup() *LayoutGroup { return dv.transportZone.transportGroup }

func (dv *DrumView) RowOffsetForTest() int { return dv.rowOffset }

func (dv *DrumView) VisibleRowsForTest() int { return dv.visibleRows() }
