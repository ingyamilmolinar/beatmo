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
	CloseButtonSize      int // mobile: BtnHeightSM, desktop: desktopPopupBtnH

	// ── Sizing: EQ ─────────────────────────────────────────
	EQSliderH      int // desktop: 14, mobile: 28
	EQHandleRadius int // desktop: eqHandleRadius+7 (12), mobile: touchMinTargetPx/2 (22)
	EQDBInputH     int // desktop: 14, mobile: 20

	// ── Sizing: Slider ─────────────────────────────────────
	SliderTrackH     int  // desktop: 4, mobile: 6
	SliderThumbH     int  // desktop: 12, mobile: 18
	SliderThumbW     int  // desktop: 8, mobile: 10
	SliderLabelAbove bool // desktop: false, mobile: true

	// ── Sizing: Grid ───────────────────────────────────────
	NodeMinPx    int // desktop: 8, mobile: 12
	NodeMaxPx    int // desktop: 16, mobile: 20
	EdgeThickMul int // desktop: 1, mobile: 2

	// ── Sizing: Layout ─────────────────────────────────────
	ControlPadding   int // desktop: buttonPad+2 (4), mobile: buttonPad+1 (3)
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
	return &LayoutProfile{
		Class: ScreenDesktop,

		// Sizing
		RowHeight:         desktopRowHeightPx,
		GrabZone:          desktopGrabZonePx,
		MinTarget:         0,
		MinCellWidth:      desktopMinCellWidthPx,
		SplitterHandleLen: desktopSplitterHandleLen,
		SplitterHandleThk: desktopSplitterHandleThick,

		// Popup
		PopupPanelW:     desktopPopupPanelW,
		PopupBtnW:       desktopPopupBtnW,
		PopupBtnH:       desktopPopupBtnH,
		PopupGap:        desktopPopupGap,
		PopupPad:        desktopPopupPad,
		PopupTextScale:  1.0,
		PopupLabelScale: 1.0,
		PopupValueScale: 1.0,
		PopupTitleScale: 1.0,
		PopupSectionGap: SpaceSM,
		PopupRowH:       desktopPopupBtnH,

		// Button sizing
		TransportBtnSize:  0,
		RowControlBtnSize: 0,

		// Header
		HeaderMinH:   desktopHeaderH,
		HeaderMaxH:   desktopHeaderH,
		TimelineBarH: timelineBarHeightDesktop,

		// Widget weights
		ColWeights: []float64{1, 3},
		RowWeights: []float64{3, 3, 3},

		// Scrollbar
		ScrollbarStyle: DefaultScrollbarStyle,

		// Button styles
		PlayBtnStyle:    PlayButtonStyle,
		PlayIconColor:   nil,
		StopBtnStyle:    StopButtonStyle,
		StopIconColor:   nil,
		BPMDecBtnStyle:  BPMDecStyle,
		BPMIncBtnStyle:  BPMIncStyle,
		BPMIconColor:    colIncDecIconHi,
		SubdivBtnStyle:  InstButtonStyle,
		TrackBtnStyle:   InstButtonStyle,
		ViewSwitchStyle: InstButtonStyle,
		OverflowStyle:   DropdownStyle,
		TransportMisc:   TransportMiscStyle,
		RowLabelStyle:   InstButtonStyle,

		// Drawing
		DrawTopEdgeHighlight: true,
		AccentStripeWidth:    3,
		AccentStripeInsetY:   0,
		SplitterHandleColor:  colSplitterHandle,
		DrawSplitterGrip:     true,
		PopupCornerRadius:    RadiusLG,
		CloseButtonSize:      desktopPopupBtnH,

		// EQ sizing
		EQSliderH:      14,
		EQHandleRadius: 16, // hit area for desktop EQ curve handles
		EQDBInputH:     14,

		// Slider sizing
		SliderTrackH:     4,
		SliderThumbH:     18,
		SliderThumbW:     12,
		SliderLabelAbove: false,

		// Grid sizing
		NodeMinPx:    8,
		NodeMaxPx:    16,
		EdgeThickMul: 1,

		// Layout sizing
		ControlPadding:   4, // buttonPad(2) + 2
		ControlLeftInset: 12,

		// Splitter
		SplitterMinFraction:   0,
		SplitterGrabThreshold: 0,

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

		// Timeline defaults
		DefaultTimelineBeats: 0, // no cap: auto-grow to full circuit

		// Behavior
		ReserveAddRowSpace: true,
	}
}

func mobileProfile() *LayoutProfile {
	return &LayoutProfile{
		Class: ScreenMobile,

		// Sizing
		RowHeight:         touchRowHeightPx,
		GrabZone:          touchGrabZonePx,
		MinTarget:         touchMinTargetPx,
		MinCellWidth:      touchMinCellWidthPx,
		SplitterHandleLen: touchSplitterHandleLen,
		SplitterHandleThk: touchSplitterHandleThick,

		// Popup
		PopupPanelW:     touchPopupPanelW,
		PopupBtnW:       touchPopupBtnW,
		PopupBtnH:       touchPopupBtnH,
		PopupGap:        touchPopupGap,
		PopupPad:        touchPopupPad,
		PopupTextScale:  popupTextScale,
		PopupLabelScale: 1.3,
		PopupValueScale: 1.5,
		PopupTitleScale: 1.7,
		PopupSectionGap: SpaceMD,
		PopupRowH:       BtnHeightMD,

		// Button sizing
		TransportBtnSize:  BtnHeightLG,
		RowControlBtnSize: BtnHeightMD,

		// Header
		HeaderMinH:   mobileHeaderH,
		HeaderMaxH:   mobileHeaderMaxH,
		TimelineBarH: timelineBarHeightMobile,

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

		// Drawing
		DrawTopEdgeHighlight: false,
		AccentStripeWidth:    5,
		AccentStripeInsetY:   2,
		SplitterHandleColor:  colSplitterHandleMobile,
		DrawSplitterGrip:     false,
		PopupCornerRadius:    RadiusXL,
		CloseButtonSize:      BtnHeightSM,

		// EQ sizing
		EQSliderH:      28,
		EQHandleRadius: touchMinTargetPx/2 + 4, // 26
		EQDBInputH:     20,

		// Slider sizing
		SliderTrackH:     6,
		SliderThumbH:     24,
		SliderThumbW:     14,
		SliderLabelAbove: true,

		// Grid sizing
		NodeMinPx:    12,
		NodeMaxPx:    20,
		EdgeThickMul: 2,

		// Layout sizing
		ControlPadding:   3, // buttonPad(2) + 1
		ControlLeftInset: 4,

		// Splitter
		SplitterMinFraction:   0.25,
		SplitterGrabThreshold: 8,

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

		// Timeline defaults
		DefaultTimelineBeats: 8, // mobile: show 8 beats by default

		// Behavior
		ReserveAddRowSpace: false,
	}
}
