package ui

// Every value derives from existing design tokens; zero fields fall back to them at use time, so
// no bespoke menu tokens duplicate them.
type MenuStyle struct {
	// Zero → Profile().RowHeight, then BtnHeightMD.
	RowHeight int
	// Zero → BtnHeightSM.
	BreadcrumbStripH int
	// Zero → BtnHeightSM.
	PaginationStripH int
	// Zero → BtnHeightSM.
	ChipMinW int
	// Zero → 4 × ChipMinW.
	JumpInputW int
	// Zero → IconSizeMD + 2×SpaceXS.
	StarColW int
	// Zero → SpaceSM.
	SegmentPaddingX int
}

// Idempotent.
func (s MenuStyle) resolved() MenuStyle {
	out := s
	if out.RowHeight <= 0 {
		out.RowHeight = Profile().RowHeight
	}
	if out.RowHeight <= 0 {
		// Profile() may be unset on the earliest init paths.
		out.RowHeight = BtnHeightMD
	}
	if out.BreadcrumbStripH <= 0 {
		out.BreadcrumbStripH = BtnHeightSM
	}
	if out.PaginationStripH <= 0 {
		out.PaginationStripH = BtnHeightSM
	}
	if out.ChipMinW <= 0 {
		out.ChipMinW = BtnHeightSM
	}
	if out.JumpInputW <= 0 {
		out.JumpInputW = out.ChipMinW * 4
	}
	if out.StarColW <= 0 {
		out.StarColW = IconSizeMD + SpaceXS*2
	}
	if out.SegmentPaddingX <= 0 {
		out.SegmentPaddingX = SpaceSM
	}
	return out
}

func DefaultMenuStyle() MenuStyle { return MenuStyle{}.resolved() }
