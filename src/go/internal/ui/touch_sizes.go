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
	touchSplitterHandleLen     = 44
	touchSplitterHandleThick   = 6
)

// SplitterHandleLen returns the pill length for the current platform.
func SplitterHandleLen() int {
	if isSmallScreen() {
		return touchSplitterHandleLen
	}
	return desktopSplitterHandleLen
}

// SplitterHandleThick returns the pill thickness for the current platform.
func SplitterHandleThick() int {
	if isSmallScreen() {
		return touchSplitterHandleThick
	}
	return desktopSplitterHandleThick
}

// Desktop defaults
const (
	desktopGrabZonePx  = 5
	desktopRowHeightPx = 24
)

// Minimum cell width (in pixels) so drum-view cells remain visually readable.
const (
	desktopMinCellWidthPx = 4 // narrow but distinguishable on desktop
	touchMinCellWidthPx   = 4 // matches desktop; users zoom via +/- buttons or pinch
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

// isSmallScreen reports whether the UI should use touch-friendly sizing.
// Returns true only on WASM builds with small screens (phones/tablets).
// Desktop builds always use desktop sizing regardless of window size.
// Laptop browsers running WASM use desktop sizing unless the window is very small.
func isSmallScreen() bool {
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

// TouchGrabZone returns the grab zone size for splitter/resize handles.
// Returns a larger zone on small screens for easier touch interaction.
func TouchGrabZone() int {
	if isSmallScreen() {
		return touchGrabZonePx
	}
	return desktopGrabZonePx
}

// TouchRowHeight returns the row height for drum view.
// Returns a larger height on small screens for easier touch interaction.
func TouchRowHeight() int {
	if isSmallScreen() {
		return touchRowHeightPx
	}
	return desktopRowHeightPx
}

// TouchMinTarget returns the minimum touch target size.
// Only applies on small screens (phones/tablets).
func TouchMinTarget() int {
	if isSmallScreen() {
		return touchMinTargetPx
	}
	return 0 // No minimum on desktop/laptop
}

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
// On small screens the threshold is larger so cells remain readable on phones.
func MinCellWidth() int {
	if isSmallScreen() {
		return touchMinCellWidthPx
	}
	return desktopMinCellWidthPx
}

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
func PopupPanelW() int {
	if isSmallScreen() {
		return touchPopupPanelW
	}
	return desktopPopupPanelW
}

// PopupBtnW returns the popup button width.
func PopupBtnW() int {
	if isSmallScreen() {
		return touchPopupBtnW
	}
	return desktopPopupBtnW
}

// PopupBtnH returns the popup button height.
func PopupBtnH() int {
	if isSmallScreen() {
		return touchPopupBtnH
	}
	return desktopPopupBtnH
}

// PopupGap returns the gap between popup elements.
func PopupGap() int {
	if isSmallScreen() {
		return touchPopupGap
	}
	return desktopPopupGap
}

// PopupPad returns the popup padding.
func PopupPad() int {
	if isSmallScreen() {
		return touchPopupPad
	}
	return desktopPopupPad
}

// popupTextScale is the text scale factor for popup labels/values on mobile.
const popupTextScale = 1.5

// PopupTextScale returns the text scale for node popup content.
// On small screens, text is rendered at 1.5× for readability; desktop uses 1×.
func PopupTextScale() float64 {
	if isSmallScreen() {
		return popupTextScale
	}
	return 1.0
}

// PopupSectionGap returns the vertical gap between popup sections.
func PopupSectionGap() int {
	if isSmallScreen() {
		return SpaceMD
	}
	return SpaceSM
}

// PopupRowH returns the unified row height for popup content rows.
func PopupRowH() int {
	if isSmallScreen() {
		return BtnHeightMD
	}
	return desktopPopupBtnH
}

// PopupLabelScale returns the text scale for popup labels.
func PopupLabelScale() float64 {
	if isSmallScreen() {
		return 1.3
	}
	return 1.0
}

// PopupValueScale returns the text scale for popup values.
func PopupValueScale() float64 {
	if isSmallScreen() {
		return 1.5
	}
	return 1.0
}

// PopupTitleScale returns the text scale for the popup title.
func PopupTitleScale() float64 {
	if isSmallScreen() {
		return 1.7
	}
	return 1.0
}

// TransportBtnSize returns the unified transport button size.
func TransportBtnSize() int {
	if isSmallScreen() {
		return BtnHeightLG
	}
	return 0 // desktop uses layout-derived sizing
}

// RowControlBtnSize returns the unified row control button size.
func RowControlBtnSize() int {
	if isSmallScreen() {
		return BtnHeightMD
	}
	return 0 // desktop uses layout-derived sizing
}
