package ui

import (
	"image/color"
	"os"
)

// Developer-only chrome: on by default it leaks 1-px ticks into the gaps between surfaces.
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

// Every mobile-vs-desktop decision in one place; call UpdateProfile() when the screen size changes.
type LayoutProfile struct {
	Class ScreenSizeClass

	// ── Sizing ──────────────────────────────────────────────
	RowHeight         int
	GrabZone          int
	MinTarget         int
	MinCellWidth      int
	SplitterHandleLen int
	SplitterHandleThk int

	// Menu width classes (DESIGN.md "Menu sizing & layout"; menu_sizing.go)
	MenuWActionMin int
	MenuWActionMax int
	MenuWList      int
	MenuWEditor    int
	MenuWWideMax   int

	// Popup sizing
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

	// Layout-tied, not density-tied: narrow viewports show all three modes as one segmented control.
	ChainModeSegmented bool // desktop: false, mobile: true

	// Caps the initial visible window when updateBeatInfos auto-grows it; 0 = no cap.
	DefaultTimelineBeats int

	// ── Behavior flags ──────────────────────────────────────
	ReserveAddRowSpace bool // desktop: subtract one rowHeight for "+" footer
}

// activeProfile is the lazily-initialised singleton.
var activeProfile *LayoutProfile

// Auto-syncs with screen detection, so callers never see a stale profile.
func Profile() *LayoutProfile {
	want := detectSmallScreen()
	if activeProfile == nil ||
		(want && activeProfile.Class != ScreenMobile) ||
		(!want && activeProfile.Class != ScreenDesktop) {
		// Density is derived from the class at rebuild, never by global mutation, so a class flip in one
		// test cannot leak a stale density into the next. SetDensityForTest's override wins.
		if want {
			activeProfile = mobileProfile()
		} else {
			activeProfile = desktopProfile()
		}
	}
	return activeProfile
}

// Call after SetTouchScreenSize and mobile↔desktop transitions.
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
	// Layout fields come from design_profile.gen.go and density fields from the density tables; only
	// float scales, color refs, bool flags and style refs are hand-coded.
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

		// Menu width classes — layout-tied
		MenuWActionMin: g.MenuWActionMin,
		MenuWActionMax: g.MenuWActionMax,
		MenuWList:      g.MenuWList,
		MenuWEditor:    g.MenuWEditor,
		MenuWWideMax:   g.MenuWWideMax,

		// Popup — layout-tied corner, density-tied buttons
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

		// Column 0 holds the controls and the instrument label; 40 % gives names room to render fully.
		ColWeights: []float64{2, 3},
		RowWeights: []float64{3, 3, 3},

		// Scrollbar
		ScrollbarStyle: DefaultScrollbarStyle,

		// Neutral like every button; the icon carries the meaning.
		PlayBtnStyle: TransportPlayStyle,
		// Muted idle icons; record stays red and playing swaps to the accent.
		PlayIconColor:  colTextSecondary,
		StopBtnStyle:   TransportStopStyle,
		StopIconColor:  colTextSecondary,
		BPMDecBtnStyle: TransportDecStyle,
		BPMIncBtnStyle: TransportIncStyle,
		BPMIconColor:   colTextSecondary,
		SubdivBtnStyle: InstButtonStyle,
		// State shows through the icon glyph and color, never a fill swap, like the play button.
		TrackBtnStyle:   TimelineKeycapStyle,
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

		// Menu width classes — layout-tied
		MenuWActionMin: g.MenuWActionMin,
		MenuWActionMax: g.MenuWActionMax,
		MenuWList:      g.MenuWList,
		MenuWEditor:    g.MenuWEditor,
		MenuWWideMax:   g.MenuWWideMax,

		// Popup — density-tied buttons
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
		TrackBtnStyle:   TimelineKeycapStyle,
		ViewSwitchStyle: TransportMiscStyle,
		OverflowStyle:   TransportMiscStyle,
		TransportMisc:   TransportMiscStyle,
		RowLabelStyle:   MobileRowLabelStyle,

		// On both platforms: visual styling must not diverge by screen class.
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
