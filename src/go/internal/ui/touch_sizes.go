package ui

// touchMinTargetPx is the minimum touch target size per Apple HIG (44px).
// Used directly by code paths that need a compile-time constant; runtime
// callers should prefer Profile().MinTarget which returns 0 on desktop.
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

// desktopRowHeightPx is the desktop row height (matches Profile().RowHeight).
// Retained as a compile-time constant for font_size_test.go which asserts
// font fits within this height.
const desktopRowHeightPx = 36

// touchMinCellWidthPx is the mobile minimum cell width (matches
// Profile().MinCellWidth). Retained for adaptive_cell_count_test.go.
const touchMinCellWidthPx = 2

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
	// Only consider touch mode when the runtime profile enables small-screen
	// detection (browser builds by default; tests can opt in by overriding
	// the profile).
	if !RuntimeProf().EnableTouchSmallScreen {
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
//
// All values below are aliased from design_tokens.gen.go, which is
// generated from DESIGN.md's `spacing:` and `rounded:` blocks. To change
// a value, edit DESIGN.md and run `make gen-design-tokens`.

// Spacing scale (px)
const (
	SpaceXS      = genSpacingXs
	SpaceSM      = genSpacingSm
	SpaceMD      = genSpacingMd
	SpaceLG      = genSpacingLg
	SpaceXL      = genSpacingXl
	SpaceXXL     = genSpacingXxl
	SpaceCluster = genSpacingCluster // inter-cluster gap on the transport bar
)

// Standard button heights (px)
const (
	BtnHeightSM = genSpacingBtnSm
	BtnHeightMD = genSpacingBtnMd
	BtnHeightLG = genSpacingBtnLg
)

// labelButtonWidth sizes a text button to comfortably fit its label
// (label width + a SpaceMD pad on each side). Shared by the audio-panel
// header rows (Synth, Sampler) so no button truncates its label and the
// width tracks the font rather than a hand-tuned magic number.
func labelButtonWidth(label string) int {
	return TextWidth(label) + 2*SpaceMD
}

// Standard icon sizes (px)
const (
	IconSizeSM = genSpacingIconSm
	IconSizeMD = genSpacingIconMd
	IconSizeLG = genSpacingIconLg
)

// Icon-system primitives (logical 24-unit grid). Every IconID renders onto
// this canvas; pixel sizes are derived per-call from the bounding rect.
// IconStrokeWeight is a fractional logical-unit width (Lucide-style 1.75)
// converted to pixels at render time as `weight * (size / grid)`.
const (
	IconGrid         = genIconGrid    // logical units per icon canvas side
	IconPadding      = genIconPadding // empty band inside the canvas
	IconCornerRadius = genIconRadius  // default radius for rectangular elements
)

// IconStrokeWeight is float32 to preserve the 1.75 fraction at small
// rendered sizes. var (not const) so future profile overrides can swap it
// without touching every call site.
var IconStrokeWeight float32 = genIconStroke

// Unified corner radii for all interactive elements
const (
	RadiusSM   = genRoundedSm   // compact buttons, context menu groups (desktop)
	RadiusMD   = genRoundedMd   // standard buttons (transport, row controls, popup items)
	RadiusLG   = genRoundedLg   // panels, bottom sheets, popups (mobile)
	RadiusXL   = genRoundedXl   // emphasized panels (mobile bottom sheet)
	RadiusFull = genRoundedFull // pill buttons (transport group bg, EQ/Scope tabs)
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

// FXToggleTrackW returns the FX panel toggle pill track width.
func FXToggleTrackW() int { return Profile().FXToggleTrackW }

// FXToggleTrackH returns the FX panel toggle pill track height.
func FXToggleTrackH() int { return Profile().FXToggleTrackH }

// FXToggleThumbD returns the FX panel toggle pill thumb diameter.
func FXToggleThumbD() int { return Profile().FXToggleThumbD }
