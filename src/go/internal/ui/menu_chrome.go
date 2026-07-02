package ui

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

// menuItemState is the visual state of a menu row.
type menuItemState int

const (
	menuItemRest menuItemState = iota
	menuItemHover
	menuItemActive
)

// accentStripeW is the density-aware left accent-stripe width used by active
// menu items (shares the row-rack accent stripe token).
func accentStripeW() int {
	w := Profile().DensityValues().AccentStripeWidth
	if w < 2 {
		w = 2
	}
	return w
}

// drawMenuHeaderBand paints a header band with a faint sunset gradient
// (grid-horizon → surface-overlay) and a 1px azure underline — the shared
// "Neon Horizon" header for identity menus.
func drawMenuHeaderBand(dst *ebiten.Image, r image.Rectangle) {
	drawMenuHeaderBandAccent(dst, r, colAccent)
}

// drawMenuHeaderBandAccent is drawMenuHeaderBand with a caller-supplied accent
// color for the underline. Per-instrument menus pass the instrument color so
// the header reads as owned by the instrument; global menus pass colAccent.
func drawMenuHeaderBandAccent(dst *ebiten.Image, r image.Rectangle, accent color.Color) {
	if r.Empty() {
		return
	}
	// Flat: no gradient band. A single hairline underline in the accent color is
	// the only chrome; the title text is drawn by the caller over the panel.
	underline := image.Rect(r.Min.X, r.Max.Y-1, r.Max.X, r.Max.Y)
	drawRect(dst, underline, WithAlphaFromColor(accent, genAlphaMenuHeaderAccent), true)
}

// drawMenuItemBackground paints the per-state background of a menu row:
// rest = nothing; hover = cushion lift + faint azure glow; active = left
// azure stripe + tinted fill.
func drawMenuItemBackground(dst *ebiten.Image, r image.Rectangle, state menuItemState) {
	drawMenuItemBackgroundAccent(dst, r, state, colAccent)
}

// drawMenuItemBackgroundAccent is drawMenuItemBackground with a caller-supplied
// accent color for the active stripe + hover/active tints. Per-instrument menus
// pass the instrument color; global menus pass colAccent.
func drawMenuItemBackgroundAccent(dst *ebiten.Image, r image.Rectangle, state menuItemState, accent color.Color) {
	if r.Empty() {
		return
	}
	switch state {
	case menuItemRest:
		// Flat: rows sit directly on the menu's single surface, no per-row pad.
	case menuItemHover:
		drawRect(dst, r, WithAlphaFromColor(accent, genAlphaSubtle), true)
	case menuItemActive:
		drawRect(dst, r, WithAlphaFromColor(accent, genAlphaMenuActiveTint), true)
		stripe := image.Rect(r.Min.X, r.Min.Y, r.Min.X+accentStripeW(), r.Max.Y)
		drawRect(dst, stripe, accent, true)
	}
}

// drawMenuSeparator draws a 1px hairline across the vertical center of r — the
// minimalist group divider that replaces the old group-container boxes.
func drawMenuSeparator(dst *ebiten.Image, r image.Rectangle) {
	if r.Empty() {
		return
	}
	y := r.Min.Y + r.Dy()/2
	line := image.Rect(r.Min.X+SpaceSM, y, r.Max.X-SpaceSM, y+1)
	drawRect(dst, line, WithAlpha(genColorBorder, AlphaSubtle), true)
}

// menuTitleRole is the unified font role for EVERY menu title (row context
// menu, node pop-up, overflow "File", color picker). Sized between RoleCaption
// and RolePanelTitle: the overflow "File" reads a bit larger, instrument-name
// titles read smaller — all identical.
const menuTitleRole = RoleSectionHeader

// menuTitleSwatchSz is the instrument swatch (rounded square) shown at the left
// of an instrument-scoped menu title.
const menuTitleSwatchSz = 10

// menuTitleSwatchRect returns the vertically-centered leading swatch rect for a
// menu title header.
func menuTitleSwatchRect(headerRect image.Rectangle) image.Rectangle {
	y := headerRect.Min.Y + (headerRect.Dy()-menuTitleSwatchSz)/2
	x := headerRect.Min.X
	return image.Rect(x, y, x+menuTitleSwatchSz, y+menuTitleSwatchSz)
}

// drawMenuTitle draws a unified menu title: an optional instrument swatch
// (rounded square + faint glow) followed by the label at menuTitleRole. Pass a
// nil swatch for global menus (e.g. the overflow "File"). Used by every menu so
// title font, size, color, and iconography are 100% consistent.
func drawMenuTitle(dst *ebiten.Image, headerRect image.Rectangle, label string, swatch color.Color) {
	if headerRect.Empty() {
		return
	}
	th := StyledTextHeight(menuTitleRole)
	ty := headerRect.Min.Y + (headerRect.Dy()-th)/2
	textX := headerRect.Min.X
	if swatch != nil {
		sr := menuTitleSwatchRect(headerRect)
		drawRoundedRect(dst, sr.Inset(-2), WithAlphaFromColor(swatch, AlphaSubtle), RadiusXXS, true)
		drawRoundedRect(dst, sr, swatch, RadiusXXS, true)
		textX = sr.Max.X + SpaceSM
	}
	DrawTextStyled(dst, label, textX, ty, menuTitleRole, colTextPrimary)
}

// menuRowIconSize is the small leading-icon size for menu rows — IconSizeMD on
// touch (mobile), IconSizeSM on desktop. Shared by the row context menu and the
// overflow menu so every menu's iconography matches.
func menuRowIconSize() int {
	if Profile().IsMobile() {
		return IconSizeMD
	}
	return IconSizeSM
}

// menuRowIconRect returns the centered, left-inset square (size menuRowIconSize)
// for a menu row's leading icon.
func menuRowIconRect(rowRect image.Rectangle) image.Rectangle {
	s := menuRowIconSize()
	ix := rowRect.Min.X + SpaceMD
	iy := rowRect.Min.Y + (rowRect.Dy()-s)/2
	return image.Rect(ix, iy, ix+s, iy+s)
}

// menuRowLabelX returns the x where a menu row's label starts — to the right of
// the leading icon.
func menuRowLabelX(rowRect image.Rectangle) int {
	return rowRect.Min.X + SpaceMD + menuRowIconSize() + SpaceSM
}

// MenuRowSpec describes the variable content of one list-menu row. The styling
// and animation (keycap chrome via btn.Draw, press-depth travel, hover glow,
// accent background/stripe) are fixed and identical for every menu; only these
// fields vary. Behavior (open/close/toggle) lives in the caller's onClick, not
// here. Shared by the instrument menu, row context menu, subdivision selector,
// and template/overflow menu so styling + animation are byte-identical.
type MenuRowSpec struct {
	Accent      color.Color   // hover/active tint + active stripe; nil → colAccent
	State       menuItemState // rest / hover / active (caller's selection model)
	Swatch      color.Color   // optional leading rounded-square swatch; nil → none
	IconID      IconID        // optional leading icon; "" → none
	IconTint    color.Color   // icon tint; nil → colMenuIcon
	Highlights  []int         // optional fuzzy-match highlight runs (rune indices)
	Label       string        // row text (drawn here; btn.Text is blanked during btn.Draw)
	LabelColor  color.Color   // nil → colTextPrimary
	LabelRole   TextRole      // zero → RoleBody
	CenterLabel bool          // true → center label in the row (subdiv); else left-align
}

// drawMenuRow renders one list-menu row with the unified styling + animation:
// background accent → keycap chrome (btn.Draw with blanked text, which also
// advances AdvancePressAnim) → optional leading swatch or icon → fuzzy-match
// highlights → label. Swatch wins the leading slot over an icon when both set
// (mirrors the instrument menu, which has a swatch and no icon).
func drawMenuRow(dst *ebiten.Image, btn *Button, spec MenuRowSpec) {
	r := btn.Rect()
	if r.Empty() {
		return
	}
	accent := spec.Accent
	if accent == nil {
		accent = colAccent
	}
	// 1. Keycap chrome + press/hover animation FIRST. drawKeycapShell paints an
	// opaque rounded fill over the full row, so the per-state accent background
	// must come AFTER it — otherwise the keycap covers the stripe/tint and the
	// accent is invisible (the node dropdown's node-color indicator vanished this
	// way; menus with a swatch/accent-label masked it). Blank the text; the label
	// is drawn below (centered/iconned variants need custom placement).
	saved := btn.Text
	btn.Text = ""
	btn.Draw(dst)
	btn.Text = saved

	// 2. Per-state background OVER the keycap (rest = nothing, hover = tint,
	// active = stripe+tint) so active/hover accents are actually visible.
	drawMenuItemBackgroundAccent(dst, r, spec.State, accent)

	role := spec.LabelRole
	if role == 0 {
		role = RoleBody
	}
	labelCol := spec.LabelColor
	if labelCol == nil {
		labelCol = colTextPrimary
	}
	th := StyledTextHeight(role)
	ty := r.Min.Y + (r.Dy()-th)/2

	// 3. Leading element: swatch takes the slot if present, else an optional icon.
	labelX := r.Min.X + SpaceMD
	switch {
	case spec.Swatch != nil:
		sz := instMenuSwatchSz
		sx := r.Min.X + SpaceSM
		sy := r.Min.Y + (r.Dy()-sz)/2
		sr := image.Rect(sx, sy, sx+sz, sy+sz)
		drawRoundedRect(dst, sr, spec.Swatch, RadiusXXS, true)
		labelX = sr.Max.X + SpaceSM
	case spec.IconID != "":
		tint := spec.IconTint
		if tint == nil {
			tint = colMenuIcon
		}
		DrawIcon(dst, spec.IconID, menuRowIconRect(r), tint)
		labelX = menuRowLabelX(r)
	}

	// 4. Centered label (subdivision selector) overrides the leading-x layout.
	if spec.CenterLabel {
		tw := StyledTextWidth(spec.Label, role)
		labelX = r.Min.X + (r.Dx()-tw)/2
	}

	// 5. Fuzzy-match highlights behind the label, then the label itself.
	if len(spec.Highlights) > 0 {
		drawButtonHighlights(dst, spec.Label, spec.Highlights, labelX, ty, 1.0)
	}
	DrawTextStyled(dst, spec.Label, labelX, ty, role, labelCol)
}

// drawMenuChevron draws the icon-system chevron (down=expanded, right=collapsed)
// centered in r, tinted col.
func drawMenuChevron(dst *ebiten.Image, r image.Rectangle, expanded bool, col color.Color) {
	id := IconChevronRight
	if expanded {
		id = IconChevronDown
	}
	dim := IconSizeMD
	cx := r.Min.X + r.Dx()/2
	cy := r.Min.Y + r.Dy()/2
	iconR := image.Rect(cx-dim/2, cy-dim/2, cx+dim/2, cy+dim/2)
	DrawIcon(dst, id, iconR, col)
}
