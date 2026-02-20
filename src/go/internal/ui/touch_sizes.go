package ui

import "runtime"

// Touch-friendly size constants (Apple HIG recommends 44px minimum)
const (
	touchMinTargetPx = 44 // minimum touch target size
	touchGrabZonePx  = 16 // grab zone for splitter/resize handles
	touchRowHeightPx = 44 // row height for drum view on touch (Apple HIG minimum)
)

// Splitter handle sizing (pill indicator on divider lines)
const (
	desktopSplitterHandleLen   = 50 // pill length (along the divider line)
	desktopSplitterHandleThick = 8  // pill thickness (perpendicular to divider)
	touchSplitterHandleLen     = 56
	touchSplitterHandleThick   = 6
)

// SplitterHandleLen returns the pill length for the current platform.
func SplitterHandleLen() int { return Profile().SplitterHandleLen }

// SplitterHandleThick returns the pill thickness for the current platform.
func SplitterHandleThick() int { return Profile().SplitterHandleThk }

// Desktop defaults
const (
	desktopGrabZonePx  = 5
	desktopRowHeightPx = 28
)

// Minimum cell width (in pixels) so drum-view cells remain visually readable.
const (
	desktopMinCellWidthPx = 2 // allows high cell counts on wide timelines
	touchMinCellWidthPx   = 2 // matches desktop; users zoom via +/- buttons or pinch
)

// Threshold for considering a screen "small" (phone/tablet in landscape)
const smallScreenWidthPx = 900

// touchScreenWidth stores the current screen width for touch detection.
// Updated by the game loop when window size changes.
var touchScreenWidth int

//nolint:unused // used in js_exports_harness.go (WASM build tag)
var touchScreenHeight int

// forceSmallScreenForTest overrides isSmallScreen() in tests.
var forceSmallScreenForTest bool

// SetTouchScreenSize updates the screen dimensions used for touch detection.
// Call this from the game loop when window size changes.
func SetTouchScreenSize(width, height int) {
	touchScreenWidth = width
	touchScreenHeight = height
}

// detectSmallScreen is the raw platform detection, used only by UpdateProfile().
func detectSmallScreen() bool {
	if forceSmallScreenForTest {
		return true
	}
	// Only consider touch mode on WASM (browser) builds
	if runtime.GOARCH != "wasm" {
		return false
	}
	// On WASM, use touch mode only for genuinely small screens
	return touchScreenWidth > 0 && touchScreenWidth < smallScreenWidthPx
}

// isSmallScreen reports whether the UI should use touch-friendly sizing.
// All production call sites have been migrated to Profile().IsMobile();
// this function remains for test code and backward compatibility.
func isSmallScreen() bool {
	return Profile().IsMobile()
}

// TouchGrabZone returns the grab zone size for splitter/resize handles.
func TouchGrabZone() int { return Profile().GrabZone }

// TouchRowHeight returns the row height for drum view.
func TouchRowHeight() int { return Profile().RowHeight }

// TouchMinTarget returns the minimum touch target size.
func TouchMinTarget() int { return Profile().MinTarget }

// ExpandHitArea expands a button/control size to meet minimum touch targets.
// Returns the original size if already large enough or on desktop.
func ExpandHitArea(size int) int {
	min := TouchMinTarget()
	if min > 0 && size < min {
		return min
	}
	return size
}

// MinCellWidth returns the minimum cell width (in pixels) for drum-view cells.
func MinCellWidth() int { return Profile().MinCellWidth }

// ─── Design Token System ───────────────────────────────────
// Standardized spacing and sizing scale used throughout the UI.

// Spacing scale (px)
const (
	SpaceXS  = 2
	SpaceSM  = 4
	SpaceMD  = 8
	SpaceLG  = 12
	SpaceXL  = 16
	SpaceXXL = 24
)

// Standard button heights (px)
const (
	BtnHeightSM = 28
	BtnHeightMD = 36
	BtnHeightLG = 44
)

// Standard icon sizes (px)
const (
	IconSizeSM = 16
	IconSizeMD = 20
	IconSizeLG = 24
)

// Unified corner radii for all interactive elements
const (
	RadiusSM = 6  // compact buttons, context menu groups (desktop)
	RadiusMD = 8  // standard buttons (transport, row controls, popup items)
	RadiusLG = 12 // panels, bottom sheets, popups (mobile)
	RadiusXL = 16 // emphasized panels (mobile bottom sheet)
)

// Node popup menu sizing constants.
const (
	// Desktop (matches existing hardcoded values)
	desktopPopupPanelW = 220
	desktopPopupBtnW   = 18
	desktopPopupBtnH   = 16
	desktopPopupGap    = 4
	desktopPopupPad    = 6

	// Mobile (touch-friendly)
	touchPopupPanelW = 300
	touchPopupBtnW   = 44
	touchPopupBtnH   = 36
	touchPopupGap    = 6
	touchPopupPad    = 10
)

// PopupPanelW returns the popup panel width.
func PopupPanelW() int { return Profile().PopupPanelW }

// PopupBtnW returns the popup button width.
func PopupBtnW() int { return Profile().PopupBtnW }

// PopupBtnH returns the popup button height.
func PopupBtnH() int { return Profile().PopupBtnH }

// PopupGap returns the gap between popup elements.
func PopupGap() int { return Profile().PopupGap }

// PopupPad returns the popup padding.
func PopupPad() int { return Profile().PopupPad }

// popupTextScale is the text scale factor for popup labels/values on mobile.
const popupTextScale = 1.5

// PopupTextScale returns the text scale for node popup content.
func PopupTextScale() float64 { return Profile().PopupTextScale }

// PopupSectionGap returns the vertical gap between popup sections.
func PopupSectionGap() int { return Profile().PopupSectionGap }

// PopupRowH returns the unified row height for popup content rows.
func PopupRowH() int { return Profile().PopupRowH }

// PopupLabelScale returns the text scale for popup labels.
func PopupLabelScale() float64 { return Profile().PopupLabelScale }

// PopupValueScale returns the text scale for popup values.
func PopupValueScale() float64 { return Profile().PopupValueScale }

// PopupTitleScale returns the text scale for the popup title.
func PopupTitleScale() float64 { return Profile().PopupTitleScale }

// TransportBtnSize returns the unified transport button size.
func TransportBtnSize() int { return Profile().TransportBtnSize }

// RowControlBtnSize returns the unified row control button size.
func RowControlBtnSize() int { return Profile().RowControlBtnSize }
