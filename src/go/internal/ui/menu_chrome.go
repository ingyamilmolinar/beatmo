package ui

import (
	"image"
	"image/color"
	"strings"

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

// drawMenuGroupHeader paints one non-interactive scrolling section row of a
// long list menu — the DESIGN.md "menu-group-header" component: a hairline
// separator at the top of the row and the uppercase caption label in muted
// text, flat on the panel surface (no container box). Group headers occupy a
// full row but never a button.
func drawMenuGroupHeader(dst *ebiten.Image, r image.Rectangle, label string) {
	if r.Empty() {
		return
	}
	line := image.Rect(r.Min.X+SpaceSM, r.Min.Y+1, r.Max.X-SpaceSM, r.Min.Y+2)
	drawRect(dst, line, WithAlpha(genColorBorder, AlphaSubtle), true)
	th := StyledTextHeight(RoleCaption)
	ty := r.Min.Y + (r.Dy()-th)/2
	DrawTextStyled(dst, strings.ToUpper(label), r.Min.X+SpaceMD, ty, RoleCaption, colTextSecondary)
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
// of an instrument-scoped menu title (spacing.swatch-sm).
const menuTitleSwatchSz = SwatchSizeSM

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
		drawRoundedRect(dst, sr.Inset(-2), WithAlphaFromColor(swatch, AlphaSubtle), fitRadius(RadiusControl, sr.Inset(-2)), true)
		drawRoundedRect(dst, sr, swatch, fitRadius(RadiusControl, sr), true)
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

// MenuRowSpec describes the variable content of one list-menu row. The
// styling (flat row on the single panel surface; hover = accent tint,
// active = accent tint + stripe) is fixed and identical for every menu;
// only these fields vary. Behavior (open/close/toggle) lives in the
// caller's onClick, not here. Shared by the instrument menu, row context
// menu, subdivision selector, and template/overflow menu so styling is
// byte-identical. Rows are deliberately NOT keycaps — the keycap is the
// signature of buttons; menus are minimalist-flat per DESIGN.md
// ("no per-row keycap pad/bevel"; 2026-07-04 design pass).
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
	TrailLabel  string        // optional right-aligned muted caption (e.g. an instrument's category, a count, "♩ BPM")
	TrailIconID IconID        // optional trailing icon at the far right (e.g. drill-in chevron); "" → none
	TrailTint   color.Color   // trailing icon tint; nil → colMenuIcon
	// LeadW reserves a custom leading slot of this width (the caller paints
	// its own art there AFTER drawMenuRow, e.g. the theme preset swatch
	// strip); the label starts after it. Ignored when Swatch/IconID is set.
	LeadW int
}

// drawMenuRow renders one list-menu row with the unified flat styling:
// per-state background accent (rest = nothing, hover = tint, active =
// stripe + tint) → optional leading swatch or icon → fuzzy-match
// highlights → label. Swatch wins the leading slot over an icon when both
// set (mirrors the instrument menu, which has a swatch and no icon). The
// btn carries geometry, text and input state only — rows never paint the
// keycap chrome (flat-menu invariant; guard: TestDrawMenuRowIsFlatNoKeycap).
// drawMenuRow is a func var (like drawRect / drawRoundedRect) so tests can
// intercept it to count how many rows a menu actually renders per frame — the
// guard behind the scrolling-menu row-culling invariant.
var drawMenuRow = func(dst *ebiten.Image, btn *Button, spec MenuRowSpec) {
	r := btn.Rect()
	if r.Empty() {
		return
	}
	accent := spec.Accent
	if accent == nil {
		accent = colAccent
	}
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
		drawRoundedRect(dst, sr, spec.Swatch, fitRadius(RadiusControl, sr), true)
		labelX = sr.Max.X + SpaceSM
	case spec.IconID != "":
		tint := spec.IconTint
		if tint == nil {
			tint = colMenuIcon
		}
		DrawIcon(dst, spec.IconID, menuRowIconRect(r), tint)
		labelX = menuRowLabelX(r)
	case spec.LeadW > 0:
		labelX = r.Min.X + SpaceSM + spec.LeadW + SpaceSM
	}

	// 4. Centered label (subdivision selector) overrides the leading-x layout.
	if spec.CenterLabel {
		tw := StyledTextWidth(spec.Label, role)
		labelX = r.Min.X + (r.Dx()-tw)/2
	}

	// 5. Trailing slots are laid out BEFORE the label is drawn, right to left:
	// an optional drill-in icon at the far right, then an optional muted
	// caption (category, count, BPM) beside it. Their left edge is what bounds
	// the label.
	//
	// Order matters. Drawing the label first and the trail afterwards let a
	// long label run straight under the caption: the template menu rendered
	// "Clara Schumann — Ich stand in dunklen Träumen" overlapping its "70 BPM"
	// readout, two sets of glyphs on the same pixels. Every menu row in the app
	// shares this renderer, so the clip belongs here rather than in a shorter
	// display string — a title long enough to collide will turn up again.
	rx := r.Max.X - SpaceMD
	if spec.TrailIconID != "" {
		s := IconSizeSM
		iy := r.Min.Y + (r.Dy()-s)/2
		tint := spec.TrailTint
		if tint == nil {
			tint = colMenuIcon
		}
		DrawIcon(dst, spec.TrailIconID, image.Rect(rx-s, iy, rx, iy+s), tint)
		rx -= s + SpaceSM
	}
	if spec.TrailLabel != "" {
		tw := StyledTextWidth(spec.TrailLabel, RoleCaption)
		tth := StyledTextHeight(RoleCaption)
		tty := r.Min.Y + (r.Dy()-tth)/2
		DrawTextStyled(dst, spec.TrailLabel, rx-tw, tty, RoleCaption, colTextSecondary)
		rx -= tw
	}

	// 6. The label, clipped to the space the trailing block leaves. A centered
	// label (the subdivision selector) is never paired with a trail, so it
	// keeps the full row width.
	label := spec.Label
	if !spec.CenterLabel {
		label = clipMenuRowLabel(label, role, labelX, rx)
	}

	// Fuzzy-match highlights sit behind the label and must be measured with the
	// same styled metric, or they drift off the glyphs. They are dropped when
	// the label had to be clipped: their rune offsets index the FULL string, so
	// painting them over a shortened one would highlight the wrong characters.
	if len(spec.Highlights) > 0 && label == spec.Label {
		drawTextHighlights(dst, spec.Label, spec.Highlights, labelX, ty, 1.0, th,
			func(s string) int { return StyledTextWidth(s, role) })
	}
	DrawTextStyled(dst, label, labelX, ty, role, labelCol)
}

// clipMenuRowLabel trims a menu row's label to the space left between its
// starting x and the left edge of the trailing block (drill-in icon and/or
// muted caption), ellipsizing when it does not fit.
//
// Extracted from drawMenuRow so the geometry is unit-testable: the collision it
// prevents is invisible to every test that only checks a row's data, and shows
// up solely as two sets of glyphs painted on the same pixels.
func clipMenuRowLabel(label string, role TextRole, labelX, trailLeft int) string {
	avail := trailLeft - SpaceSM - labelX
	measure := func(s string) int { return StyledTextWidth(s, role) }
	if avail >= measure(label) {
		return label
	}
	return clipTextToWidthMeasure(label, avail, measure)
}

// drawSheetDragHandle paints the standard bottom-sheet drag-handle pill
// (36×4, top-center, 8px inset). ONE helper so every mobile sheet — row
// context menu, File/overflow menu, instrument picker — carries identical
// chrome; the File sheet used to be the odd one out with no handle.
func drawSheetDragHandle(dst *ebiten.Image, sheetRect image.Rectangle) {
	const handleW, handleH = 36, 4
	hx := sheetRect.Min.X + sheetRect.Dx()/2 - handleW/2
	hy := sheetRect.Min.Y + 8
	drawCapsule(dst, image.Rect(hx, hy, hx+handleW, hy+handleH),
		WithAlpha(genColorBorder, genAlphaScrollbarThumb), true)
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
