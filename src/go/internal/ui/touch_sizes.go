package ui

// Runtime callers should prefer Profile().MinTarget, which is 0 on desktop.
const touchMinTargetPx = 44

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

const desktopRowHeightPx = 36

const touchMinCellWidthPx = 2

// Threshold for considering a screen "small" (phone/tablet in landscape)
const smallScreenWidthPx = 900

var touchScreenWidth int

//nolint:unused // used in js_exports_harness.go (WASM build tag)
var touchScreenHeight int

// forceSmallScreenForTest overrides isSmallScreen() in tests.
var forceSmallScreenForTest bool

func SetTouchScreenSize(width, height int) {
	touchScreenWidth = width
	touchScreenHeight = height
}

// detectSmallScreen is the raw platform detection, used only by UpdateProfile().
func detectSmallScreen() bool {
	if forceSmallScreenForTest {
		return true
	}
	// Only when the runtime profile enables small-screen detection.
	if !RuntimeProf().EnableTouchSmallScreen {
		return false
	}
	// On WASM, use touch mode only for genuinely small screens
	return touchScreenWidth > 0 && touchScreenWidth < smallScreenWidthPx
}

// Production uses Profile().IsMobile(); kept for tests.
func isSmallScreen() bool {
	return Profile().IsMobile()
}

// TouchGrabZone returns the grab zone size for splitter/resize handles.
func TouchGrabZone() int { return Profile().GrabZone }

// TouchRowHeight returns the row height for drum view.
func TouchRowHeight() int { return Profile().RowHeight }

// TouchMinTarget returns the minimum touch target size.
func TouchMinTarget() int { return Profile().MinTarget }

// Unchanged when already large enough or on desktop.
func ExpandHitArea(size int) int {
	min := TouchMinTarget()
	if min > 0 && size < min {
		return min
	}
	return size
}

// MinCellWidth returns the minimum cell width (in pixels) for drum-view cells.
func MinCellWidth() int { return Profile().MinCellWidth }

// Aliased from design_tokens.gen.go: edit DESIGN.md and run `make gen-design-tokens`.

// Spacing scale (px)
const (
	SpaceXS      = genSpacingXs
	SpaceSM      = genSpacingSm
	SpaceMD      = genSpacingMd
	SpaceLG      = genSpacingLg
	SpaceXL      = genSpacingXl
	SpaceXXL     = genSpacingXxl
	SpaceCluster = genSpacingCluster // inter-cluster gap on the transport bar

	SwatchSizeSM = genSpacingSwatchSm
	SwatchSizeMD = genSpacingSwatchMd
)

// Standard button heights (px)
const (
	BtnHeightSM = genSpacingBtnSm
	BtnHeightMD = genSpacingBtnMd
	BtnHeightLG = genSpacingBtnLg
)

// From the font, never a magic number, so no button truncates its label.
func labelButtonWidth(label string) int {
	return TextWidth(label) + 2*SpaceMD
}

// Standard icon sizes (px)
const (
	IconSizeSM = genSpacingIconSm
	IconSizeMD = genSpacingIconMd
	IconSizeLG = genSpacingIconLg
)

// A 24-unit logical grid; stroke weight converts to pixels as weight * (size / grid).
const (
	IconGrid         = genIconGrid    // logical units per icon canvas side
	IconPadding      = genIconPadding // empty band inside the canvas
	IconCornerRadius = genIconRadius  // default radius for rectangular elements
)

// float32 keeps the 1.75 fraction at small sizes.
var IconStrokeWeight float32 = genIconStroke

// Chosen by ROLE, not size: every interactive control is RadiusControl, so a toolbar reads as one system.
const (
	RadiusNone      = genRoundedNone      // tiling surfaces: grid cells, meter segments, track fills
	RadiusControl   = genRoundedControl   // EVERY interactive control — buttons, chips, tabs, inputs, keycaps
	RadiusContainer = genRoundedContainer // popups, menus, cards, tooltips, panel chrome
	RadiusSheet     = genRoundedSheet     // mobile bottom sheet, full-width overlays
)

// Prefer a role token: raw steps are how controls drifted into pills.
const (
	RadiusXXS = genRoundedXxs // 4
	RadiusXS  = genRoundedXs  // 6
	RadiusSM  = genRoundedSm  // 8
	RadiusMD  = genRoundedMd  // 12
	RadiusLG  = genRoundedLg  // 16
	RadiusXL  = genRoundedXl  // 20
)

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

// FXToggleTrackW returns the FX panel toggle pill track width.
func FXToggleTrackW() int { return Profile().FXToggleTrackW }

// FXToggleTrackH returns the FX panel toggle pill track height.
func FXToggleTrackH() int { return Profile().FXToggleTrackH }

// FXToggleThumbD returns the FX panel toggle pill thumb diameter.
func FXToggleThumbD() int { return Profile().FXToggleThumbD }
