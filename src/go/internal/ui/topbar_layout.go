package ui

// Every visual dimension comes from DESIGN.md through Profile(); only ColumnWeights, layout intent, live here.

// Pure integers with no Ebiten state, so it is trivially testable.
type TopBarSpec struct {
	// headerMinH == headerMaxH, so the bar is fixed-height.
	Height int

	Padding int

	// A smaller button is centred and grown back; 0 = no minimum (fills its cell).
	BtnMinSize int

	GroupOutlinePad int

	ControlGap int

	// In display order; its length matches the active layout's cell count.
	ColumnWeights []float64
}

// Play | Stop | Record | BPM box | BPM ± | Subdiv | spacer | Volume | Undo | Redo | Overflow. File ops live in the
// overflow menu, so the narrow ~308 px column gives the rest readable widths.
var desktopTopBarColumnWeights = []float64{1.3, 1.3, 1.0, 2.2, 0.7, 1.0, 0.6, 1.0, 1.0, 1.0, 1.0}

// Mobile has its own two-row toolbar with weights in transport_zone.go, so ColumnWeights is nil there.

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
