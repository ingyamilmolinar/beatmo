package ui

import (
	"image/color"
	"os"
)

// debugLayoutGuidesEnabled reports whether BEATMO_DEBUG_LAYOUT is set to a
// truthy value. Layout guides (column/row dividers + splitter pills) are
// developer-only chrome — enabled by default would leak 1-px ticks into the
// gaps between widget surfaces (visible as "garbage" at the top edge of the
// drum pane). Production builds keep them off.
func debugLayoutGuidesEnabled() bool {
	switch os.Getenv("BEATMO_DEBUG_LAYOUT") {
	case "1", "true", "TRUE", "yes", "on":
		return true
	}
	return false
}

// ScreenSizeClass distinguishes desktop from mobile layout modes.
type ScreenSizeClass int

const (
	ScreenDesktop ScreenSizeClass = iota
	ScreenMobile
)

// LayoutProfile centralises every mobile-vs-desktop decision into a single
// configuration object. Call UpdateProfile() whenever the screen size changes;
// all sizing/style/behavior functions read from the active profile.
type LayoutProfile struct {
	Class ScreenSizeClass

	// ── Sizing ──────────────────────────────────────────────
	RowHeight         int
	GrabZone          int
	MinTarget         int
	MinCellWidth      int
	SplitterHandleLen int
	SplitterHandleThk int

	// Popup sizing
	PopupPanelW     int
	PopupBtnW       int
	PopupBtnH       int
	PopupGap        int
	PopupPad        int
	PopupTextScale  float64
	PopupLabelScale float64
	PopupValueScale float64
	PopupTitleScale float64
	PopupSectionGap int
	PopupRowH       int

	// Button sizing (0 = use layout-derived)
	TransportBtnSize  int
	RowControlBtnSize int

	// Header sizing
	HeaderMinH int
	HeaderMaxH int

	// Timeline bar
	TimelineBarH int

	// ── Widget board weights ────────────────────────────────
	ColWeights []float64
	RowWeights []float64

	// ── Scrollbar ───────────────────────────────────────────
	ScrollbarStyle ScrollbarStyle

	// ── Button styles ───────────────────────────────────────
	PlayBtnStyle    ButtonVisual
	PlayIconColor   color.Color
	StopBtnStyle    ButtonVisual
	StopIconColor   color.Color
	BPMDecBtnStyle  ButtonVisual
	BPMIncBtnStyle  ButtonVisual
	BPMIconColor    color.Color
	SubdivBtnStyle  ButtonVisual
	TrackBtnStyle   ButtonVisual
	ViewSwitchStyle ButtonVisual
	OverflowStyle   ButtonVisual
	TransportMisc   ButtonVisual
	RowLabelStyle   ButtonVisual

	// ── Drawing flags ───────────────────────────────────────
	DrawTopEdgeHighlight bool // desktop: subtle 3D depth on buttons
	AccentStripeWidth    int  // desktop: 3, mobile: 5
	AccentStripeInsetY   int  // desktop: 0, mobile: 2
	SplitterHandleColor  color.Color
	PopupCornerRadius    int
	CloseButtonSize      int // mobile: BtnHeightSM, desktop: 16

	// ── Sizing: EQ ─────────────────────────────────────────
	EQSliderH      int // desktop: 14, mobile: 28
	EQHandleRadius int // desktop: eqHandleRadius+7 (12), mobile: touchMinTargetPx/2 (22)
	EQDBInputH     int // desktop: 14, mobile: 20

	// SynthHeaderH is the synth/sampler tab header strip height — layout-tied
	// (desktop 48 / mobile 38), sourced from profileOverrides.synthHeaderH.
	SynthHeaderH int

	// ── Sizing: Slider ─────────────────────────────────────
	SliderTrackH     int  // desktop: 4, mobile: 6
	SliderThumbH     int  // desktop: 12, mobile: 18
	SliderThumbW     int  // desktop: 8, mobile: 10
	SliderLabelAbove bool // desktop: false, mobile: true

	// ── Sizing: FX toggle pill (per-slot enable/disable switch) ────
	FXToggleTrackW int // desktop: 32, mobile: 40
	FXToggleTrackH int // desktop: 18, mobile: 22
	FXToggleThumbD int // desktop: 14, mobile: 18

	// ── Sizing: Grid ───────────────────────────────────────
	NodeMinPx    int // desktop: 8, mobile: 12
	NodeMaxPx    int // desktop: 16, mobile: 20
	EdgeThickMul int // desktop: 1, mobile: 2

	// ── Sizing: Layout ─────────────────────────────────────
	ControlPadding   int // desktop: SpaceXS+2 (4), mobile: SpaceXS+2 (4) — both on the 4pt grid
	ControlGap       int // gap between adjacent transport buttons (desktop: 2, mobile: 4)
	ControlGroupPad  int // outline pad around a control group (BPM, transport, file-ops): desktop 3, mobile 4
	ControlLeftInset int // desktop: 12, mobile: 4

	// ── Splitter ────────────────────────────────────────────
	SplitterMinFraction   float64 // desktop: 0 (use 120px abs), mobile: 0.25
	SplitterGrabThreshold int     // desktop: 0 (use full grab zone), mobile: 8

	// ── Feature flags ──────────────────────────────────────
	ShowLayoutGuides   bool // desktop: true, mobile: false
	ShowCursorLabel    bool // desktop: true, mobile: false
	ShowEscHint        bool // desktop: true, mobile: false
	EnableLayoutResize bool // desktop: true, mobile: false
	ShowRackSurface    bool // desktop: true, mobile: false
	DirectDrawRows     bool // desktop: false, mobile: true
	UseBottomSheet     bool // desktop: false, mobile: true
	DrawToolbarSep     bool // desktop: false, mobile: true
	DrawMasterVolIcon  bool // desktop: false, mobile: true

	// ChainModeSegmented renders the Chain tab's three display modes
	// (OVR / SPL / DIF) as one always-visible contiguous segmented control
	// on its own row below the chrome strip, instead of three separate
	// pills in the chrome row. Used on narrow viewports so all three modes
	// stay visible and touch-min without a hidden dropdown. The canonical
	// example of the layout vs density split — pill width is density-tied,
	// but the segmented-vs-separate choice is layout-tied (a tablet at
	// desktop layout keeps three separate pills even at Spacious density).
	ChainModeSegmented bool // desktop: false, mobile: true

	// ── Timeline defaults ──────────────────────────────────
	// DefaultTimelineBeats caps the initial visible drum-view window
	// (in beats) when updateBeatInfos auto-grows it to fit the circuit.
	// 0 means no cap (desktop default: show the full circuit).
	DefaultTimelineBeats int

	// ── Behavior flags ──────────────────────────────────────
	ReserveAddRowSpace bool // desktop: subtract one rowHeight for "+" footer
}

// activeProfile is the lazily-initialised singleton.
var activeProfile *LayoutProfile

// Profile returns the current layout profile. It auto-syncs with the
// current screen detection so callers never see a stale profile — this
// avoids the need to sprinkle UpdateProfile() into every test site that
// sets forceSmallScreenForTest.
func Profile() *LayoutProfile {
	want := detectSmallScreen()
	if activeProfile == nil ||
		(want && activeProfile.Class != ScreenMobile) ||
		(!want && activeProfile.Class != ScreenDesktop) {
		// Phase 0 audio-panel redesign: density is derived from class
		// at rebuild time via densityForClass() — no global mutation.
		// SetDensityForTest's override (densityOverride pointer) wins
		// when set, otherwise defaultDensityFor(class). The non-
		// mutating contract is load-bearing: it prevents class flips
		// in one test from leaking a stale density into the next.
		if want {
			activeProfile = mobileProfile()
		} else {
			activeProfile = desktopProfile()
		}
	}
	return activeProfile
}

// UpdateProfile re-evaluates the screen size and rebuilds the active profile.
// Call this after SetTouchScreenSize and after mobile↔desktop transitions.
func UpdateProfile() {
	if detectSmallScreen() {
		activeProfile = mobileProfile()
	} else {
		activeProfile = desktopProfile()
	}
}

// IsMobile is a convenience shorthand for Profile().Class == ScreenMobile.
func (p *LayoutProfile) IsMobile() bool { return p.Class == ScreenMobile }

// ── Builders ────────────────────────────────────────────────────────────────

func desktopProfile() *LayoutProfile {
	// Phase 5a + audio-panel Phase 0: layout-arrangement fields come
	// from design_profile.gen.go (DESIGN.md `profileOverrides.desktop`).
	// **Density-tied** fields come from densityValuesFor(activeDensity)
	// — see design_density.gen.go. Hand-coded fields are now limited to:
	//   - float scales (PopupTextScale family — fractional)
	//   - color references (PlayIconColor family)
	//   - bool feature flags (DrawTopEdgeHighlight, …)
	//   - ButtonVisual style references (PlayBtnStyle family)
	g := genDesktopProfile
	dv := densityValuesFor(densityForClass(ScreenDesktop))
	return &LayoutProfile{
		Class: ScreenDesktop,

		// Sizing — density-tied
		RowHeight:         dv.RowHeight,
		GrabZone:          dv.GrabZone,
		MinTarget:         dv.MinTarget,
		MinCellWidth:      g.MinCellWidth, // layout-tied
		SplitterHandleLen: dv.SplitterHandleLen,
		SplitterHandleThk: dv.SplitterHandleThk,

		// Popup — layout-tied for panel width + corner, density-tied for buttons
		PopupPanelW:     g.PopupPanelW, // layout-tied
		PopupBtnW:       dv.PopupBtnW,
		PopupBtnH:       dv.PopupBtnH,
		PopupGap:        dv.PopupGap,
		PopupPad:        dv.PopupPad,
		PopupTextScale:  1.0,
		PopupLabelScale: 1.0,
		PopupValueScale: 1.0,
		PopupTitleScale: 1.0,
		PopupSectionGap: dv.PopupSectionGap,
		PopupRowH:       dv.PopupBtnH,

		// Button sizing — density-tied
		TransportBtnSize:  dv.TransportBtnSize,
		RowControlBtnSize: dv.RowControlBtnSize,

		// Header — density-tied
		HeaderMinH:   dv.HeaderMinH,
		HeaderMaxH:   dv.HeaderMaxH,
		TimelineBarH: dv.TimelineBarH,

		// Widget weights. Column 0 hosts the per-row control cluster AND the
		// instrument-name label (the label flexes to fill whatever col0 width
		// is left of the right-anchored cluster — see layoutRowControls). At the
		// old {1,3} (25%) the label was squeezed to ~110px, clipping all but the
		// shortest names. Widening col0 to 40% ({2,3}) roughly triples the label
		// area so instrument names render fully with room for renamed/long ones.
		// controlsW absorbs the slack (leftW-labelW), so needW == col0W and the
		// "tighten to content" pass in drumview_geometry.go leaves col0 intact.
		ColWeights: []float64{2, 3},
		RowWeights: []float64{3, 3, 3},

		// Scrollbar
		ScrollbarStyle: DefaultScrollbarStyle,

		// Button styles — unified neutral transport look. Play/stop/record
		// used to be stoplight green/red; they're now Surface2 with subtle
		// borders like every other button, with the icon carrying the
		// semantics (red for record, neutral for play/stop).
		PlayBtnStyle:   TransportPlayStyle,
		PlayIconColor:  colTextPrimary,
		StopBtnStyle:   TransportStopStyle,
		StopIconColor:  colTextPrimary,
		BPMDecBtnStyle: TransportDecStyle,
		BPMIncBtnStyle: TransportIncStyle,
		BPMIconColor:   colTextSecondary,
		SubdivBtnStyle: InstButtonStyle,
		// TrackBtnStyle uses TransportMiscStyle on both profiles so the
		// track button shares the play/stop chrome (ComponentButtonSecondary).
		// State is expressed via icon glyph + IconColor (see syncTrackBtnVisual),
		// not a fill-style swap — mirroring how SetPlaying flips the play
		// button's icon and tint while keeping its frame fixed.
		TrackBtnStyle:   TransportMiscStyle,
		ViewSwitchStyle: InstButtonStyle,
		OverflowStyle:   DropdownStyle,
		TransportMisc:   TransportMiscStyle,
		RowLabelStyle:   InstButtonStyle,

		// Drawing — density-tied stripe + close, layout-tied corner radius
		DrawTopEdgeHighlight: true,
		AccentStripeWidth:    dv.AccentStripeWidth,
		AccentStripeInsetY:   dv.AccentStripeInsetY,
		SplitterHandleColor:  colSplitterHandle,
		PopupCornerRadius:    g.PopupCornerRadius, // layout-tied
		CloseButtonSize:      dv.CloseButtonSize,

		// EQ sizing — density-tied
		EQSliderH:      dv.EqSliderH,
		EQHandleRadius: dv.EqHandleRadius,
		EQDBInputH:     dv.EqDBInputH,
		SynthHeaderH:   g.SynthHeaderH, // layout-tied (profileOverride)

		// Slider sizing — density-tied
		SliderTrackH:     dv.SliderTrackH,
		SliderThumbH:     dv.SliderThumbH,
		SliderThumbW:     dv.SliderThumbW,
		SliderLabelAbove: false,

		// FX toggle pill — density-tied
		FXToggleTrackW: dv.FxToggleTrackW,
		FXToggleTrackH: dv.FxToggleTrackH,
		FXToggleThumbD: dv.FxToggleThumbD,

		// Grid sizing — density-tied
		NodeMinPx:    dv.NodeMinPx,
		NodeMaxPx:    dv.NodeMaxPx,
		EdgeThickMul: dv.EdgeThickMul,

		// Layout sizing — density-tied gaps, layout-tied inset
		ControlPadding:   dv.ControlPadding,
		ControlGap:       dv.ControlGap,
		ControlGroupPad:  dv.ControlGroupPad,
		ControlLeftInset: g.ControlLeftInset, // layout-tied

		// Splitter — density-tied
		SplitterMinFraction:   0,
		SplitterGrabThreshold: dv.SplitterGrabThreshold,

		// Feature flags
		ShowLayoutGuides:   debugLayoutGuidesEnabled(),
		ShowCursorLabel:    true,
		ShowEscHint:        true,
		EnableLayoutResize: true,
		ShowRackSurface:    true,
		DirectDrawRows:     false,
		UseBottomSheet:     false,
		DrawToolbarSep:     false,
		DrawMasterVolIcon:  true,

		ChainModeSegmented: false,

		// Timeline defaults — generated
		DefaultTimelineBeats: g.DefaultTimelineBeats,

		// Behavior
		ReserveAddRowSpace: true,
	}
}

func mobileProfile() *LayoutProfile {
	// Phase 5a (extended): all dimensional fields come from
	// design_profile.gen.go (DESIGN.md `profileOverrides.mobile`).
	// See desktopProfile() for the list of fields still hand-coded
	// (color refs, bool flags, ButtonVisual style refs).
	g := genMobileProfile
	dv := densityValuesFor(densityForClass(ScreenMobile))
	return &LayoutProfile{
		Class: ScreenMobile,

		// Sizing — density-tied
		RowHeight:         dv.RowHeight,
		GrabZone:          dv.GrabZone,
		MinTarget:         dv.MinTarget,
		MinCellWidth:      g.MinCellWidth, // layout-tied
		SplitterHandleLen: dv.SplitterHandleLen,
		SplitterHandleThk: dv.SplitterHandleThk,

		// Popup — layout-tied panel width, density-tied buttons
		PopupPanelW:     g.PopupPanelW, // layout-tied
		PopupBtnW:       dv.PopupBtnW,
		PopupBtnH:       dv.PopupBtnH,
		PopupGap:        dv.PopupGap,
		PopupPad:        dv.PopupPad,
		PopupTextScale:  popupTextScale,
		PopupLabelScale: 1.3,
		PopupValueScale: 1.5,
		PopupTitleScale: 1.7,
		PopupSectionGap: dv.PopupSectionGap,
		PopupRowH:       BtnHeightMD,

		// Button sizing — density-tied
		TransportBtnSize:  dv.TransportBtnSize,
		RowControlBtnSize: dv.RowControlBtnSize,

		// Header — density-tied
		HeaderMinH:   dv.HeaderMinH,
		HeaderMaxH:   dv.HeaderMaxH,
		TimelineBarH: dv.TimelineBarH,

		// Widget weights
		ColWeights: []float64{1.5, 1.5},
		RowWeights: []float64{3, 5, 2},

		// Scrollbar
		ScrollbarStyle: MobileScrollbarStyle,

		// Button styles
		PlayBtnStyle:    TransportPlayStyle,
		PlayIconColor:   colPlayIconTint,
		StopBtnStyle:    TransportStopStyle,
		StopIconColor:   colStopIconTint,
		BPMDecBtnStyle:  TransportDecStyle,
		BPMIncBtnStyle:  TransportIncStyle,
		BPMIconColor:    colIncDecIcon,
		SubdivBtnStyle:  TransportMiscStyle,
		TrackBtnStyle:   TransportMiscStyle,
		ViewSwitchStyle: TransportMiscStyle,
		OverflowStyle:   TransportMiscStyle,
		TransportMisc:   TransportMiscStyle,
		RowLabelStyle:   MobileRowLabelStyle,

		// Drawing — density-tied stripe + close, layout-tied corner radius
		DrawTopEdgeHighlight: false,
		AccentStripeWidth:    dv.AccentStripeWidth,
		AccentStripeInsetY:   dv.AccentStripeInsetY,
		SplitterHandleColor:  colSplitterHandle,
		PopupCornerRadius:    g.PopupCornerRadius, // layout-tied
		CloseButtonSize:      dv.CloseButtonSize,

		// EQ sizing — density-tied
		EQSliderH:      dv.EqSliderH,
		EQHandleRadius: dv.EqHandleRadius,
		EQDBInputH:     dv.EqDBInputH,
		SynthHeaderH:   g.SynthHeaderH, // layout-tied (profileOverride)

		// Slider sizing — density-tied
		SliderTrackH:     dv.SliderTrackH,
		SliderThumbH:     dv.SliderThumbH,
		SliderThumbW:     dv.SliderThumbW,
		SliderLabelAbove: true,

		// FX toggle pill — density-tied
		FXToggleTrackW: dv.FxToggleTrackW,
		FXToggleTrackH: dv.FxToggleTrackH,
		FXToggleThumbD: dv.FxToggleThumbD,

		// Grid sizing — density-tied
		NodeMinPx:    dv.NodeMinPx,
		NodeMaxPx:    dv.NodeMaxPx,
		EdgeThickMul: dv.EdgeThickMul,

		// Layout sizing — density-tied gaps, layout-tied inset
		ControlPadding:   dv.ControlPadding,
		ControlGap:       dv.ControlGap,
		ControlGroupPad:  dv.ControlGroupPad,
		ControlLeftInset: g.ControlLeftInset, // layout-tied

		// Splitter — density-tied
		SplitterMinFraction:   0.25,
		SplitterGrabThreshold: dv.SplitterGrabThreshold,

		// Feature flags
		ShowLayoutGuides:   false,
		ShowCursorLabel:    false,
		ShowEscHint:        false,
		EnableLayoutResize: false,
		ShowRackSurface:    false,
		DirectDrawRows:     true,
		UseBottomSheet:     true,
		DrawToolbarSep:     true,
		DrawMasterVolIcon:  true,

		ChainModeSegmented: true,

		// Timeline defaults — generated
		DefaultTimelineBeats: g.DefaultTimelineBeats,

		// Behavior
		ReserveAddRowSpace: true,
	}
}
