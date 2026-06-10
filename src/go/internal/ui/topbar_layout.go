package ui

// topbar_layout.go — declarative spec for the DrumView top control bar
// (Play / Stop / Record / BPM ± / Subdiv / Volume / Upload / Import / Export).
//
// All visual dimensions live in DESIGN.md (`profileOverrides:`) and reach
// this spec via Profile(). The only value that lives in code is
// ColumnWeights — those are layout *intent* (the running order of controls
// + their relative widths), not visual style. Keeping weights here mirrors
// the precedent set by MenuSpec / PanelSpec in ui_style.go.
//
// To change the bar height, button minimum, gap, or group outline pad:
// edit DESIGN.md and run `make gen-design-tokens`. To change which
// controls appear or their relative widths: edit DesktopColumnWeights /
// MobileColumnWeights here.

// TopBarSpec is the layout-and-sizing recipe consumed by TransportZone
// (transport_zone.go) and DrumView geometry (drumview_geometry.go). All
// fields are pure integers; there are no Ebiten state references, so the
// spec is trivially testable.
type TopBarSpec struct {
	// Height is the locked-in pixel height of the top control bar.
	// Sourced from profileOverrides.headerMinH/headerMaxH (kept equal so
	// the bar is fixed-height; the cap path in drumview_geometry.go
	// enforces both bounds).
	Height int

	// Padding is the inset applied per cell when laying out a button
	// inside its grid cell (safeInsetTransport pad arg). From
	// profileOverrides.controlPadding.
	Padding int

	// BtnMinSize is the minimum width and height enforced on individual
	// transport buttons (Play / Stop / Record). Below this size, the
	// button is centered and grown back up. From
	// profileOverrides.transportBtnSize. A value of 0 means "no minimum"
	// (the button fills its cell).
	BtnMinSize int

	// GroupOutlinePad is the pixel pad added around a control group
	// (transport, BPM, file-ops) when computing its bounding rect for
	// the group outline / background. From profileOverrides.controlGroupPad.
	GroupOutlinePad int

	// ControlGap is the minimum horizontal gap between two adjacent
	// transport buttons. From profileOverrides.controlGap.
	ControlGap int

	// ColumnWeights is the relative width of each cell in the top-bar
	// grid, in display order. Length matches the number of cells the
	// active layout (desktop or mobile) consumes.
	ColumnWeights []float64
}

// Desktop layout: single row, 13 cells in this order:
//
//	0: Play  | 1: Stop  | 2: Record | 3: BPM box | 4: BPM ±
//	5: Subdiv | 6: spacer | 7: Volume | 8: Upload | 9: Import | 10: Export
//	11: Undo | 12: Redo
var desktopTopBarColumnWeights = []float64{1.3, 1.3, 1.0, 2.2, 0.7, 1.0, 0.3, 1.0, 0.8, 0.8, 0.8, 0.9, 0.9}

// Mobile uses a structurally different two-row toolbar (transport + tools),
// so it carries its own per-row weights inline in transport_zone.go's
// layoutMobile rather than a single ColumnWeights array. Mobile callers
// of TopBarSpec consume Height / Padding / BtnMinSize / GroupOutlinePad /
// ControlGap and ignore ColumnWeights (which is left nil for mobile).

// DesktopTopBarSpec returns the desktop top-bar recipe. Reads from the
// active profile (genDesktopProfile via desktopProfile() builder).
func DesktopTopBarSpec() TopBarSpec {
	return topBarSpecFor(desktopProfile(), desktopTopBarColumnWeights)
}

// MobileTopBarSpec returns the mobile top-bar recipe.
func MobileTopBarSpec() TopBarSpec {
	return topBarSpecFor(mobileProfile(), nil)
}

// ActiveTopBarSpec dispatches on the current Profile().
func ActiveTopBarSpec() TopBarSpec {
	if Profile().IsMobile() {
		return MobileTopBarSpec()
	}
	return DesktopTopBarSpec()
}

func topBarSpecFor(p *LayoutProfile, weights []float64) TopBarSpec {
	return TopBarSpec{
		Height:          p.HeaderMaxH,
		Padding:         p.ControlPadding,
		BtnMinSize:      p.TransportBtnSize,
		GroupOutlinePad: p.ControlGroupPad,
		ControlGap:      p.ControlGap,
		ColumnWeights:   weights,
	}
}
