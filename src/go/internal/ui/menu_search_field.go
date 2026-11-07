package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// Text, cursor and hint all start after it (PadLeft).
func menuSearchIconPad() int { return SpaceSM + IconSizeSM }

// The one menu search widget, so every search field looks and behaves the same; the caller owns the query.
func ensureMenuSearchBox(box *TextInput, searchRect image.Rectangle, reg *textFieldRegistry, spec TextFieldSpec) *TextInput {
	if box == nil {
		box = NewTextField(reg, spec, 40)
	}
	box.Rect = insetRect(searchRect, SpaceSM)
	box.PadLeft = menuSearchIconPad()
	// Reserved always, so the text does not jump when the first rune arrives.
	box.PadRight = menuSearchClearRect(box).Dx()
	return box
}

// Hit-test only while the query is non-empty, matching when it is drawn.
func menuSearchClearRect(box *TextInput) image.Rectangle {
	if box == nil || box.Rect.Empty() {
		return image.Rectangle{}
	}
	side := box.Rect.Dy()
	return image.Rect(box.Rect.Max.X-side, box.Rect.Min.Y, box.Rect.Max.X, box.Rect.Max.Y)
}

// false while the native <input> overlays the box.
func menuSearchAdornmentsActive(box *TextInput) bool {
	if box == nil {
		return false
	}
	return !(Profile().IsMobile() && box.MobileInputID != "" && mobileInputActive(box.MobileInputID))
}

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
