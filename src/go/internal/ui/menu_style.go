package ui

// MenuStyle bundles the tunable element dimensions used by the
// instrument-menu component (and any future menu-style overlay)
// so element sizes flow from the design-system layer
// (touch_sizes.go + runtime_profile.go) instead of being hard-coded
// inline. Override fields explicitly when a caller needs a non-
// default — leaving a field at the zero value falls back to the
// design-system default resolved at use time.
//
// Why this lives here and not in DESIGN.md: every value below is
// already derived from existing DESIGN.md tokens (rowHeight,
// popupPad, btn-md, icon-md, …). Adding bespoke "inst-menu-*"
// tokens to DESIGN.md would duplicate the same intent under more
// names. The MenuStyle abstraction lets the menu reuse the
// canonical tokens while exposing one named seam for future
// per-element overrides.
type MenuStyle struct {
	// RowHeight is the per-row height of the menu (search row,
	// breadcrumb row, instrument row, category row). Zero →
	// Profile().RowHeight; if that is also zero, BtnHeightMD.
	RowHeight int
	// BreadcrumbStripH is the height of the breadcrumb segment row
	// shown above the search box in instruments mode. Zero →
	// BtnHeightSM.
	BreadcrumbStripH int
	// PaginationStripH is the height of the pagination strip
	// (numbered chips or "Page N/M" jump-input) shown below the
	// instrument list. Zero → BtnHeightSM.
	PaginationStripH int
	// ChipMinW is the minimum width of a numbered page chip.
	// Zero → BtnHeightSM (square chip the size of a small button).
	ChipMinW int
	// JumpInputW is the width of the numeric jump-input shown when
	// PageCount > pageChipsThreshold. Zero → 4 × ChipMinW.
	JumpInputW int
	// StarColW is the width of the per-row favorite-star column.
	// Zero → IconSizeMD + 2×SpaceXS.
	StarColW int
	// SegmentPaddingX is the horizontal padding inside a breadcrumb
	// segment label. Zero → SpaceSM.
	SegmentPaddingX int
}

// resolved returns a copy of s with every zero-value field replaced
// by its design-system default. Idempotent: calling resolved on an
// already-resolved MenuStyle returns an identical value.
func (s MenuStyle) resolved() MenuStyle {
	out := s
	if out.RowHeight <= 0 {
		out.RowHeight = Profile().RowHeight
	}
	if out.RowHeight <= 0 {
		// Last-resort floor in case Profile() is unset (e.g. earliest
		// init paths in tests). BtnHeightMD is the design's default
		// touch-friendly button height.
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

// DefaultMenuStyle returns the canonical zero-overrides MenuStyle.
// Most call sites should use this; tests that need to pin specific
// dimensions construct a MenuStyle literal and override only the
// relevant fields.
func DefaultMenuStyle() MenuStyle { return MenuStyle{}.resolved() }
