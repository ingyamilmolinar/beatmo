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
	viewModeSynth                    // mobile audio panel showing Synth tab (Phase 4)
	viewModeSampler                  // mobile audio panel showing Sampler tab
)

// viewModeSlug returns the canonical lowercase slug for a view mode, matching
// the bottomNav segment slugs exposed in fullLayoutSnapshot. "pads" is the
// Rows view (desktop is always "pads"); the rest mirror the audio-panel tabs.
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

// viewModeFromSlug is the inverse of viewModeSlug. Returns (mode, true) for a
// recognized slug, or (viewModeRows, false) otherwise.
func viewModeFromSlug(slug string) (viewMode, bool) {
	switch slug {
	case "pads", "rows":
		return viewModeRows, true
	case "eq":
		return viewModeEQ, true
	case "wave":
		return viewModeWave, true
	case "spectrum":
		return viewModeSpectrum, true
	case "levels", "meters":
		return viewModeMeters, true
	case "chain", "scope":
		return viewModeChain, true
	case "synth":
		return viewModeSynth, true
	case "sampler":
		return viewModeSampler, true
	default:
		return viewModeRows, false
	}
}

var eqPanelHeight = 190

// eqPanelHeightForTest, when > 0, overrides the test-mode collapse of the audio
// panel so functional tests can drive the REAL production-floored layout. See
// the runningUnderGoTest gate in (*DrumView) layout. Always 0 outside tests.
var eqPanelHeightForTest int

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
	Role        string             // Phase 6: explicit kit role tag (kick/snare/hat/…); empty = audio.RoleForInstrument heuristic
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

// ProjectPins returns a snapshot copy of the currently loaded project's pin
// set. Callers (the instrument menu's PinSource builder) treat the result as
// read-only — mutating the returned map does not affect DrumView state. Nil
// is fine; callers handle empty maps the same as a nil map.
func (dv *DrumView) ProjectPins() map[string]struct{} {
	if dv == nil || len(dv.projectPins) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(dv.projectPins))
	for k := range dv.projectPins {
		out[k] = struct{}{}
	}
	return out
}

// SetProjectPins replaces the per-project pin set. Called from Game.Import
// after parsing the project JSON's pinned_instruments field; tests may also
// drive it directly. Empty / nil clears the set.
func (dv *DrumView) SetProjectPins(ids []string) {
	if dv == nil {
		return
	}
	if len(ids) == 0 {
		dv.projectPins = nil
		return
	}
	m := make(map[string]struct{}, len(ids))
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

// exportPinnedInstrumentIDs returns the pin set as a deterministic
// (alphabetical) slice for JSON serialisation. Empty result → caller emits
// an absent field (omitempty keeps v1 projects byte-identical when no pins
// are set).
func (dv *DrumView) exportPinnedInstrumentIDs() []string {
	if dv == nil || len(dv.projectPins) == 0 {
		return nil
	}
	out := make([]string, 0, len(dv.projectPins))
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
	// projectPins is the per-project instrument pin set, populated from the
	// project JSON's pinned_instruments field on import and serialised back on
	// export. It feeds the instrument menu's tier-0 pin source (see
	// PinSource.ProjectPins). No UI to mutate the set ships in this PR — that
	// affordance lands in a follow-up; the storage and read path are wired
	// now so the JSON schema doesn't need to bump later.
	projectPins map[string]struct{}
	// instMenuShowFavoritesCategory wires the production builder's
	// ShowFavoritesCategory prop. See ctor comment.
	instMenuShowFavoritesCategory bool
	logger                        *game_log.Logger
	tree                          *DrumViewTree // zone-based component tree (Phase 1+)
	audioTree                     *DrumViewTree // audio-panel subtree (eq-panel + divider + view-switch)
	rootTree                      *RootTree     // composes [tree, audioTree] for isolated input/draw
	eqPanelZone                   *EQPanelZone  // Phase 2: EQ panel zone (owns EQ buttons/sliders/state)
	scopeVisible                  bool
	transportZone                 *TransportZone // Phase 3: transport zone (owns transport buttons/state)
	// viewSwitchSegmented is the mobile 3-segment view selector
	// (Pads/EQ/Wave). Replaces the binary viewSwitchBtn on mobile; nil on
	// desktop. See B4 in the mobile UX overhaul plan.
	viewSwitchSegmented *SegmentedControl
	rowRackZone         *RowRackZone      // Phase 4: row rack zone (owns per-row buttons/sliders/scroll)
	timelineZone        *TimelineZone     // Phase 5: timeline zone (owns drag/scrub)
	layoutResizeZone    *layoutResizeZone // layout resize zone (column/row divider pills)

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
	// bottomActionBarRect is the mobile-only bottom sheet host for the
	// volume icon, view-switch, and overflow buttons. Empty on desktop.
	// Set by recalcButtons when LayoutProfile.UseBottomSheet is true.
	bottomActionBarRect image.Rectangle
	// eqPeekRect is the 24-px sparkline strip directly above
	// bottomActionBarRect, visible only when mobileEQCollapsed is true.
	// Tap on this rect expands the EQ panel.
	eqPeekRect image.Rectangle

	// per-row components: owned by rowRackZone, accessed via accessor methods.
	selRow int

	// instrument selection dropdown
	instMenuRow                int
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
	fxExpandedSlots    map[int]bool    // which effect slots are expanded (mobile only)
	fxScrollOffsetPx   int             // pixel scroll offset for FX panel content area
	fxScrollTS         TouchScroller   // touch scroll tracking for FX panel
	fxScrollMaxPx      int             // max scroll offset (contentH - viewportH), 0 = no scroll
	fxSliderLeft       int             // computed label area width for FX param rows (desktop)
	fxViewportRect     image.Rectangle // scrollable content area (between header and footer)

	// Instrument-editor (Phase 4 → redesigned per
	// hey-please-review-the-vectorized-rocket.md) state. The editor lives
	// as a Synth tab alongside EQ/Wave/Spectrum/Levels/Chain (see
	// synth_panel_zone.go). State is carried on DrumView so per-row scroll
	// position survives tab switches and the EQ-panel routing can reach the
	// widget lists.
	//
	// Slider/Knob duality: each wired ParamDef gets a (Slider, Knob) pair.
	// Knob is the visual + interaction widget rendered inside the section
	// card; Slider is a value-store that the legacy hit-adapter / coalesce
	// path continues to read. Drag handlers
	// keep the two values in sync so either widget can drive playback. The
	// Slider's rect is set equal to the Knob's bounds so hit-test paths
	// that still consult slider rects land on the knob area.
	instEditorSliders    []*Slider
	instEditorKnobs      []*Knob
	instEditorStepBadges []*KnobStepBadge   // index-aligned with instEditorKnobs; nil for discrete params
	instEditorBindings   []instParamBinding // maps widget index → ParamDef
	synthWheelPopup      *MobileWheelPopup  // mobile vertical scroll-wheel for synth knobs
	samplerWheelPopup    *MobileWheelPopup  // mobile vertical scroll-wheel for sampler knobs
	instEditorBtns       []*Button          // footer buttons: Reset, Save, Save-As (close lives on the sticky bar)
	instEditorSections   []synthSection     // PITCH | ENVELOPE | TONE | DRIVE cards
	// instEditorPreviewRect holds the geometry of the right-half
	// preview pane (osc + ADSR + filter plots). Empty when the panel
	// is too narrow for a horizontal split (mobile) or when the
	// content area is below the minimum width for the pane to fit.
	// Phase 4 of the audio-panel redesign.
	instEditorPreviewRect image.Rectangle
	// instEditorMobileFocusRect is the stacked band reserved at the TOP of
	// the sections area on mobile (where there is no side preview pane) for
	// the "Your sound" mirror + the focus graph. Empty on desktop (the side
	// pane carries those instead).
	instEditorMobileFocusRect image.Rectangle
	// instEditorSectionsRect is the post-shrink knob-grid rect that
	// layoutSynthSections consumes (after reserving the desktop preview pane
	// and/or the mobile focus band). Stored for test introspection.
	instEditorSectionsRect image.Rectangle
	// instEditorNoSynth is set when the active row's instrument has no
	// synth recipe (it plays a loaded WAV sample). In that state the synth
	// tab replaces its grid of section cards with a single explanatory
	// banner (instEditorBannerRect) instead of a grid of empty section
	// cards plus their distracting trigger-pulse borders.
	instEditorNoSynth     bool
	instEditorBannerRect  image.Rectangle
	instEditorHeader      synthHeaderLayout  // header strip rects
	instEditorHeaderBtns  []*Button          // header retrigger + vol buttons (shared *Button chrome)
	saveAsDialog          *synthSaveAsDialog // active Save-As name dialog (nil = closed)
	instEditorDragging    bool
	instEditorDragIdx     int
	instEditorScrollPx    int
	instEditorScrollMax   int
	instEditorScrollTS    TouchScroller
	instEditorDeferredTap DeferredTap
	// instEditorSectionGrids holds the per-section adaptive control grid +
	// scroll state, keyed by section id so the scroll position survives both
	// the per-frame re-layout and a recipe switch. Lazily created by
	// sectionGrid. See control_grid.go.
	instEditorSectionGrids map[synthSectionID]*ControlGrid
	// Pipeline chip strip + expand-one detail pane (synth-tab redesign).
	// Exactly one stage is expanded at a time; the rest collapse to chips in
	// audio-pipeline order. Selection is per-session, per-instrument (no
	// userprefs) and survives re-Layout per
	// [[feedback_runtime_profile_derivation]] — geometry is re-derived every
	// Layout from instEditorSelectedSection.
	instEditorSelectedSection map[string]synthSectionID // resolved instID → open stage
	// instEditorSelectedKnob is the per-instrument index (into instEditorKnobs /
	// instEditorBindings) of the knob explained by the focus graph. Set on knob
	// press (click-to-select); defaults to the open stage's main knob.
	instEditorSelectedKnob  map[string]int
	instEditorChips         []synthChip       // rebuilt (reused [:0]) every Layout
	instEditorDetailR       image.Rectangle   // detail pane rect (selected stage's knobs)
	instEditorDetailHeaderH int               // effective detail header height (token, shrunk on short panels)
	instEditorReadoutRects  []image.Rectangle // index-aligned with instEditorKnobs; readout tap hit rects
	paramEditor             *ParamValueEditor // shared numeric entry box for synth knob readout taps

	// synthMirror caches the live final-output re-render for the Synth tab's
	// right-pane mirror (Stage 4). Lazily created on first requestSynthMirror.
	synthMirror *synthMirror

	// synthGhost holds the per-knob pre-drag param snapshots + fade state for
	// the Synth tab concept overlays (Stage 5). Lazily created on first capture.
	synthGhost *synthGhostState

	// sampler holds the Sampler tab's working buffer + edit params + widgets.
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

	// import handler. source is a short label (filename / template name) the
	// success notification names; "" falls back to the generic "Imported" toast.
	onImport            func(data []byte, source string) error
	onImportDialogStart func()
	onImportDialogEnd   func()

	// onStructuralMutation is invoked for every runtime mutation that changes
	// state observable by parity (instrument id, BPM, length, row count,
	// effect chain, EQ, mute/solo, send levels, etc.). Game injects this in
	// New() to bump the parity generation, grant a grace window, and clear
	// parity buffers. Must NOT call back into Game paths that re-acquire
	// seqMu — DrumView methods are reachable from Game.Update under that
	// lock and a re-acquire would deadlock.
	onStructuralMutation func(reason string)

	// game is a weak back-reference to the owning Game, set by game_new.go
	// after NewDrumView() returns. Used exclusively by callback closures
	// (e.g. EQCallbacks.BeatGridFrac) that must read transient Game state
	// like beatInfosByRow / playheadFloor at render time. May be nil during
	// the brief window before Game wires it; closures must guard.
	game *Game

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
	rowsLayer       *ebiten.Image
	rowsLayerW      int
	rowsLayerH      int
	rowsLayerOffset int
	rowsLayerRowOff int
	rowsLayerBaseX  int
	// rowsLayerRowWidth and rowsLayerLength snapshot the timeline-rect width
	// and step count the composite was built with. The shift-and-fill reuse
	// path copies stale pixels left and only refills a small right strip, so
	// if either dimension changes without the caller setting rowsLayerDirty,
	// the leftmost pixels keep an old cell pitch while the right strip is
	// painted at the new pitch — producing the mixed-pitch artifact users
	// see after long sessions.
	rowsLayerRowWidth int
	rowsLayerLength   int
	rowsLayerGen      int
	// lastRowsRenderPath / lastLegacyRebuildKind record which compositing path
	// produced the most recent drum-row frame, for the slim-bar diagnostic
	// (drum_render_diag.go). lastRowsRenderPath is "windowed" | "legacy" |
	// "direct" | ""; lastLegacyRebuildKind details the legacy branch taken
	// ("full" | "shift" | "overdraw" | "patch-skip" | "stale-accept" | "skip").
	lastRowsRenderPath    string
	lastLegacyRebuildKind string
	rowsLayerDirty        bool
	rowsLayerPadPx        int
	rowsLayerScratch      *ebiten.Image // double-buffer scratch for layer shifts
	rowsLayerBytes        int64
	rowsLayerFrame        int64

	// ── Windowed scroll cache (perf: avoid per-scroll full-layer recompose) ──
	// During steady follow-scroll playback the row CONTENT does not change, the
	// window merely translates. The legacy path recomposited (a full-layer
	// shift-copy + double-buffer swap → Ebiten dependency-graph churn) on every
	// recenter. The windowed path renders cells into a buffer WIDER than the
	// visible window, fills the leading edge incrementally as the playhead
	// scrolls, and blits a moving sub-rectangle each frame — recompositing only
	// when the playhead scrolls past the pad (every rowsWinPadCells cells) or
	// when content/size changes. See drumview_cache_rows_window.go and
	// rows_layer_scroll_recompose_test.go.
	rowsWinBuf          *ebiten.Image // wide cache: width = rowWidth + padPx
	rowsWinBufW         int
	rowsWinBufH         int
	rowsWinBakeOffset   int  // dv.Offset the buffer's cell 0 corresponds to
	rowsWinRenderedTo   int  // exclusive buffer-cell index rendered so far
	rowsWinRowWidth     int  // visible window width (px) the buffer pitch uses
	rowsWinLength       int  // dv.Length the buffer was baked with
	rowsWinBaseX        int  // baseX the buffer was baked with
	rowsWinRowOff       int  // dv.rowOffset (vertical scroll) the buffer was baked with
	rowsWinContentDirty bool // a real content/edit change → force re-bake
	rowsWinValid        bool

	// Row-stripes path: a horizontal split of the rows layer into multiple
	// sprites so a wide layer can stream as separate textures. These
	// fields are kept as no-op zero-value stubs while the stripes path is
	// being refactored — production callers (js_exports_*.go,
	// image_metrics.go, drumview_cache_row_sprite.go) read but never
	// initialise them, so the layer-based path runs unchanged.
	rowsStripingEnabled bool
	rowsStripeCount     int
	rowsStripes         []*ebiten.Image
	rowsStripeStarts    []int
	rowsStripeWidths    []int
	rowsStripeLastFrame int64
	rowsStripeOffset    int
	rowsStripeRowOff    int
	rowsStripeCachedW   []int
	rowsStripeCachedH   []int
	rowsStripeAuto      bool
	rowsStripeScratch   []*ebiten.Image

	// follow mirrors TransportZone.followPlayback for backward-compatible
	// access on test paths that build a DrumView without a TransportZone.
	follow bool
	// WASM-only adaptive pad helpers: expand pad when frequent full rebuilds
	// happen due to horizontal pans; decay toward default when stationary.
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

	// rowFireDecay holds a per-row "now-playing" tint intensity in [0,1].
	// MarkRowFired snaps the entry to 1.0 when the row's audible step fires;
	// decayAnims attenuates it each frame so the tint fades out smoothly.
	// Read at draw time on mobile (drawRowsDirect overlay) to wash the row
	// strip in WithAlpha(genColorPrimary, AlphaFaint*intensity).
	rowFireDecay []float64

	// Mobile EQ collapse: hides the Wave/EQ panel by default on mobile.
	mobileEQCollapsed bool
	mobileEQInited    bool // true once the mobile-default has been applied
	// NOTE: previously held a parallel `mobileEQMode bool` — now derived
	// from currentViewMode via dv.MobileEQMode() to keep the mobile tab
	// swap on a single source of truth (see drumview_context_menu.go).

	// userAdjustedLength is set when the user manually changes the timeline
	// length via +/- buttons. When false, the mobile default cap is applied
	// in SetBounds and updateBeatInfos to keep the initial view at 8 beats.
	userAdjustedLength bool

	// Mobile view switching (Rows → EQ → Wave cycle)
	currentViewMode viewMode

	// rowZoom* host the +/− chip pair inline with the addRow button on
	// mobile. The chips resize the *entire drum-view pane* (not per-row
	// dimensions) — see `Game.adjustDrumViewHeight` and the ctor where
	// the OnClick callbacks dispatch into it. The historical row-height
	// scaling has been retired; per-row dimensions are now fixed.
	rowZoomChipRect image.Rectangle
	rowZoomIncBtn   *Button
	rowZoomDecBtn   *Button

	// Mobile overflow menu for Upload/Import/Export
	overflowMenuScroll *MenuScroll // shared scroll component for the overflow menu
	overflowBtns       []*Button   // persisted overflow/template rows (rebuilt on open/page switch); rects updated per frame for scroll
	overflowBtnsPage   int         // page the persisted overflowBtns were built for; rebuild on page change

	// overflowPage selects the overflow popup page: 0 = File actions, 1 = the
	// genre template list. Reset to 0 whenever the menu closes or a template is
	// chosen so reopening always starts at the File page.
	overflowPage int

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
	contextMenuItemIcons   map[*Button]string // per-button leading icon, keyed by button identity (not a parallel slice — can't drift out of lockstep)
	contextMenuHeaderRect  image.Rectangle    // mobile bottom sheet header area
	contextMenuScroll      *ScrollBehavior    // scroll when items overflow

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
	fxPanelDeferredTap     DeferredTap

	// Deferred release-commit state for continuous controls. The live
	// audio.Set* calls stay per-frame; the emit + undo record fire ONCE at
	// drag release via commitEQBand / commitMainVolume / commitRowVolume.
	eqPendingChannel    string
	eqPendingBand       int
	eqPendingGainDB     float64
	eqPendingDirty      bool
	mainVolPending      float64
	mainVolPendingDirty bool

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

	// lastElapsedBeats is the elapsedBeats value from the most recent Draw.
	// recalcButtons (which runs every frame) reads it to size the beat-counter
	// slot to the LIVE readout width, so the notification area can hug the
	// rendered beat/timer text instead of a fixed worst-case slot. At most one
	// frame stale; defaults to 0 ("Beat 1 · 0:00") before the first Draw.
	lastElapsedBeats float64

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

	// twoFingerPan tracks an in-progress two-finger pan gesture over the drum
	// cell grid so it can drive the timeline (horizontal) / row scroll
	// (vertical) exactly like the single-finger grid drag, direction-locked.
	twoFingerPan twoFingerPanState

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

	// notifications: single source of truth for the in-band notification
	// area + history popup. Replaces the former floating top-right toast.
	notifStore *notificationStore
	// notifRect is the dedicated in-band notification area (right of the
	// beat counter); computed in calcLayout.
	notifRect image.Rectangle
	// notifHistoryRect is the history popup's on-screen rect when open.
	notifHistoryRect image.Rectangle
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
func (dv *DrumView) overflowBtn() *Button         { return dv.transportZone.overflowBtn }
func (dv *DrumView) mainVolSlider() *Slider       { return dv.transportZone.mainVolSlider }
func (dv *DrumView) mainVolGroup() *SliderGroup   { return dv.transportZone.mainVolGroup }
func (dv *DrumView) transportGroup() *LayoutGroup { return dv.transportZone.transportGroup }

// RowOffsetForTest reports the index of the first visible drum row
// (dv.rowOffset). Test-only read accessor — production code reads the field
// directly or via the row-rack zone. Used by input-isolation tests that
// assert an unrelated wheel gesture did NOT scroll the drum rows.
func (dv *DrumView) RowOffsetForTest() int { return dv.rowOffset }

// VisibleRowsForTest reports how many drum rows are currently visible
// (dv.visibleRows()). Test-only read accessor used to prove a row-scroll
// test is non-vacuous (more rows exist than fit, so scrolling is possible).
func (dv *DrumView) VisibleRowsForTest() int { return dv.visibleRows() }
