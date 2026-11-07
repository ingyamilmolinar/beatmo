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

func accentStripeW() int {
	w := Profile().DensityValues().AccentStripeWidth
	if w < 2 {
		w = 2
	}
	return w
}

func drawMenuHeaderBand(dst *ebiten.Image, r image.Rectangle) {
	drawMenuHeaderBandAccent(dst, r, colAccent)
}

// Per-instrument menus pass the instrument color; global menus pass colAccent.
func drawMenuHeaderBandAccent(dst *ebiten.Image, r image.Rectangle, accent color.Color) {
	if r.Empty() {
		return
	}
	// Flat: a single hairline is the only chrome.
	underline := image.Rect(r.Min.X, r.Max.Y-1, r.Max.X, r.Max.Y)
	drawRect(dst, underline, WithAlphaFromColor(accent, genAlphaMenuHeaderAccent), true)
}

func drawMenuItemBackground(dst *ebiten.Image, r image.Rectangle, state menuItemState) {
	drawMenuItemBackgroundAccent(dst, r, state, colAccent)
}

// Per-instrument menus pass the instrument color; global menus pass colAccent.
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

// A full row but never a button.
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

func drawMenuSeparator(dst *ebiten.Image, r image.Rectangle) {
	if r.Empty() {
		return
	}
	y := r.Min.Y + r.Dy()/2
	line := image.Rect(r.Min.X+SpaceSM, y, r.Max.X-SpaceSM, y+1)
	drawRect(dst, line, WithAlpha(genColorBorder, AlphaSubtle), true)
}

// One role for every menu title.
const menuTitleRole = RoleSectionHeader

// One step above the section labels inside the grid-button cards (they are panels, not inline
// menus), so every card shows the same Title > Section > Body ladder.
const cardTitleRole = RolePanelTitle

// Shared so the cards' titles and rows align.
const cardContentPadX = SpaceLG

// One helper, so all three cards place their titles identically.
func drawCardTitleIn(dst *ebiten.Image, titleRect image.Rectangle, label string) {
	if titleRect.Empty() {
		return
	}
	th := StyledTextHeight(cardTitleRole)
	ty := titleRect.Min.Y + (titleRect.Dy()-th)/2
	DrawTextStyled(dst, label, titleRect.Min.X, ty, cardTitleRole, colTextPrimary)
}

const menuTitleSwatchSz = SwatchSizeSM

func menuTitleSwatchRect(headerRect image.Rectangle) image.Rectangle {
	y := headerRect.Min.Y + (headerRect.Dy()-menuTitleSwatchSz)/2
	x := headerRect.Min.X
	return image.Rect(x, y, x+menuTitleSwatchSz, y+menuTitleSwatchSz)
}

// nil swatch for global menus. Every menu title goes through this, so all are consistent.
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

// Shared so every menu's iconography matches.
func menuRowIconSize() int {
	if Profile().IsMobile() {
		return IconSizeMD
	}
	return IconSizeSM
}

func menuRowIconRect(rowRect image.Rectangle) image.Rectangle {
	s := menuRowIconSize()
	ix := rowRect.Min.X + SpaceMD
	iy := rowRect.Min.Y + (rowRect.Dy()-s)/2
	return image.Rect(ix, iy, ix+s, iy+s)
}

func menuRowLabelX(rowRect image.Rectangle) int {
	return rowRect.Min.X + SpaceMD + menuRowIconSize() + SpaceSM
}

// Styling is fixed and identical for every menu; only these fields vary. Rows are deliberately NOT
// keycaps: menus are flat, the keycap is the signature of buttons.
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
	// The caller paints its own art there after drawMenuRow; ignored when Swatch/IconID is set.
	LeadW int
}

// A swatch wins the leading slot over an icon. Rows never paint keycap chrome. A func var so tests
// can count the rows a menu renders per frame (the culling invariant).
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

	// Trailing slots are laid out BEFORE the label, right to left, so their left edge bounds the label;
	// the other order let long titles run under the caption. The clip lives here because every row shares it.
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

	// A centered label never has a trail, so it keeps the full width.
	label := spec.Label
	if !spec.CenterLabel {
		label = clipMenuRowLabel(label, role, labelX, rx)
	}

	// Same styled metric as the label; dropped on a clipped label, since their offsets index the full string.
	if len(spec.Highlights) > 0 && label == spec.Label {
		drawTextHighlights(dst, spec.Label, spec.Highlights, labelX, ty, 1.0, th,
			func(s string) int { return StyledTextWidth(s, role) })
	}
	DrawTextStyled(dst, label, labelX, ty, role, labelCol)
}

// Extracted so the overlap it prevents (two glyph runs on the same pixels) is unit-testable.
func clipMenuRowLabel(label string, role TextRole, labelX, trailLeft int) string {
	avail := trailLeft - SpaceSM - labelX
	measure := func(s string) int { return StyledTextWidth(s, role) }
	if avail >= measure(label) {
		return label
	}
	return clipTextToWidthMeasure(label, avail, measure)
}

// One helper, so every mobile sheet has the same handle.
func drawSheetDragHandle(dst *ebiten.Image, sheetRect image.Rectangle) {
	const handleW, handleH = 36, 4
	hx := sheetRect.Min.X + sheetRect.Dx()/2 - handleW/2
	hy := sheetRect.Min.Y + 8
	drawCapsule(dst, image.Rect(hx, hy, hx+handleW, hy+handleH),
		WithAlpha(genColorBorder, genAlphaScrollbarThumb), true)
}

// Down = expanded, right = collapsed.
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
