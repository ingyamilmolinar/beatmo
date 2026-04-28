package ui

import "image/color"

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
	DrawSplitterGrip     bool // desktop: grip lines inside pill
	PopupCornerRadius    int
	CloseButtonSize      int // mobile: BtnHeightSM, desktop: 16

	// ── Sizing: EQ ─────────────────────────────────────────
	EQSliderH      int // desktop: 14, mobile: 28
	EQHandleRadius int // desktop: eqHandleRadius+7 (12), mobile: touchMinTargetPx/2 (22)
	EQDBInputH     int // desktop: 14, mobile: 20

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
	ControlPadding   int // desktop: SpaceXS+2 (4), mobile: SpaceXS+1 (3)
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
	// Phase 5a (extended): all dimensional fields come from
	// design_profile.gen.go (DESIGN.md `profileOverrides.desktop`).
	// Hand-coded fields are now limited to:
	//   - float scales (PopupTextScale family — fractional)
	//   - color references (PlayIconColor family — needs the iconColor
	//     emit + variant system from B3)
	//   - bool feature flags (DrawTopEdgeHighlight, DrawSplitterGrip,
	//     etc. — covered by future schema extensions)
	//   - ButtonVisual style references (PlayBtnStyle family — pending
	//     M2 migration to Spec(ID))
	g := genDesktopProfile
	return &LayoutProfile{
		Class: ScreenDesktop,

		// Sizing — generated
		RowHeight:         g.RowHeight,
		GrabZone:          g.GrabZone,
		MinTarget:         g.MinTarget,
		MinCellWidth:      g.MinCellWidth,
		SplitterHandleLen: g.SplitterHandleLen,
		SplitterHandleThk: g.SplitterHandleThk,

		// Popup — generated for sizing primitives, hand-coded for scales
		PopupPanelW:     g.PopupPanelW,
		PopupBtnW:       g.PopupBtnW,
		PopupBtnH:       g.PopupBtnH,
		PopupGap:        g.PopupGap,
		PopupPad:        g.PopupPad,
		PopupTextScale:  1.0,
		PopupLabelScale: 1.0,
		PopupValueScale: 1.0,
		PopupTitleScale: 1.0,
		PopupSectionGap: g.PopupSectionGap,
		PopupRowH:       g.PopupBtnH,

		// Button sizing — generated
		TransportBtnSize:  g.TransportBtnSize,
		RowControlBtnSize: g.RowControlBtnSize,

		// Header — generated
		HeaderMinH:   g.HeaderMinH,
		HeaderMaxH:   g.HeaderMaxH,
		TimelineBarH: g.TimelineBarH,

		// Widget weights
		ColWeights: []float64{1, 3},
		RowWeights: []float64{3, 3, 3},

		// Scrollbar
		ScrollbarStyle: DefaultScrollbarStyle,

		// Button styles — unified neutral transport look. Play/stop/record
		// used to be stoplight green/red; they're now Surface2 with subtle
		// borders like every other button, with the icon carrying the
		// semantics (red for record, neutral for play/stop).
		PlayBtnStyle:    TransportPlayStyle,
		PlayIconColor:   colTextPrimary,
		StopBtnStyle:    TransportStopStyle,
		StopIconColor:   colTextPrimary,
		BPMDecBtnStyle:  TransportDecStyle,
		BPMIncBtnStyle:  TransportIncStyle,
		BPMIconColor:    colTextSecondary,
		SubdivBtnStyle:  InstButtonStyle,
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

		// Drawing — generated for dimensional, hand-coded for bool flags
		DrawTopEdgeHighlight: true,
		AccentStripeWidth:    g.AccentStripeWidth,
		AccentStripeInsetY:   g.AccentStripeInsetY,
		SplitterHandleColor:  colSplitterHandle,
		DrawSplitterGrip:     true,
		PopupCornerRadius:    g.PopupCornerRadius,
		CloseButtonSize:      g.CloseButtonSize,

		// EQ sizing — generated
		EQSliderH:      g.EqSliderH,
		EQHandleRadius: g.EqHandleRadius,
		EQDBInputH:     g.EqDBInputH,

		// Slider sizing — generated
		SliderTrackH:     g.SliderTrackH,
		SliderThumbH:     g.SliderThumbH,
		SliderThumbW:     g.SliderThumbW,
		SliderLabelAbove: false,

		// FX toggle pill — generated
		FXToggleTrackW: g.FxToggleTrackW,
		FXToggleTrackH: g.FxToggleTrackH,
		FXToggleThumbD: g.FxToggleThumbD,

		// Grid sizing — generated
		NodeMinPx:    g.NodeMinPx,
		NodeMaxPx:    g.NodeMaxPx,
		EdgeThickMul: g.EdgeThickMul,

		// Layout sizing — generated
		ControlPadding:   g.ControlPadding,
		ControlGap:       g.ControlGap,
		ControlGroupPad:  g.ControlGroupPad,
		ControlLeftInset: g.ControlLeftInset,

		// Splitter — generated
		SplitterMinFraction:   0,
		SplitterGrabThreshold: g.SplitterGrabThreshold,

		// Feature flags
		ShowLayoutGuides:   true,
		ShowCursorLabel:    true,
		ShowEscHint:        true,
		EnableLayoutResize: true,
		ShowRackSurface:    true,
		DirectDrawRows:     false,
		UseBottomSheet:     false,
		DrawToolbarSep:     false,
		DrawMasterVolIcon:  true,

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
	return &LayoutProfile{
		Class: ScreenMobile,

		// Sizing — generated
		RowHeight:         g.RowHeight,
		GrabZone:          g.GrabZone,
		MinTarget:         g.MinTarget,
		MinCellWidth:      g.MinCellWidth,
		SplitterHandleLen: g.SplitterHandleLen,
		SplitterHandleThk: g.SplitterHandleThk,

		// Popup — generated for sizing primitives, hand-coded for scales
		PopupPanelW:     g.PopupPanelW,
		PopupBtnW:       g.PopupBtnW,
		PopupBtnH:       g.PopupBtnH,
		PopupGap:        g.PopupGap,
		PopupPad:        g.PopupPad,
		PopupTextScale:  popupTextScale,
		PopupLabelScale: 1.3,
		PopupValueScale: 1.5,
		PopupTitleScale: 1.7,
		PopupSectionGap: g.PopupSectionGap,
		PopupRowH:       BtnHeightMD,

		// Button sizing — generated
		TransportBtnSize:  g.TransportBtnSize,
		RowControlBtnSize: g.RowControlBtnSize,

		// Header — generated
		HeaderMinH:   g.HeaderMinH,
		HeaderMaxH:   g.HeaderMaxH,
		TimelineBarH: g.TimelineBarH,

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

		// Drawing — generated for dimensional, hand-coded for bool flags
		DrawTopEdgeHighlight: false,
		AccentStripeWidth:    g.AccentStripeWidth,
		AccentStripeInsetY:   g.AccentStripeInsetY,
		SplitterHandleColor:  colSplitterHandleMobile,
		DrawSplitterGrip:     false,
		PopupCornerRadius:    g.PopupCornerRadius,
		CloseButtonSize:      g.CloseButtonSize,

		// EQ sizing — generated
		EQSliderH:      g.EqSliderH,
		EQHandleRadius: g.EqHandleRadius,
		EQDBInputH:     g.EqDBInputH,

		// Slider sizing — generated
		SliderTrackH:     g.SliderTrackH,
		SliderThumbH:     g.SliderThumbH,
		SliderThumbW:     g.SliderThumbW,
		SliderLabelAbove: true,

		// FX toggle pill — generated
		FXToggleTrackW: g.FxToggleTrackW,
		FXToggleTrackH: g.FxToggleTrackH,
		FXToggleThumbD: g.FxToggleThumbD,

		// Grid sizing — generated
		NodeMinPx:    g.NodeMinPx,
		NodeMaxPx:    g.NodeMaxPx,
		EdgeThickMul: g.EdgeThickMul,

		// Layout sizing — generated
		ControlPadding:   g.ControlPadding,
		ControlGap:       g.ControlGap,
		ControlGroupPad:  g.ControlGroupPad,
		ControlLeftInset: g.ControlLeftInset,

		// Splitter — generated
		SplitterMinFraction:   0.25,
		SplitterGrabThreshold: g.SplitterGrabThreshold,

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

		// Timeline defaults — generated
		DefaultTimelineBeats: g.DefaultTimelineBeats,

		// Behavior
		ReserveAddRowSpace: false,
	}
}
