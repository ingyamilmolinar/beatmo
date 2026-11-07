package ui

import "image"

// ── Button constructors ───────────────────────────────────────────────────

// For slots where DESIGN.md makes the icon mandatory.
func IconOnlyButton(id IconID, style ButtonVisual) *Button {
	b := NewButton("", style, nil)
	b.Icon = string(id)
	b.IconColor = colTextSecondary
	return b
}

func RowControlButton(id IconID) *Button {
	b := IconOnlyButton(id, InstButtonStyle)
	b.IconColor = colTextSecondary
	return b
}

// id must match the icon passed to RowControlButton.
func ActiveRowControl(b *Button, active bool) {
	if b == nil {
		return
	}
	if !active {
		b.Style = InstButtonStyle
		b.IconColor = colTextSecondary
		return
	}
	switch IconID(b.Icon) {
	case IconMute:
		b.Style = MuteActiveStyle
		b.IconColor = colTextPrimary
	case IconSolo:
		b.Style = SoloActiveStyle
		b.IconColor = colAccentBright
	case IconFx:
		b.Style = FXActiveStyle
		b.IconColor = colAccent
	case IconTarget:
		b.Style = ButtonStyle{Fill: colAccentSubtle, Border: colAccent}
		b.IconColor = colAccent
	default:
		b.Style = InstButtonStyle
		b.IconColor = colTextPrimary
	}
}

// ConsumeOnPress, so rapid clicks cannot cascade through the layout change.
func DestructiveButton() *Button {
	b := NewButton("", DeleteButtonStyle, nil)
	b.Icon = string(IconTrash)
	b.IconColor = colStopRed
	b.ConsumeOnPress = true
	return b
}

// Never the raw "+" character.
func AddButton() *Button {
	return IconOnlyButton(IconPlus, InstButtonStyle)
}

func CloseButton() *Button {
	b := IconOnlyButton(IconClose, TransportMiscStyle)
	b.IconColor = colTextSecondary
	return b
}

// The SINGLE answer to "does this pop-up show a close ×?": never on mobile (tap-away keeps it clean), always on
// desktop beside Esc and click-outside, through the shared MenuCard.
func desktopCloseVisible() bool { return !Profile().UseBottomSheet }

// nil on mobile, so callers' existing nil guards make the × vanish there. ConsumeOnPress so a double tap cannot
// cascade through the teardown.
func newPopupCloseButton(onClose func()) *Button {
	if !desktopCloseVisible() {
		return nil
	}
	b := NewButton("", PopupButtonStyle, onClose)
	b.Icon = string(IconClose)
	b.IconColor = closeIconColor()
	b.ConsumeOnPress = true
	return b
}

// ── Panel spec ────────────────────────────────────────────────────────────

type PanelSpec struct {
	Title     string
	Width     int
	MinHeight int
}

func (s PanelSpec) HeaderRect(origin image.Point) image.Rectangle {
	h := Profile().PopupBtnH + 2*SpaceSM
	return image.Rect(origin.X, origin.Y, origin.X+s.Width, origin.Y+h)
}

// ContentRect returns the area below the header for the panel body.
func (s PanelSpec) ContentRect(origin image.Point, bodyHeight int) image.Rectangle {
	hdr := s.HeaderRect(origin)
	return image.Rect(hdr.Min.X, hdr.Max.Y, hdr.Min.X+s.Width, hdr.Max.Y+bodyHeight)
}

// ── Menu spec ─────────────────────────────────────────────────────────────

// MenuItemSpec describes one item in a context menu.
type MenuItemSpec struct {
	Label       string
	Icon        IconID
	Destructive bool // true → red background (DeleteButtonStyle)
	OnClick     func()
}

// MenuSpec describes a context menu (desktop floating or mobile bottom sheet).
type MenuSpec struct {
	Title string // optional — shown as header on mobile bottom sheet
	Items []MenuItemSpec
}

// ItemHeight returns the per-item height for the current profile.
func (s MenuSpec) ItemHeight() int {
	if Profile().IsMobile() {
		return BtnHeightLG + SpaceMD // ~48 px touch target
	}
	return BtnHeightMD + SpaceSM // ~40 px desktop
}

// TotalHeight returns the full panel height: header (if Title set) + all items.
func (s MenuSpec) TotalHeight() int {
	itemsH := len(s.Items) * s.ItemHeight()
	if s.Title != "" {
		return Profile().PopupBtnH + itemsH
	}
	return itemsH
}
