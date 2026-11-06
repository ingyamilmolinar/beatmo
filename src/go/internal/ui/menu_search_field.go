package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// menuSearchIconPad is the horizontal room reserved inside the field for the
// leading magnifier (icon + its gap). Text, cursor, and the hint all start
// after it via TextInput.PadLeft.
func menuSearchIconPad() int { return SpaceSM + IconSizeSM }

// ensureMenuSearchBox lazily creates a fuzzy-search TextInput styled exactly
// like the instrument menu's search box (BPMBoxStyle, MaxLen 40) and keeps its
// Rect synced to searchRect (inset by SpaceSM). Returns the box so callers can
// store it. The caller owns the query text; this handles creation + geometry
// only. Shared by the instrument menu and the template/overflow menu so both
// search fields are the same widget with the same styling — the shared
// "menu-search-field" component of DESIGN.md's Menu anatomy: leading
// IconSearch magnifier, query text, trailing clear-× while a query is
// non-empty.
func ensureMenuSearchBox(box *TextInput, searchRect image.Rectangle, reg *textFieldRegistry, spec TextFieldSpec) *TextInput {
	if box == nil {
		box = NewTextField(reg, spec, 40)
	}
	box.Rect = insetRect(searchRect, SpaceSM)
	box.PadLeft = menuSearchIconPad()
	// Reserve the clear-× slot unconditionally so the text window doesn't
	// jump when the first rune is typed.
	box.PadRight = menuSearchClearRect(box).Dx()
	return box
}

// menuSearchClearRect returns the trailing clear-× hit rect inside the field
// (a full-height square flush with the right edge). Callers hit-test it only
// while the query is non-empty — drawMenuSearchBox renders the glyph under the
// same condition.
func menuSearchClearRect(box *TextInput) image.Rectangle {
	if box == nil || box.Rect.Empty() {
		return image.Rectangle{}
	}
	side := box.Rect.Dy()
	return image.Rect(box.Rect.Max.X-side, box.Rect.Min.Y, box.Rect.Max.X, box.Rect.Max.Y)
}

// menuSearchAdornmentsActive reports whether the drawn adornments (magnifier,
// clear-×) are live — false while the mobile native <input> overlays the box
// (TextInput.Draw skips rendering under the same condition).
func menuSearchAdornmentsActive(box *TextInput) bool {
	if box == nil {
		return false
	}
	return !(Profile().IsMobile() && box.MobileInputID != "" && mobileInputActive(box.MobileInputID))
}

// drawMenuSearchBox draws the search TextInput plus its shared adornments: a
// leading IconSearch magnifier, a muted "Search" hint when the box is empty
// and unfocused, and a trailing clear-× while a query is present. Shared by
// the instrument menu and the template/overflow menu so both search fields
// render identically.
func drawMenuSearchBox(dst *ebiten.Image, box *TextInput) {
	if box == nil {
		return
	}
	box.Draw(dst)
	if !menuSearchAdornmentsActive(box) {
		return
	}
	r := box.Rect
	// Leading magnifier, vertically centered in the icon slot.
	iy := r.Min.Y + (r.Dy()-IconSizeSM)/2
	iconR := image.Rect(r.Min.X+SpaceSM, iy, r.Min.X+SpaceSM+IconSizeSM, iy+IconSizeSM)
	DrawIcon(dst, IconSearch, iconR, colTextSecondary)

	if box.Value() == "" && !box.Focused() {
		th := StyledTextHeight(RoleBody)
		ty := r.Min.Y + (r.Dy()-th)/2
		DrawTextStyled(dst, i18n.T(i18n.KeyCapSearch), r.Min.X+4+box.PadLeft, ty, RoleBody, colTextSecondary)
	}

	if box.Value() != "" {
		cr := menuSearchClearRect(box)
		cy := cr.Min.Y + (cr.Dy()-IconSizeSM)/2
		cx := cr.Min.X + (cr.Dx()-IconSizeSM)/2
		DrawIcon(dst, IconClose, image.Rect(cx, cy, cx+IconSizeSM, cy+IconSizeSM), colTextSecondary)
	}
}
